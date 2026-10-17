// Package search is pm search: it finds the record sections, decision blocks, tasks and needs that hold every word of
// a query, across every project, ranks them with BM25 and prints one line per hit (the pm-search design page). It is
// pure: it takes the parsed records, the items and the design pages' dates, and returns the lines, with no I/O.
//
// units.go splits the records and the items into units; tokens.go holds the tokenizer, the S-stemmer and the
// snippet.
package search

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// BM25's parameters, Robertson and Zaragoza's defaults.
const (
	k1 = 1.2
	b  = 0.75
)

// Query is what pm search is asked: the terms, the kinds and the project to keep ("" and nil keep all), and how many
// hits to print.
type Query struct {
	Terms   []string
	Kinds   []string
	Project string
	N       int
}

// Hit is a unit that holds the whole query, with its score.
type Hit struct {
	*Unit
	Score float64
}

// Check refuses a query with no word in it and an -n below 1, before the stores are read.
func Check(q Query) error {
	var tokens []string
	for _, t := range q.Terms {
		tokens = append(tokens, Tokens(t)...)
	}
	if len(tokens) == 0 {
		return fmt.Errorf("give one or more words to search for, such as pm search claim history")
	}
	if q.N < 1 {
		return fmt.Errorf("-n is how many hits to print: give 1 or more, not %d", q.N)
	}
	return nil
}

// Run is pm search's output for q over the records, the items and the design pages' dates; it refuses a query Check
// refuses and an unknown project, naming the projects.
func Run(q Query, recs []*records.Record, items []item.Item, dates map[string]store.Dates) (string, error) {
	if err := Check(q); err != nil {
		return "", err
	}
	if q.Project != "" {
		var names []string
		for _, rec := range recs {
			if rec.Type() == "project" {
				if q.Project == rec.Bead() {
					q.Project = rec.Name()
				}
				names = append(names, rec.Name())
			}
		}
		sort.Strings(names)
		if !slices.Contains(names, q.Project) {
			return "", fmt.Errorf("no project %q; projects: %s", q.Project, strings.Join(names, ", "))
		}
	}
	return Format(q, Rank(Units(recs, items, dates), q)), nil
}

// Rank is the units that hold every token of q's terms, and every term that holds a space, a '/' or a '.' as written,
// kept by q's kinds and project, best first. The scores are BM25 with the statistics of all the units, so a filter
// changes no score; equal scores go by date, newest first, then by address.
func Rank(units []Unit, q Query) []Hit {
	var tokens, literals []string
	for _, term := range q.Terms {
		for _, t := range Tokens(term) {
			if !slices.Contains(tokens, t) {
				tokens = append(tokens, t)
			}
		}
		if literal(term) {
			literals = append(literals, flat(term))
		}
	}
	df := map[string]int{}
	total := 0
	for _, u := range units {
		total += len(u.tokens)
		seen := map[string]bool{}
		for _, t := range u.tokens {
			if !seen[t] && slices.Contains(tokens, t) {
				seen[t] = true
				df[t]++
			}
		}
	}
	n := float64(len(units))
	avgdl := float64(total) / max(n, 1)
	var hits []Hit
	for i := range units {
		u := &units[i]
		if q.Kinds != nil && !slices.Contains(q.Kinds, u.Kind) || q.Project != "" && u.Project != q.Project {
			continue
		}
		tf := map[string]int{}
		for _, t := range u.tokens {
			tf[t]++
		}
		score, all := 0.0, true
		for _, t := range tokens {
			f := float64(tf[t])
			if f == 0 {
				all = false
				break
			}
			idf := math.Log(1 + (n-float64(df[t])+0.5)/(float64(df[t])+0.5))
			score += idf * f * (k1 + 1) / (f + k1*(1-b+b*float64(len(u.tokens))/avgdl))
		}
		if !all {
			continue
		}
		text := flat(u.Text)
		for _, l := range literals {
			all = all && strings.Contains(text, l)
		}
		if all {
			hits = append(hits, Hit{u, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		x, y := hits[i], hits[j]
		switch {
		case x.Score != y.Score:
			return x.Score > y.Score
		case x.Date != y.Date:
			return x.Date > y.Date
		}
		return x.Address < y.Address
	})
	return hits
}

// Format is the output for hits: a header with their count, at most q.N lines of kind, date, state, address and
// snippet in aligned columns, and how to read one; or the one no-match line.
func Format(q Query, hits []Hit) string {
	terms := strings.Join(q.Terms, ", ")
	if len(hits) == 0 {
		return "no unit holds all of: " + terms + "; try other words"
	}
	tokens := map[string]bool{}
	for _, t := range q.Terms {
		for _, tok := range Tokens(t) {
			tokens[tok] = true
		}
	}
	head := fmt.Sprintf("%d units hold all of: %s", len(hits), terms)
	if len(hits) == 1 {
		head = "1 unit holds all of: " + terms
	}
	shown := hits[:min(len(hits), q.N)]
	var w [4]int
	rows := make([][5]string, len(shown))
	for i, h := range shown {
		rows[i] = [5]string{h.Kind, h.Date, h.State, h.Address, snippet(h.snips, tokens)}
		for c := range w {
			w[c] = max(w[c], len([]rune(rows[i][c])))
		}
	}
	lines := []string{head}
	for _, r := range rows {
		line := ""
		for c := range w {
			line += r[c] + strings.Repeat(" ", w[c]-len([]rune(r[c]))+2)
		}
		lines = append(lines, strings.TrimRight(line+r[4], " "))
	}
	return strings.Join(append(lines, "read one: pm show --record <record> --section <section>; an item: pm show <id>"),
		"\n")
}
