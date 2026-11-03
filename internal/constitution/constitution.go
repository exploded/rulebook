// Package constitution reads structure out of the Constitution's text: the
// heads of legislative power in s 51 and the section references that
// referendum results use.
package constitution

import (
	"regexp"
	"strings"
)

// TitleID is the Constitution's id on the Federal Register of Legislation.
const TitleID = "C2004Q00685"

// Head is one paragraph of s 51, a subject the Parliament may make laws about.
type Head struct {
	Number string // "i", "xxiiiA"
	Text   string
}

var headRE = regexp.MustCompile(`^\(([ivxl]+A?)\.?\)\s*(.*)$`)

// HeadsOfPower returns the paragraphs of s 51's body, in order.
func HeadsOfPower(body []string) []Head {
	var out []Head
	for _, l := range body {
		m := headRE.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		text := strings.TrimRight(m[2], ":;.— ")
		out = append(out, Head{Number: m[1], Text: text})
	}
	return out
}

// Ref is a section, optionally narrowed to a paragraph: s 51(xxvi).
type Ref struct {
	Section   string
	Paragraph string
}

var refRE = regexp.MustCompile(`s\s*(\d+[A-Z]*)(?:\(([ivxl]+A?)\))?`)

// ParseRefs reads references like "s 51(xxvi); s 127".
func ParseRefs(s string) []Ref {
	var out []Ref
	for _, m := range refRE.FindAllStringSubmatch(s, -1) {
		out = append(out, Ref{Section: m[1], Paragraph: m[2]})
	}
	return out
}
