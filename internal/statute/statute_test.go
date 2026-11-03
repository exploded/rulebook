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
	ps, err := ParseConstitution(doc...)
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

func TestParseActVolumes(t *testing.T) {
	vol1 := []byte(`<html><body>
<p class="ShortT">Example Act 2001</p>
<p class="TOC5">1-1 Short title</p>
<p class="ActHead1">Chapter&#xa0;1—Preliminary</p>
<p class="ActHead2">Part&#xa0;1&#x2011;1—Introduction</p>
<p class="ActHead3">Division&#xa0;1—Start</p>
<p class="ActHead5">1&#x2011;1&#xa0; Short title</p>
<p class="subsection">This Act may be cited as the Example Act 2001.</p>
<p class="ActHead4">Subdivision&#xa0;1-A—Offences</p>
<p class="ActHead5">1&#x2011;5&#xa0; Offence</p>
<p class="subsection">A person commits an offence if the person does a thing.</p>
<p class="Penalty">Penalty: 10 penalty units.</p>
</body></html>`)
	vol2 := []byte(`<html><body>
<p class="ShortT">Example Act 2001</p>
<p class="CompiledActNo">No. 1, 2001</p>
<p class="ActHead2">Part&#xa0;1&#x2011;2—More</p>
<p class="ActHead5">1&#x2011;10&#xa0; Another</p>
<p class="subsection">Text.</p>
<p class="ActHead6">Schedule&#xa0;1—Forms</p>
<p class="ActHead9">Form 1</p>
<p class="Item">A form.</p>
<p class="ActHead5">1&#xa0; First clause</p>
<p class="subsection">Clause text.</p>
<p class="ENotesHeading1">Endnotes</p>
<p class="ENotesText">Not counted.</p>
</body></html>`)
	ps, err := Parse(vol1, vol2)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range ps {
		got = append(got, p.Kind+" "+p.Number+" "+p.Part+" "+p.Heading+" ("+strings.Join(p.Body, " / ")+")")
	}
	want := []string{
		"section 1-1 1-1 Short title (This Act may be cited as the Example Act 2001.)",
		"section 1-5 1-1 Offence (A person commits an offence if the person does a thing. / Penalty: 10 penalty units.)",
		"section 1-10 1-2 Another (Text.)",
		"schedule   Schedule 1—Forms (Form 1 / A form.)",
		"clause 1  First clause (Clause text.)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if last := ps[len(ps)-1]; last.PartTitle != "Schedule 1—Forms" {
		t.Errorf("clause's schedule = %q", last.PartTitle)
	}
}

// Older compilations, like the Surplus Revenue Act 1908's of 1999, mark
// headings as bare <h1> to <h6> and start their notes with NotesSection.
func TestParseOlderCompilation(t *testing.T) {
	doc := []byte(`<html><body>
<p class="ShortT">Surplus Revenue Act 1908</p>
<p class="TOC5"><span>1</span><span>&#xa0;</span><span>Short title [see Note 1]</span></p>
<p class="LongT">An Act relating to the payment to the several States of the Surplus Revenue</p>
<h5 id="navPoint_2"><a><span class="CharSectno">1</span><span>&#xa0; </span><span>Short title </span><span>[</span><span>see</span><span> Note 1]</span></a></h5>
<p class="subsection">This Act may be cited as the Surplus Revenue Act 1908.</p>
<h5 id="navPoint_3"><a><span class="CharSectno">2</span><span>&#xa0; </span><span>Commencement</span></a></h5>
<p class="subsection">This Act shall commence on a day to be fixed by proclamation.</p>
<h1 id="navPoint_5"><a><span class="CharChapNo">The Schedule</span></a></h1>
<p style="margin-top:6pt">AGREEMENT made the second day of September.</p>
<p class="NotesSection">Notes to the Surplus Revenue Act 1908</p>
<p class="TableOfActs1">Surplus Revenue Act 1910</p>
</body></html>`)
	ps, err := Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range ps {
		got = append(got, p.Kind+" "+p.Number+" "+p.Heading+" ("+strings.Join(p.Body, " / ")+")")
	}
	want := []string{
		"section 1 Short title (This Act may be cited as the Surplus Revenue Act 1908.)",
		"section 2 Commencement (This Act shall commence on a day to be fixed by proclamation.)",
		"schedule  The Schedule (AGREEMENT made the second day of September.)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
