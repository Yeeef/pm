package work

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// The store-access benchmarks of the service-held store, on a copy of a real store, each client a fresh process as
// a pm command is:
//
//	cp -R <main checkout>/.pm/store/work /tmp/copy && PM_BENCH_STORE=/tmp/copy \
//	  go test -tags gms_pure_go -run Bench -v ./internal/work
//
// TestBenchLoad: 10 processes, each connecting, loading every item and disconnecting, timed inside the process; the
// median and the spread. TestBenchWriters: 8 processes at once, each making 20 writes (a comment on one of 3 shared
// items, a new connection per write, as 20 pm commands would), all 160 checked after, with the wall time, the retries
// and the most attempts one write took. Without PM_BENCH_STORE both skip. The copy is served by a host in this test
// process; the store it was copied from is never opened.

const benchChild = "PM_BENCH_CHILD" // set in a child: what it runs, "load" or "write <n> <id>,<id>,<id>"

func TestMain(m *testing.M) {
	if what := os.Getenv(benchChild); what != "" {
		benchRun(what)
		return
	}
	os.Exit(m.Run())
}

// benchRun is a child process: it prints one JSON line of what it measured.
func benchRun(what string) {
	main := os.Getenv("PM_BENCH_MAIN")
	fail := func(err error) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if what == "load" {
		start := time.Now()
		d, err := DialSock(Sock(main), main, Options{Prefix: "bench"})
		if err != nil {
			fail(err)
		}
		items, err := d.Items()
		if err != nil {
			fail(err)
		}
		if err := d.Shutdown(); err != nil {
			fail(err)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"ms": float64(time.Since(start).Microseconds()) / 1000,
			"items": len(items)})
		return
	}
	var n int
	var ids string
	fmt.Sscanf(what, "write %d %s", &n, &ids)
	shared := strings.Split(ids, ",")
	for i := range 20 {
		d, err := DialSock(Sock(main), main, Options{Prefix: "bench"})
		if err != nil {
			fail(err)
		}
		if _, err := d.Comment(shared[(n+i)%len(shared)], Note, "bench", fmt.Sprintf("w%d-%d", n, i)); err != nil {
			fail(err)
		}
		if err := d.Shutdown(); err != nil {
			fail(err)
		}
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"retries": Attempts.Retries.Load(), "max": Attempts.Max.Load()})
}

// benchStore serves a copy of $PM_BENCH_STORE, skipping without it.
func benchStore(t *testing.T) *served {
	src := os.Getenv("PM_BENCH_STORE")
	if src == "" {
		t.Skip("PM_BENCH_STORE names no copy of a store")
	}
	main := shortMain(t)
	dir, _ := Locations(main)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-R", src, dir).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
	return host(t, main, Options{Prefix: "bench"}, Ops{})
}

// child runs a child process; a failure is the test's error, and an empty result.
func child(t *testing.T, main, what string) map[string]float64 {
	c := exec.Command(os.Args[0], "-test.run", "^$")
	c.Env = append(os.Environ(), benchChild+"="+what, "PM_BENCH_MAIN="+main)
	var stderr strings.Builder
	c.Stderr = &stderr
	out, err := c.Output()
	got := map[string]float64{}
	if err != nil {
		t.Errorf("%s: %v %s", what, err, stderr.String())
	} else if err := json.Unmarshal(out, &got); err != nil {
		t.Errorf("%s printed %q", what, out)
	}
	return got
}

func TestBenchLoad(t *testing.T) {
	s := benchStore(t)
	var ms []float64
	var items float64
	for range 10 {
		got := child(t, s.main, "load")
		ms, items = append(ms, got["ms"]), got["items"]
	}
	slices.Sort(ms)
	t.Logf("load of %v items in a fresh process, 10 runs: median %.1f ms, min %.1f, max %.1f; all %v", items,
		(ms[4]+ms[5])/2, ms[0], ms[9], ms)
}

func TestBenchWriters(t *testing.T) {
	s := benchStore(t)
	d := s.dial(t)
	var parent string
	for _, it := range must(d.Items()) {
		if it.Type == Sprint && it.Status == Open {
			parent = it.ID
		}
	}
	if parent == "" {
		t.Fatal("no open sprint to add the shared items under")
	}
	var ids []string
	for i := range 3 {
		ids = append(ids, must(d.createLocal(New{Type: Task, Parent: parent, Title: fmt.Sprint("bench ", i)})).ID)
	}
	start := time.Now()
	var wg sync.WaitGroup
	results := make([]map[string]float64, 8)
	for n := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[n] = child(t, s.main, fmt.Sprintf("write %d %s", n, strings.Join(ids, ",")))
		}()
	}
	wg.Wait()
	took := time.Since(start)
	var retries, most float64
	for _, r := range results {
		retries, most = retries+r["retries"], max(most, r["max"])
	}
	seen := map[string]int{}
	for _, it := range must(d.Get(ids...)) {
		for _, c := range it.Comments {
			seen[c.Text]++
		}
	}
	landed := 0
	for n := range 8 {
		for i := range 20 {
			if seen[fmt.Sprintf("w%d-%d", n, i)] == 1 {
				landed++
			}
		}
	}
	if landed != 160 {
		t.Errorf("%d of 160 writes landed once", landed)
	}
	t.Logf("8 processes x 20 writes: %d of 160 landed in %s; %v retries, at most %v attempts for one write (bound %d)",
		landed, took.Round(time.Millisecond), retries, most, WriteAttempts)
}
