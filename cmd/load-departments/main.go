// Command load-departments loads the departments that administer each Act
// in force, as the Federal Register of Legislation lists them.
//
//	go run ./cmd/load-departments
//
// Run it after load-acts, which loads the titles it refers to. Like
// load-acts, it waits for the Register's quiet hours (20:00-08:00 AEST) and
// pauses after each live request; it is about 50 requests.
package main

import (
	"context"
	"crypto/sha256"
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
	anyTime := flag.Bool("any-time", false, "don't wait for the Register's quiet hours")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := register.NewClient(*cache)
	c.Pause = *pause
	c.OffPeak = !*anyTime

	var raw []byte
	acts, err := c.ListAdministeredActs(ctx, 100, &raw)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	sum := sha256.Sum256(raw)
	docID, err := source.RegisterHash(ctx, q, hex.EncodeToString(sum[:]), "departments-"+time.Now().Format("2006-01-02")+".json",
		"Acts in force: administering departments", "Office of Parliamentary Counsel",
		register.BaseURL+"/Titles?$filter=collection eq 'Act' and isInForce eq true&$expand=administeringDepartments", "register_api")
	if err != nil {
		log.Fatal(err)
	}
	if err := q.DeleteAdministration(ctx); err != nil {
		log.Fatal(err)
	}
	known := map[string]bool{}
	ids, err := tx.QueryContext(ctx, "SELECT id FROM title")
	if err != nil {
		log.Fatal(err)
	}
	for ids.Next() {
		var id string
		if err := ids.Scan(&id); err != nil {
			log.Fatal(err)
		}
		known[id] = true
	}
	if err := ids.Err(); err != nil {
		log.Fatal(err)
	}

	var rows, none, unknown int
	for _, a := range acts {
		if !known[a.ID] {
			unknown++
			continue
		}
		if len(a.Departments) == 0 {
			none++
		}
		for _, d := range a.Departments {
			if err := q.InsertAdministration(ctx, db.InsertAdministrationParams{
				TitleID: a.ID, DepartmentID: d.ID, Department: d.Name, Portfolio: d.Portfolio, SourceDocumentID: docID,
			}); err != nil {
				log.Fatalf("%s: %v", a.ID, err)
			}
			rows++
		}
	}
	notes := fmt.Sprintf("%d Acts in force, %d administering departments, %d with none listed, %d not yet loaded by load-acts", len(acts), rows, none, unknown)
	if err := q.InsertLoadRun(ctx, db.InsertLoadRunParams{
		Command: "load-departments", SourceDocumentID: docID,
		RowsRead: int64(len(acts)), RowsWritten: int64(rows), Notes: notes,
	}); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	log.Print(notes)
}
