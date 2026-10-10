package work

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// exportOf is pm export of the store d reaches: every item, one JSON object per line.
func exportOf(t *testing.T, d *Dolt) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Export(&buf, must(d.Items())); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pm init --import's round trip: a store's pm export, read by FromExport and imported into another clone's empty store
// (another prefix, as a project moving to a new repo has), exports byte for byte as the input. The input is the bd
// fixture imported, so it holds every kind of item, holders, comments, needs and reviews. A second import into that
// store is refused and writes nothing.
func TestFromExportRoundTripsAStoreIntoAnother(t *testing.T) {
	raw, err := os.ReadFile("testdata/bd-export.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	recs, err := ReadBDRecords("testdata/records")
	if err != nil {
		t.Fatal(err)
	}
	items, err := FromBD(bytes.NewReader(raw), recs)
	if err != nil {
		t.Fatal(err)
	}
	from, _ := serve(t, Options{Prefix: "x"})
	if err := from.Import(items, "pm: import the bd export"); err != nil {
		t.Fatal(err)
	}
	in := exportOf(t, from)

	read, err := FromExport(bytes.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	to, _ := serve(t, Options{Prefix: "y"})
	n := commits(t, to)
	if err := to.Import(read, "pm: import the pm export"); err != nil {
		t.Fatal(err)
	}
	if commits(t, to) != n+1 {
		t.Fatalf("the import made %d Dolt commits, not one", commits(t, to)-n)
	}
	if out := exportOf(t, to); !bytes.Equal(out, in) {
		t.Fatalf("pm export after the import differs from its input:\n%s\nwant\n%s", out, in)
	}

	refusal := fmt.Sprintf("work store: it holds %d items already; an import goes into an empty store only", len(read))
	if err := to.Import(read, "again"); err == nil || !strings.Contains(err.Error(), refusal) {
		t.Fatalf("a second import: %v", err)
	}
	if commits(t, to) != n+1 || !bytes.Equal(exportOf(t, to), in) {
		t.Fatal("a refused import wrote the store")
	}
}

// FromExport refuses a line that is not one item in pm export's form, naming the line; blank lines count but are
// skipped.
func TestFromExportRefusesALineThatIsNotAnItem(t *testing.T) {
	project := `{"id":"y-abc","type":"project","title":"P","description":"","status":"open","resolution":null,` +
		`"close_reason":null,"number":null,"parent":null,"blocked_by":[],"labels":[],"holder":null,"started_at":null,` +
		`"created_at":"2026-10-08T12:00:00Z","updated_at":"2026-10-08T12:00:00Z","closed_at":null,"closed_by":null,` +
		`"comments":[],"need":null}`
	items, err := FromExport(strings.NewReader("\n" + project + "\n\n"))
	if err != nil || len(items) != 1 || items[0].ID != "y-abc" || items[0].Parent != "" || items[0].Holder != nil {
		t.Fatalf("one project: %v %+v", err, items)
	}
	for _, c := range []struct{ name, input, want string }{
		{"malformed", project + "\n{\"id\": \n", "import: line 2: "},
		{"unknown key", project + "\n\n" + strings.Replace(project, `"id":"y-abc"`, `"id":"y-def","owner":"me"`, 1),
			`import: line 3: json: unknown field "owner"`},
		{"two values", project + project, "import: line 1: more than one JSON value"},
		{"a stamp not in whole seconds", strings.Replace(project, "12:00:00Z", "12:00:00.5Z", 1),
			"import: work store: y-abc has created_at"},
		{"an id twice", project + "\n" + project, "import: work store: y-abc appears twice"},
	} {
		if _, err := FromExport(strings.NewReader(c.input)); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}
