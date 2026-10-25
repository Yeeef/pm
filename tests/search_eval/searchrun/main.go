// Command searchrun is the retrieval eval's way into pm search (tests/search_eval/eval.py): it prints what pm search
// prints for each query, run on one as-of corpus per case, as one JSON array on stdout. It calls internal/search's Run,
// the function pm search's command calls, on records read from a directory and items read from a pm export file, so
// it needs no pm service; design pages' dates are not read, so they show as "-". The eval builds it at each revision
// it compares, so it uses only what every compared revision has: records.Read and search.Run.
//
//	searchrun -records DIR -items FILE.jsonl.gz < cases.json
//
// A case is {"id", "keep": record rels kept, "items_before": RFC 3339 time, "queries": [[term, …], …], "n"}; the
// output is [{"id", "outputs": [text per query]}], in the cases' order.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/search"
)

type evalCase struct {
	ID          string     `json:"id"`
	Keep        []string   `json:"keep"`
	ItemsBefore time.Time  `json:"items_before"`
	Queries     [][]string `json:"queries"`
	N           int        `json:"n"`
}

type result struct {
	ID      string   `json:"id"`
	Outputs []string `json:"outputs"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("records", "", "the records tree of the corpus commit")
	itemsPath := flag.String("items", "", "the frozen items, pm export's lines, gzipped")
	flag.Parse()
	recs, err := records.Read(*dir, nil)
	if err != nil {
		return err
	}
	items, err := readItems(*itemsPath)
	if err != nil {
		return err
	}
	var cases []evalCase
	if err := json.NewDecoder(os.Stdin).Decode(&cases); err != nil {
		return err
	}
	out := make([]result, len(cases))
	errs := make([]error, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, c := range cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i], errs[i] = runCase(c, recs, items)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// runCase is pm search's output for each of c's queries over the records c keeps and the items created before
// c.ItemsBefore.
func runCase(c evalCase, recs []*records.Record, items []item.Item) (result, error) {
	keep := map[string]bool{}
	for _, rel := range c.Keep {
		keep[rel] = true
	}
	var rs []*records.Record
	for _, r := range recs {
		if keep[r.Rel] {
			rs = append(rs, r)
		}
	}
	var its []item.Item
	for _, it := range items {
		if it.CreatedAt.Before(c.ItemsBefore) {
			its = append(its, it)
		}
	}
	res := result{ID: c.ID}
	for _, terms := range c.Queries {
		text, err := search.Run(search.Query{Terms: terms, N: c.N}, rs, its, nil)
		if err != nil {
			return res, fmt.Errorf("case %s, query %q: %w", c.ID, terms, err)
		}
		res.Outputs = append(res.Outputs, text)
	}
	return res, nil
}

func readItems(path string) ([]item.Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var out []item.Item
	sc := bufio.NewScanner(z)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var it item.Item
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, sc.Err()
}
