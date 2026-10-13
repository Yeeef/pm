package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/textwrap"
)

// Expected values were Python pm 0.1.x's on the same inputs (its section parser, first sentence and textwrap.fill as
// pm finding add wraps), frozen: Python's \s and str.strip take Unicode spaces, Go's \s only ASCII.

func TestWrapFillPutsALongWordOnItsOwnLine(t *testing.T) {
	want := "- " + strings.TrimSpace(strings.Repeat("a ", 38)) + "\n  " + strings.TrimSpace(strings.Repeat("a ", 12)) +
		"\n  " + strings.Repeat("v", 90) + "\n  end"
	if got := textwrap.Fill(strings.Repeat("a ", 50)+strings.Repeat("v", 90)+" end", 78, "- ", "  "); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	t.Run("ParseSectionsTakesUnicodeSpaceAfterAHeading", func(t *testing.T) {
		got, err := parseSections("## Goal \nShip.\n## Scope \n**In:** a. **Out:** b.\n", frame)
		want := map[string]string{"Goal": "Ship.", "Scope": "**In:** a. **Out:** b."}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, %v; want %q", got, err, want)
		}
	})
	t.Run("FirstSentenceEndsAtAUnicodeSpace", func(t *testing.T) {
		if got := firstSentence("Ends here. Next one.", 110); got != "Ends here." {
			t.Errorf("got %q", got)
		}
	})
	t.Run("RecordSectionCutsAtTheNextHeadingOfItsLevelAndRefusesAnUnknownName", func(t *testing.T) {
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
	})
}
