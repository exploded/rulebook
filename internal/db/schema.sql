-- Every row traces back to a source_document, and every loader run is
-- logged in load_run so the site can publish a data changelog.

CREATE TABLE IF NOT EXISTS source_document (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  title         TEXT NOT NULL,
  publisher     TEXT NOT NULL,
  url           TEXT NOT NULL,
  file_name     TEXT NOT NULL,
  sha256        TEXT NOT NULL UNIQUE,
  doc_type      TEXT NOT NULL,           -- register_api | register_epub | referendums_csv
  retrieved_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS load_run (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  command             TEXT NOT NULL,
  source_document_id  INTEGER NOT NULL REFERENCES source_document(id),
  rows_read           INTEGER NOT NULL,
  rows_written        INTEGER NOT NULL,
  notes               TEXT NOT NULL DEFAULT '',
  finished_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- A law on the Federal Register of Legislation, identified by its title id
-- (C2004Q00685 for the Constitution, C2004A01224 for an Act).
CREATE TABLE IF NOT EXISTS title (
  id                  TEXT PRIMARY KEY,
  name                TEXT NOT NULL,
  collection          TEXT NOT NULL,     -- Constitution | Act | LegislativeInstrument | ...
  status              TEXT NOT NULL,     -- InForce | Repealed | Ceased | NeverEffective
  in_force            INTEGER NOT NULL,
  is_principal        INTEGER NOT NULL,  -- as the Register reports it
  making_date         TEXT NOT NULL,     -- YYYY-MM-DD
  year                INTEGER,
  number              INTEGER,
  bill_uri            TEXT NOT NULL DEFAULT '',
  ended_on            TEXT NOT NULL DEFAULT '',  -- date it was repealed or ceased
  ended_by            TEXT NOT NULL DEFAULT '',  -- title id of the law that ended it
  ended_by_name       TEXT NOT NULL DEFAULT '',
  source_document_id  INTEGER NOT NULL REFERENCES source_document(id)
);

CREATE INDEX IF NOT EXISTS title_collection ON title(collection, in_force);

-- A compilation: the text of a title as in force from start until end.
CREATE TABLE IF NOT EXISTS version (
  title_id            TEXT NOT NULL REFERENCES title(id),
  compilation_number  INTEGER NOT NULL,
  register_id         TEXT NOT NULL,
  start_date          TEXT NOT NULL,
  end_date            TEXT NOT NULL DEFAULT '',  -- '' while current
  is_latest           INTEGER NOT NULL,
  pages               INTEGER,           -- from the compilation's PDF; NULL if unknown
  PRIMARY KEY (title_id, compilation_number)
);

-- One amending title named in a compilation's reasons.
CREATE TABLE IF NOT EXISTS amendment (
  title_id            TEXT NOT NULL REFERENCES title(id),
  compilation_number  INTEGER NOT NULL,
  affect              TEXT NOT NULL,     -- Amend | Repeal | ...
  by_title_id         TEXT NOT NULL,
  by_name             TEXT NOT NULL,
  by_provisions       TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (title_id, compilation_number, by_title_id, by_provisions)
);

CREATE INDEX IF NOT EXISTS amendment_by ON amendment(by_title_id);

-- A section (or covering clause, or schedule) of a title's latest text.
CREATE TABLE IF NOT EXISTS provision (
  title_id       TEXT NOT NULL REFERENCES title(id),
  seq            INTEGER NOT NULL,       -- order in the text
  kind           TEXT NOT NULL,          -- covering | section | schedule
  number         TEXT NOT NULL,          -- 51, 105A; '' for a schedule
  heading        TEXT NOT NULL,
  chapter        TEXT NOT NULL DEFAULT '',  -- I, II ... ('' outside a chapter)
  chapter_title  TEXT NOT NULL DEFAULT '',
  part           TEXT NOT NULL DEFAULT '',
  part_title     TEXT NOT NULL DEFAULT '',
  body           TEXT NOT NULL,          -- paragraphs separated by newlines
  words          INTEGER NOT NULL,
  PRIMARY KEY (title_id, seq)
);

CREATE INDEX IF NOT EXISTS provision_number ON provision(title_id, kind, number);

-- A proposal to alter the Constitution put to electors (s 128).
CREATE TABLE IF NOT EXISTS referendum (
  number             INTEGER PRIMARY KEY,  -- 1..45 in the Parliamentary Handbook's order
  held_on            TEXT NOT NULL,
  subject            TEXT NOT NULL,
  yes_pct            REAL NOT NULL,        -- national
  states_in_favour   TEXT NOT NULL,        -- as the AEC words it: All, None, "Qld, WA, Tas"
  nsw REAL NOT NULL, vic REAL NOT NULL, qld REAL NOT NULL,
  sa  REAL NOT NULL, wa  REAL NOT NULL, tas REAL NOT NULL,
  carried            INTEGER NOT NULL,
  alteration_act     TEXT NOT NULL DEFAULT '',  -- title id of the Constitution Alteration Act
  sections_changed   TEXT NOT NULL DEFAULT '',
  source_document_id INTEGER NOT NULL REFERENCES source_document(id)
);

-- The government of the day: one row per prime ministerial term, checked
-- against the Parliamentary Handbook's ministry dates.
CREATE TABLE IF NOT EXISTS government (
  id                 INTEGER PRIMARY KEY,  -- order in office
  prime_minister     TEXT NOT NULL,
  party              TEXT NOT NULL,
  side               TEXT NOT NULL,        -- Labor | Non-Labor
  start_date         TEXT NOT NULL,
  end_date           TEXT NOT NULL DEFAULT '',  -- '' while in office
  source_document_id INTEGER NOT NULL REFERENCES source_document(id)
);

-- Derived by cmd/derive from title, version, amendment and government.
-- One row per Act.
CREATE TABLE IF NOT EXISTS act_summary (
  id              TEXT PRIMARY KEY REFERENCES title(id),
  name            TEXT NOT NULL,
  status          TEXT NOT NULL,
  in_force        INTEGER NOT NULL,
  is_principal    INTEGER NOT NULL,
  making_date     TEXT NOT NULL,
  ended_on        TEXT NOT NULL,
  compilations    INTEGER NOT NULL,
  pages           INTEGER,               -- latest compilation
  first_pages     INTEGER,               -- as made
  amended_by      INTEGER NOT NULL,      -- distinct laws that amended it
  amends          INTEGER NOT NULL,      -- distinct Acts it amended
  last_changed    TEXT NOT NULL,         -- start of its latest compilation after the first
  government_id   INTEGER REFERENCES government(id),
  prime_minister  TEXT NOT NULL,
  party           TEXT NOT NULL,
  side            TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS act_summary_force ON act_summary(in_force, is_principal);
CREATE INDEX IF NOT EXISTS act_summary_made ON act_summary(making_date);

-- The statute book on 1 January of each year.
CREATE TABLE IF NOT EXISTS statute_year (
  year                INTEGER PRIMARY KEY,
  principal_in_force  INTEGER NOT NULL,
  amending_in_force   INTEGER NOT NULL,
  pages_in_force      INTEGER NOT NULL,  -- Acts whose compilation in force has a page count
  acts_with_pages     INTEGER NOT NULL,
  made_labor          INTEGER NOT NULL,  -- Acts made during the year, by side of government
  made_non_labor      INTEGER NOT NULL
);
