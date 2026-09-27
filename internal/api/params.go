package api

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/store"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	maxPage      = 1000 // deep OFFSET pages get slow; nobody reads page 5000
	maxQueryLen  = 200
)

// parseSearchParams turns query-string values into validated SearchParams.
// Everything a client sends is checked here, so the store can trust its
// input. Bad input gets a 400 that says exactly what was wrong.
//
//	?q=backend engineer&level=senior,lead&country=AE,SA&remote=false&page=2
func parseSearchParams(v url.Values) (p store.SearchParams, page int, err error) {
	p = store.SearchParams{
		Query:    strings.TrimSpace(v.Get("q")),
		Location: strings.TrimSpace(v.Get("location")),
		Company:  strings.TrimSpace(v.Get("company")),
		Limit:    defaultLimit,
	}
	if len(p.Query) > maxQueryLen {
		return p, 0, fmt.Errorf("q must be at most %d characters", maxQueryLen)
	}

	for _, l := range splitList(v.Get("level")) {
		s, ok := job.ParseSeniority(l)
		if !ok {
			return p, 0, fmt.Errorf("unknown level %q; use one of: intern, junior, mid, senior, lead, executive", l)
		}
		p.Seniority = append(p.Seniority, string(s))
	}

	// Countries can be codes or names: "AE", "uae", "Saudi Arabia".
	for _, c := range splitList(v.Get("country")) {
		code := job.CountryCode(c)
		if code == "" {
			return p, 0, fmt.Errorf("unknown country %q; use a code like AE or a name like Saudi Arabia", c)
		}
		p.Countries = append(p.Countries, code)
	}

	// Skills use the detector's canonical names, so "python" becomes "Python".
	for _, name := range splitList(v.Get("skill")) {
		s, ok := job.LookupSkill(name)
		if !ok {
			return p, 0, fmt.Errorf("unknown skill %q; GET /stats lists the most common ones", name)
		}
		p.Skills = append(p.Skills, s.Name)
	}

	if s := v.Get("remote"); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return p, 0, errors.New("remote must be true or false")
		}
		p.Remote = &b
	}
	if s := v.Get("include_closed"); s != "" {
		if p.IncludeClosed, err = strconv.ParseBool(s); err != nil {
			return p, 0, errors.New("include_closed must be true or false")
		}
	}

	switch sort := v.Get("sort"); sort {
	case "":
		// Best match first when searching, newest first when just browsing.
		p.Sort = store.SortNewest
		if p.Query != "" {
			p.Sort = store.SortRelevance
		}
	case store.SortRelevance, store.SortNewest:
		p.Sort = sort
	default:
		return p, 0, errors.New("sort must be relevance or newest")
	}

	page = 1
	if s := v.Get("page"); s != "" {
		if page, err = strconv.Atoi(s); err != nil || page < 1 || page > maxPage {
			return p, 0, fmt.Errorf("page must be between 1 and %d", maxPage)
		}
	}
	if s := v.Get("limit"); s != "" {
		if p.Limit, err = strconv.Atoi(s); err != nil || p.Limit < 1 || p.Limit > maxLimit {
			return p, 0, fmt.Errorf("limit must be between 1 and %d", maxLimit)
		}
	}
	p.Offset = (page - 1) * p.Limit
	return p, page, nil
}

// splitList splits "a, b,,c" into ["a" "b" "c"].
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
