package cli

import (
	"reflect"
	"strings"
	"testing"
)

// Expected values are Python pm's (cli.parse_sections, cli.first_sentence, textwrap.fill as cmd_finding_add calls it)
// on the same inputs: Python's \s and str.strip take Unicode spaces, Go's \s only ASCII.

func TestParseSectionsTakesUnicodeSpaceAfterAHeading(t *testing.T) {
	got, err := parseSections("## Goal \nShip.\n## Scope \n**In:** a. **Out:** b.\n", frame)
	want := map[string]string{"Goal": "Ship.", "Scope": "**In:** a. **Out:** b."}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestFirstSentenceEndsAtAUnicodeSpace(t *testing.T) {
	if got := firstSentence("Ends here. Next one.", 110); got != "Ends here." {
		t.Errorf("got %q", got)
	}
}

func TestWrapFillPutsALongWordOnItsOwnLine(t *testing.T) {
	want := "- " + strings.TrimSpace(strings.Repeat("a ", 38)) + "\n  " + strings.TrimSpace(strings.Repeat("a ", 12)) +
		"\n  " + strings.Repeat("v", 90) + "\n  end"
	if got := wrapFill(strings.Repeat("a ", 50)+strings.Repeat("v", 90)+" end", 78, "- ", "  "); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
