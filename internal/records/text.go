package records

import (
	"slices"
	"strings"
	"unicode"

	"github.com/Yeeef/pm/internal/config"
)

// Sentences and words as pm counts them in a need's text (ASD-STE100's sentence limit), and a line kept a plain list
// item.

const tick = '\x60' // the backtick that opens and closes a code span

// codeSpanAt is the end of the code span Python's pattern for one (a backtick run, at least one character, the same
// run) matches at rune i of t, or -1: the longest backtick run first, then the nearest closing run, with no line break
// inside.
func codeSpanAt(t []rune, i int) int {
	k := 0
	for i+k < len(t) && t[i+k] == tick {
		k++
	}
	for g := k; g >= 1; g-- {
		for j := i + g + 1; j+g <= len(t); j++ {
			if t[j-1] == '\n' {
				break
			}
			if string(t[j:j+g]) == strings.Repeat(string(tick), g) {
				return j + g
			}
		}
	}
	return -1
}

// Sentences splits text at . ! or ? followed by white space; a code span is one unit, so a period in it ends no
// sentence. Python source: sentences.
func Sentences(text string) []string {
	t := []rune(text)
	var out []string
	start := 0
	for i := 0; i < len(t); {
		if t[i] == tick {
			if end := codeSpanAt(t, i); end >= 0 {
				i = end
				continue
			}
		}
		if (t[i] == '.' || t[i] == '!' || t[i] == '?') && i+1 < len(t) && config.IsPySpace(t[i+1]) {
			out = append(out, Strip(string(t[start:i+1])))
			start = i + 1
		}
		i++
	}
	if rest := Strip(string(t[start:])); rest != "" {
		out = append(out, rest)
	}
	return out
}

// Words is the runs between white space that hold a letter or digit; a code span is one word. Python source: words.
func Words(sentence string) []string {
	t := []rune(sentence)
	var out []string
	for i := 0; i < len(t); {
		if config.IsPySpace(t[i]) {
			i++
			continue
		}
		start := i
		for i < len(t) {
			if t[i] == tick {
				if end := codeSpanAt(t, i); end >= 0 {
					i = end
					continue
				}
				i++
				continue
			}
			if config.IsPySpace(t[i]) {
				break
			}
			i++
		}
		w := t[start:i]
		if slices.Contains(w, tick) || slices.ContainsFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			out = append(out, string(w))
		}
	}
	return out
}

// PlainItem escapes a leading block marker (heading, quote, list item), so a fact stays one plain list item.
func PlainItem(text string) string {
	t := []rune(text)
	after := func(i int) bool { return i >= len(t) || config.IsPySpace(t[i]) }
	if len(t) > 0 && (t[0] == '#' || t[0] == '>' || ((t[0] == '-' || t[0] == '+' || t[0] == '*') && after(1))) {
		text = `\` + text
		t = []rune(text)
	}
	n := 0
	for n < len(t) && n < 10 && unicode.IsDigit(t[n]) {
		n++
	}
	if n >= 1 && n <= 9 && n < len(t) && (t[n] == '.' || t[n] == ')') && after(n+1) {
		text = string(t[:n]) + `\` + string(t[n:])
	}
	return text
}
