package analysis

import "testing"

// Lines are from the Crimes Act 1914, the Privacy Act 1988 and the Income
// Tax Assessment Act 1997 as compiled in 2026.
func TestAnalyse(t *testing.T) {
	c := Analyse([]string{
		"(4) A person commits an offence if the person fails to comply with the notice.",
		"Penalty: 30 penalty units.",
		"(5) Subsection (4) is an offence of strict liability.",
		"Note: For strict liability, see section 6.1 of the Criminal Code.",
		"(6) Subsection (4) does not apply if the person has a reasonable excuse.",
		"Note: A defendant bears an evidential burden in relation to the matter in subsection (6) (see subsection 13.3(3) of the Criminal Code).",
		"Penalty: Imprisonment for 10 years or 600 penalty units, or both.",
		"Civil penalty: 2,000 penalty units.",
		"Note 1: For absolute liability, see section 6.2 of the Criminal Code.",
		"The Minister may, by legislative instrument, make rules prescribing matters required or permitted by this Act.",
		"The Governor-General may make regulations prescribing matters required or permitted by this Act.",
		"The regulations may modify the operation of this Act in relation to a transitional matter.",
	})
	if len(c.Penalties) != 3 {
		t.Fatalf("penalties = %+v", c.Penalties)
	}
	if p := c.Penalties[0]; p.Civil || p.Units != 30 || p.Months != 0 {
		t.Errorf("first penalty = %+v", p)
	}
	if p := c.Penalties[1]; p.Months != 120 || p.Units != 600 {
		t.Errorf("second penalty = %+v", p)
	}
	if p := c.Penalties[2]; !p.Civil || p.Units != 2000 {
		t.Errorf("civil penalty = %+v", p)
	}
	if c.StrictLiability != 1 || c.AbsoluteLiability != 0 || c.EvidentialBurden != 1 ||
		c.LegalBurden != 0 || c.Instruments != 1 || c.Regulations != 1 || c.HenryVIII != 1 {
		t.Errorf("counts = %+v", c)
	}
}

func TestPenaltyOnFollowingLines(t *testing.T) {
	c := Analyse([]string{
		"Penalty:",
		"(a) for an individual—imprisonment for 6 months; or",
		"(b) for a body corporate—300 penalty units.",
		"(2) Another subsection.",
	})
	if len(c.Penalties) != 1 {
		t.Fatalf("penalties = %+v", c.Penalties)
	}
	if p := c.Penalties[0]; p.Months != 6 || p.Units != 300 {
		t.Errorf("penalty = %+v", p)
	}
}

// Forms from the Customs Act 1901, the Excise Act 1901, the Crimes Act 1914
// and the Commonwealth Electoral Act 1918.
func TestPenaltyForms(t *testing.T) {
	cases := []struct {
		lines         []string
		months, units int
	}{
		{[]string{"Penalty: 1 penalty unit."}, 0, 1},
		{[]string{"Penalty: One penalty unit."}, 0, 1},
		{[]string{"Penalty: Imprisonment for five years."}, 60, 0},
		{[]string{"Penalty: Five years’ imprisonment."}, 60, 0},
		{[]string{"Penalty: 2 years imprisonment or the greater of:", "(a) 500 penalty units; and", "(b) 5 times the amount of duty."}, 24, 500},
		{[]string{"Civil penalty:", "The higher of the following amounts:", "(a) 200 penalty units;", "(b) 3 times that amount."}, 0, 200},
		{[]string{"Penalty for contravention of this subsection: 60 penalty units."}, 0, 60},
	}
	for _, c := range cases {
		got := Analyse(c.lines)
		if len(got.Penalties) != 1 {
			t.Errorf("%q: penalties = %+v", c.lines[0], got.Penalties)
			continue
		}
		if p := got.Penalties[0]; p.Months != c.months || p.Units != c.units {
			t.Errorf("%q: months %d units %d, want %d and %d", c.lines[0], p.Months, p.Units, c.months, c.units)
		}
	}
}

func TestLife(t *testing.T) {
	c := Analyse([]string{"Penalty: Imprisonment for life."})
	if len(c.Penalties) != 1 || !c.Penalties[0].Life {
		t.Errorf("penalties = %+v", c.Penalties)
	}
}
