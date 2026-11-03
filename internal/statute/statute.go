// Package statute splits the text of a law, as the Federal Register of
// Legislation publishes it in EPUB form, into its sections.
//
// The Register's EPUBs hold XHTML converted from the Word original, one
// document per volume, and each paragraph keeps its Word style as its class:
// ActHead1 for a chapter, ActHead2 for a part, ActHead3 and ActHead4 for a
// division and subdivision, ActHead5 for a section, ActHead6 for a
// schedule, and subsection, paragraph and the like for the text. The
// endnotes start at ENotesHeading1.
package statute

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Provision is one section of a law, or a covering clause of the
// Constitution Act, or a schedule, or a numbered clause of a schedule. A
// clause's PartTitle is its schedule's heading.
type Provision struct {
	Kind         string // covering | section | schedule | clause (of a schedule)
	Number       string // "51", "105A", "6-5"; "" for a schedule
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

// ReadEPUB returns the XHTML documents of an EPUB from the Register in
// order. A long Act comes as several documents, one per volume, with the
// endnotes last.
func ReadEPUB(data []byte) ([][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	type doc struct {
		n int
		f *zip.File
	}
	var docs []doc
	for _, f := range zr.File {
		if ext := path.Ext(f.Name); ext != ".html" && ext != ".xhtml" {
			continue
		}
		// OEBPS/document_2/document_2.html is volume 2.
		n, _ := strconv.Atoi(strings.TrimPrefix(strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name)), "document_"))
		docs = append(docs, doc{n, f})
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no HTML document in EPUB")
	}
	slices.SortStableFunc(docs, func(a, b doc) int { return a.n - b.n })
	out := make([][]byte, 0, len(docs))
	for _, d := range docs {
		rc, err := d.f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

var (
	// The heading is optional: ss 86 and 87 of the Constitution, long
	// spent, keep only a number. Some Acts number sections by Division:
	// 6-5, 995-1.
	// The Criminal Code numbers them by Division with a point: 6.1.
	sectionRE = regexp.MustCompile(`^(\d+[A-Z]*(?:[-.]\d+[A-Z]*)?)\.?(?:\s+(.*))?$`)
	divRE     = regexp.MustCompile(`^(?i:(chapter|part))\s+([0-9IVXLC]+[A-Z]*(?:-[0-9]+[A-Z]*)?)\.?\s*[—–-]?\s*(.*)$`)
)

// skipped paragraph styles: tables of contents, page headers and the
// "table of sections" summary at the front of the Constitution.
func skipped(class string) bool {
	return strings.HasPrefix(class, "TOC") || class == "TofSectsSection" || class == "Header"
}

// heading reports whether a paragraph style is a heading the body of a
// volume starts at: chapter, part, division, subdivision, section or
// schedule.
func heading(class string) bool {
	switch class {
	case "ActHead1", "ActHead2", "ActHead3", "ActHead4", "ActHead5", "ActHead6":
		return true
	}
	return false
}

// Parse returns the provisions of an Act from its EPUB documents, in order.
func Parse(docs ...[]byte) ([]Provision, error) {
	return parse(docs, false)
}

// ParseConstitution is Parse for the Commonwealth of Australia Constitution
// Act, whose sections before Chapter I are covering clauses.
func ParseConstitution(docs ...[]byte) ([]Provision, error) {
	return parse(docs, true)
}

func parse(docs [][]byte, covering bool) ([]Provision, error) {
	var (
		out            []Provision
		cur            *Provision
		chapter, chapT string
		part, partT    string
		inChapters     bool
		sched          string // heading of the schedule we are in
		inserting      bool   // in text an amending item inserts in another Act
	)
	// An amending schedule's items insert text into other Acts ("Insert:",
	// "Repeal the section, substitute:"), with the same heading styles as
	// the Act's own. That text runs to the next item, or the next heading
	// listed in the contents, which lists only the Act's own headings: an
	// ActHeadN heading is listed as TOCN, so contents keys are N and text.
	contents := map[string]bool{}
	volumes := make([][]para, len(docs))
	for i, doc := range docs {
		paras, err := paragraphs(doc)
		if err != nil {
			return nil, err
		}
		for _, p := range paras {
			if level, ok := strings.CutPrefix(p.class, "TOC"); ok {
				contents[level+" "+p.text] = true
			}
		}
		volumes[i] = paras
	}
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	schedule := func(text string) {
		flush()
		cur = &Provision{Kind: "schedule", Heading: trimHeading(text)}
		chapter, chapT, part, partT = "", "", "", ""
		sched = cur.Heading
	}
	for _, paras := range volumes {
		started := false
	volume:
		for _, p := range paras {
			if p.class == "ENotesHeading1" || p.class == "NotesSection" {
				// The endnotes, or in older compilations the notes.
				break volume
			}
			if !started {
				// Each volume's front matter (title, compilation details,
				// contents) runs until its first heading.
				if !heading(p.class) {
					continue
				}
				started = true
			}
			if skipped(p.class) || p.text == "" {
				continue
			}
			if inserting {
				level, isHead := strings.CutPrefix(p.class, "ActHead")
				if p.class == "ItemHead" || isHead && contents[level+" "+p.text] {
					inserting = false
				}
			}
			if !inserting && sched != "" && p.class == "Item" && strings.HasSuffix(p.text, ":") {
				inserting = true
			}
			if inserting {
				if cur != nil {
					cur.Body = append(cur.Body, p.text)
				}
				continue
			}
			switch p.class {
			case "ActHead1", "ActHead2":
				up := strings.ToUpper(p.text)
				if strings.HasPrefix(up, "SCHEDULE") || strings.HasPrefix(up, "THE SCHEDULE") {
					schedule(p.text)
					continue
				}
				flush()
				m := divRE.FindStringSubmatch(p.text)
				if m == nil {
					// An unusual heading: keep it as an unnumbered part.
					part, partT = "", trimHeading(p.text)
					continue
				}
				if strings.EqualFold(m[1], "chapter") {
					chapter, chapT, part, partT = m[2], trimHeading(m[3]), "", ""
					inChapters = true
				} else {
					part, partT = m[2], trimHeading(m[3])
				}
			case "ActHead3", "ActHead4":
				// Divisions and subdivisions end the section before them.
				flush()
			case "ActHead6":
				schedule(p.text)
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
				if sched != "" {
					// Numbered headings after a schedule's are its clauses.
					cur = &Provision{Kind: "clause", Number: m[1], Heading: trimHeading(m[2]), PartTitle: sched}
					continue
				}
				kind := "section"
				if covering && !inChapters {
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
	}
	return out, nil
}

// noteRef is a pointer to the notes in an older compilation's heading:
// "Short title [see Note 1]".
var noteRef = regexp.MustCompile(`\s*\[see Notes? [^\]]*\]`)

func trimHeading(s string) string {
	s = noteRef.ReplaceAllString(s, "")
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
}

type para struct{ class, text string }

// paragraphs returns each <p> in the document with its class and its text,
// whitespace collapsed. Compilations prepared before about 2005 mark
// headings as <h1> to <h6> with no class instead; those come back with the
// style the newer ones use, so <h5> is ActHead5.
func paragraphs(doc []byte) ([]para, error) {
	z := html.NewTokenizer(bytes.NewReader(doc))
	var (
		out   []para
		depth int // >0 inside a <p>
		cls   string
		text  strings.Builder
	)
	block := func(name string) bool {
		return name == "p" || len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
	}
	for {
		switch z.Next() {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				return out, nil
			}
			return nil, z.Err()
		case html.StartTagToken:
			n, hasAttr := z.TagName()
			name := string(n)
			if !block(name) {
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
				if cls == "" && name[0] == 'h' {
					cls = "ActHead" + name[1:]
				}
			}
			depth++
		case html.EndTagToken:
			if name, _ := z.TagName(); block(string(name)) && depth > 0 {
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

// clean collapses whitespace, including no-break spaces, to single spaces,
// and turns non-breaking hyphens (as in section 6-5) into plain ones.
func clean(s string) string {
	s = strings.NewReplacer("\u00a0", " ", "\u2011", "-").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
