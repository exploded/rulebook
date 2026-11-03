package handlers

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"rulebook/internal/db"
)

// The referendum chart's SVG coordinate space; the CSS scales it to fit.
const (
	chartW, chartH                 = 1000.0, 300.0
	chartL, chartR, chartT, chartB = 40.0, 12.0, 12.0, 28.0
	chartFromYear, chartToYear     = 1900, 2030
)

type point struct {
	X, Y    float64
	Carried bool
	Number  int64
	Tip     string
}

type tick struct {
	Pos   float64
	Label string
}

type referendumChart struct {
	W, H, Left, Right, Top, Bottom float64
	Majority                       float64 // y of the 50% line
	Points                         []point
	XTicks, YTicks                 []tick
}

func chartX(date string) float64 {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return chartL
	}
	from := time.Date(chartFromYear, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(chartToYear, 1, 1, 0, 0, 0, 0, time.UTC)
	return chartL + (chartW-chartL-chartR)*t.Sub(from).Hours()/to.Sub(from).Hours()
}

func chartY(pct float64) float64 {
	return chartT + (chartH-chartT-chartB)*(100-pct)/100
}

func newReferendumChart(rs []db.Referendum) referendumChart {
	c := referendumChart{W: chartW, H: chartH, Left: chartL, Right: chartW - chartR, Top: chartT, Bottom: chartH - chartB, Majority: chartY(50)}
	for y := chartFromYear; y <= chartToYear; y += 20 {
		c.XTicks = append(c.XTicks, tick{chartX(strconv.Itoa(y) + "-01-01"), strconv.Itoa(y)})
	}
	for _, p := range []float64{0, 25, 50, 75, 100} {
		c.YTicks = append(c.YTicks, tick{chartY(p), strconv.Itoa(int(p)) + "%"})
	}
	for _, r := range rs {
		outcome := "Not carried"
		if r.Carried == 1 {
			outcome = "Carried"
		}
		c.Points = append(c.Points, point{
			X: chartX(r.HeldOn), Y: chartY(r.YesPct), Carried: r.Carried == 1, Number: r.Number,
			Tip: r.Subject + " (" + longDate(r.HeldOn) + ")|" + strconv.FormatFloat(r.YesPct, 'f', 2, 64) +
				"% voted yes|States in favour: " + r.StatesInFavour + "|" + outcome,
		})
	}
	return c
}

type referendumRow struct {
	db.Referendum
	States []stateResult
}

type stateResult struct {
	Name string
	Pct  float64
}

type referendumsPage struct {
	Crumbs   []crumb
	Chart    referendumChart
	Rows     []referendumRow
	Carried  int
	Majority int // proposals with a national majority that still failed
}

func (h *Handler) referendums(w http.ResponseWriter, r *http.Request) {
	rs, err := h.q.ListReferendums(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	p := referendumsPage{Crumbs: []crumb{{"Home", "/"}, {"The Constitution", "/constitution"}}, Chart: newReferendumChart(rs)}
	for _, x := range rs {
		if x.Carried == 1 {
			p.Carried++
		} else if x.YesPct > 50 {
			p.Majority++
		}
		p.Rows = append(p.Rows, referendumRow{x, []stateResult{
			{"NSW", x.Nsw}, {"Vic", x.Vic}, {"Qld", x.Qld}, {"SA", x.Sa}, {"WA", x.Wa}, {"Tas", x.Tas},
		}})
	}
	h.render(w, "referendums.html", p)
}

func (h *Handler) referendumsCSV(w http.ResponseWriter, r *http.Request) {
	rs, err := h.q.ListReferendums(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="referendums.csv"`)
	cw := csv.NewWriter(w)
	cw.Write([]string{"number", "date", "subject", "yes_pct", "states_in_favour", "nsw", "vic", "qld", "sa", "wa", "tas", "carried", "alteration_act", "sections_changed"})
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	for _, x := range rs {
		carried := "no"
		if x.Carried == 1 {
			carried = "yes"
		}
		cw.Write([]string{strconv.FormatInt(x.Number, 10), x.HeldOn, x.Subject, f(x.YesPct), x.StatesInFavour,
			f(x.Nsw), f(x.Vic), f(x.Qld), f(x.Sa), f(x.Wa), f(x.Tas), carried, x.AlterationAct, x.SectionsChanged})
	}
	cw.Flush()
}
