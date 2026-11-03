# Rulebook

Australia's federal laws, measured: https://rulebook.mchugh.au

Rulebook starts with the Constitution and is adding every Act of Parliament,
with each one's size, age, amendments, offences, delegated powers and High
Court history.

## Run locally

PowerShell on your PC, in `C:\Projects\go\laws`:

```
sh reload.sh
.\build.bat
```

`build.bat` builds the server and starts it on `PORT` from `.env` (8999 by
default). The web app only reads `rulebook.db`; the loaders under `cmd/`
write it.

## Deploy

A push to `main` runs the tests and deploys the binary (templates and static
files are embedded). The database isn't deployed by CI: build it on the PC
and upload it.

PowerShell on your PC, in `C:\Projects\go\laws`:

```
go run ./cmd/snapshot-db
scp -C dist/rulebook.db linode:/tmp/rulebook.db
```

On the Linode (`ssh linode`):

```
sudo install-db-rulebook /tmp/rulebook.db
```

`snapshot-db` uses `VACUUM INTO`, so a loader running at the same time can't
leave a half-written copy. It also switches the copy to a rollback journal.
It leaves out the Acts' text from `load-text`, which the site doesn't show
yet; pass `-act-text` to keep it.
`install-db-rulebook` checks the file is SQLite, swaps it in and restarts
the service.

## Sources

### Federal Register of Legislation

