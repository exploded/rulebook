// Command derive rebuilds the summary tables the pages read: act_summary
// (one row per Act) and statute_year (the statute book on 1 January of each
// year). Run it after any loader.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"rulebook/internal/db"
)

const actSummarySQL = `
INSERT INTO act_summary
SELECT t.id, t.name, t.status, t.in_force, t.is_principal, t.making_date, t.ended_on,
  (SELECT COUNT(*) FROM version v WHERE v.title_id = t.id AND v.register_id <> ''),
  (SELECT v.pages FROM version v WHERE v.title_id = t.id AND v.pages IS NOT NULL ORDER BY v.seq DESC LIMIT 1),
  (SELECT v.pages FROM version v WHERE v.title_id = t.id AND v.seq = 0),
  (SELECT COUNT(DISTINCT a.by_title_id) FROM amendment a WHERE a.title_id = t.id),
  (SELECT COUNT(DISTINCT a.title_id) FROM amendment a WHERE a.by_title_id = t.id),
  COALESCE((SELECT MAX(v.start_date) FROM version v WHERE v.title_id = t.id AND v.seq > 0), ''),
  g.id, COALESCE(g.prime_minister, ''), COALESCE(g.party, ''), COALESCE(g.side, ''),
  COALESCE((SELECT group_concat(portfolio, '; ') FROM
    (SELECT DISTINCT a.portfolio FROM administration a WHERE a.title_id = t.id ORDER BY a.portfolio)), '')
FROM title t
LEFT JOIN government g ON t.making_date >= g.start_date AND (g.end_date = '' OR t.making_date < g.end_date)
WHERE t.collection = 'Act'`

// portfolioSummarySQL splits each Act in force between the portfolios that
// administer it: it counts once in each, and its pages are shared equally.
const portfolioSummarySQL = `
INSERT INTO portfolio_summary
SELECT p.portfolio, COUNT(*), SUM(s.is_principal), SUM(n.k > 1),
  CAST(ROUND(SUM(COALESCE(s.pages, 0) * 1.0 / n.k)) AS INTEGER)
FROM (SELECT DISTINCT title_id, portfolio FROM administration) p
JOIN (SELECT title_id, COUNT(DISTINCT portfolio) AS k FROM administration GROUP BY title_id) n USING (title_id)
JOIN act_summary s ON s.id = p.title_id AND s.in_force = 1
GROUP BY p.portfolio`

func main() {
	dbPath := flag.String("db", "rulebook.db", "SQLite database path")
	flag.Parse()
	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM act_summary"); err != nil {
		log.Fatal(err)
	}
	res, err := tx.ExecContext(ctx, actSummarySQL)
	if err != nil {
		log.Fatal(err)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("act_summary: %d Acts\n", n)

	if _, err := tx.ExecContext(ctx, "DELETE FROM portfolio_summary"); err != nil {
		log.Fatal(err)
	}
	if res, err = tx.ExecContext(ctx, portfolioSummarySQL); err != nil {
		log.Fatal(err)
	}
	n, _ = res.RowsAffected()
	fmt.Printf("portfolio_summary: %d portfolios\n", n)

	years, err := statuteYears(ctx, tx, time.Now().Year())
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM statute_year"); err != nil {
		log.Fatal(err)
	}
	for _, y := range years {
		if _, err := tx.ExecContext(ctx, `INSERT INTO statute_year VALUES (?, ?, ?, ?, ?, ?, ?)`,
			y.Year, y.PrincipalInForce, y.AmendingInForce, y.PagesInForce, y.ActsWithPages, y.MadeLabor, y.MadeNonLabor); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("statute_year: %d years\n", len(years))
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
}

type act struct {
	id, made, ended, side string
	principal             bool
	versions              []ver
}

type ver struct {
	start, end string
	pages      sql.NullInt64
}

// statuteYears counts, for 1 January of each year from 1902 to thisYear,
// the Acts in force and the pages of the compilation each had in force; and
// the Acts made in each year, by side of government.
func statuteYears(ctx context.Context, tx *sql.Tx, thisYear int) ([]db.StatuteYear, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, making_date, ended_on, side, is_principal FROM act_summary`)
	if err != nil {
		return nil, err
	}
	acts := map[string]*act{}
	for rows.Next() {
		a := &act{}
		if err := rows.Scan(&a.id, &a.made, &a.ended, &a.side, &a.principal); err != nil {
			return nil, err
		}
		acts[a.id] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT v.title_id, v.start_date, v.end_date, v.pages FROM version v
		JOIN act_summary s ON s.id = v.title_id ORDER BY v.title_id, v.seq`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var v ver
		if err := rows.Scan(&id, &v.start, &v.end, &v.pages); err != nil {
			return nil, err
		}
		acts[id].versions = append(acts[id].versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []db.StatuteYear
	for y := 1901; y <= thisYear; y++ {
		s := db.StatuteYear{Year: int64(y)}
		day := fmt.Sprintf("%d-01-01", y)
		for _, a := range acts {
			if strings.HasPrefix(a.made, fmt.Sprint(y)) {
				switch a.side {
				case "Labor":
					s.MadeLabor++
				case "Non-Labor":
					s.MadeNonLabor++
				}
			}
			if y == 1901 || a.made == "" || a.made >= day || (a.ended != "" && a.ended <= day) {
				continue
			}
			if a.principal {
				s.PrincipalInForce++
			} else {
				s.AmendingInForce++
			}
			// A version with no published PDF takes the page count of the
			// latest earlier one that has it.
			var pages sql.NullInt64
			for _, v := range a.versions {
				if v.start > day {
					break
				}
				if v.pages.Valid {
					pages = v.pages
				}
				if v.end == "" || v.end > day {
					break
				}
			}
			if pages.Valid {
				s.PagesInForce += pages.Int64
				s.ActsWithPages++
			}
		}
		out = append(out, s)
	}
	return out, nil
}
