// Package search is pm search: it finds the records and items that hold any word of a query, across every project,
// ranks them with BM25, each record one document, and prints one line per hit at the record's best unit: a section, a
// decision block, a task or a need (the pm-search design page; retrieval model 2 of the search-index page). It is
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

// Hit is a record or an item that holds a query term: its best unit for the query, which its line shows, its
// document's score and whether it holds every term.
type Hit struct {
	*Unit
	Score float64
	All   bool
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

// stats is BM25's corpus statistics over a set of token lists: their count, mean length and each query token's
// document frequency.
type stats struct {
	n, avgdl float64
	df       map[string]int
}

func newStats(lists [][]string, tokens []string) stats {
	st := stats{n: float64(len(lists)), df: map[string]int{}}
	total := 0
	for _, l := range lists {
		total += len(l)
		seen := map[string]bool{}
		for _, t := range l {
			if !seen[t] && slices.Contains(tokens, t) {
				seen[t] = true
				st.df[t]++
			}
		}
	}
	st.avgdl = float64(total) / max(st.n, 1)
	return st
}

// score is the BM25 score of a token list of length dl with term counts tf for tokens, any of them (a sum over the
// tokens it holds), and whether it holds all of them.
func (st stats) score(tf map[string]int, dl int, tokens []string) (float64, bool) {
	score, all := 0.0, true
	for _, t := range tokens {
		f := float64(tf[t])
		if f == 0 {
			all = false
			continue
		}
		idf := math.Log(1 + (st.n-float64(st.df[t])+0.5)/(float64(st.df[t])+0.5))
		score += idf * f * (k1 + 1) / (f + k1*(1-b+b*float64(dl)/st.avgdl))
	}
	return score, all
}

func counts(tokens []string) map[string]int {
	tf := map[string]int{}
	for _, t := range tokens {
		tf[t]++
	}
	return tf
}

// document is a record's units, or an item's one unit, with the tokens BM25 scores them by: the record's title once,
// then each unit's own tokens.
type document struct {
	units  []*Unit
	tokens []string
	item   bool
}

func documents(units []Unit) []document {
	var out []document
	at := map[string]int{}
	for i := range units {
		u := &units[i]
		j, ok := at[u.doc]
		if !ok {
			j = len(out)
			at[u.doc] = j
			out = append(out, document{tokens: slices.Clone(u.tokens[:u.lead]), item: u.Kind == "task" || u.Kind == "need"})
		}
		out[j].units = append(out[j].units, u)
		out[j].tokens = append(out[j].tokens, u.tokens[u.lead:]...)
	}
	return out
}

// Rank is the records and items that hold any token of q's terms, and every term that holds a space, a '/' or a '.'
// as written, kept by q's project, best first (retrieval model 2 of the search-index design page). A record or item is
// one BM25 document, scored with the statistics of all of them, so a filter changes no score; its hit shows its best
// unit: the one of q's kinds with the highest BM25 score over all the units' statistics, the first on a tie. The
// document frequencies count records and items together; the mean length is the records' for a record and the items'
// for an item. With kinds, a document is kept only when a unit of those kinds holds a token. Equal scores go by the
// shown unit's date, newest first, then by address.
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
	docs := documents(units)
	lists := make([][]string, len(docs))
	for i, d := range docs {
		lists[i] = d.tokens
	}
	docStats := newStats(lists, tokens)
	// A record is long and an item short, so each is normalized by the mean length of its own set: against one mean,
	// the items would outrank the records that hold the same words.
	length, count := map[bool]float64{}, map[bool]float64{}
	for _, d := range docs {
		length[d.item] += float64(len(d.tokens))
		count[d.item]++
	}
	lists = make([][]string, len(units))
	for i := range units {
		lists[i] = units[i].tokens
	}
	unitStats := newStats(lists, tokens)
	var hits []Hit
	for _, d := range docs {
		if q.Project != "" && d.units[0].Project != q.Project {
			continue
		}
		st := docStats
		st.avgdl = length[d.item] / count[d.item]
		score, all := st.score(counts(d.tokens), len(d.tokens), tokens)
		if score == 0 || !holdsLiterals(d, literals) {
			continue
		}
		var best *Unit
		top := 0.0
		for _, u := range d.units {
			if q.Kinds != nil && !slices.Contains(q.Kinds, u.Kind) {
				continue
			}
			s, _ := unitStats.score(counts(u.tokens), len(u.tokens), tokens)
			if best == nil && q.Kinds == nil || s > top {
				best, top = u, s
			}
		}
		if best != nil {
			hits = append(hits, Hit{best, score, all})
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

// holdsLiterals is whether a document's text holds every literal as written.
func holdsLiterals(d document, literals []string) bool {
	if literals == nil {
		return true
	}
	texts := make([]string, len(d.units))
	for i, u := range d.units {
		texts[i] = u.Text
	}
	text := flat(strings.Join(texts, "\n"))
	for _, l := range literals {
		if !strings.Contains(text, l) {
			return false
		}
	}
	return true
}

// Format is the output for hits: a header with how many hold all of the terms and how many hold some, at most q.N
// lines of kind, date, state, address and snippet in aligned columns, and how to read one; or the one no-match line.
func Format(q Query, hits []Hit) string {
	terms := strings.Join(q.Terms, ", ")
	if len(hits) == 0 {
		return "no record or item holds any of: " + terms + "; try other words"
	}
	tokens := map[string]bool{}
	for _, t := range q.Terms {
		for _, tok := range Tokens(t) {
			tokens[tok] = true
		}
	}
	all := 0
	for _, h := range hits {
		if h.All {
			all++
		}
	}
	head := fmt.Sprintf("%d records hold all of: %s; ", all, terms)
	if all == 1 {
		head = "1 record holds all of: " + terms + "; "
	}
	if len(hits) == 1 {
		head += "1 holds some"
	} else {
		head += fmt.Sprintf("%d hold some", len(hits))
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
