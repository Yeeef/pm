package design

import (
	"reflect"
	"testing"

	"github.com/Yeeef/pm/internal/records"
)

func page(t *testing.T, rel, header string) *records.Record {
	t.Helper()
	rec, err := records.Parse(rel+".md", rel, "---\ntype: design\ntitle: T\nproject: demo\n"+header+"---\n\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestCheckFailsOnADeadPatternAndListsUncoveredSources(t *testing.T) {
	tracked := map[string]bool{"internal/a/a.go": true, "internal/b/b.go": true, "prime.md": true, "README.md": true}
	recs := []*records.Record{page(t, "design/a", "covers: [internal/a/**]\n"), page(t, "design/none", "")}
	got, err := Check(recs, tracked, []string{"internal/**", "prime.md"})
	if err != nil || !reflect.DeepEqual(got, []string{"internal/b/b.go", "prime.md"}) {
		t.Errorf("Check = %v, %v; want the two uncovered sources", got, err)
	}
	if got, err := Check(recs, tracked, nil); err != nil || got != nil {
		t.Errorf("Check without sources = %v, %v; want nothing", got, err)
	}
	recs = append(recs, page(t, "design/dead", "covers: [internal/c/**, prime.md]\n"))
	_, err = Check(recs, tracked, []string{"internal/**"})
	want := `design/dead: covers pattern "internal/c/**" matches no tracked file; fix or remove it (a directory's files ` +
		`are dir/**)`
	if err == nil || err.Error() != want {
		t.Errorf("Check with a dead pattern: %v, want %q", err, want)
	}
}

func TestWaiversReadWhatWaiverTextWrites(t *testing.T) {
	text := "---\ntype: sprint\ntitle: S\nbead: demo.1\n---\n\n## Decisions\n\n::: decision {source=agent date=2026-10-12}\n" +
		WaiverText("parser", "The change renames a test only.") + "\n:::\n\n::: decision {source=agent date=2026-10-12}\n" +
		"The design page lexer stays as it is.\nNot the fixed form.\n:::\n"
	rec, err := records.Parse("sprints/demo-1.md", "sprints/demo-1", text)
	if err != nil {
		t.Fatal(err)
	}
	if got := Waivers(rec); !reflect.DeepEqual(got, map[string]string{"parser": "The change renames a test only."}) {
		t.Errorf("Waivers = %v", got)
	}
}