`cmd/load-register` reads the Register's OData API
(https://api.prod.legislation.gov.au/swagger/index.html). It's free, needs no
key, and its content is CC BY 4.0.

- **Title**: details and status (`/v1/Titles('<id>')`).
- **Compilations**: each period of a law's text, with the laws that amended
  it in `reasons` (`/v1/Versions`).
- **Text**: the latest compilation as an EPUB
  (`/v1/documents/find(...)`). `internal/statute` splits it into sections
  using the Word paragraph styles the Office of Parliamentary Counsel applies
  (`ActHead1` chapter, `ActHead2` part, `ActHead5` section).

Responses are cached gzipped under `.local/register`; pass `-refresh` to
fetch again. The client waits a second between live requests. The Register
asks that full crawls run outside 08:00–20:00 AEST, with notice given first
through its feedback form.

API notes:

- `Versions` lists every period in which a title's text stayed the same.
  Most have a compilation number and register id, but some, mostly before
  2004, have neither, and numbers can carry a letter (`12A`). The loaders
  key versions by their order (`seq`), not their number.

- `$top` is capped at 500; follow `@odata.nextLink`.
- Filter on `isInForce eq true`, not `status`.
- The `Affect` entity returns 404. Amendment history comes from `Versions`
  `reasons` instead.
- Tested on 2026-11-03 with about 10 requests:
  - **Counts:** `$count=true` with `$top=1` returns 13,754 Acts, 4,783 of them
    in force, in under 0.3 seconds.
  - **Don't expand `versions` from `Titles`.** That request hit a 504 after
    3 minutes, even for 5 titles.
  - **Batch `Versions` with an `or` filter instead:** `titleId eq 'A' or
    titleId eq 'B' ...` returned 5 titles' compilations in 2 seconds. Batches
    of about 20 cover every Act in about 700 requests.
  - **Expanding `administeringDepartments` is fast** (0.2 seconds). Each entry
    has `id`, `name` and `portfolio`. The navigation path
    `Titles('<id>')/administeringDepartments` returns 404, so use `$expand`.
  - **Page counts:** `Documents`, filtered on `format eq 'Pdf' and type eq
    'Primary'`, gives `pageCount` for every compilation, so we can chart each
    Act's size over time.
  - **`isPrincipal` reads correctly**, though it can't be used as a filter.
    Many in-force Acts are amending Acts whose changes were made long ago (for
    example *Taxation Laws Amendment Act (No. 3) 2000*).

### Acts

`cmd/load-acts` loads every Act: its details and status history (repeal
date and the repealing Act), every compilation, the laws named as amending
it, and each compilation's page count from its PDF. Then `cmd/derive`
rebuilds `act_summary` and `statute_year`, which the pages read.

The loader is deliberately slow. It waits 3 seconds after each live request,
asks for 10 Acts per batch, and only fetches during the Register's quiet hours
(20:00–08:00 AEST, which is 21:00–09:00 during daylight saving), waiting for
them if started during the day. A first run is about 2,900 requests. Cached
responses are reused, so a stopped run resumes.

PowerShell on your PC, in `C:\Projects\go\laws`:

```
go run ./cmd/load-acts
go run ./cmd/load-departments
go run ./cmd/derive
```

### Act text

`cmd/load-text` fetches the latest compilation of every principal Act in
force as an EPUB and splits it into sections with `internal/statute`
(`-amending` adds the amending Acts). It keeps the same quiet hours and
pause, at one request an Act. Notes on the EPUBs:

- A long Act comes as several documents, one per volume
  (`OEBPS/document_N/document_N.html`), with the endnotes last. Each volume
  repeats the title and contents before its first heading.
- Headings: `ActHead1` chapter, `ActHead2` part, `ActHead3` division,
  `ActHead4` subdivision, `ActHead5` section, `ActHead6` schedule. Numbered
  `ActHead5` headings after a schedule are its clauses.
- Section numbers can be hyphenated (`6-5`), written with a non-breaking
  hyphen (U+2011), or dotted in the Criminal Code (`6.1`).
- Compilations prepared before about 2005 mark headings as bare `<h1>` to
  `<h6>` with no class, and start their notes at a `NotesSection`
  paragraph. The parser reads `<h5>` as `ActHead5`, and so on.
- Amending schedules insert text into other Acts with the same heading
  styles. The inserted text follows an `Item` ending in a colon ("Insert:")
  and runs to the next `ItemHead` or the next of the Act's own headings.
  The contents (`TOCN`) list only the Act's own headings, at the same level
  N as their `ActHeadN` style.
- Some Acts never compiled (Loan Acts, Constitution Alteration Acts) come
  as EPUBs converted from print, with no heading styles: 34 of the 1,262
  principal Acts in force in November 2026. They load no sections.
- Check: on 4 November 2026, 1,250 of 1,262 Acts gave exactly as many
  sections and clauses as their contents list.

### Departments

`cmd/load-departments` loads the departments that administer each Act in
force (`$expand=administeringDepartments` on `Titles`, 100 Acts a request,
about 50 requests). Run it after `load-acts`. It keeps the same quiet hours
and pause. `cmd/derive` then fills `act_summary.portfolios` and
`portfolio_summary`, which drives the statute book's treemap. An Act shared
by several portfolios has its pages split equally between them.

### Governments

`data/governments.csv` lists each prime ministerial term since 1901 with the
PM's party and side (Labor or non-Labor). Every start and end date is a
ministry boundary in the Parliamentary Handbook
(https://handbookapi.aph.gov.au/api/ministries); a check script compared all
39 rows on 2026-11-03. `cmd/load-governments` loads it.

### Referendums

`data/referendums.csv` holds all 45 proposals put to electors. It joins:

- the AEC's past referenda fact sheet
  (https://www.aec.gov.au/Elections/referendums/files/past-referenda-fact-sheet.pdf):
  dates, subjects, national yes % and States in favour, for 1906–1999;
- per-State results from the Parliamentary Handbook, as tabled in
  Wikipedia's "Referendums in Australia";
- the AEC's official 2023 result.

Same-day proposals are listed in a different order in each source, so they
were matched on the national yes %, and each match was checked by name. The
fact sheet calls proposal 3 "Finance"; the Handbook calls it "Surplus
Revenue". The `alteration_act` and `sections_changed` columns for the 8
carried proposals come from the Register's compilation history of the
Constitution. `cmd/load-referendums` loads the file.
