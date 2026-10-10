package cli

import (
	"reflect"
	"testing"
)

// The expected values were Python pm 0.1.x's on these inputs (sentences, words, plain, and its PR-named and
// review-asked patterns), frozen.

func TestSentencesAndWordsAsPython(t *testing.T) {
	for _, c := range []struct {
		text      string
		sentences []string
		words     [][]string
	}{
		{"One. Two! Three? four", []string{"One.", "Two!", "Three?", "four"},
			[][]string{{"One."}, {"Two!"}, {"Three?"}, {"four"}}},
		{"Keep \x60a. b\x60 here. Next.", []string{"Keep \x60a. b\x60 here.", "Next."},
			[][]string{{"Keep", "\x60a. b\x60", "here."}, {"Next."}}},
		{"\x60\x60a\x60 b. c", []string{"\x60\x60a\x60 b.", "c"}, [][]string{{"\x60\x60a\x60", "b."}, {"c"}}},
		{"a\x60b c\x60 d. e", []string{"a\x60b c\x60 d.", "e"}, [][]string{{"a\x60b c\x60", "d."}, {"e"}}},
		{"x \x60\x60 y. z", []string{"x \x60\x60 y.", "z"}, [][]string{{"x", "\x60\x60", "y."}, {"z"}}},
		{"\x60code\x60 _ ok.", []string{"\x60code\x60 _ ok."}, [][]string{{"\x60code\x60", "ok."}}},
		{"Run \x60pm show\x60. Then go.", []string{"Run \x60pm show\x60.", "Then go."},
			[][]string{{"Run", "\x60pm show\x60."}, {"Then", "go."}}},
	} {
		got := sentences(c.text)
		if !reflect.DeepEqual(got, c.sentences) {
			t.Errorf("sentences(%q) = %q, want %q", c.text, got, c.sentences)
			continue
		}
		for i, s := range got {
			if w := words(s); !reflect.DeepEqual(w, c.words[i]) {
				t.Errorf("words(%q) = %q, want %q", s, w, c.words[i])
			}
		}
	}
}

func TestPlainAsPython(t *testing.T) {
	for in, want := range map[string]string{"# h": `\# h`, "> q": `\> q`, "- x": `\- x`, "-x": "-x", "+ y": `\+ y`,
		"* z": `\* z`, "12. item": `12\. item`, "1) item": `1\) item`, "1234567890. x": "1234567890. x",
		"3.5 is": "3.5 is", "x": "x"} {
		if got := plain(in); got != want {
			t.Errorf("plain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAsksReviewAsPython(t *testing.T) {
	for in, want := range map[string]bool{"Please review PR #12": true, "PR#3 merged": true,
		"see https://github.com/o/r/pull/9 and approve": true, "xPR #3 review": false, "reviewer of PR #4": true,
		"preview PR #4": false, "Merging PR  #5": true} {
		if got := asksReview(in); got != want {
			t.Errorf("asksReview(%q) = %v, want %v", in, got, want)
		}
	}
}
