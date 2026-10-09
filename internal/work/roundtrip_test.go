package work

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The round trip of the work-store page's Migration from bd, "Check": import a bd export, read it back with pm
// export, and compare with the bd export on item count, each id's type, status, parent, blockers, title and
// description, the comment count per item, and every need's raised_by and delivered. The expected values are computed
// here from the raw bd JSON by the page's mapping table, not by the importer.
//
// It runs on testdata/bd-export.jsonl (one issue per mapping row), and on a real export when PM_BD_EXPORT names one
// (bd export > file) with PM_BD_RECORDS naming the records store it goes with:
//
//	PM_BD_EXPORT=/tmp/bd-export.jsonl PM_BD_RECORDS=$(pm where records) go test -tags gms_pure_go -run RoundTrip ./internal/work/

func TestRoundTripOfTheFixture(t *testing.T) {
	roundTrip(t, "testdata/bd-export.jsonl", "testdata/records")
}

func TestRoundTripOfARealExport(t *testing.T) {
	export, records := os.Getenv("PM_BD_EXPORT"), os.Getenv("PM_BD_RECORDS")
	if export == "" || records == "" {
		t.Skip("PM_BD_EXPORT and PM_BD_RECORDS name no real bd export and records store")
	}
	roundTrip(t, export, records)
}

// bdExpect is what the page's mapping says pm export holds for one bd issue, on the checked fields.
type bdExpect struct {
	Type, Status, Parent, Title, Description string
	Blockers                                 []string
	Comments                                 int
	RaisedBy                                 map[string]string // nil: no raised_by
	Delivered                                int
}

func expectFromBD(t *testing.T, raw []byte) map[string]bdExpect {
	t.Helper()
	var issues []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		issues = append(issues, m)
	}
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	parentOf := func(m map[string]any) (string, []string) {
		parent, blockers := "", []string{}
		deps, _ := m["dependencies"].([]any)
		for _, d := range deps {
			d := d.(map[string]any)
			if d["type"] == "parent-child" {
				parent = str(d, "depends_on_id")
			} else {
				blockers = append(blockers, str(d, "depends_on_id"))
			}
		}
		return parent, blockers
	}
	out := map[string]bdExpect{}
	for _, m := range issues {
		parent, blockers := parentOf(m)
		labels, _ := m["labels"].([]any)
		human := slices.Contains(labels, any("human"))
		e := bdExpect{Status: "open", Parent: parent, Title: str(m, "title"), Blockers: blockers}
		switch {
		case str(m, "issue_type") == "epic" && parent == "":
			e.Type = "project"
		case str(m, "issue_type") == "epic":
			e.Type = "sprint"
		case human:
			e.Type = "need"
		default:
			e.Type = "task"
		}
		if str(m, "status") == "closed" {
			e.Status = "closed"
		}
		e.Description = str(m, "description")
		if notes := str(m, "notes"); notes != "" {
			if e.Description != "" {
				e.Description += "\n\n"
			}
			e.Description += "## Notes\n\n" + notes
		}
		comments, _ := m["comments"].([]any)
		e.Comments = len(comments)
		if e.Type == "need" {
			meta, _ := m["metadata"].(map[string]any)
			if s := str(meta, "session"); s != "" {
				e.RaisedBy = map[string]string{"session": s, "inbox": str(meta, "inbox"), "host": str(meta, "inbox_host")}
			}
			if n, ok := meta["picked_up"].(float64); ok {
				e.Delivered = int(n)
			}
		}
		out[str(m, "id")] = e
	}
	return out
}

func roundTrip(t *testing.T, exportPath, records string) {
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := ReadBDRecords(records)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	items, err := FromBD(bytes.NewReader(raw), recs)
	if err != nil {
		t.Fatal(err)
	}
	mapped := time.Since(start)

	dir := t.TempDir()
	o := Options{Dir: filepath.Join(dir, "store", "work"), RunDir: filepath.Join(dir, "run"), Prefix: "x"}
	d, err := CreateStore(o)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	if err := d.Import(items, "pm: import the bd export"); err != nil {
		t.Fatal(err)
	}
	imported := time.Since(start)
	if err := d.Shutdown(); err != nil {
		t.Fatal(err)
	}
	d, err = OpenStore(o)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Shutdown()
	back, err := d.Items()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Export(&buf, back); err != nil {
		t.Fatal(err)
	}

	// pm export, read as JSON, against the bd export.
	want := expectFromBD(t, raw)
	got := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		got[m["id"].(string)] = m
	}
	if len(got) != len(want) {
		t.Fatalf("item count: pm export %d, bd export %d", len(got), len(want))
	}
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	comments, blockers, needs := 0, 0, 0
	for id, e := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("%s: not in pm export", id)
			continue
		}
		var gb []string
		for _, b := range asList(g["blocked_by"]) {
			gb = append(gb, b.(string))
		}
		slices.Sort(gb)
		slices.Sort(e.Blockers)
		for _, c := range []struct{ field, got, want string }{
			{"type", str(g, "type"), e.Type}, {"status", str(g, "status"), e.Status},
			{"parent", str(g, "parent"), e.Parent}, {"title", str(g, "title"), e.Title},
			{"description", str(g, "description"), e.Description},
			{"blocked_by", strings.Join(gb, ","), strings.Join(e.Blockers, ",")},
		} {
			if c.got != c.want {
				t.Errorf("%s %s: pm export %q, bd export %q", id, c.field, c.got, c.want)
			}
		}
		if n := len(asList(g["comments"])); n != e.Comments {
			t.Errorf("%s comments: pm export %d, bd export %d", id, n, e.Comments)
		}
		comments += e.Comments
		blockers += len(e.Blockers)
		if e.Type != "need" {
			continue
		}
		needs++
		need, _ := g["need"].(map[string]any)
		if need == nil {
			t.Errorf("%s: a need without need fields in pm export", id)
			continue
		}
		var rb map[string]string
		if r, ok := need["raised_by"].(map[string]any); ok {
			rb = map[string]string{"session": str(r, "session"), "inbox": str(r, "inbox"), "host": str(r, "host")}
		}
		if (rb == nil) != (e.RaisedBy == nil) || rb != nil && (rb["session"] != e.RaisedBy["session"] ||
			rb["inbox"] != e.RaisedBy["inbox"] || rb["host"] != e.RaisedBy["host"]) {
			t.Errorf("%s raised_by: pm export %v, bd export %v", id, rb, e.RaisedBy)
		}
		if n, _ := need["delivered"].(float64); int(n) != e.Delivered {
			t.Errorf("%s delivered: pm export %v, bd export %d", id, need["delivered"], e.Delivered)
		}
	}
	t.Logf("%s: %d items, %d comments, %d blockers, %d needs equal on every checked field; mapped in %s, imported "+
		"in %s", exportPath, len(got), comments, blockers, needs, mapped.Round(time.Millisecond),
		imported.Round(time.Millisecond))
}

func asList(v any) []any { l, _ := v.([]any); return l }
