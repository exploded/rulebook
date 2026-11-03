package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"rulebook/internal/constitution"
	"rulebook/internal/db"
)

// change is a carried referendum that altered a section.
type change struct {
	Number    int64
	Year      string
	Subject   string
	Paragraph string // the s 51 paragraph it changed, if any
}

// changes maps each section number to the carried referendums that changed it.
func changes(rs []db.Referendum) map[string][]change {
	m := map[string][]change{}
	for _, r := range rs {
		if r.Carried == 0 {
			continue
		}
		for _, ref := range constitution.ParseRefs(r.SectionsChanged) {
			m[ref.Section] = append(m[ref.Section], change{r.Number, r.HeldOn[:4], r.Subject, ref.Paragraph})
		}
	}
	return m
}

// provisionRef is the URL path segment for a provision: s51, c9, schedule.
func provisionRef(kind, number string) string {
	switch kind {
	case "covering":
		return "c" + number
	case "schedule":
		return "schedule"
	}
	return "s" + number
}

func provisionLabel(kind, number string) string {
	switch kind {
	case "covering":
		return "Clause " + number
	case "schedule":
		return "Schedule"
	}
	return "s " + number
}

type tile struct {
	Ref, Label, Heading string
	Words               int64
	Changes             []change
	Repealed            bool
}

type chapter struct {
	Number, Title string
	Words         int64
	Tiles         []tile
}

type head struct {
	constitution.Head
	Changes []change
}

type version struct {
	db.Version
	Amendments []db.Amendment
}

type constitutionPage struct {
	Crumbs        []crumb
	Title         db.Title
	Sections      int
	Words         int64
	CoveringWords int64
	Chapters      []chapter
	Heads         []head
	Versions      []version
	Referendums   int
	Carried       int
}

func (h *Handler) constitution(w http.ResponseWriter, r *http.Request) {
	p, err := h.constitutionData(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	h.render(w, "constitution.html", p)
}

func (h *Handler) constitutionData(ctx context.Context) (constitutionPage, error) {
	var p constitutionPage
	t, err := h.q.GetTitle(ctx, constitution.TitleID)
	if err != nil {
		return p, err
	}
	p.Title = t
	p.Crumbs = []crumb{{"Home", "/"}}
	ps, err := h.q.ListProvisions(ctx, constitution.TitleID)
	if err != nil {
		return p, err
	}
	rs, err := h.q.ListReferendums(ctx)
	if err != nil {
		return p, err
	}
	p.Referendums = len(rs)
	ch := changes(rs)
	for _, r := range rs {
		if r.Carried == 1 {
			p.Carried++
		}
	}

	present := map[string]bool{}
	for _, pr := range ps {
		if pr.Kind == "section" {
			present[pr.Number] = true
		}
	}
	for _, pr := range ps {
		if pr.Kind == "covering" {
			p.CoveringWords += pr.Words
			continue
		}
		p.Words += pr.Words
		if pr.Kind == "section" {
			p.Sections++
		}
		num, title := pr.Chapter, pr.ChapterTitle
		if pr.Kind == "schedule" {
			num, title = "", "The Schedule"
		}
		if len(p.Chapters) == 0 || p.Chapters[len(p.Chapters)-1].Number != num || p.Chapters[len(p.Chapters)-1].Title != title {
			p.Chapters = append(p.Chapters, chapter{Number: num, Title: title})
		}
		c := &p.Chapters[len(p.Chapters)-1]
		c.Words += pr.Words
		c.Tiles = append(c.Tiles, tile{
			Ref: provisionRef(pr.Kind, pr.Number), Label: provisionLabel(pr.Kind, pr.Number),
			Heading: pr.Heading, Words: pr.Words, Changes: ch[pr.Number],
		})
		// A section a referendum repealed is gone from the text; show where
		// it stood, after the section before it.
		if pr.Kind == "section" {
			next := nextNumber(pr.Number)
			if cs, ok := ch[next]; ok && !present[next] {
				c.Tiles = append(c.Tiles, tile{Label: "s " + next, Heading: "Repealed", Changes: cs, Repealed: true})
			}
		}
		if pr.Kind == "section" && pr.Number == "51" {
			for _, hd := range constitution.HeadsOfPower(strings.Split(pr.Body, "\n")) {
				var hc []change
				for _, c := range ch["51"] {
					if c.Paragraph == hd.Number {
						hc = append(hc, c)
					}
				}
				p.Heads = append(p.Heads, head{hd, hc})
			}
		}
	}

	vs, err := h.q.ListVersions(ctx, constitution.TitleID)
	if err != nil {
		return p, err
	}
	as, err := h.q.ListAmendments(ctx, constitution.TitleID)
	if err != nil {
		return p, err
	}
	for _, v := range vs {
		var va []db.Amendment
		for _, a := range as {
			if a.Seq == v.Seq {
				va = append(va, a)
			}
		}
		p.Versions = append(p.Versions, version{v, va})
	}
	return p, nil
}

// nextNumber returns the next whole section number: 126 -> 127.
func nextNumber(n string) string {
	var i int
	if _, err := fmt.Sscanf(n, "%d", &i); err != nil {
		return ""
	}
	return fmt.Sprint(i + 1)
}

type line struct {
	ID, Text string
}

type provisionPage struct {
	Crumbs     []crumb
	Provision  db.Provision
	Label      string
	Lines      []line
	Changes    []change
	Prev, Next *tile
}

var paraIDRE = regexp.MustCompile(`^\(([ivxl]+A?|[a-z])\.?\)`)

func (h *Handler) provision(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	kind, number := "section", ""
	switch {
	case ref == "schedule":
		kind = "schedule"
	case strings.HasPrefix(ref, "c"):
		kind, number = "covering", ref[1:]
	case strings.HasPrefix(ref, "s"):
		number = ref[1:]
	default:
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	pr, err := h.q.GetProvision(ctx, db.GetProvisionParams{TitleID: constitution.TitleID, Kind: kind, Number: number})
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		h.fail(w, err)
		return
	}
	rs, err := h.q.ListReferendums(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	ps, err := h.q.ListProvisions(ctx, constitution.TitleID)
	if err != nil {
		h.fail(w, err)
		return
	}
	p := provisionPage{
		Crumbs:    []crumb{{"Home", "/"}, {"The Constitution", "/constitution"}},
		Provision: pr,
		Label:     provisionLabel(pr.Kind, pr.Number),
	}
	if pr.Kind == "section" {
		p.Changes = changes(rs)[pr.Number]
	}
	for _, l := range strings.Split(pr.Body, "\n") {
		id := ""
		if m := paraIDRE.FindStringSubmatch(l); m != nil {
			id = m[1]
		}
		p.Lines = append(p.Lines, line{id, l})
	}
	for i, o := range ps {
		if o.Seq != pr.Seq {
			continue
		}
		if i > 0 {
			q := ps[i-1]
			p.Prev = &tile{Ref: provisionRef(q.Kind, q.Number), Label: provisionLabel(q.Kind, q.Number), Heading: q.Heading}
		}
		if i+1 < len(ps) {
			q := ps[i+1]
			p.Next = &tile{Ref: provisionRef(q.Kind, q.Number), Label: provisionLabel(q.Kind, q.Number), Heading: q.Heading}
		}
	}
	h.render(w, "provision.html", p)
}
