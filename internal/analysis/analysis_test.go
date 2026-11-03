package analysis

import (
	"fmt"
	"strings"
	"testing"
)

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
	if p := c.Penalties[0]; strings.Contains(p.Text, "Another subsection") {
		t.Errorf("penalty text runs into the next subsection: %q", p.Text)
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
		{[]string{"(13) An operator of a ship or aircraft who contravenes this section commits an offence punishable, on conviction, by a penalty not exceeding 60 penalty units."}, 0, 60},
		{[]string{"commits an offence and shall be liable, upon summary conviction, to a fine not exceeding 5 penalty units or to imprisonment for any period not exceeding 2 years."}, 24, 5},
		{[]string{"(4) An offence under subsection (1) is punishable on conviction by imprisonment for not more than 10 years or a fine not exceeding the amount worked out under subsection (5), or both."}, 120, 0},
	}
	// Only the mode of trial: the penalty is elsewhere.
	if c := Analyse([]string{"(3) An offence against this section is punishable upon summary conviction."}); len(c.Penalties) != 0 {
		t.Errorf("mode of trial counted as a penalty: %+v", c.Penalties)
	}
	// General penalty rules, from s 1311A of the Corporations Act 2001 and
	// s 29 of the Defence (Special Undertakings) Act 1952.
	for _, l := range []string{
		"(1) A person who commits an offence against this Act is punishable on conviction by a penalty not exceeding the penalty applicable to the offence.",
		"(1) An offence against this Act is an indictable offence and, subject to subsections (2) and (3), is punishable on conviction by a penalty not exceeding the penalty specified.",
	} {
		if c := Analyse([]string{l}); len(c.Penalties) != 0 {
			t.Errorf("general penalty rule counted as an offence: %q", l)
		}
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

// Rows are from the Corporations Act 2001's Schedule 3 and s 1317E and the
// Fair Work Act 2009's s 539, as compiled in 2026.
func TestPenaltyTables(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  []string // provision, civil, units, months
	}{
		{"schedule 3", []string{
			"Note: See sections 1311 to 1311E.",
			"\tPenalties",
			"\tProvision\tPenalty",
			"\tSubsections 148(2), (3), (4) and (5)\t30 penalty units",
			"\tSubsection 200B(1)\t(a) for an individual—6 months imprisonment, 180 penalty units, or both; and (b) for a body corporate—1,800 penalty units",
			"\tSection 111AU\t5 years imprisonment",
		}, []string{
			"148(2) false 30 0", "148(3) false 30 0", "148(4) false 30 0", "148(5) false 30 0",
			"200B(1) false 1800 6", "111AU false 0 60",
		}},
		{"civil penalty provisions", []string{
			"(a) a provision specified in column 1 of the following table is a civil penalty provision; and",
			"\tCivil penalty provisions",
			"\tColumn 1 Provision\tColumn 2 Brief description of the provision\tColumn 3 Categorisation of the provision",
			"\tsubsections 180(1), 181(1) and (2), 182(1) and (2) and 183(1) and (2)\tofficers’ duties\tcorporation/scheme",
		}, []string{
			"180(1) true 0 0", "181(1) true 0 0", "181(2) true 0 0", "182(1) true 0 0",
			"182(2) true 0 0", "183(1) true 0 0", "183(2) true 0 0",
		}},
		{"civil remedy provisions", []string{
			"\tStanding, jurisdiction and maximum penalties",
			"\tItem\tColumn 1 Civil remedy provision\tColumn 2 Persons\tColumn 3 Courts\tColumn 4 Maximum penalty",
			"\tPart 2-1—Core provisions",
			"\t2\t45 (other than in relation to a contravention of an outworker term (see section 12))\t(a) an employee; (b) an employer\t(a) the Federal Court\tfor a serious contravention—600 penalty units; or otherwise—60 penalty units",
			"\t3\tsubsection 601FC(1)(a) and (b)\t(a) an inspector\t(a) the Federal Court\t60 penalty units",
		}, []string{"45 true 600 0", "601FC(1)(a) true 60 0", "601FC(1)(b) true 60 0"}},
		{"other tables", []string{
			"\tItem\tColumn 1 Provision\tColumn 2 Time limit",
			"\t1\tSection 12\t28 days",
			"Penalty: 10 penalty units.",
		}, []string{" false 10 0"}},
	}
	for _, tc := range cases {
		var got []string
		for _, p := range Analyse(tc.lines).Penalties {
			got = append(got, fmt.Sprintf("%s %t %d %d", p.Provision, p.Civil, p.Units, p.Months))
		}
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s: got\n%s\nwant\n%s", tc.name, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}
