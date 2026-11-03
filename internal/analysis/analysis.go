// Package analysis counts what a law's text says about offences, penalties
// and delegated powers, by matching the fixed phrases Commonwealth drafting
// uses. Every count is of lines a reader can find in the Act, and none needs
// judgement:
//
//   - An offence is a penalty at the foot of a section or subsection
//     ("Penalty: Imprisonment for 2 years."). Section 4D of the Crimes Act
//     1914 makes such a penalty mean that a contravention is an offence.
//     Older drafting states the punishment in the text instead ("commits
//     an offence punishable, on conviction, by a penalty not exceeding 60
//     penalty units"); a line in which a person commits, or is guilty of,
//     an offence that is punishable, or for which they are liable, counts
//     as one too.
//   - A civil penalty provision ends "Civil penalty: ...".
//   - Some Acts list their penalties in a table instead. The Corporations
//     Act 2001's Schedule 3 sets a penalty beside each provision, and s 1311
//     makes contravening them offences; its s 1317E lists the civil penalty
//     provisions, and the Fair Work Act 2009's s 539 the civil remedy
//     provisions, with a maximum penalty for each. A table counts when its
//     header names a provision column ("Provision", "Civil remedy
//     provision") and a penalty column, or its title is "Civil penalty
//     provisions". Each provision a row names counts once, as a penalty at
//     the foot of each would.
//   - Strict and absolute liability are declared in the text ("An offence
//     against subsection (1) is an offence of strict liability."); the
//     notes that follow, pointing to the Criminal Code, aren't counted.
//   - A reversed burden is a defendant bearing an evidential or legal
//     burden, which Commonwealth drafting states in the text or a note.
//   - A delegated power is a power to make legislative instruments, and a
//     Henry VIII clause lets those instruments modify the Act itself.
package analysis

import (
	"regexp"
	"strconv"
	"strings"

	"rulebook/internal/statute"
)

// Penalty is one penalty at the foot of a section or subsection, or one
// provision in a table of penalties.
type Penalty struct {
	Civil     bool
	Provision string // the provision a table names, "200B(1)"; "" for a penalty at its foot
	Text      string // as written, after the colon
	Life      bool   // imprisonment for life
	Months    int    // longest term of imprisonment, in months; 0 if none
	Units     int    // most penalty units; 0 if none
}

// Counts are the matches in one provision.
type Counts struct {
	Penalties         []Penalty
	StrictLiability   int // declarations, not notes
	AbsoluteLiability int
	EvidentialBurden  int // a defendant bears an evidential burden
	LegalBurden       int // a defendant bears a legal burden
	Instruments       int // powers to make a legislative instrument
	Regulations       int // powers to make regulations
	HenryVIII         int // instruments that may modify the Act
}

// numbers are the counts older Acts write in words ("Imprisonment for five
// years", "One penalty unit").
var numbers = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
	"nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "fourteen": 14, "fifteen": 15,
	"twenty": 20, "twenty-five": 25, "thirty": 30, "fifty": 50, "sixty": 60, "hundred": 100,
}

const num = `(\d[\d,]*|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|fourteen|fifteen|twenty-five|twenty|thirty|fifty|sixty|hundred)`

