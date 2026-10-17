package search

import (
	"reflect"
	"testing"
)

// The S-stemmer's rules as the pm-search design page gives them (Harman 1991), on tokens of 4 runes or more.
func TestStemFollowsTheSStemmerRules(t *testing.T) {
	for in, want := range map[string]string{
		"pages":   "page",  // -es → -e
		"claims":  "claim", // -s → nothing
		"queries": "query", // -ies → -y
		"status":  "status",
		"access":  "access",
		"does":    "doe",   // -oes keeps rule 2 off; rule 3 drops the s
		"shoes":   "shoe",  // likewise
		"goes":    "goe",   // likewise
		"trees":   "tree",  // -ees: rule 3
		"aeries":  "aery",  // -ies, not -eies or -aies
		"zombies": "zomby", // -ies → -y
		"eies":    "eie",   // -eies keeps rule 1 off; -es → -e
		"has":     "has",   // under 4 runes
		"its":     "its",
		"news":    "new",
		"gas":     "gas",
		"bus":     "bus",
		"virus":   "virus",
		"class":   "class",
		"page":    "page",
	} {
		if got := Stem(in); got != want {
			t.Errorf("Stem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokensLowerCaseSplitOnNonLetterOrDigitAndStem(t *testing.T) {
	for in, want := range map[string][]string{
		"`internal/cli/task.go`":         {"internal", "cli", "task", "go"},
		"Owner-request judge's Pages":    {"owner", "request", "judge", "s", "page"},
		"BM25 k1=1.2, Claims":            {"bm25", "k1", "1", "2", "claim"},
		"Größe über Queries":             {"größe", "über", "query"},
		"--- ... ///":                    {},
		"design pages: claim's history.": {"design", "page", "claim", "s", "history"},
	} {
		if got := Tokens(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Tokens(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnippetIsTheFirstSentenceHoldingAQueryTokenWithoutMarkdownCutAt80Runes(t *testing.T) {
	body := "Nothing here. **Claims** keep the `holder`; see [the page](../design/x.md). Later claim.\n\n" +
		"| Fact | Claim |\n|---|---|\n| a | b |"
	if got, want := snippet([]string{body}, map[string]bool{"claim": true}),
		"Claims keep the holder; see the page."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := snippet([]string{body}, map[string]bool{"fact": true}), "Fact; Claim"; got != want {
		t.Errorf("table row: got %q, want %q", got, want)
	}
	if got, want := snippet([]string{"- First item.\n- Second."}, map[string]bool{"none": true}), "First item."; got != want {
		t.Errorf("no sentence holds a token: got %q, want the first, %q", got, want)
	}
	long := "Claim aaaaaaaaa bbbbbbbbb ccccccccc ddddddddd eeeeeeeee fffffffff ggggggggg hhhhhhhhh iiiii"
	got := snippet([]string{long}, map[string]bool{"claim": true})
	if r := []rune(got); len(r) != 80 || r[79] != '…' {
		t.Errorf("cut: got %q (%d runes), want 80 runes ending in …", got, len(r))
	}
}
