// Package analysis counts what a law's text says about offences, penalties
// and delegated powers, by matching the fixed phrases Commonwealth drafting
// uses. Every count is of lines a reader can find in the Act, and none needs
// judgement:
//
//   - An offence is a penalty at the foot of a section or subsection
//     ("Penalty: Imprisonment for 2 years."). Section 4D of the Crimes Act
//     1914 makes such a penalty mean that a contravention is an offence.
//   - A civil penalty provision ends "Civil penalty: ...".
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
)

// Penalty is one penalty at the foot of a section or subsection.
type Penalty struct {
	Civil  bool
	Text   string // as written, after the colon
	Life   bool   // imprisonment for life
	Months int    // longest term of imprisonment, in months; 0 if none
	Units  int    // most penalty units; 0 if none
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
	noteRE     = regexp.MustCompile(`^(Note|Notes|Example|Examples)( \d+)?:`)
	penaltyRE  = regexp.MustCompile(`^(Civil penalty|Penalty)( [^:]{1,80})?:\s*(.*)$`)
	prisonRE   = regexp.MustCompile(`(?i)imprisonment for (life|` + num + ` (years?|months?))|\b` + num + ` (years?|months?)['\x{2019}]? imprisonment`)
	unitsRE    = regexp.MustCompile(`(?i)\b` + num + ` penalty units?\b`)
	strictRE   = regexp.MustCompile(`(?i)(offences? of strict liability|strict liability applies)`)
	absoluteRE = regexp.MustCompile(`(?i)(offences? of absolute liability|absolute liability applies)`)
	regsRE     = regexp.MustCompile(`may make regulations`)
	henryRE    = regexp.MustCompile(`(?i)\b(regulations|rules|instrument)s? (made under [^,]+, )?(may|can) (modify|amend|vary|alter)\b[^.]*\bthis Act\b`)
)

// Analyse returns the counts for a provision's paragraphs.
func Analyse(lines []string) Counts {
	var c Counts
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
		if m := penaltyRE.FindStringSubmatch(l); m != nil {
			text, j := m[3], i+1
			if text == "" && j < len(lines) && !strings.HasPrefix(lines[j], "(") && !noteRE.MatchString(lines[j]) {
				// "Civil penalty:" then "The higher of the following
				// amounts:" on the next paragraph.
				text, j = lines[j], j+1
			}
			if text == "" || strings.HasSuffix(text, ":") {
				// A penalty that varies ("Penalty:" then "(a) for an
				// individual—...") continues on the next paragraphs.
				for k := j; k < len(lines) && k < j+4 && strings.HasPrefix(lines[k], "("); k++ {
					text = strings.TrimSpace(text + " " + lines[k])
				}
			}
			c.Penalties = append(c.Penalties, parsePenalty(m[1] == "Civil penalty", text))
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
