package records

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Python's results for the code Go pm writes itself, from tests/go_parity_corpus.py ($PM_PARITY/reference.json):
// the scanning code that replaces records.py's lookaround regexes, YAML 1.1 quoting and typing on every header value
// of every corpus, the templates and insert_entry.
type reference struct {
	YAMLStr        [][2]*string         `json:"yaml_str"`
	HeaderType     [][2]json.RawMessage `json:"header_type"`
	SectionText    [][3]string          `json:"section_text"`
	SubsectionText [][3]string          `json:"subsection_text"`
	Outcome        [][3]*string         `json:"outcome"`
	FirstPara      [][2]string          `json:"first_para"`
	SummaryLine    [][2]string          `json:"summary_line"`
	Templates      map[string]string    `json:"templates"`
	InsertEntry    [][4]string          `json:"insert_entry"`
}

func loadReference(t *testing.T) reference {
	dir := os.Getenv("PM_PARITY")
	if dir == "" {
		t.Skip("PM_PARITY names no parity corpus; make test-go writes one with tests/go_parity_corpus.py")
	}
	var ref reference
	b, err := os.ReadFile(filepath.Join(dir, "reference.json"))
	if err == nil {
		err = json.Unmarshal(b, &ref)
	}
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestYAMLStrQuotesAsPythonDoes(t *testing.T) {
	ref := loadReference(t)
	for _, c := range ref.YAMLStr {
		if c[1] == nil {
			continue // Python raises: pyyaml constructs an invalid date before comparing
		}
		if got := YAMLStr(*c[0]); got != *c[1] {
			t.Errorf("YAMLStr(%q) = %q, Python %q", *c[0], got, *c[1])
		}
	}
	t.Logf("%d values", len(ref.YAMLStr))
}

func TestHeaderValuesTypeAsPyyamlDoes(t *testing.T) {
	ref := loadReference(t)
	for _, c := range ref.HeaderType {
		var value string
		var want []any
		json.Unmarshal(c[0], &value)
		json.Unmarshal(c[1], &want)
		meta, err := parseHeader("k: "+value, "x")
		if want == nil {
			if err == nil {
				t.Errorf("k: %q gives %v on Go; pyyaml fails", value, meta["k"])
			}
			continue
		}
		if err != nil {
			t.Errorf("k: %q fails on Go (%v); pyyaml gives %v", value, err, want)
			continue
		}
		got := []any{meta["k"].Kind, meta["k"].Str, meta["k"].Truthy}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("k: %q gives %v on Go, %v on Python", value, got, want)
		}
	}
}

func TestSectionsMatchPythonsRegexes(t *testing.T) {
	ref := loadReference(t)
	for _, c := range ref.SectionText {
		if got := SectionText(c[0], c[1]); got != c[2] {
			t.Errorf("SectionText(%q, %q) = %q, Python %q", c[0], c[1], got, c[2])
		}
	}
	for _, c := range ref.SubsectionText {
		if got := SubsectionText(c[0], c[1]); got != c[2] {
			t.Errorf("SubsectionText(%q, %q) = %q, Python %q", c[0], c[1], got, c[2])
		}
	}
	for _, c := range ref.Outcome {
		got, err := Outcome(&Record{Rel: "sprints/x", Body: *c[0]})
		switch {
		case c[2] != nil && (err == nil || err.Error() != *c[2]):
			t.Errorf("Outcome(%q) error %v, Python %q", *c[0], err, *c[2])
		case c[2] == nil && (err != nil || got != *c[1]):
			t.Errorf("Outcome(%q) = %q, %v; Python %q", *c[0], got, err, *c[1])
		}
	}
	for _, c := range ref.FirstPara {
		if got := FirstPara(c[0]); got != c[1] {
			t.Errorf("FirstPara(%q) = %q, Python %q", c[0], got, c[1])
		}
	}
	for _, c := range ref.SummaryLine {
		if got := SummaryLine(c[0]); got != c[1] {
			t.Errorf("SummaryLine(%q) = %q, Python %q", c[0], got, c[1])
		}
	}
}

func TestTemplatesAndInsertEntryMatchPythons(t *testing.T) {
	ref := loadReference(t)
	got := map[string]string{
		"sprint":     SprintText("Go port", "demo.9", map[string]string{"Goal": "G.", "Scope": "**In:** a.", "Done when": "- d."}),
		"project":    ProjectText("Demo", "demo", "Goal."),
		"design":     DesignText("The parser", "demo"),
		"postmortem": PostmortemText("Outage", "2026-10-03", "sprint: demo.1"),
	}
	for k, want := range ref.Templates {
		if got[k] != want {
			t.Errorf("%s template differs from Python's:\n%s\n---\n%s", k, got[k], want)
		}
	}
	for _, c := range ref.InsertEntry {
		out, err := InsertEntry(c[0], c[1], c[2])
		if err != nil {
			out = "error: " + err.Error()
		}
		if out != c[3] {
			t.Errorf("InsertEntry(%q, %q, %q) = %q, Python %q", c[0], c[1], c[2], out, c[3])
		}
	}
}

// The tests below need no corpus: their expectations come from Python's documented behaviour.

func TestSplitLinesIsPythons(t *testing.T) {
	for in, want := range map[string][]string{
		"": nil, "a": {"a"}, "a\n": {"a"}, "\n": {""}, "a\r\nb": {"a", "b"}, "a\rb\n\nc": {"a", "b", "", "c"},
		"a\x0bb\x1cc\u2028d": {"a", "b", "c", "d"},
	} {
		if got := SplitLines(in); !reflect.DeepEqual(got, want) {
			t.Errorf("SplitLines(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSortsByPathParts(t *testing.T) {
	paths := []string{"/r/a-c.md", "/r/a/b.md", "/r/a.md"}
	SortPaths(paths)
	// sorted() of these Paths in Python 3.13; a string sort would put a-c.md first, since '-' < '/'
	if want := []string{"/r/a/b.md", "/r/a-c.md", "/r/a.md"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("SortPaths = %v, want %v", paths, want)
	}
}

func TestParseReadsTheHeaderWithYAML11(t *testing.T) {
	rec, err := Parse("/x", "docs/2026-10-01-a", "---\ntype: doc\ntitle: yes\ndate: 2026-10-01\nproject: on\n---\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Title() != "True" || rec.Meta["date"].Kind != KindDate || rec.Meta["date"].Str != "2026-10-01" ||
		rec.Meta["project"].Kind != KindBool || rec.Body != "body" {
		t.Errorf("parsed %+v", rec.Meta)
	}
	if _, err := Parse("/x", "x", "---\ntype: day\n---"); err == nil || !strings.Contains(err.Error(), "missing YAML header") {
		t.Errorf("a header without its closing line: %v", err)
	}
}