var (
	noteRE = regexp.MustCompile(`^(Note|Notes|Example|Examples)( \d+)?:`)
	// A paragraph, "(a)" or "(iv)", continues a penalty; a subsection,
	// "(2)", doesn't.
	paraRE    = regexp.MustCompile(`^\([a-z]+\) `)
	penaltyRE = regexp.MustCompile(`^(Civil penalty|Penalty)( [^:]{1,80})?:\s*(.*)$`)
	// "commits an offence punishable ...", "is guilty of an offence and
	// liable to ...", "An offence under subsection (1) is punishable on
	// conviction by ...".
	inlineRE = regexp.MustCompile(`(?i)\b(commits|is guilty of|shall be guilty of) an offen[cs]e\b[^.;:]{0,80}?\b(punishable|liable)\b` +
		`|^(\(\w+\) )?An offen[cs]e (under|against) [^.;:]{0,80}? is punishable,? (on|upon) (summary )?conviction,? by\b`)
	// A general penalty rule creates no offence of its own: "A person who
	// commits an offence against this Act is punishable on conviction by a
	// penalty not exceeding the penalty applicable to the offence." One
	// about an offence against this section is that section's penalty.
	generalRE = regexp.MustCompile(`(?i)\bwho (commits|is guilty of) an offen[cs]e against this Act\b|^(\(\w+\) )?An offen[cs]e against this Act\b`)
	// "imprisonment for 2 years", "imprisonment for any period not
	// exceeding 2 years", "2 years imprisonment".
	prisonRE   = regexp.MustCompile(`(?i)imprisonment (?:for )?(life|(?:[a-z ]{0,30} )?` + num + ` (years?|months?))|\b` + num + ` (years?|months?)['\x{2019}]? imprisonment`)
	unitsRE    = regexp.MustCompile(`(?i)\b` + num + ` penalty units?\b`)
	strictRE   = regexp.MustCompile(`(?i)(offences? of strict liability|strict liability applies)`)
	absoluteRE = regexp.MustCompile(`(?i)(offences? of absolute liability|absolute liability applies)`)
	regsRE     = regexp.MustCompile(`may make regulations`)
	henryRE    = regexp.MustCompile(`(?i)\b(regulations|rules|instrument)s? (made under [^,]+, )?(may|can) (modify|amend|vary|alter)\b[^.]*\bthis Act\b`)
)

// Analyse returns the counts for a provision's paragraphs.
func Analyse(lines []string) Counts {
	var (
		c     Counts
		title string        // the title row of the table we are in
		table *penaltyTable // the penalty table we are in
	)
	for i, l := range lines {
		if strings.Contains(l, "bears an evidential burden") {
			c.EvidentialBurden++
		}
		if strings.Contains(l, "bears a legal burden") {
			c.LegalBurden++
		}
		if noteRE.MatchString(l) {
			continue
		}
		cells := statute.Cells(l)
		if cells == nil {
			title, table = "", nil
		}
		switch {
		case cells == nil:
		case len(cells) == 1:
			// A table's title, or a heading across it ("Part 2-1—Core
			// provisions").
			if table == nil {
				title = cells[0]
			}
		case table == nil:
			table = header(cells, title)
		case table.provision < len(cells):
			var text string
			if table.penalty >= 0 && table.penalty < len(cells) {
				text = cells[table.penalty]
			}
			for _, ref := range provisions(cells[table.provision]) {
				p := parsePenalty(table.civil, text)
				p.Provision = ref
				c.Penalties = append(c.Penalties, p)
			}
		}
		if m := penaltyRE.FindStringSubmatch(l); cells == nil && m != nil {
			text, j := m[3], i+1
			if text == "" && j < len(lines) && !strings.HasPrefix(lines[j], "(") && !noteRE.MatchString(lines[j]) {
				// "Civil penalty:" then "The higher of the following
				// amounts:" on the next paragraph.
				text, j = lines[j], j+1
			}
			if text == "" || strings.HasSuffix(text, ":") {
				// A penalty that varies ("Penalty:" then "(a) for an
				// individual—...") continues on the next paragraphs.
				for k := j; k < len(lines) && k < j+4 && paraRE.MatchString(lines[k]); k++ {
					text = strings.TrimSpace(text + " " + lines[k])
				}
			}
			c.Penalties = append(c.Penalties, parsePenalty(m[1] == "Civil penalty", text))
		} else if cells == nil && inlineRE.MatchString(l) && !generalRE.MatchString(l) {
			text := l
			if strings.HasSuffix(text, ":") {
				// "commits an offence punishable upon conviction:" then
				// "(a) if the offender is an individual—...".
				for k := i + 1; k < len(lines) && k <= i+4 && paraRE.MatchString(lines[k]); k++ {
					text += " " + lines[k]
				}
			}
			c.Penalties = append(c.Penalties, parsePenalty(false, text))
		}
		if strictRE.MatchString(l) {
			c.StrictLiability++
		}
		if absoluteRE.MatchString(l) {
			c.AbsoluteLiability++
		}
		c.Instruments += strings.Count(l, "by legislative instrument")
		if regsRE.MatchString(l) {
			c.Regulations++
		}
		if henryRE.MatchString(l) {
			c.HenryVIII++
		}
	}
	return c
}

