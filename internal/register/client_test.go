package register

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"
)

func TestQuietIn(t *testing.T) {
	cases := []struct {
		at   string // UTC
		want time.Duration
	}{
		{"2026-11-03T09:30:00Z", 0},              // 19:30 AEST: busy until 20:00
		{"2026-11-03T10:00:00Z", 0},              // 20:00 AEST: quiet
		{"2026-11-03T21:59:00Z", 0},              // 07:59 AEST: quiet
		{"2026-11-03T22:00:00Z", 12 * time.Hour}, // 08:00 AEST: busy
		{"2026-11-04T03:00:00Z", 7 * time.Hour},  // 13:00 AEST
	}
	cases[0].want = 30 * time.Minute
	for _, c := range cases {
		now, _ := time.Parse(time.RFC3339, c.at)
		if got := quietIn(now); got != c.want {
			t.Errorf("quietIn(%s) = %s, want %s", c.at, got, c.want)
		}
	}
}

func TestEnded(t *testing.T) {
	b, err := os.ReadFile("testdata/repealed_acts.json")
	if err != nil {
		t.Fatal(err)
	}
	var page struct{ Value []Title }
	if err := json.Unmarshal(b, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Value) == 0 {
		t.Fatal("no titles")
	}
	date, id, name := page.Value[0].Ended()
	if date != "2014-06-24" || id != "C2014A00028" || name != "Amending Acts 1901 to 1969 Repeal Act 2014" {
		t.Errorf("Ended = %q %q %q", date, id, name)
	}
	in := Title{StatusHistory: []StatusChange{{Status: "InForce", Start: "1999-12-10T00:00:00"}}}
	if d, _, _ := in.Ended(); d != "" {
		t.Errorf("in-force title ended %q", d)
	}
}

func TestVersionsBatch(t *testing.T) {
	b, err := os.ReadFile("testdata/versions_batch.json")
	if err != nil {
		t.Fatal(err)
	}
	var page struct{ Value []Version }
	if err := json.Unmarshal(b, &page); err != nil {
		t.Fatal(err)
	}
	per := map[string]int{}
	for _, v := range page.Value {
		per[v.TitleID]++
	}
	// The Electronic Transactions Act 1999 has 5 compilations; the four
	// amending Acts have only their as-made text.
	if per["C2004A00553"] != 5 || len(per) != 5 {
		t.Errorf("compilations per title = %v", per)
	}
}

func TestPages(t *testing.T) {
	b, err := os.ReadFile("testdata/pdf_documents.json")
	if err != nil {
		t.Fatal(err)
	}
	var page struct{ Value []Document }
	if err := json.Unmarshal(b, &page); err != nil {
		t.Fatal(err)
	}
	for i := range page.Value {
		page.Value[i].TitleID = "C2004A00553" // the fixture's $select left it out
	}
	p := Pages(page.Value)
	if p["C2004A00553/0"] != 26 || p["C2004A00553/4"] != 38 {
		t.Errorf("pages = %v", p)
	}
	// Volumes add up; a later rectification replaces an earlier one.
	n := func(i int64) *int64 { return &i }
	docs := []Document{
		{TitleID: "X", CompilationNumber: "3", VolumeNumber: 1, PageCount: n(400)},
		{TitleID: "X", CompilationNumber: "3", VolumeNumber: 2, PageCount: n(350)},
		{TitleID: "X", CompilationNumber: "3", VolumeNumber: 2, RectificationVersionNumber: 1, PageCount: n(352)},
	}
	if got := Pages(docs)["X/3"]; got != 752 {
		t.Errorf("multi-volume pages = %d, want 752", got)
	}
}

func TestAdministered(t *testing.T) {
	b, err := os.ReadFile("testdata/administered.json")
	if err != nil {
		t.Fatal(err)
	}
	var page struct{ Value []Administered }
	if err := json.Unmarshal(b, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Value) != 20 {
		t.Fatalf("%d titles, want 20", len(page.Value))
	}
	// The Defence Act 1903 is administered by four departments.
	for _, a := range page.Value {
		if a.ID == "C1903A00020" {
			if len(a.Departments) != 4 || a.Departments[1].Portfolio != "Defence" {
				t.Errorf("C1903A00020 departments = %+v", a.Departments)
			}
			return
		}
	}
	t.Error("C1903A00020 missing")
}

func TestSortVersions(t *testing.T) {
	vs := []Version{
		{CompilationNumber: "13", Start: "2010-01-01"},
		{CompilationNumber: "12A", Start: "2009-06-01"},
		{CompilationNumber: "0", Start: "1990-01-01"},
		{CompilationNumber: "12", Start: "2009-06-01"},
		{CompilationNumber: "9", Start: "2005-01-01"},
	}
	SortVersions(vs)
	var got []string
	for _, v := range vs {
		got = append(got, v.CompilationNumber)
	}
	if want := []string{"0", "9", "12", "12A", "13"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
