# Rulebook

An explorer for Australian federal law at https://rulebook.mchugh.au.
Repo `exploded/rulebook`, service `rulebook`, port 8999. A sibling of
spendingexplorer (`C:\Projects\go\spendingexplorer`), and copies its
architecture; look there first for a pattern before inventing one.

## Stack
Go, stdlib `net/http` ServeMux, `html/template`, HTMX 1.9 (self-hosted),
SQLite (`modernc.org/sqlite`) with sqlc (`internal/db/queries.sql`, run
`sqlc generate`). Charts are SVG drawn from Go-computed geometry; no JS chart
library. Templates and static files are `go:embed`-ed (`web/embed.go`).

## Rules
- **The web app only reads the database.** Each source has its own loader
  under `cmd/load-*`. Raw responses are cached in `.local/` (gitignored) so
  parsing changes never need a re-fetch.
- **Every row traces to a `source_document`**, and every loader run logs a
  `load_run`. `/sources` publishes them.
- **Stay objective.** The site never states a political stance. Count what
  a reader can check. Where judgement is needed, answer a fixed, published
  question for every law the same way, quote the provision the answer rests
  on, and don't combine answers into scores. Draw questions from Parliament's
  own standards: Senate Scrutiny of Bills principles and ALRC Report 129,
  *Traditional Rights and Freedoms*. "No identifiable victim" is a filter over
  offence harm categories, never a label the site hands out.
- Australian spelling in all copy.
- **Never start, restart or kill James's server on 8999.** Test on another
  port (9001) and stop it before ending the turn.

## Data sources
- **Federal Register of Legislation API**, `https://api.prod.legislation.gov.au/v1`:
  free, no key, CC BY 4.0. Crawl at 1 request a second, outside 08:00–20:00
  AEST for full crawls. See README for API quirks.
- **Open Australian Legal Corpus** (Hugging Face, `isaacus/open-australian-legal-corpus`,
  CC BY 4.0): bulk text of Acts and High Court decisions to March 2025. Planned
  for M3 and M4.

## Ruled out
- **AustLII** (2026-11-03): its terms ban all automated access, including for
  AI use. Don't scrape it or LawCite.
- **Jade** (2026-11-03): subscription only, no public API.

## Decisions
- 2026-11-03: v1 is the Constitution plus every Act; legislative instruments
  (about 24,000 in force) come in v2. Plan:
  `~/.claude/plans/lets-investigate-building-a-snazzy-elephant.md`.
- 2026-11-03: amendment history comes from the Register's `Versions.reasons`,
  which name each amending title. The `Affect` entity returns 404.
- 2026-11-03: the Constitution has 128 sections in its text: 1–128 and 105A,
  less s 127, which the 1967 referendum repealed. s 51 has 40 heads of power,
  (i)–(xxxix) and (xxiiiA). The tests pin these counts.

## Milestones
- M0 scaffold and deploy pipeline; M1 Constitution and referendums (done
  locally 2026-11-03).
- M2: every Act's Register metadata, a hand-curated `data/governments.csv` and
  the growth chart, treemap, Act page and browse page.
- M3: text analysis (offences, strict liability, delegations, the LLM rubric
  through the Batch API).
- M4: High Court citations.
