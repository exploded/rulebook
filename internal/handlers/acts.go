package handlers

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"rulebook/internal/chart"
	"rulebook/internal/db"
)

const actsPerPage = 50

// treemapAspect is the treemap's width / height, and phoneTreemapAspect the
// taller one phones get, so names fit; the CSS uses the same ratios.
const (
	treemapAspect      = 2.2
	phoneTreemapAspect = 0.8
)

// actFilter is the Acts list's query string. Every value but Portfolio and
// Q is checked against a fixed list, so it can go into SQL as written; those
// two go in as parameters.
type actFilter struct {
	Status, Kind, Side, Portfolio, Q, Sort string
	Page                                   int
}

type option struct{ Value, Label string }

var (
	statusOptions = []option{{"inforce", "In force"}, {"ended", "Repealed or ceased"}, {"all", "All"}}
	kindOptions   = []option{{"all", "Principal and amending"}, {"principal", "Principal Acts"}, {"amending", "Amending Acts"}}
	sideOptions   = []option{{"", "Any government"}, {"Non-Labor", "Non-Labor governments"}, {"Labor", "Labor governments"}}
	sortOptions   = []option{{"newest", "Newest"}, {"oldest", "Oldest"}, {"pages", "Most pages"}, {"amended", "Most amended"}, {"unchanged", "Longest unchanged"}, {"name", "Name"}}
	sortSQL       = map[string]string{
		"newest":    "making_date DESC, id DESC",
		"oldest":    "making_date, id",
		"pages":     "pages IS NULL, pages DESC, id",
		"amended":   "amended_by DESC, id",
		"unchanged": "COALESCE(NULLIF(last_changed, ''), making_date), id",
		"name":      "name COLLATE NOCASE, id",
	}
)

func pick(v string, opts []option) string {
	for _, o := range opts {
		if o.Value == v {
			return v
		}
	}
	return opts[0].Value
}

func parseActFilter(v url.Values) actFilter {
	f := actFilter{
		Status: pick(v.Get("status"), statusOptions),
		Kind:   pick(v.Get("kind"), kindOptions),
		Side:   pick(v.Get("side"), sideOptions),
		Sort:   pick(v.Get("sort"), sortOptions),
		Q:      strings.TrimSpace(v.Get("q")),
	}
	f.Portfolio = strings.TrimSpace(v.Get("portfolio"))
	f.Page, _ = strconv.Atoi(v.Get("page"))
	f.Page = max(f.Page, 1)
	return f
}

// where returns the SQL conditions and arguments for f.
func (f actFilter) where() (string, []any) {
	conds := []string{"1 = 1"}
	var args []any
	switch f.Status {
	case "inforce":
		conds = append(conds, "in_force = 1")
	case "ended":
		conds = append(conds, "in_force = 0")
	}
	switch f.Kind {
	case "principal":
		conds = append(conds, "is_principal = 1")
	case "amending":
		conds = append(conds, "is_principal = 0")
	}
	if f.Side != "" {
		conds = append(conds, "side = ?")
		args = append(args, f.Side)
	}
	if f.Portfolio != "" {
		conds = append(conds, "id IN (SELECT title_id FROM administration WHERE portfolio = ?)")
		args = append(args, f.Portfolio)
	}
	if f.Q != "" {
		conds = append(conds, "name LIKE ?")
		args = append(args, "%"+f.Q+"%")
	}
	return strings.Join(conds, " AND "), args
}

