package statute

import (
	"os"
	"strings"
	"testing"
)

func constitution(t *testing.T) []Provision {
	t.Helper()
	epub, err := os.ReadFile("testdata/constitution.epub")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ReadEPUB(epub)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

func TestConstitutionStructure(t *testing.T) {
	ps := constitution(t)
	kinds := map[string]int{}
	chapters := map[string]bool{}
	for _, p := range ps {
		kinds[p.Kind]++
		if p.Chapter != "" {
			chapters[p.Chapter] = true
		}
	}
	// 9 covering clauses; sections 1-128 and 105A less s 127, which the
	// 1967 referendum repealed; and the Schedule (the oath and affirmation).
	if kinds["covering"] != 9 {
		t.Errorf("covering clauses = %d, want 9", kinds["covering"])
	}
	if kinds["section"] != 128 {
		t.Errorf("sections = %d, want 128", kinds["section"])
	}
	if kinds["schedule"] != 1 {
		t.Errorf("schedules = %d, want 1", kinds["schedule"])
	}
	if len(chapters) != 8 {
		t.Errorf("chapters = %d, want 8", len(chapters))
	}
}

func TestConstitutionSection51(t *testing.T) {
	var s51 *Provision
	for _, p := range constitution(t) {
		if p.Kind == "section" && p.Number == "51" {
			s51 = &p
			break
		}
	}
	if s51 == nil {
		t.Fatal("no section 51")
	}
	if s51.Heading != "Legislative powers of the Parliament" || s51.Chapter != "I" || s51.Part != "V" {
		t.Errorf("s 51 = %q chapter %q part %q", s51.Heading, s51.Chapter, s51.Part)
	}
	body := strings.Join(s51.Body, "\n")
	for _, want := range []string{"(i.) Trade and commerce", "(xx.) Foreign corporations", "(xxiiiA.)", "(xxix.) External affairs", "(xxxix.) Matters incidental"} {
		if !strings.Contains(body, want) {
			t.Errorf("s 51 body lacks %q", want)
		}
	}
}
