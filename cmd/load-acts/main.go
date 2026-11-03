// Command load-acts loads every Act on the Federal Register of Legislation:
// its details and status, every compilation with its page count, and the
// laws that amended it.
//
//	go run ./cmd/load-acts
//
// It is gentle by default: one request every 3 seconds, and live requests
// only in the Register's quiet hours (20:00-08:00 AEST), waiting for them
// if need be. Responses are cached under .local/register, so a stopped run
// resumes where it left off. Nothing is written until everything is fetched.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"rulebook/internal/db"
	"rulebook/internal/register"
	"rulebook/internal/source"
)

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	cache := flag.String("cache", ".local/register", "raw response cache directory")
	pause := flag.Duration("pause", 3*time.Second, "pause after each live request")
	batch := flag.Int("batch", 10, "Acts per compilations or documents request")
	limit := flag.Int("limit", 0, "load only the first n Acts (0 = all)")
	anyTime := flag.Bool("any-time", false, "don't wait for the Register's quiet hours")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := register.NewClient(*cache)
	c.Pause = *pause
	c.OffPeak = !*anyTime

	var raw []byte
	start := time.Now()
	acts, err := c.ListActs(ctx, 100, &raw)
	if err != nil {
		log.Fatal(err)
	}
	if *limit > 0 && *limit < len(acts) {
		acts = acts[:*limit]
	}
	log.Printf("%d Acts listed", len(acts))

	var versions []register.Version
	var docs []register.Document
	for i := 0; i < len(acts); i += *batch {
		ids := make([]string, 0, *batch)
		for _, t := range acts[i:min(i+*batch, len(acts))] {
			ids = append(ids, t.ID)
		}
		vs, err := c.VersionsFor(ctx, ids, &raw)
		if err != nil {
			log.Fatalf("versions for %v: %v", ids, err)
		}
		ds, err := c.PDFDocumentsFor(ctx, ids, &raw)
		if err != nil {
			log.Fatalf("documents for %v: %v", ids, err)
		}
		versions = append(versions, vs...)
		docs = append(docs, ds...)
		if n := i + len(ids); n%500 < *batch || n == len(acts) {
			log.Printf("%d/%d Acts: %d compilations, %s elapsed", n, len(acts), len(versions), time.Since(start).Round(time.Second))
		}
	}

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if err := write(ctx, conn, acts, versions, register.Pages(docs), raw); err != nil {
		log.Fatal(err)
	}
}

func write(ctx context.Context, conn *sql.DB, acts []register.Title, versions []register.Version, pages map[string]int64, raw []byte) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	sum := sha256.Sum256(raw)
	docID, err := source.RegisterHash(ctx, q, hex.EncodeToString(sum[:]), "acts-"+time.Now().Format("2006-01-02")+".json",
		"Acts: details, compilations and page counts", "Office of Parliamentary Counsel",
		register.BaseURL+"/Titles?$filter=collection eq 'Act'", "register_api")
	if err != nil {
		return err
	}

	byTitle := map[string][]register.Version{}
	for _, v := range versions {
		byTitle[v.TitleID] = append(byTitle[v.TitleID], v)
	}
	var nVersions, nAmend, withPages int
	for _, t := range acts {
		endedOn, endedBy, endedByName := t.Ended()
		if err := q.UpsertTitle(ctx, db.UpsertTitleParams{
			ID: t.ID, Name: t.Name, Collection: t.Collection, Status: t.Status,
			InForce: b2i(t.IsInForce), IsPrincipal: b2i(t.IsPrincipal),
			MakingDate: register.Date(t.MakingDate), Year: nullInt(t.Year), Number: nullInt(t.Number),
			BillUri: t.OriginatingBillURI, EndedOn: endedOn, EndedBy: endedBy, EndedByName: endedByName,
			SourceDocumentID: docID,
		}); err != nil {
			return fmt.Errorf("%s: %w", t.ID, err)
		}
		if err := q.DeleteVersions(ctx, t.ID); err != nil {
			return err
		}
		if err := q.DeleteAmendments(ctx, t.ID); err != nil {
			return err
		}
		vs := byTitle[t.ID]
		register.SortVersions(vs)
		for seq, v := range vs {
			p := sql.NullInt64{}
			if pg, ok := pages[t.ID+"/"+v.CompilationNumber]; ok && v.CompilationNumber != "" {
				p = sql.NullInt64{Int64: pg, Valid: true}
				withPages++
			}
			if err := q.InsertVersion(ctx, db.InsertVersionParams{
				TitleID: t.ID, CompilationNumber: v.CompilationNumber, Seq: int64(seq), RegisterID: v.RegisterID,
				StartDate: register.Date(v.Start), EndDate: register.Date(v.End), IsLatest: b2i(v.IsLatest), Pages: p,
			}); err != nil {
				return fmt.Errorf("%s version %d: %w", t.ID, seq, err)
			}
			nVersions++
			for _, r := range v.Reasons {
				if r.AffectedByTitle == nil {
					continue
				}
				if err := q.InsertAmendment(ctx, db.InsertAmendmentParams{
					TitleID: t.ID, Seq: int64(seq), Affect: r.Affect,
					ByTitleID: r.AffectedByTitle.TitleID, ByName: r.AffectedByTitle.Name, ByProvisions: r.AffectedByTitle.Provisions,
				}); err != nil {
					return err
				}
				nAmend++
			}
		}
	}
	notes := fmt.Sprintf("%d Acts, %d compilations (%d with page counts), %d amending references", len(acts), nVersions, withPages, nAmend)
	if err := q.InsertLoadRun(ctx, db.InsertLoadRunParams{
		Command: "load-acts", SourceDocumentID: docID,
		RowsRead: int64(len(acts)), RowsWritten: int64(len(acts) + nVersions + nAmend), Notes: notes,
	}); err != nil {
		return err
	}
	log.Print(notes)
	return tx.Commit()
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