// query is f as a query string, with page replaced; page 1 is left out.
func (f actFilter) query(page int) string {
	v := url.Values{}
	set := func(k, val, def string) {
		if val != def {
			v.Set(k, val)
		}
	}
	set("status", f.Status, "inforce")
	set("kind", f.Kind, "all")
	set("side", f.Side, "")
	set("portfolio", f.Portfolio, "")
	set("sort", f.Sort, "newest")
	set("q", f.Q, "")
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

func (h *Handler) listActs(ctx context.Context, f actFilter, limit, offset int) ([]db.ActSummary, int, error) {
	where, args := f.where()
	var total int
	if err := h.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM act_summary WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := "SELECT * FROM act_summary WHERE " + where + " ORDER BY " + sortSQL[f.Sort]
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
	rows, err := h.conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []db.ActSummary
	for rows.Next() {
		var a db.ActSummary
		if err := rows.Scan(&a.ID, &a.Name, &a.Status, &a.InForce, &a.IsPrincipal, &a.MakingDate, &a.EndedOn,
			&a.Compilations, &a.Pages, &a.FirstPages, &a.AmendedBy, &a.Amends, &a.LastChanged,
			&a.GovernmentID, &a.PrimeMinister, &a.Party, &a.Side, &a.Portfolios); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

type actsPage struct {
	Crumbs                                         []crumb
	Filter                                         actFilter
	StatusOptions, KindOptions, SideOptions, Sorts []option
	PortfolioOptions                               []option // empty until departments are loaded
	Acts                                           []db.ActSummary
	Total, From, To                                int
	PrevURL, NextURL, CSVURL                       string
}

func (h *Handler) acts(w http.ResponseWriter, r *http.Request) {
	f := parseActFilter(r.URL.Query())
	acts, total, err := h.listActs(r.Context(), f, actsPerPage, (f.Page-1)*actsPerPage)
	if err != nil {
		h.fail(w, err)
		return
	}
	portfolios, err := h.q.ListPortfolios(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	var portfolioOptions []option
	if len(portfolios) > 0 {
		portfolioOptions = append(portfolioOptions, option{"", "Any portfolio"})
		for _, pf := range portfolios {
			portfolioOptions = append(portfolioOptions, option{pf.Portfolio, pf.Portfolio})
		}
		slices.SortFunc(portfolioOptions[1:], func(a, b option) int { return strings.Compare(a.Label, b.Label) })
	}
	p := actsPage{
		Crumbs: []crumb{{"Home", "/"}, {"The statute book", "/statute-book"}},
		Filter: f, StatusOptions: statusOptions, KindOptions: kindOptions, SideOptions: sideOptions, Sorts: sortOptions, PortfolioOptions: portfolioOptions,
		Acts: acts, Total: total, From: (f.Page-1)*actsPerPage + 1, To: (f.Page-1)*actsPerPage + len(acts),
		CSVURL: "/acts.csv" + f.query(1),
	}
	if f.Page > 1 {
		p.PrevURL = "/acts" + f.query(f.Page-1)
	}
	if p.To < total {
		p.NextURL = "/acts" + f.query(f.Page+1)
	}
	h.render(w, "acts.html", p)
}

func (h *Handler) actsCSV(w http.ResponseWriter, r *http.Request) {
	f := parseActFilter(r.URL.Query())
	acts, _, err := h.listActs(r.Context(), f, 0, 0)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="acts.csv"`)
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "name", "status", "principal", "made", "ended", "compilations", "pages", "pages_as_made", "amended_by", "amends", "last_changed", "prime_minister", "party", "side", "portfolios"})
	n := func(v sql.NullInt64) string {
		if !v.Valid {
			return ""
		}
		return strconv.FormatInt(v.Int64, 10)
	}
	for _, a := range acts {
		principal := "no"
		if a.IsPrincipal == 1 {
			principal = "yes"
		}
		cw.Write([]string{a.ID, a.Name, a.Status, principal, a.MakingDate, a.EndedOn,
			strconv.FormatInt(a.Compilations, 10), n(a.Pages), n(a.FirstPages),
			strconv.FormatInt(a.AmendedBy, 10), strconv.FormatInt(a.Amends, 10), a.LastChanged,
			a.PrimeMinister, a.Party, a.Side, a.Portfolios})
	}
	cw.Flush()
}

type compilation struct {
	db.Version
	Share float64 // pages as a share of the largest compilation
}

type actPage struct {
	Crumbs       []crumb
	Act          db.ActSummary
	Title        db.Title
	Compilations []compilation
	Uncompiled   int64    // past periods with no compilation, left out of Compilations
	Upcoming     []string // start dates of versions not yet in force
	AmendedBy    []db.ListAmendedByRow
	Amends       []db.ListAmendsRow
	Departments  []db.ListAdministrationRow
	Text         *db.ActText // nil if the text isn't loaded
	Penalties    []db.Penalty
	Scanned      bool // a principal Act in force whose text couldn't be read
}

func (h *Handler) act(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	a, err := h.q.GetActSummary(ctx, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		h.fail(w, err)
		return
	}
	t, err := h.q.GetTitle(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	vs, err := h.q.ListVersions(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	by, err := h.q.ListAmendedBy(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	amends, err := h.q.ListAmends(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	deps, err := h.q.ListAdministration(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	p := actPage{Crumbs: []crumb{{"Home", "/"}, {"The statute book", "/statute-book"}, {"Acts", "/acts"}}, Act: a, Title: t, AmendedBy: by, Amends: amends, Departments: deps}
	if x, err := h.q.GetActText(ctx, id); err == nil {
		if p.Penalties, err = h.q.ListPenalties(ctx, id); err != nil {
			h.fail(w, err)
			return
		}
		p.Text = &x
	} else if err != sql.ErrNoRows {
		h.fail(w, err)
		return
	}
	if p.Text == nil && a.IsPrincipal == 1 && a.InForce == 1 {
		// Only once the text has been analysed: an older database has none.
		if err := h.conn.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM act_text)").Scan(&p.Scanned); err != nil {
			h.fail(w, err)
			return
		}
	}
	var most int64
	for _, v := range vs {
		if v.Pages.Valid {
			most = max(most, v.Pages.Int64)
		}
	}
	today := time.Now().Format("2006-01-02")
	for _, v := range vs {
		switch {
		case v.StartDate > today:
			p.Upcoming = append(p.Upcoming, v.StartDate)
			continue
		case v.RegisterID == "":
			p.Uncompiled++
			continue
		}
		c := compilation{Version: v}
		if v.Pages.Valid && most > 0 {
			c.Share = 100 * float64(v.Pages.Int64) / float64(most)
		}
		p.Compilations = append(p.Compilations, c)
	}
	h.render(w, "act.html", p)
}

type statutePage struct {
	Crumbs       []crumb
	Totals       db.StatuteTotalsRow
	Latest       db.StatuteYear
	Pages        chart.Area
	InForce      chart.Columns
	Made         chart.Columns
	Governments  []db.GovernmentStatsRow
	Largest      []db.ActSummary
	MostAmended  []db.ActSummary
	MadeLabor    int64
	MadeNonLabor int64
	Portfolios   []db.PortfolioSummary
	Tiles        []chart.Tile
	PhoneTiles   []chart.Tile
}

func (h *Handler) statuteBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	totals, err := h.q.StatuteTotals(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	ys, err := h.q.ListStatuteYears(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	govs, err := h.q.GovernmentStats(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	largest, err := h.q.LargestActs(ctx, 10)
	if err != nil {
		h.fail(w, err)
		return
	}
	amended, err := h.q.MostAmendedActs(ctx, 10)
	if err != nil {
		h.fail(w, err)
		return
	}
	portfolios, err := h.q.ListPortfolios(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	p := statutePage{Crumbs: []crumb{{"Home", "/"}}, Totals: totals, Governments: govs, Largest: largest, MostAmended: amended, Portfolios: portfolios}
	var items []chart.Item
	for _, pf := range portfolios {
		items = append(items, chart.Item{
			Name: pf.Portfolio, Value: float64(pf.Pages),
			URL: "/acts?" + url.Values{"portfolio": {pf.Portfolio}, "sort": {"pages"}}.Encode(),
			Tip: fmt.Sprintf("%s|%s pages|%s Acts in force, %s of them principal Acts", pf.Portfolio, chart.Count(float64(pf.Pages)), chart.Count(float64(pf.Acts)), chart.Count(float64(pf.Principal))),
		})
	}
	p.Tiles = chart.Treemap(items, treemapAspect)
	p.PhoneTiles = chart.Treemap(items, phoneTreemapAspect)
	// A phone's treemap is about 360 pixels wide: a name needs a fifth of it.
	for i := range p.PhoneTiles {
		t := &p.PhoneTiles[i]
		t.Label = t.W >= 20 && t.H >= 8
	}
	// 1 January 1901 had no Acts; the series start with 1902.
	var years []int
	var pages, principal, amending, labor, nonLabor []float64
	for _, y := range ys {
		p.MadeLabor += y.MadeLabor
		p.MadeNonLabor += y.MadeNonLabor
		if y.Year == 1901 {
			continue
		}
		years = append(years, int(y.Year))
		pages = append(pages, float64(y.PagesInForce))
		principal = append(principal, float64(y.PrincipalInForce))
		amending = append(amending, float64(y.AmendingInForce))
		labor = append(labor, float64(y.MadeLabor))
		nonLabor = append(nonLabor, float64(y.MadeNonLabor))
		p.Latest = y
	}
	p.Pages = chart.AreaYears(years, pages, func(i int) string {
		y := ys[i+1]
		return fmt.Sprintf("1 January %d|%s pages|Page counts for %s of %s Acts in force", y.Year,
			chart.Count(float64(y.PagesInForce)), chart.Count(float64(y.ActsWithPages)), chart.Count(float64(y.PrincipalInForce+y.AmendingInForce)))
	})
	p.InForce = chart.StackedYears(years, []chart.Series{
		{Name: "Principal Acts", Class: "s-principal", Values: principal},
		{Name: "Amending Acts", Class: "s-amending", Values: amending},
	}, func(i int) string {
		return fmt.Sprintf("1 January %d|Principal Acts: %s|Amending Acts: %s", years[i], chart.Count(principal[i]), chart.Count(amending[i]))
	})
	p.Made = chart.StackedYears(years, []chart.Series{
		{Name: "Non-Labor government", Class: "s-nonlabor", Values: nonLabor},
		{Name: "Labor government", Class: "s-labor", Values: labor},
	}, func(i int) string {
		return fmt.Sprintf("%d|Made under a non-Labor government: %s|Made under a Labor government: %s", years[i], chart.Count(nonLabor[i]), chart.Count(labor[i]))
	})
	h.render(w, "statute.html", p)
}

func (h *Handler) statuteBookCSV(w http.ResponseWriter, r *http.Request) {
	ys, err := h.q.ListStatuteYears(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="statute-book.csv"`)
	cw := csv.NewWriter(w)
	cw.Write([]string{"year", "principal_in_force_1_jan", "amending_in_force_1_jan", "pages_in_force_1_jan", "acts_with_page_counts", "made_labor", "made_non_labor"})
	for _, y := range ys {
		cw.Write([]string{strconv.FormatInt(y.Year, 10), strconv.FormatInt(y.PrincipalInForce, 10), strconv.FormatInt(y.AmendingInForce, 10),
			strconv.FormatInt(y.PagesInForce, 10), strconv.FormatInt(y.ActsWithPages, 10), strconv.FormatInt(y.MadeLabor, 10), strconv.FormatInt(y.MadeNonLabor, 10)})
	}
	cw.Flush()
}
