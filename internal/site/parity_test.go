package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The parity corpus: Python pm's pages and check results, written by pm/tests/go_parity_corpus.py into $PM_PARITY
// (make test-go does it). Each corpus holds its records store, its items as the neutral-test mapper gives them, and
// Python's result; Go renders the same records with the same items and must give the same pages after Normalise, but
// for the reviewed differences in testdata/parity-allow.txt.

func parityDir(t *testing.T) string {
	dir := os.Getenv("PM_PARITY")
	if dir == "" {
		t.Skip("PM_PARITY names no parity corpus; make test-go writes one with pm/tests/go_parity_corpus.py")
	}
	return dir
}

type corpus struct {
	name, dir string
	recs      []*records.Record
	items     *records.Items
	siteName  string
}

func loadItems(t *testing.T, file string) *records.Items {
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var items []work.Item
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return records.NewItems(items)
}

func corpora(t *testing.T, kind string) []string {
	found, err := filepath.Glob(filepath.Join(parityDir(t), "*", kind+".json"))
	if err != nil || len(found) == 0 {
		t.Fatalf("no %s.json under %s", kind, parityDir(t))
	}
	var out []string
	for _, f := range found {
		out = append(out, filepath.Dir(f))
	}
	return out
}

// result is Python's: pages (or a page count) or the error.
type result struct {
	Pages json.RawMessage `json:"pages"`
	Error *string         `json:"error"`
}

