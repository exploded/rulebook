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

- `$top` is capped at 500; follow `@odata.nextLink`.
- Filter on `isInForce eq true`, not `status`.
- The `Affect` entity returns 404. Amendment history comes from `Versions`
  `reasons` instead.

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
