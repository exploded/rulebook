// Command snapshot-db writes a consistent, compacted copy of rulebook.db for
// upload to the server (see db.Snapshot).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"rulebook/internal/db"
)

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	out := flag.String("out", "dist/rulebook.db", "snapshot path (replaced if it exists)")
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

	fi, err := os.Stat(*out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d MB)\n", *out, fi.Size()>>20)
}
