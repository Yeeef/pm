package cli

import (
	"strings"
	"unicode/utf8"
)

// wrapFill is Python's textwrap.fill(text, width, initial_indent=first, subsequent_indent=rest,
// break_long_words=False, break_on_hyphens=False): tabs expanded, each ASCII whitespace character made a space, the
// text cut into words and whitespace runs, lines filled greedily, a word longer than a line put on a line of its own,
// whitespace dropped at the start of each line after the first and at the end of every line.
func wrapFill(text string, width int, first, rest string) string {
	text = expandTabs(text, 8)
	text = strings.Map(func(r rune) rune {
		if strings.ContainsRune("\t\n\x0b\x0c\r", r) {
			return ' '
		}
		return r
	}, text)
	var chunks []string // in reverse, as textwrap pops them
	for start, i := 0, 0; i <= len(text); {
		if i == len(text) || (text[i] == ' ') != (text[start] == ' ') {
			if i > start {
				chunks = append(chunks, text[start:i])
			}
			if i == len(text) {
				break
			}
			start = i
		}
		i++
	}
	for l, r := 0, len(chunks)-1; l < r; l, r = l+1, r-1 {
		chunks[l], chunks[r] = chunks[r], chunks[l]
	}
	n := func(s string) int { return utf8.RuneCountInString(s) }
	blank := func(s string) bool { return strings.TrimSpace(s) == "" }
	var lines []string
	for len(chunks) > 0 {
		var cur []string
		curLen := 0
		indent := first
		if len(lines) > 0 {
			indent = rest
		}
		w := width - n(indent)
		if blank(chunks[len(chunks)-1]) && len(lines) > 0 {
			chunks = chunks[:len(chunks)-1]
		}
		for len(chunks) > 0 {
			l := n(chunks[len(chunks)-1])
			if curLen+l > w {
				break
			}
			cur = append(cur, chunks[len(chunks)-1])
			chunks = chunks[:len(chunks)-1]
			curLen += l
		}
		if len(chunks) > 0 && n(chunks[len(chunks)-1]) > w && cur == nil {
			cur = append(cur, chunks[len(chunks)-1])
			chunks = chunks[:len(chunks)-1]
		}
		if len(cur) > 0 && blank(cur[len(cur)-1]) {
			cur = cur[:len(cur)-1]
		}
		if len(cur) > 0 {
			lines = append(lines, indent+strings.Join(cur, ""))
		}
	}
	return strings.Join(lines, "\n")
}

// expandTabs is Python's str.expandtabs(size).
func expandTabs(s string, size int) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			pad := size - col%size
			b.WriteString(strings.Repeat(" ", pad))
			col += pad
		case '\n', '\r':
			b.WriteRune(r)
			col = 0
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}
