// Command load-governments loads data/governments.csv: each prime
// ministerial term since 1901 with the PM's party, so an Act can be matched
// to the government in office on the day it was made.
//
// The dates were checked against the Parliamentary Handbook's ministries
// (https://handbookapi.aph.gov.au/api/ministries): every start and end is a
// ministry boundary. See README, "Governments".
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"

	"rulebook/internal/db"
	"rulebook/internal/source"
)

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	path := flag.String("csv", "data/governments.csv", "prime ministerial terms")
	flag.Parse()

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()

	f, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	rows, err := csv.NewReader(f).ReadAll()
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	docID, err := source.Register(ctx, q, *path, "Prime ministerial terms and parties",
		"Parliamentary Library (Parliamentary Handbook)", "https://handbookapi.aph.gov.au/api/ministries", "governments_csv")
	if err != nil {
		log.Fatal(err)
	}
	if err := q.DeleteGovernments(ctx); err != nil {
		log.Fatal(err)
	}
	for i, r := range rows[1:] {
		if err := q.InsertGovernment(ctx, db.InsertGovernmentParams{
			ID: int64(i + 1), PrimeMinister: r[0], Party: r[1], Side: r[2],
			StartDate: r[3], EndDate: r[4], SourceDocumentID: docID,
		}); err != nil {
			log.Fatal(err)
		}
	}
	if err := q.InsertLoadRun(ctx, db.InsertLoadRunParams{
		Command: "load-governments", SourceDocumentID: docID,
		RowsRead: int64(len(rows) - 1), RowsWritten: int64(len(rows) - 1),
		Notes: fmt.Sprintf("%d prime ministerial terms", len(rows)-1),
	}); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d prime ministerial terms\n", len(rows)-1)
}
