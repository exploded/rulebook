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

func TestLife(t *testing.T) {
	c := Analyse([]string{"Penalty: Imprisonment for life."})
	if len(c.Penalties) != 1 || !c.Penalties[0].Life {
		t.Errorf("penalties = %+v", c.Penalties)
	}
}