func readResult(t *testing.T, file string) result {
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var r result
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func siteName(t *testing.T, dir string) string {
	var meta struct {
		SiteName string `json:"site_name"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err == nil {
		err = json.Unmarshal(b, &meta)
	}
	if err != nil {
		t.Fatal(err)
	}
	return meta.SiteName
}

// goPages renders a corpus as pm's service does: the working store, design dates from git, the day summaries.
func goPages(dir string, items *records.Items, name string) (map[string]string, error) {
	recs, err := records.Read(filepath.Join(dir, "records"), nil)
	if err != nil {
		return nil, err
	}
	store := filepath.Join(dir, "records")
	dates, err := storeDates(store, recs)
	if err != nil {
		return nil, err
	}
	summaries, err := records.ReadSummaries(store)
	if err != nil {
		return nil, err
	}
	return RenderPages(recs, items, name, dates, summaries)
}

func storeDates(dir string, recs []*records.Record) (Dates, error) {
	return store.DesignDates(dir, recs, store.Today())
}

func goCheck(dir string, items *records.Items, name string) (int, error) {
	store := filepath.Join(dir, "records")
	recs, err := records.Read(store, nil)
	if err != nil {
		return 0, err
	}
	summaries, err := records.ReadSummaries(store)
	if err != nil {
		return 0, err
	}
	return Check(recs, items, name, summaries)
}

func TestPagesEqualPythonsAfterNormalisation(t *testing.T) {
	allow := readAllow(t, "testdata/parity-allow.txt")
	var report []string
	for _, dir := range corpora(t, "pages") {
		name := filepath.Base(dir)
		py := readResult(t, filepath.Join(dir, "pages.json"))
		items := loadItems(t, filepath.Join(dir, "items.json"))
		pages, err := goPages(dir, items, siteName(t, dir))
		if py.Error != nil || err != nil {
			if py.Error == nil || err == nil || err.Error() != *py.Error {
				t.Errorf("%s: Python's error %v, Go's %v", name, deref(py.Error), err)
			}
			continue
		}
		var want map[string]string
		if err := json.Unmarshal(py.Pages, &want); err != nil {
			t.Fatal(err)
		}
		compared, equal, allowed := 0, 0, 0
		for _, p := range union(want, pages) {
			pyPage, inPy := want[p]
			goPage, inGo := pages[p]
			if !inPy || !inGo {
				t.Errorf("%s: page %s rendered by Python %v, by Go %v", name, p, inPy, inGo)
				continue
			}
			compared++
			pn, err1 := Normalise(pyPage)
			gn, err2 := Normalise(goPage)
			if err1 != nil || err2 != nil {
				t.Fatalf("%s %s: %v %v", name, p, err1, err2)
			}
			if pn == gn {
				equal++
				continue
			}
			pn = allow.apply(name, p, pn)
			if pn == gn {
				allowed++
				continue
			}
			t.Errorf("%s %s differs from Python's after normalisation (- Python, + Go):\n%s", name, p, Diff(pn, gn))
		}
		report = append(report, fmt.Sprintf("%s: %d pages compared, %d equal, %d equal with allowed differences",
			name, compared, equal, allowed))
	}
	for _, e := range allow.entries {
		if e.ran && e.used == 0 {
			t.Errorf("parity-allow.txt: the entry %q matched no page; remove it", e.name)
		}
	}
	t.Log("\n" + strings.Join(report, "\n"))
}

func TestCheckEqualsPythons(t *testing.T) {
	n := 0
	for _, dir := range corpora(t, "check") {
		py := readResult(t, filepath.Join(dir, "check.json"))
		pages, err := goCheck(dir, loadItems(t, filepath.Join(dir, "items.json")), siteName(t, dir))
		var got, want string
		if err != nil {
			got = "error: " + err.Error()
		} else {
			got = fmt.Sprintf("%d pages", pages)
		}
		if py.Error != nil {
			want = "error: " + *py.Error
		} else {
			want = string(py.Pages) + " pages"
		}
		if got != want {
			t.Errorf("%s: pm check gives %q on Go, %q on Python", filepath.Base(dir), got, want)
		}
		n++
	}
	t.Logf("%d pm check cases compared", n)
}

func TestCitesMatchesPythonsRegex(t *testing.T) {
	ref := reference(t)
	for _, c := range ref.Cites {
		var text, id string
		var want bool
		json.Unmarshal(c[0], &text)
		json.Unmarshal(c[1], &id)
		json.Unmarshal(c[2], &want)
		if got := Cites(text, id); got != want {
			t.Errorf("Cites(%q, %q) = %v, Python %v", text, id, got, want)
		}
	}
}

func reference(t *testing.T) struct{ Cites [][3]json.RawMessage } {
	var ref struct{ Cites [][3]json.RawMessage }
	b, err := os.ReadFile(filepath.Join(parityDir(t), "reference.json"))
	if err == nil {
		err = json.Unmarshal(b, &ref)
	}
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func union(a, b map[string]string) []string {
	set := map[string]bool{}
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- the allow-list

// allowEntry is one reviewed difference: on the pages its globs name, Python's normalised page with the python regexp
// replaced by go equals Go's.
type allowEntry struct {
	name, reason string
	pages        []string // corpus:glob
	python       *regexp.Regexp
	goForm       string
	ran          bool // a corpus it names was compared
	used         int
}

type allowList struct{ entries []*allowEntry }

func readAllow(t *testing.T, file string) *allowList {
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	list := &allowList{}
	var e *allowEntry
	finish := func() {
		if e == nil {
			return
		}
		if e.name == "" || e.reason == "" || len(e.pages) == 0 || e.python == nil {
			t.Fatalf("%s: the entry %q needs name, reason, pages, python and go", file, e.name)
		}
		list.entries = append(list.entries, e)
		e = nil
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			finish()
			continue
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("%s: %q is no `key: value` line", file, line)
		}
		if e == nil {
			e = &allowEntry{}
		}
		switch key {
		case "name":
			e.name = value
		case "reason":
			e.reason = value
		case "pages":
			e.pages = strings.Fields(value)
		case "python":
			e.python = regexp.MustCompile(value)
		case "go":
			e.goForm = value
		default:
			t.Fatalf("%s: unknown key %q", file, key)
		}
	}
	finish()
	return list
}

func (l *allowList) apply(corpus, page, normalised string) string {
	for _, e := range l.entries {
		for _, g := range e.pages {
			c, glob, _ := strings.Cut(g, ":")
			if c != corpus {
				continue
			}
			e.ran = true
			if ok, _ := path.Match(glob, page); ok && e.python.MatchString(normalised) {
				normalised = e.python.ReplaceAllString(normalised, e.goForm)
				e.used++
			}
		}
	}
	return normalised
}

// ---------------------------------------------------------------- line diff

// Diff is the changed lines of b against a, each run with the line before it: "  " context, "- " a only, "+ " b only.
func Diff(a, b string) string {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	mx, my := x[pre:len(x)-suf], y[pre:len(y)-suf]
	var ops []string
	if len(mx)*len(my) > 4_000_000 {
		for _, l := range mx {
			ops = append(ops, "- "+l)
		}
		for _, l := range my {
			ops = append(ops, "+ "+l)
		}
	} else {
		lcs := make([][]int, len(mx)+1)
		for i := range lcs {
			lcs[i] = make([]int, len(my)+1)
		}
		for i := len(mx) - 1; i >= 0; i-- {
			for j := len(my) - 1; j >= 0; j-- {
				if mx[i] == my[j] {
					lcs[i][j] = lcs[i+1][j+1] + 1
				} else {
					lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
				}
			}
		}
		i, j := 0, 0
		for i < len(mx) || j < len(my) {
			switch {
			case i < len(mx) && j < len(my) && mx[i] == my[j]:
				ops = append(ops, "  "+mx[i])
				i, j = i+1, j+1
			case j < len(my) && (i == len(mx) || lcs[i][j+1] >= lcs[i+1][j]):
				ops = append(ops, "+ "+my[j])
				j++
			default:
				ops = append(ops, "- "+mx[i])
				i++
			}
		}
	}
	var out []string
	if pre > 0 {
		out = append(out, "  "+x[pre-1])
	}
	for k, op := range ops {
		if strings.HasPrefix(op, "  ") && (k+1 >= len(ops) || strings.HasPrefix(ops[k+1], "  ")) {
			continue
		}
		out = append(out, op)
	}
	return strings.Join(out, "\n")
}
