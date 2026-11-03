package constitution

import (
	"os"
	"testing"

	"rulebook/internal/statute"
)

func TestHeadsOfPower(t *testing.T) {
	epub, err := os.ReadFile("../statute/testdata/constitution.epub")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := statute.ReadEPUB(epub)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := statute.ParseConstitution(doc...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.Kind != "section" || p.Number != "51" {
			continue
		}
		heads := HeadsOfPower(p.Body)
		// (i) to (xxxix), and (xxiiiA) added by the 1946 referendum.
		if len(heads) != 40 {
			t.Fatalf("heads = %d, want 40", len(heads))
		}
		want := map[string]string{
			"i":      "Trade and commerce with other countries, and among the States",
			"xx":     "Foreign corporations, and trading or financial corporations formed within the limits of the Commonwealth",
			"xxix":   "External affairs",
			"xxxix":  "Matters incidental to the execution of any power vested by this Constitution in the Parliament or in either House thereof, or in the Government of the Commonwealth, or in the Federal Judicature, or in any department or officer of the Commonwealth",
			"xxiiiA": "",
		}
		for _, h := range heads {
			if w, ok := want[h.Number]; ok {
				if w != "" && h.Text != w {
					t.Errorf("(%s) = %q, want %q", h.Number, h.Text, w)
				}
				delete(want, h.Number)
			}
		}
		if len(want) > 0 {
			t.Errorf("missing heads %v", want)
		}
		return
	}
	t.Fatal("no s 51")
}

func TestParseRefs(t *testing.T) {
	got := ParseRefs("s 51(xxvi); s 127")
	if len(got) != 2 || got[0] != (Ref{"51", "xxvi"}) || got[1] != (Ref{"127", ""}) {
		t.Errorf("ParseRefs = %v", got)
	}
	if got := ParseRefs("s 105A"); len(got) != 1 || got[0].Section != "105A" {
		t.Errorf("ParseRefs(s 105A) = %v", got)
	}
}
