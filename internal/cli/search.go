package cli

import (
	"strconv"

	"github.com/Yeeef/pm/internal/search"
	"github.com/Yeeef/pm/internal/store"
)

// cmdSearch is pm search: the records as pm show reads them, the items and the design pages' dates, which
// internal/search ranks and prints.
func cmdSearch(e *env, p *Parsed) (string, error) {
	n := 10 // -n's default
	if v := p.Get("limit"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil {
			return "", err
		}
	}
	q := search.Query{Terms: p.Values["terms"], Kinds: p.Values["kind"], Project: p.Get("project"), N: n}
	if err := search.Check(q); err != nil {
		return "", err
	}
	r, err := e.load(true)
	if err != nil {
		return "", err
	}
	dates, err := store.DesignDates(r.records, r.recs)
	if err != nil {
		return "", err
	}
	return search.Run(q, r.recs, r.items, dates)
}
