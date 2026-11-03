// Package handlers serves the site's pages. The database is read-only;
// loaders under cmd/ write it.
package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"rulebook/internal/db"
)

type Handler struct {
	conn  *sql.DB // for the few queries built at run time, like the Acts filter
	q     *db.Queries
	pages map[string]*template.Template
}

func New(conn *sql.DB, templates, static fs.FS) (*Handler, error) {
	assets, err := assetURLs(static)
	if err != nil {
		return nil, err
	}
	funcs := template.FuncMap{
		"static": func(name string) string { return assets[name] },
		"count":  count,
		"f1":     func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) },
		"f2":     func(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) },
		"lines":  func(s string) []string { return strings.Split(s, "\n") },
		"date":   longDate,
		"year":   func(d string) string { return d[:min(4, len(d))] },
		"add":    func(a, b int) int { return a + b },
		"neg":    func(a int) int { return -a },
		"int64":  func(a int) int64 { return int64(a) },
		"lower":  strings.ToLower,
		"add2":   func(a, b float64) float64 { return a + b },
		"sub":    func(a, b float64) float64 { return a - b },
		"term":   term,
		"plus":   func(a, b int64) int64 { return a + b },
		"trimS":  func(s string) string { return strings.TrimPrefix(s, "s ") },
		"pctCSS": func(f float64) template.CSS { return template.CSS(strconv.FormatFloat(f, 'f', 3, 64) + "%") },
		// basis is a section tile's minimum width in pixels: wide enough for
		// its number, wider for long sections, so a row wraps sensibly.
		"basis": func(words int64) int64 { return max(30, min(words/6, 220)) },
		"nint": func(n sql.NullInt64) string {
			if !n.Valid {
				return "–"
			}
			return count(n.Int64)
		},
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[kv[i].(string)] = kv[i+1]
			}
			return m
		},
	}
	h := &Handler{conn: conn, q: db.New(conn), pages: map[string]*template.Template{}}
	for _, page := range []string{"home.html", "constitution.html", "provision.html", "referendums.html", "about.html", "methodology.html", "sources.html", "acts.html", "act.html", "statute.html", "offences.html"} {
		t, err := template.New("").Funcs(funcs).ParseFS(templates, "layout.html", "partials.html", page)
		if err != nil {
			return nil, err
		}
		h.pages[page] = t
	}
	return h, nil
}

// assetURLs maps each static file to its URL with a hash of its contents, so
// browsers can keep it until it changes.
func assetURLs(static fs.FS) (map[string]string, error) {
	urls := map[string]string{}
	err := fs.WalkDir(static, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(static, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		urls[path] = "/static/" + path + "?v=" + hex.EncodeToString(sum[:6])
		return nil
	})
	return urls, err
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /constitution", h.constitution)
	mux.HandleFunc("GET /constitution/{ref}", h.provision)
	mux.HandleFunc("GET /referendums", h.referendums)
	mux.HandleFunc("GET /referendums.csv", h.referendumsCSV)
	mux.HandleFunc("GET /acts", h.acts)
	mux.HandleFunc("GET /acts.csv", h.actsCSV)
	mux.HandleFunc("GET /act/{id}", h.act)
	mux.HandleFunc("GET /statute-book", h.statuteBook)
	mux.HandleFunc("GET /offences", h.offences)
	mux.HandleFunc("GET /offences.csv", h.offencesCSV)
	mux.HandleFunc("GET /statute-book.csv", h.statuteBookCSV)
	mux.HandleFunc("GET /about", h.static("about.html"))
	mux.HandleFunc("GET /methodology", h.static("methodology.html"))
	mux.HandleFunc("GET /sources", h.sources)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(robotsTxt))
	})
}

// robotsTxt welcomes crawlers; the rate limit in front of the app is what
// actually protects it.
const robotsTxt = `# Crawlers are welcome. Each client may make about 4 requests a second;
# faster than that gets 429 Too Many Requests with Retry-After.
User-agent: *
Crawl-delay: 1
Disallow: /*.csv
`

type crumb struct{ Name, URL string }

func (h *Handler) static(page string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { h.render(w, page, nil) }
}

type sourcesPage struct {
	Runs []db.ListLoadRunsRow
}

func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	runs, err := h.q.ListLoadRuns(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	h.render(w, "sources.html", sourcesPage{runs})
}

func (h *Handler) render(w http.ResponseWriter, page string, data any) {
	var buf bytes.Buffer
	if err := h.pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	// The client went away; nobody is left to read a response.
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, nil)
		return
	}
	slog.Error("request failed", "error", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

// count formats n with thousands separators: 12,192.
func count(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

var months = [...]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

// longDate turns 1967-05-27 into 27 May 1967.
func longDate(d string) string {
	if len(d) != 10 {
		return d
	}
	m, err1 := strconv.Atoi(d[5:7])
	day, err2 := strconv.Atoi(d[8:10])
	if err1 != nil || err2 != nil || m < 1 || m > 12 {
		return d
	}
	return strconv.Itoa(day) + " " + months[m-1] + " " + d[:4]
}