// penaltyTable is where a table of penalties keeps its columns.
type penaltyTable struct {
	provision, penalty int // column indexes; penalty is -1 if none
	civil              bool
}

var (
	provisionCol = regexp.MustCompile(`(?i)^(column \d+ ?)?(civil (remedy|penalty) )?provisions?$`)
	penaltyCol   = regexp.MustCompile(`(?i)^(column \d+ ?)?(maximum )?penalty$`)
)

// header returns the columns of a table of penalties from its header row
// and title, or nil if the table isn't one.
func header(cells []string, title string) *penaltyTable {
	t := &penaltyTable{provision: -1, penalty: -1}
	for i, c := range cells {
		switch {
		case t.provision < 0 && provisionCol.MatchString(c):
			t.provision = i
			t.civil = strings.Contains(strings.ToLower(c), "civil")
		case t.penalty < 0 && penaltyCol.MatchString(c):
			t.penalty = i
		}
	}
	if strings.EqualFold(title, "civil penalty provisions") {
		t.civil = true
	} else if t.penalty < 0 {
		return nil
	}
	if t.provision < 0 {
		return nil
	}
	return t
}

var refRE = regexp.MustCompile(`\d+[A-Z]*(?:[-.]\d+[A-Z]*)?(?:\([0-9A-Za-z]+\))*|\([0-9A-Za-z]+\)(?:\([0-9A-Za-z]+\))*`)

// provisions returns the provisions a table cell names: "Subsections
// 148(2), (3) and (4)" names 148(2), 148(3) and 148(4). Words in brackets,
// like "45 (other than in relation to an outworker term)", are passed over.
// A cell naming no numbered provision ("the regulations") names itself.
func provisions(cell string) []string {
	var (
		b     strings.Builder
		opens []int // where each open bracket is in b
	)
	for _, r := range cell {
		b.WriteRune(r)
		switch {
		case r == '(':
			opens = append(opens, b.Len()-1)
		case r == ')' && len(opens) > 0:
			open := opens[len(opens)-1]
			opens = opens[:len(opens)-1]
			if s := b.String(); len(opens) == 0 && strings.ContainsRune(s[open:], ' ') {
				b.Reset()
				b.WriteString(s[:open])
			}
		}
	}
	var out []string
	for _, m := range refRE.FindAllString(b.String(), -1) {
		if m[0] != '(' {
			out = append(out, m)
			continue
		}
		if len(out) == 0 {
			continue
		}
		// "(3)" after "148(2)" is 148(3); "(b)" after "601FC(1)(a)" is
		// 601FC(1)(b).
		prev := out[len(out)-1]
		for range strings.Count(m, "(") {
			if j := strings.LastIndex(prev, "("); j > 0 {
				prev = prev[:j]
			}
		}
		out = append(out, prev+m)
	}
	if len(out) == 0 && cell != "" {
		out = append(out, cell)
	}
	return out
}

func parsePenalty(civil bool, text string) Penalty {
	p := Penalty{Civil: civil, Text: text}
	for _, m := range prisonRE.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(m[1], "life") {
			p.Life = true
			continue
		}
		// "imprisonment for 2 years" fills m[2] and m[3]; "2 years
		// imprisonment" fills m[4] and m[5].
		n, unit := count(m[2]), m[3]
		if m[4] != "" {
			n, unit = count(m[4]), m[5]
		}
		if strings.HasPrefix(strings.ToLower(unit), "year") {
			n *= 12
		}
		p.Months = max(p.Months, n)
	}
	for _, m := range unitsRE.FindAllStringSubmatch(text, -1) {
		p.Units = max(p.Units, count(m[1]))
	}
	return p
}

// count reads a number written in digits or in words.
func count(s string) int {
	if n, ok := numbers[strings.ToLower(s)]; ok {
		return n
	}
	n, _ := strconv.Atoi(strings.ReplaceAll(s, ",", ""))
	return n
}
