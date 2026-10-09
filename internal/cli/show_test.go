package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Yeeef/yeeef-agents/pm/internal/records"
)

// The expected headings are what Python's records.headings() (markdown-it, commonmark + html + table) gives.
func TestShowHeadingsMatchMarkdownIt(t *testing.T) {
	cases := []struct {
		body string
		want []showHeading
	}{
		{"# A\n\ntext\n## B\n", []showHeading{{1, "A", 0}, {2, "B", 3}}},
		{"```\n# not\n```\n## Real\n", []showHeading{{2, "Real", 3}}},
		{"<div>\n# not\n</div>\n\n## After html\n", []showHeading{{2, "After html", 4}}},
		{"Setext one\n===\n\nTwo\nlines\n---\n", []showHeading{{1, "Setext one", 0}, {2, "Two\nlines", 3}}},
		{"#\n## x ##\n### y #\n", []showHeading{{1, "", 0}, {2, "x", 1}, {3, "y", 2}}},
		{"> ## quoted\n\n- # in list\n", []showHeading{{2, "quoted", 0}, {1, "in list", 2}}},
		{"| a | b |\n|---|---|\n| # c | d |\n\n## T\n", []showHeading{{2, "T", 4}}},
		{"    # indented code\n\n  ##   spaced   \n", []showHeading{{2, "spaced", 2}}},
		{"## Goal\n> prompt\n\ntext\n\n### Sub\n\n## Scope\n", []showHeading{{2, "Goal", 0}, {3, "Sub", 5}, {2, "Scope", 7}}},
		{"Para\n## right after\n", []showHeading{{2, "right after", 1}}},
		{"  Setext indented\n  lines here\n---\n", []showHeading{{2, "Setext indented\n  lines here", 0}}},
	}
	for _, c := range cases {
		if got := showHeadings(c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("showHeadings(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

func TestRecordSectionCutsAtTheNextHeadingOfItsLevelAndRefusesAnUnknownName(t *testing.T) {
	rec := &records.Record{Rel: "sprints/demo-1", Body: "## Goal\n> prompt\n\nG.\n\n### Sub\n\ns\n\n## Scope\n\nIn.\n"}
	got, err := recordSection(rec, "Goal")
	if err != nil || got != "## Goal\n> prompt\n\nG.\n\n### Sub\n\ns" {
		t.Fatalf("Goal: %q %v", got, err)
	}
	if got, err = recordSection(rec, "Sub"); err != nil || got != "### Sub\n\ns" {
		t.Fatalf("Sub: %q %v", got, err)
	}
	_, err = recordSection(rec, "Nope")
	want := "records/sprints/demo-1.md has no section named 'Nope'; its sections:\n  Goal\n    Sub\n  Scope"
	if err == nil || err.Error() != want {
		t.Fatalf("Nope: %v", err)
	}
}

// The expected text is json.dumps(..., indent=1) of the same value.
func TestShowDumpsIsPythonsIndentOne(t *testing.T) {
	v := showObj{{"a", []any{}}, {"b", showObj{}}, {"c", []any{1, nil, true, "é\"x"}}, {"d", showObj{{"e", "f"}}}}
	want := strings.Join([]string{"{", ` "a": [],`, ` "b": {},`, ` "c": [`, `  1,`, `  null,`, `  true,`, "  \"\\u00e9\\\"x\"",
		` ],`, ` "d": {`, `  "e": "f"`, ` }`, "}"}, "\n")
	if got := showDumps(v); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}
