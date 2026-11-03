package handlers

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"rulebook/internal/db"
)

// bandNames are OffenceBands' bands, by number.
var bandNames = []string{
	"No size found",
	"A fine only",
	"Up to 1 year",
	"More than 1 year, up to 2",
	"More than 2 years, up to 5",
	"More than 5 years, up to 10",
	"More than 10 years",
	"Life",
}

type band struct {
	Name     string
	Offences int64
	Share    float64 // of the largest band, for the bar
	Pct      float64 // of all offences
}

type offencesPage struct {
	Crumbs      []crumb
	Totals      db.OffenceTotalsRow
	Bands       []band
	Most        []db.MostOffencesRow
	MostCivil   []db.MostCivilPenaltiesRow
	Portfolios  []db.OffencesByPortfolioRow
	Unreadable  int64 // principal Acts in force whose text has no sections
	InForcePrin int64
}

func (h *Handler) offences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	totals, err := h.q.OffenceTotals(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	if totals.Acts == 0 {
		http.NotFound(w, r)
		return
	}
	bands, err := h.q.OffenceBands(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	most, err := h.q.MostOffences(ctx, 15)
	if err != nil {
		h.fail(w, err)
		return
	}
	civil, err := h.q.MostCivilPenalties(ctx, 10)
	if err != nil {
		h.fail(w, err)
		return
	}
	portfolios, err := h.q.OffencesByPortfolio(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	st, err := h.q.StatuteTotals(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	p := offencesPage{Crumbs: []crumb{{"Home", "/"}, {"The statute book", "/statute-book"}}, Totals: totals,
		Most: most, MostCivil: civil, Portfolios: portfolios, InForcePrin: st.Principal, Unreadable: st.Principal - totals.Acts}
	var largest int64
	for _, b := range bands {
		largest = max(largest, b.Offences)
	}
	for _, b := range bands {
		if b.Band < 0 || int(b.Band) >= len(bandNames) {
			continue
		}
		p.Bands = append(p.Bands, band{Name: bandNames[b.Band], Offences: b.Offences,
			Share: 100 * float64(b.Offences) / float64(largest), Pct: 100 * float64(b.Offences) / float64(totals.Offences)})
	}
	h.render(w, "offences.html", p)
}

func (h *Handler) offencesCSV(w http.ResponseWriter, r *http.Request) {
	ps, err := h.q.AllPenalties(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="penalties.csv"`)
	cw := csv.NewWriter(w)
	cw.Write([]string{"act_id", "act", "provision", "kind", "life", "imprisonment_months", "penalty_units", "penalty_as_written"})
	for _, p := range ps {
		kind := "offence"
		if p.Civil == 1 {
			kind = "civil penalty"
		}
		life := "no"
		if p.Life == 1 {
			life = "yes"
		}
		cw.Write([]string{p.TitleID, p.Name, p.Number, kind, life, strconv.FormatInt(p.Months, 10), strconv.FormatInt(p.Units, 10), p.Text})
	}
	cw.Flush()
}

// term writes a term of imprisonment in months: "6 months", "10 years".
func term(months int64) string {
	switch {
	case months == 0:
		return "–"
	case months%12 == 0 && months/12 == 1:
		return "1 year"
	case months%12 == 0:
		return strconv.FormatInt(months/12, 10) + " years"
	case months == 1:
		return "1 month"
	}
	return strconv.FormatInt(months, 10) + " months"
}
