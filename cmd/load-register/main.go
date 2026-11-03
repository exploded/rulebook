// Command load-register loads titles from the Federal Register of
// Legislation: each title's details, its compilations and the titles that
// amended it, and optionally the sections of its latest text.
//
//	go run ./cmd/load-register -titles C2004Q00685 -text
//
// Raw API responses are cached gzipped under .local/register, so a re-run
// only fetches what it hasn't seen (use -refresh to fetch again).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	"rulebook/internal/db"
	"rulebook/internal/register"
	"rulebook/internal/source"
	"rulebook/internal/statute"
)

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	cache := flag.String("cache", ".local/register", "raw response cache directory")
	titles := flag.String("titles", "C2004Q00685", "comma-separated title ids")
	text := flag.Bool("text", false, "also load the sections of each title's latest text")
	refresh := flag.Bool("refresh", false, "ignore cached responses")
	flag.Parse()

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	c := register.NewClient(*cache)
	c.Refresh = *refresh
	ctx := context.Background()
	for _, id := range strings.Split(*titles, ",") {
		if err := load(ctx, conn, c, strings.TrimSpace(id), *text); err != nil {
			log.Fatalf("%s: %v", id, err)
		}
	}
}

func load(ctx context.Context, conn *sql.DB, c *register.Client, id string, withText bool) error {
	t, rawTitle, err := c.GetTitle(ctx, id)
	if err != nil {
		return err
	}
	versions, rawVersions, err := c.GetVersions(ctx, id)
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)

	docID, err := source.RegisterData(ctx, q, append(rawTitle, rawVersions...), id+".json",
		t.Name+" (Register details and compilations)", "Office of Parliamentary Counsel",
		register.TitleURL(id), "register_api")
	if err != nil {
		return err
	}
	if err := q.UpsertTitle(ctx, db.UpsertTitleParams{
		ID:               t.ID,
		Name:             t.Name,
		Collection:       t.Collection,
		Status:           t.Status,
		InForce:          b2i(t.IsInForce),
		IsPrincipal:      b2i(t.IsPrincipal),
		MakingDate:       register.Date(t.MakingDate),
		Year:             nullInt(t.Year),
		Number:           nullInt(t.Number),
		BillUri:          t.OriginatingBillURI,
		SourceDocumentID: docID,
	}); err != nil {
		return err
	}
	if err := q.DeleteVersions(ctx, id); err != nil {
		return err
	}
	if err := q.DeleteAmendments(ctx, id); err != nil {
		return err
	}
	amendments := 0
	for _, v := range versions {
		n, err := strconv.Atoi(v.CompilationNumber)
		if err != nil {
			return fmt.Errorf("compilation number %q: %w", v.CompilationNumber, err)
		}
		if err := q.InsertVersion(ctx, db.InsertVersionParams{
			TitleID:           id,
			CompilationNumber: int64(n),
			RegisterID:        v.RegisterID,
			StartDate:         register.Date(v.Start),
			EndDate:           register.Date(v.End),
			IsLatest:          b2i(v.IsLatest),
		}); err != nil {
			return err
		}
		for _, r := range v.Reasons {
			if r.AffectedByTitle == nil {
				continue
			}
			if err := q.InsertAmendment(ctx, db.InsertAmendmentParams{
				TitleID:           id,
				CompilationNumber: int64(n),
				Affect:            r.Affect,
				ByTitleID:         r.AffectedByTitle.TitleID,
				ByName:            r.AffectedByTitle.Name,
				ByProvisions:      r.AffectedByTitle.Provisions,
			}); err != nil {
				return err
			}
			amendments++
		}
	}
	if err := q.InsertLoadRun(ctx, db.InsertLoadRunParams{
		Command: "load-register", SourceDocumentID: docID,
		RowsRead: int64(len(versions)), RowsWritten: int64(len(versions) + amendments),
		Notes: fmt.Sprintf("%s: %d compilations, %d amending titles", id, len(versions), amendments),
	}); err != nil {
		return err
	}
	fmt.Printf("%s %s: %d compilations, %d amending titles\n", id, t.Name, len(versions), amendments)

	if withText {
		if err := loadText(ctx, q, c, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadText(ctx context.Context, q *db.Queries, c *register.Client, t register.Title) error {
	url := register.LatestEpubURL(t.ID)
	epub, err := c.Get(ctx, url)
	if err != nil {
		return err
	}
	doc, err := statute.ReadEPUB(epub)
	if err != nil {
		return err
	}
	ps, err := statute.Parse(doc)
	if err != nil {
		return err
	}
	docID, err := source.RegisterData(ctx, q, epub, t.ID+".epub",
		t.Name+" (latest compilation, EPUB)", "Office of Parliamentary Counsel", url, "register_epub")
	if err != nil {
		return err
	}
	if err := q.DeleteProvisions(ctx, t.ID); err != nil {
		return err
	}
	words := 0
	for i, p := range ps {
		words += p.Words()
		if err := q.InsertProvision(ctx, db.InsertProvisionParams{
			TitleID:      t.ID,
			Seq:          int64(i + 1),
			Kind:         p.Kind,
			Number:       p.Number,
			Heading:      p.Heading,
			Chapter:      p.Chapter,
			ChapterTitle: p.ChapterTitle,
			Part:         p.Part,
			PartTitle:    p.PartTitle,
			Body:         strings.Join(p.Body, "\n"),
			Words:        int64(p.Words()),
		}); err != nil {
			return err
		}
	}
	if err := q.InsertLoadRun(ctx, db.InsertLoadRunParams{
		Command: "load-register -text", SourceDocumentID: docID,
		RowsRead: int64(len(ps)), RowsWritten: int64(len(ps)),
		Notes: fmt.Sprintf("%s: %d provisions, %d words", t.ID, len(ps), words),
	}); err != nil {
		return err
	}
	fmt.Printf("%s text: %d provisions, %d words\n", t.ID, len(ps), words)
	return nil
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullInt(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}
