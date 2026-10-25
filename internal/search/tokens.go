package search

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/Yeeef/pm/internal/records"
)

// Tokens is a text's tokens: lower-cased, split on every rune that is not a letter or digit, each stemmed.
func Tokens(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for i, w := range words {
		words[i] = Stem(w)
	}
	return words
}

// Stem is the S-stemmer (Harman 1991) on a token of 4 runes or more; the first rule that matches applies:
// -ies → -y unless -eies or -aies; -es → -e unless -aes, -ees or -oes; -s → nothing unless -us or -ss.
func Stem(t string) string {
	if len([]rune(t)) < 4 {
		return t
	}
	switch {
	case strings.HasSuffix(t, "ies") && !strings.HasSuffix(t, "eies") && !strings.HasSuffix(t, "aies"):
		return strings.TrimSuffix(t, "ies") + "y"
	case strings.HasSuffix(t, "es") && !strings.HasSuffix(t, "aes") && !strings.HasSuffix(t, "ees") &&
		!strings.HasSuffix(t, "oes"):
		return strings.TrimSuffix(t, "s")
	case strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "us") && !strings.HasSuffix(t, "ss"):
		return strings.TrimSuffix(t, "s")
	}
	return t
}

// literal is whether a query term must also appear in a unit's text as written: a phrase or a path, a term holding a
// space, a '/' or a '.'.
func literal(term string) bool { return strings.ContainsAny(term, " /.") }

// flat is a text lower-cased with each run of white space as one space, so a phrase matches across a record's
// line breaks.
func flat(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

var (
	mdLink  = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	mdLead  = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+|#+\s+|>\s*|:::.*$)`)
	mdMarks = strings.NewReplacer("**", "", "__", "", "`", "")
)

// plain is a Markdown text's paragraphs as plain text, one string each: lines of a paragraph joined, list, heading,
// quote and block marks, emphasis and code ticks removed, a link reduced to its text, a table row as its cells joined
// by "; "; code fences and table rule lines are left out.
func plain(text string) []string {
	var out, para []string
	fence := false
	flush := func() {
		if len(para) > 0 {
			out = append(out, strings.Join(strings.Fields(strings.Join(para, " ")), " "))
			para = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = !fence
			flush()
			continue
		}
		if fence || t == "" || strings.Trim(t, "|-: ") == "" {
			flush()
			continue
		}
		if mdLead.MatchString(t) || strings.HasPrefix(t, "|") {
			flush() // a heading, a list item, a quote, a block line or a table row starts its own paragraph
		}
		if strings.HasPrefix(t, "|") { // a table row: its cells, separated
			cells := strings.Split(strings.Trim(t, "| "), "|")
			for i, c := range cells {
				cells[i] = strings.TrimSpace(c)
			}
			t = strings.Join(cells, "; ")
		}
		para = append(para, mdMarks.Replace(mdLead.ReplaceAllString(mdLink.ReplaceAllString(t, "$1"), "")))
	}
	flush()
	return out
}

// snippet is the first sentence of texts, in order, that holds one of the tokens, as plain text cut at 80 runes; the
// first sentence when none does.
func snippet(texts []string, tokens map[string]bool) string {
	first := ""
	for _, text := range texts {
		for _, para := range plain(text) {
			for _, s := range records.Sentences(para) {
				if first == "" {
					first = s
				}
				for _, t := range Tokens(s) {
					if tokens[t] {
						return cut(s, 80)
					}
				}
			}
		}
	}
	return cut(first, 80)
}

// cut is s at most n runes long, ending in … when it was cut.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimRightFunc(string(r[:n-1]), unicode.IsSpace) + "…"
	}
	return s
}
