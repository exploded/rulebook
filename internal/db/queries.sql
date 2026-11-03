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
INSERT INTO title (id, name, collection, status, in_force, is_principal, making_date, year, number, bill_uri, source_document_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
  name = excluded.name, collection = excluded.collection, status = excluded.status,
  in_force = excluded.in_force, is_principal = excluded.is_principal,
  making_date = excluded.making_date, year = excluded.year, number = excluded.number,
  bill_uri = excluded.bill_uri, source_document_id = excluded.source_document_id;

-- name: DeleteVersions :exec
DELETE FROM version WHERE title_id = ?;

-- name: InsertVersion :exec
INSERT INTO version (title_id, compilation_number, register_id, start_date, end_date, is_latest)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteAmendments :exec
DELETE FROM amendment WHERE title_id = ?;

-- name: InsertAmendment :exec
INSERT OR IGNORE INTO amendment (title_id, compilation_number, affect, by_title_id, by_name, by_provisions)
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
SELECT * FROM version WHERE title_id = ? ORDER BY compilation_number;

-- name: ListAmendments :many
SELECT * FROM amendment WHERE title_id = ? ORDER BY compilation_number, by_title_id;

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
