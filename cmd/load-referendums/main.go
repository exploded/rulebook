// Command load-referendums loads data/referendums.csv: every proposal to
// alter the Constitution put to electors, with national and state results.
//
// The CSV joins the AEC's past referenda fact sheet (dates, subjects, states
// in favour, national yes %) with the Parliamentary Handbook's per-state
// results, plus the 2023 referendum. See README, "Referendums".
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"rulebook/internal/db"
	"rulebook/internal/source"
)

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	path := flag.String("csv", "data/referendums.csv", "referendum results")
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
	docID, err := source.Register(ctx, q, *path, "Constitutional referendum results",
		"Australian Electoral Commission; Parliamentary Library",
		"https://www.aec.gov.au/Elections/referendums/files/past-referenda-fact-sheet.pdf", "referendums_csv")
	if err != nil {
		log.Fatal(err)
	}
	if err := q.DeleteReferendums(ctx); err != nil {
		log.Fatal(err)
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[h] = i
	}
	get := func(r []string, name string) string { return r[col[name]] }
	num := func(r []string, name string) float64 {
		f, err := strconv.ParseFloat(get(r, name), 64)
		if err != nil {
			log.Fatalf("referendum %s: %s: %v", get(r, "number"), name, err)
		}
		return f
	}
	carried := 0
	for _, r := range rows[1:] {
		n, err := strconv.Atoi(get(r, "number"))
		if err != nil {
			log.Fatal(err)
		}
		c := int64(0)
		if get(r, "carried") == "yes" {
			c = 1
			carried++
		}
		if err := q.InsertReferendum(ctx, db.InsertReferendumParams{
			Number:           int64(n),
			HeldOn:           get(r, "date"),
			Subject:          get(r, "subject"),
			YesPct:           num(r, "yes_pct"),
			StatesInFavour:   get(r, "states_in_favour"),
			Nsw:              num(r, "nsw"),
			Vic:              num(r, "vic"),
			Qld:              num(r, "qld"),
			Sa:               num(r, "sa"),
			Wa:               num(r, "wa"),
			Tas:              num(r, "tas"),
			Carried:          c,
			AlterationAct:    get(r, "alteration_act"),
			SectionsChanged:  get(r, "sections_changed"),
			SourceDocumentID: docID,
		}); err != nil {
			log.Fatal(err)
		}
	}
	if err := q.InsertLoadRun(ctx, db.InsertLoadRunParams{
		Command: "load-referendums", SourceDocumentID: docID,
		RowsRead: int64(len(rows) - 1), RowsWritten: int64(len(rows) - 1),
		Notes: fmt.Sprintf("%d proposals, %d carried", len(rows)-1, carried),
	}); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d proposals, %d carried\n", len(rows)-1, carried)
}
