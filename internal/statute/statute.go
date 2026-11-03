// Package statute splits the text of a law, as the Federal Register of
// Legislation publishes it in EPUB form, into its sections.
//
// The Register's EPUBs hold one XHTML file converted from the Word original,
// and each paragraph keeps its Word style as its class: ActHead1 for a
// chapter, ActHead2 for a part, ActHead5 for a section, and subsection,
// paragraph and the like for the text. The endnotes start at ENotesHeading1.
package statute

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Provision is one section of a law, or a covering clause of the
// Constitution Act, or a schedule.
type Provision struct {
	Kind         string // covering | section | schedule
	Number       string // "51", "105A"; "" for a schedule
	Heading      string
	Chapter      string // "I" ... ; "" outside a chapter
	ChapterTitle string
	Part         string
	PartTitle    string
	Body         []string // one entry per paragraph
}

// Words counts the words in the heading and body.
func (p Provision) Words() int {
	n := len(strings.Fields(p.Heading))
	for _, l := range p.Body {
		n += len(strings.Fields(l))
	}
	return n
}

// ReadEPUB returns the XHTML text of an EPUB from the Register: the largest
// .html or .xhtml file in it.
func ReadEPUB(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var best *zip.File
	for _, f := range zr.File {
		if ext := path.Ext(f.Name); ext != ".html" && ext != ".xhtml" {
			continue
		}
		if best == nil || f.UncompressedSize64 > best.UncompressedSize64 {
			best = f
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no HTML document in EPUB")
	}
	rc, err := best.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

var (
	// The heading is optional: ss 86 and 87, long spent, keep only a number.
	sectionRE = regexp.MustCompile(`^(\d+[A-Z]*)\.?(?:\s+(.*))?$`)
	divRE     = regexp.MustCompile(`^(?i:(chapter|part))\s+([0-9IVXLC]+[A-Z]*)\.?\s*[—–-]?\s*(.*)$`)
)

// skipped paragraph styles: tables of contents, page headers and the
// "table of sections" summary at the front of the Constitution.
func skipped(class string) bool {
	return strings.HasPrefix(class, "TOC") || class == "TofSectsSection" || class == "Header"
}

// Parse returns the provisions in an EPUB's XHTML, in order.
func Parse(doc []byte) ([]Provision, error) {
	paras, err := paragraphs(doc)
	if err != nil {
		return nil, err
	}
	var (
		out            []Provision
		cur            *Provision
		chapter, chapT string
		part, partT    string
		inChapters     bool
		started        bool
	)
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, p := range paras {
		if p.class == "ENotesHeading1" {
			break
		}
		if !started {
			// Front matter (title, long title, contents) runs until the
			// first section heading.
			if p.class != "ActHead5" && p.class != "ActHead1" {
				continue
			}
			started = true
		}
		if skipped(p.class) || p.text == "" {
			continue
		}
		switch p.class {
		case "ActHead1", "ActHead2":
			if strings.HasPrefix(strings.ToUpper(p.text), "SCHEDULE") || strings.HasPrefix(strings.ToUpper(p.text), "THE SCHEDULE") {
				flush()
				cur = &Provision{Kind: "schedule", Heading: trimHeading(p.text)}
				chapter, chapT, part, partT = "", "", "", ""
				continue
			}
			m := divRE.FindStringSubmatch(p.text)
			if m == nil {
				return nil, fmt.Errorf("unrecognised %s heading %q", p.class, p.text)
			}
			flush()
			if strings.EqualFold(m[1], "chapter") {
				chapter, chapT, part, partT = m[2], trimHeading(m[3]), "", ""
				inChapters = true
			} else {
				part, partT = m[2], trimHeading(m[3])
			}
		case "ActHead5":
			m := sectionRE.FindStringSubmatch(p.text)
			if m == nil {
				// An unnumbered heading, like "The Constitution." inside
				// covering clause 9, belongs to the text around it.
				if cur != nil {
					cur.Body = append(cur.Body, p.text)
				}
				continue
			}
			flush()
			kind := "section"
			if !inChapters {
				kind = "covering"
			}
			cur = &Provision{Kind: kind, Number: m[1], Heading: trimHeading(m[2]),
				Chapter: chapter, ChapterTitle: chapT, Part: part, PartTitle: partT}
		default:
			if cur != nil {
				cur.Body = append(cur.Body, p.text)
			}
		}
	}
	flush()
	return out, nil
}

func trimHeading(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
}

type para struct{ class, text string }

// paragraphs returns each <p> in the document with its class and its text,
// whitespace collapsed.
func paragraphs(doc []byte) ([]para, error) {
	z := html.NewTokenizer(bytes.NewReader(doc))
	var (
		out   []para
		depth int // >0 inside a <p>
		cls   string
		text  strings.Builder
	)
	for {
		switch z.Next() {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				return out, nil
			}
			return nil, z.Err()
		case html.StartTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "p" {
				continue
			}
			if depth == 0 {
				cls = ""
				text.Reset()
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "class" {
						cls = string(v)
					}
				}
			}
			depth++
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "p" && depth > 0 {
				depth--
				if depth == 0 {
					out = append(out, para{cls, clean(text.String())})
				}
			}
		case html.TextToken:
			if depth > 0 {
				text.Write(z.Text())
			}
		}
	}
}

// clean collapses whitespace, including no-break spaces, to single spaces.
func clean(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}
