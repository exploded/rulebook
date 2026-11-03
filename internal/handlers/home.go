package handlers

import "net/http"

type homePage struct {
	constitutionPage
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	c, err := h.constitutionData(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	c.Crumbs = nil
	h.render(w, "home.html", homePage{c})
}
