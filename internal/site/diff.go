package site

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A version's diff, by section: a record's text is cut at its ## and ### headings (outside code fences), the sections
// of the two texts are matched by heading, and each one that changed shows a line diff of its Markdown; an unchanged
// one folds. Both diffs are longest-common-subsequence alignments, exact: a record has hundreds of lines at most.

// section is one part of a record's text: the lines from a heading to the next, or the header (front matter and any
// text) before the first heading, whose head is "".
type section struct {
	head  string
	lines []string
}

var (
	sectionHead = regexp.MustCompile(`^#{2,3} `)
	fence       = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
)

// sections cuts text at its ## and ### headings; a heading inside a code fence is text.
func sections(text string) []section {
	if text == "" {
		return nil
	}
	out := []section{{}}
	inFence := ""
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		// CommonMark: a fence closes on a run of its own character at least as long, with nothing after it
		if m := fence.FindStringSubmatch(line); m != nil {
			switch {
			case inFence == "":
				inFence = m[1]
			case m[1][0] == inFence[0] && len(m[1]) >= len(inFence) && strings.TrimSpace(m[2]) == "":
				inFence = ""
			}
		}
		if inFence == "" && sectionHead.MatchString(line) {
			out = append(out, section{head: line})
		}
		out[len(out)-1].lines = append(out[len(out)-1].lines, line)
	}
	if len(out[0].lines) == 0 {
		out = out[1:]
	}
	return out
}

// keys is each section's heading, numbered when it repeats, so that sections match by heading and order.
func keys(secs []section) []string {
	seen := map[string]int{}
	out := make([]string, len(secs))
	for i, s := range secs {
		seen[s.head]++
		out[i] = fmt.Sprintf("%s\x00%d", s.head, seen[s.head])
	}
	return out
}

// edit is one step of an alignment of a and b: a[A] kept as b[B] (op ' '), a[A] removed (op '-') or b[B] added (op
// '+').
type edit struct {
	op   byte
	A, B int
}

// align is a shortest edit script from a to b: a longest common subsequence kept, the rest removed or added, removals
// first at each change.
func align(a, b []string) []edit {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1) // lcs[i][j]: the longest common subsequence of a[i:] and b[j:]
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []edit
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			out = append(out, edit{' ', i, j})
			i, j = i+1, j+1
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, edit{'-', i, j})
			i++
		default:
			out = append(out, edit{'+', i, j})
			j++
		}
	}
	return out
}

// sectionDiff is the HTML of the diff from was to now, section by section: a changed, added or removed section as its
// heading and its lines, each marked kept, removed (class del) or added (class add); an unchanged one folded.
func sectionDiff(was, now string) string {
	a, b := sections(was), sections(now)
	var out strings.Builder
	changed := 0
	for _, e := range align(keys(a), keys(b)) {
		var head string
		var al, bl []string
		switch e.op {
		case ' ':
			head, al, bl = a[e.A].head, a[e.A].lines, b[e.B].lines
		case '-':
			head, al = a[e.A].head, a[e.A].lines
		default:
			head, bl = b[e.B].head, b[e.B].lines
		}
		name := cmp.Or(head, "Header")
		if e.op == ' ' && slices.Equal(al, bl) {
			out.WriteString(`<details class="same"><summary>` + esc(name) + ` <span class="k">unchanged</span></summary>` +
				"<pre>" + esc(strings.Join(al, "\n")) + "</pre></details>\n")
			continue
		}
		changed++
		what := map[byte]string{' ': "changed", '-': "removed", '+': "added"}[e.op]
		out.WriteString(`<h3 class="diffhead">` + esc(name) + ` <span class="k">` + what + "</span></h3>\n" +
			`<pre class="diff">`)
		for _, l := range align(al, bl) {
			switch l.op {
			case ' ':
				out.WriteString("<span>  " + esc(al[l.A]) + "</span>\n")
			case '-':
				out.WriteString(`<span class="del">- ` + esc(al[l.A]) + "</span>\n")
			default:
				out.WriteString(`<span class="add">+ ` + esc(bl[l.B]) + "</span>\n")
			}
		}
		out.WriteString("</pre>\n")
	}
	summary := fmt.Sprintf(`<p class="k">%d sections changed</p>`, changed)
	if changed == 1 {
		summary = `<p class="k">1 section changed</p>`
	}
	return summary + "\n" + out.String()
}
