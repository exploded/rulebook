// Package register reads the Federal Register of Legislation's OData API
// (https://api.prod.legislation.gov.au/swagger/index.html). The API is free
// and needs no key; the Office of Parliamentary Counsel asks that full crawls
// run outside 08:00-20:00 AEST and that heavy users give notice first.
package register

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const BaseURL = "https://api.prod.legislation.gov.au/v1"

// Client fetches one request at a time, retrying with backoff, and caches raw
// responses (gzipped) so parsing changes never need a re-fetch.
type Client struct {
	HTTP     *http.Client
	CacheDir string // empty disables the cache
	Pause    time.Duration
	Refresh  bool // ignore cached copies
}

// NewClient pauses a second between live requests, which keeps a full crawl
// well inside what the Register asks of bulk users.
func NewClient(cacheDir string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 90 * time.Second}, CacheDir: cacheDir, Pause: time.Second}
}

// Get returns the body for url, from the cache when present.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	path := c.cachePath(url)
	if path != "" && !c.Refresh {
		if b, err := readGzip(path); err == nil {
			return b, nil
		}
	}
	var body []byte
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
		body, err = c.fetch(ctx, url)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if path != "" {
		if err := writeGzip(path, body); err != nil {
			return nil, err
		}
	}
	time.Sleep(c.Pause)
	return body, nil
}

func (c *Client) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rulebook (+https://github.com/exploded/rulebook)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) cachePath(url string) string {
	if c.CacheDir == "" {
		return ""
	}
	h := sha256.Sum256([]byte(url))
	return filepath.Join(c.CacheDir, hex.EncodeToString(h[:8])+".gz")
}

func readGzip(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

func writeGzip(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(data)
	zw.Close()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Title is one law on the Register.
type Title struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	MakingDate         string `json:"makingDate"`
	Collection         string `json:"collection"`
	IsPrincipal        bool   `json:"isPrincipal"`
	IsInForce          bool   `json:"isInForce"`
	Status             string `json:"status"`
	OriginatingBillURI string `json:"originatingBillUri"`
	Year               *int64 `json:"year"`
	Number             *int64 `json:"number"`
}

// Version is one compilation of a title.
type Version struct {
	TitleID           string   `json:"titleId"`
	Start             string   `json:"start"`
	End               string   `json:"end"`
	IsLatest          bool     `json:"isLatest"`
	RegisterID        string   `json:"registerId"`
	CompilationNumber string   `json:"compilationNumber"`
	Reasons           []Reason `json:"reasons"`
}

// Reason names a title that changed this compilation.
type Reason struct {
	Affect          string `json:"affect"`
	AffectedByTitle *struct {
		TitleID    string `json:"titleId"`
		Name       string `json:"name"`
		Provisions string `json:"provisions"`
	} `json:"affectedByTitle"`
}

// Date returns the YYYY-MM-DD part of an API timestamp ("" stays "").
func Date(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// TitleURL is the API URL for one title.
func TitleURL(id string) string {
	return fmt.Sprintf("%s/Titles('%s')", BaseURL, url.PathEscape(id))
}

// VersionsURL lists every compilation of a title, oldest first.
func VersionsURL(id string) string {
	return BaseURL + "/Versions?" + query(url.Values{
		"$filter":  {fmt.Sprintf("titleId eq '%s'", id)},
		"$orderby": {"start"},
	})
}

// LatestEpubURL is the latest compilation's text as an EPUB.
func LatestEpubURL(id string) string {
	return fmt.Sprintf("%s/documents/find(titleid='%s',asatspecification='Latest',type='Primary',format='Epub',uniqueTypeNumber=0,volumeNumber=0,rectificationSpecification='Latest')", BaseURL, id)
}

// query encodes v with %20 for spaces, which OData expects.
func query(v url.Values) string {
	return strings.ReplaceAll(v.Encode(), "+", "%20")
}

// GetTitle fetches one title.
func (c *Client) GetTitle(ctx context.Context, id string) (Title, []byte, error) {
	var t Title
	b, err := c.Get(ctx, TitleURL(id))
	if err != nil {
		return t, nil, err
	}
	return t, b, json.Unmarshal(b, &t)
}

// GetVersions fetches every compilation of a title, following nextLink.
func (c *Client) GetVersions(ctx context.Context, id string) ([]Version, []byte, error) {
	var all []Version
	var raw []byte
	next := VersionsURL(id)
	for next != "" {
		b, err := c.Get(ctx, next)
		if err != nil {
			return nil, nil, err
		}
		raw = append(raw, b...)
		var page struct {
			Value    []Version `json:"value"`
			NextLink string    `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(b, &page); err != nil {
			return nil, nil, err
		}
		all = append(all, page.Value...)
		next = page.NextLink
	}
	return all, raw, nil
}
