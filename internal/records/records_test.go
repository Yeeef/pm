package records

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The expected results of the scanning functions on a table of edge cases, in testdata/expected.json: YAML 1.1
// quoting and header typing (pyyaml's, the reference YAML 1.1 implementation: type name, str() and truth) on every
// header value of the site's test records, section and subsection text, the outcome, first paragraph and summary
// line, the record templates and InsertEntry. They are frozen: a change to one is a change to pm's behaviour, made
// in that file by hand.
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
	t.Helper()
	var ref reference
	b, err := os.ReadFile(filepath.Join("testdata", "expected.json"))
	if err == nil {
		err = json.Unmarshal(b, &ref)
	}
	if err != nil {
		t.Fatal(err)
	}
	return ref
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
			t.Errorf("k: %q gives %v on Go, pyyaml %v", value, got, want)
		}
	}
	t.Run("YAMLStrQuotes", func(t *testing.T) {
		ref := loadReference(t)
		for _, c := range ref.YAMLStr {
			if c[1] == nil {
				continue // pyyaml raises: it constructs an invalid date before comparing
			}
			if got := YAMLStr(*c[0]); got != *c[1] {
				t.Errorf("YAMLStr(%q) = %q, want %q", *c[0], got, *c[1])
			}
		}
		t.Logf("%d values", len(ref.YAMLStr))
	})
	t.Run("Sections", func(t *testing.T) {
		ref := loadReference(t)
		for _, c := range ref.SectionText {
			if got := SectionText(c[0], c[1]); got != c[2] {
				t.Errorf("SectionText(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
			}
		}
		for _, c := range ref.SubsectionText {
			if got := SubsectionText(c[0], c[1]); got != c[2] {
				t.Errorf("SubsectionText(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
			}
		}
		for _, c := range ref.Outcome {
			got, err := Outcome(&Record{Rel: "sprints/x", Body: *c[0]})
			switch {
			case c[2] != nil && (err == nil || err.Error() != *c[2]):
				t.Errorf("Outcome(%q) error %v, want %q", *c[0], err, *c[2])
			case c[2] == nil && (err != nil || got != *c[1]):
				t.Errorf("Outcome(%q) = %q, %v; want %q", *c[0], got, err, *c[1])
			}
		}
		for _, c := range ref.FirstPara {
			if got := FirstPara(c[0]); got != c[1] {
				t.Errorf("FirstPara(%q) = %q, want %q", c[0], got, c[1])
			}
		}
		for _, c := range ref.SummaryLine {
			if got := SummaryLine(c[0]); got != c[1] {
				t.Errorf("SummaryLine(%q) = %q, want %q", c[0], got, c[1])
			}
		}
	})
	t.Run("TemplatesAndInsertEntry", func(t *testing.T) {
		ref := loadReference(t)
		got := map[string]string{
			"sprint":     SprintText("Go port", "demo.9", map[string]string{"Goal": "G.", "Scope": "**In:** a.", "Done when": "- d."}),
			"project":    ProjectText("Demo", "demo", "Goal."),
			"design":     DesignText("The parser", "demo"),
			"postmortem": PostmortemText("Outage", "2026-10-03", "sprint: demo.1"),
		}
		for k, want := range ref.Templates {
			if got[k] != want {
				t.Errorf("%s template differs from the expected one:\n%s\n---\n%s", k, got[k], want)
			}
		}
		for _, c := range ref.InsertEntry {
			out, err := InsertEntry(c[0], c[1], c[2])
			if err != nil {
				out = "error: " + err.Error()
			}
			if out != c[3] {
				t.Errorf("InsertEntry(%q, %q, %q) = %q, want %q", c[0], c[1], c[2], out, c[3])
			}
		}
	})
}

// A summary the listing finds but that is gone when it is read (git rewrites a file by unlinking it, mid-sync) is left
// out, not an error that every page would show: a dangling link is such a file, listed and never readable.
func TestReadSummariesLeavesOutASummaryGoneWhenRead(t *testing.T) {
	root := t.TempDir()
	days := filepath.Join(root, "days")
	if err := os.MkdirAll(days, 0o755); err != nil {
		t.Fatal(err)
	}
	text := `{"date": "2026-10-09", "generated_at": "2026-10-10T00:00:00Z", "digest": "d", "text": "t"}`
	if err := os.WriteFile(filepath.Join(days, "2026-10-09.summary.json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("gone", filepath.Join(days, "2026-10-10.summary.json")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSummaries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["2026-10-09"] == nil {
		t.Fatalf("summaries %v, want 2026-10-09's alone", got)
	}
}
