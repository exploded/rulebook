// Command snapshot-db writes a consistent, compacted copy of rulebook.db for
// upload to the server (see db.Snapshot). The copy keeps the Constitution's
// text but not the Acts' (from load-text), which the site doesn't show yet;
// pass -act-text to keep it.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"rulebook/internal/constitution"
	"rulebook/internal/db"
)

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	out := flag.String("out", "dist/rulebook.db", "snapshot path (replaced if it exists)")
	actText := flag.Bool("act-text", false, "keep the text of Acts")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	os.Remove(*out)

	src, err := db.OpenReadOnly(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer src.Close()
	if err := db.Snapshot(src, *out); err != nil {
		log.Fatal(err)
	}
	if !*actText {
		if err := dropActText(*out); err != nil {
			log.Fatal(err)
		}
	}

	fi, err := os.Stat(*out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d MB)\n", *out, fi.Size()>>20)
}

// dropActText deletes every provision but the Constitution's from the
// snapshot at path and compacts it.
func dropActText(path string) error {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Exec("DELETE FROM provision WHERE title_id <> ?", constitution.TitleID); err != nil {
		return err
	}
	_, err = conn.Exec("VACUUM")
	return err
}
