// Command load-text loads the latest text of every principal Act in force,
// split into sections, from the EPUBs the Federal Register of Legislation
// publishes.
//
//	go run ./cmd/load-text
//
// Run it after load-acts, which lists the Acts. Like load-acts, it fetches
// only in the Register's quiet hours (20:00-08:00 AEST) and pauses after
// each live request. EPUBs are cached under .local/register, so a stopped
// run resumes, and a parser change re-runs with no requests at all. Each
// Act's text is written as soon as it is parsed.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"rulebook/internal/db"
	"rulebook/internal/register"
	"rulebook/internal/source"
	"rulebook/internal/statute"
)

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	cache := flag.String("cache", ".local/register", "raw response cache directory")
	pause := flag.Duration("pause", 3*time.Second, "pause after each live request")
	amending := flag.Bool("amending", false, "also load amending Acts in force")
	limit := flag.Int("limit", 0, "load only the first n Acts (0 = all)")
	anyTime := flag.Bool("any-time", false, "don't wait for the Register's quiet hours")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := register.NewClient(*cache)
	c.Pause = *pause
	c.OffPeak = !*anyTime

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	q := db.New(conn)

	where := "collection = 'Act' AND in_force = 1"
	if !*amending {
		where += " AND is_principal = 1"
	}
	rows, err := conn.QueryContext(ctx, "SELECT id, name FROM title WHERE "+where+" ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	type act struct{ id, name string }
	var acts []act
	for rows.Next() {
		var a act
		if err := rows.Scan(&a.id, &a.name); err != nil {
			log.Fatal(err)
		}
		acts = append(acts, a)
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	if *limit > 0 && *limit < len(acts) {
		acts = acts[:*limit]
	}
	log.Printf("%d Acts to load", len(acts))

	start := time.Now()
	runHash := sha256.New()
	var loaded, sections, words, missing, failed int
	for i, a := range acts {
		if ctx.Err() != nil {
			break
		}
		epub, err := c.Get(ctx, register.LatestEpubURL(a.id))
		var se *register.StatusError
		if errors.As(err, &se) && se.Code == 404 {
			log.Printf("%s: no EPUB", a.id)
			missing++
			continue
		} else if err != nil {
			log.Printf("%s: %v", a.id, err)
			failed++
			continue
		}
		sum := sha256.Sum256(epub)
		runHash.Write(sum[:])
		n, w, err := store(ctx, conn, a.id, epub)
		if err != nil {
			log.Printf("%s: %v", a.id, err)
			failed++
			continue
		}
		if n == 0 {
			log.Printf("%s: no sections found", a.id)
		}
		loaded++
		sections += n
		words += w
		if (i+1)%100 == 0 || i+1 == len(acts) {
			log.Printf("%d/%d Acts: %d sections, %d words, %s elapsed", i+1, len(acts), sections, words, time.Since(start).Round(time.Second))
		}
	}

	docID, err := source.RegisterHash(ctx, q, hex.EncodeToString(runHash.Sum(nil)), "act-text-"+time.Now().Format("2006-01-02")+".json",
		"Acts in force: latest compilations (EPUB)", "Office of Parliamentary Counsel",
		register.BaseURL+"/documents/find(titleid=...,asatspecification='Latest',type='Primary',format='Epub')", "register_epub")
	if err != nil {
		log.Fatal(err)
	}
	notes := fmt.Sprintf("%d of %d Acts loaded: %d sections, %d words; %d with no EPUB, %d failed", loaded, len(acts), sections, words, missing, failed)
	if err := q.InsertLoadRun(ctx, db.InsertLoadRunParams{
		Command: "load-text", SourceDocumentID: docID,
		RowsRead: int64(len(acts)), RowsWritten: int64(sections), Notes: notes,
	}); err != nil {
		log.Fatal(err)
	}
	log.Print(notes)
}

// store replaces an Act's provisions with those parsed from its EPUB, and
// returns how many sections and words it has.
func store(ctx context.Context, conn *sql.DB, id string, epub []byte) (sections, words int, err error) {
	docs, err := statute.ReadEPUB(epub)
	if err != nil {
		return 0, 0, err
	}
	ps, err := statute.Parse(docs...)
	if err != nil {
		return 0, 0, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if err := q.DeleteProvisions(ctx, id); err != nil {
		return 0, 0, err
	}
	for i, p := range ps {
		w := p.Words()
		words += w
		if p.Kind == "section" {
			sections++
		}
		if err := q.InsertProvision(ctx, db.InsertProvisionParams{
			TitleID: id, Seq: int64(i + 1), Kind: p.Kind, Number: p.Number, Heading: p.Heading,
			Chapter: p.Chapter, ChapterTitle: p.ChapterTitle, Part: p.Part, PartTitle: p.PartTitle,
			Body: strings.Join(p.Body, "\n"), Words: int64(w),
		}); err != nil {
			return 0, 0, err
		}
	}
	return sections, words, tx.Commit()
}
