-- name: UpsertSourceDocument :one
INSERT INTO source_document (title, publisher, url, file_name, sha256, doc_type)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (sha256) DO UPDATE SET
  title = excluded.title, url = excluded.url, file_name = excluded.file_name
RETURNING id;

-- name: InsertLoadRun :exec
INSERT INTO load_run (command, source_document_id, rows_read, rows_written, notes)
VALUES (?, ?, ?, ?, ?);

-- name: UpsertTitle :exec
INSERT INTO title (id, name, collection, status, in_force, is_principal, making_date, year, number, bill_uri, ended_on, ended_by, ended_by_name, source_document_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
  name = excluded.name, collection = excluded.collection, status = excluded.status,
  in_force = excluded.in_force, is_principal = excluded.is_principal,
  making_date = excluded.making_date, year = excluded.year, number = excluded.number,
  bill_uri = excluded.bill_uri, ended_on = excluded.ended_on, ended_by = excluded.ended_by,
  ended_by_name = excluded.ended_by_name, source_document_id = excluded.source_document_id;

-- name: DeleteVersions :exec
DELETE FROM version WHERE title_id = ?;

-- name: InsertVersion :exec
INSERT INTO version (title_id, seq, compilation_number, register_id, start_date, end_date, is_latest, pages)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteAmendments :exec
DELETE FROM amendment WHERE title_id = ?;

-- name: InsertAmendment :exec
INSERT OR IGNORE INTO amendment (title_id, seq, affect, by_title_id, by_name, by_provisions)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteProvisions :exec
DELETE FROM provision WHERE title_id = ?;

-- name: InsertProvision :exec
INSERT INTO provision (title_id, seq, kind, number, heading, chapter, chapter_title, part, part_title, body, words)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteReferendums :exec
DELETE FROM referendum;

-- name: InsertReferendum :exec
INSERT INTO referendum (number, held_on, subject, yes_pct, states_in_favour, nsw, vic, qld, sa, wa, tas, carried, alteration_act, sections_changed, source_document_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetTitle :one
SELECT * FROM title WHERE id = ?;

-- name: ListVersions :many
SELECT * FROM version WHERE title_id = ? ORDER BY seq;

-- name: ListAmendments :many
SELECT * FROM amendment WHERE title_id = ? ORDER BY seq, by_title_id;

-- name: ListProvisions :many
SELECT * FROM provision WHERE title_id = ? ORDER BY seq;

-- name: GetProvision :one
SELECT * FROM provision WHERE title_id = ? AND kind = ? AND number = ?;

-- name: ListReferendums :many
SELECT * FROM referendum ORDER BY number;

-- name: ListLoadRuns :many
SELECT lr.command, lr.rows_read, lr.rows_written, lr.notes, lr.finished_at,
       sd.title, sd.publisher, sd.url, sd.file_name, sd.sha256
FROM load_run lr JOIN source_document sd ON sd.id = lr.source_document_id
ORDER BY lr.finished_at DESC, lr.id DESC
LIMIT 200;

-- name: DeleteGovernments :exec
DELETE FROM government;

-- name: InsertGovernment :exec
INSERT INTO government (id, prime_minister, party, side, start_date, end_date, source_document_id)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetActSummary :one
SELECT * FROM act_summary WHERE id = ?;

-- name: ListAmendedBy :many
SELECT a.by_title_id, a.by_name, COALESCE(t.making_date, '') AS made,
       CAST(MIN(v.start_date) AS TEXT) AS took_effect
FROM amendment a
JOIN version v ON v.title_id = a.title_id AND v.seq = a.seq
LEFT JOIN title t ON t.id = a.by_title_id
WHERE a.title_id = ?
GROUP BY a.by_title_id, a.by_name
ORDER BY made DESC, a.by_title_id;

-- name: ListAmends :many
SELECT a.title_id, t.name, t.collection, t.in_force
FROM amendment a JOIN title t ON t.id = a.title_id
WHERE a.by_title_id = ?
GROUP BY a.title_id, t.name, t.collection, t.in_force
ORDER BY t.name;

-- name: ListStatuteYears :many
SELECT * FROM statute_year ORDER BY year;

-- name: StatuteTotals :one
SELECT CAST(COUNT(*) AS INTEGER) AS acts,
       CAST(COALESCE(SUM(in_force), 0) AS INTEGER) AS in_force,
       CAST(COALESCE(SUM(CASE WHEN in_force = 1 AND is_principal = 1 THEN 1 ELSE 0 END), 0) AS INTEGER) AS principal,
       CAST(COALESCE(SUM(CASE WHEN in_force = 1 AND is_principal = 0 THEN 1 ELSE 0 END), 0) AS INTEGER) AS amending,
       CAST(COALESCE(SUM(CASE WHEN in_force = 1 THEN pages END), 0) AS INTEGER) AS pages,
       CAST(COALESCE(SUM(CASE WHEN in_force = 1 AND pages IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS with_pages
FROM act_summary;

-- name: GovernmentStats :many
SELECT g.id, g.prime_minister, g.party, g.side, g.start_date, g.end_date,
       CAST(COUNT(s.id) AS INTEGER) AS made,
       CAST(COALESCE(SUM(s.is_principal), 0) AS INTEGER) AS principal_made,
       CAST(COALESCE(SUM(CASE WHEN s.is_principal = 1 AND s.in_force = 1 THEN 1 ELSE 0 END), 0) AS INTEGER) AS principal_in_force,
       CAST(COALESCE(SUM(CASE WHEN s.is_principal = 1 AND s.in_force = 1 THEN s.pages END), 0) AS INTEGER) AS pages_in_force
FROM government g LEFT JOIN act_summary s ON s.government_id = g.id
GROUP BY g.id
ORDER BY g.id;

-- name: LargestActs :many
SELECT * FROM act_summary WHERE in_force = 1 AND pages IS NOT NULL ORDER BY pages DESC LIMIT ?;

-- name: MostAmendedActs :many
SELECT * FROM act_summary WHERE in_force = 1 ORDER BY amended_by DESC LIMIT ?;

-- name: DeleteAdministration :exec
DELETE FROM administration;

-- name: InsertAdministration :exec
INSERT OR IGNORE INTO administration (title_id, department_id, department, portfolio, source_document_id)
VALUES (?, ?, ?, ?, ?);

-- name: ListAdministration :many
SELECT department_id, department, portfolio FROM administration WHERE title_id = ? ORDER BY portfolio, department;

-- name: ListPortfolios :many
SELECT * FROM portfolio_summary ORDER BY pages DESC, portfolio;
