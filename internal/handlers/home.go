package handlers

import (
	"net/http"

	"rulebook/internal/db"
)

type homePage struct {
	constitutionPage
	Statute db.StatuteTotalsRow
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	c, err := h.constitutionData(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	c.Crumbs = nil
	t, err := h.q.StatuteTotals(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	h.render(w, "home.html", homePage{c, t})
}
