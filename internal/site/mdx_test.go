package site

import (
	"strings"
	"testing"
)

// An MDX diagram stays one HTML block across its blank lines, written as HTML: class for className, SVG names for
// JSX presentation attributes, no {/* */} comments; nothing inside it becomes a code block or a paragraph.
func TestAnMDXDiagramRendersAsOneHTMLBlock(t *testing.T) {
	src := "<div className=\"viz\">\n  <svg className=\"scene\" viewBox=\"0 0 10 10\">\n\n" +
		"    {/* a box */}\n    <rect x=\"1\" strokeWidth=\"2\" textAnchor=\"middle\" stopColor=\"#fff\" markerEnd=\"url(#a)\" />\n\n" +
		"    <svg viewBox=\"0 0 1 1\"><g /></svg>\n  </svg>\n  <p className=\"note\">A *note*.</p>\n</div>\n\nAfter.\n"
	got, _ := mdBody(t, src)
	want := "<div class=\"viz\">\n  <svg class=\"scene\" viewBox=\"0 0 10 10\">\n\n    \n" +
		"    <rect x=\"1\" stroke-width=\"2\" text-anchor=\"middle\" stop-color=\"#fff\" marker-end=\"url(#a)\" />\n\n" +
		"    <svg viewBox=\"0 0 1 1\"><g /></svg>\n  </svg>\n  <p class=\"note\">A *note*.</p>\n</div>\n<p>After.</p>\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Only a block MDX marks (an <svg>, or a tag with className) runs past a blank line; any other HTML block ends there as
// CommonMark has it, so Markdown inside <details> still renders, and an unclosed tag is an ordinary HTML block.
func TestOtherHTMLBlocksEndAtABlankLine(t *testing.T) {
	cases := []struct{ src, want string }{
		{"<details>\n<summary>S</summary>\n\n*inside*\n\n</details>\n",
			"<details>\n<summary>S</summary>\n<p><em>inside</em></p>\n</details>\n"},
		{"<div className=\"open\">\nno close\n\n*after*\n", "<div className=\"open\">\nno close\n<p><em>after</em></p>\n"},
		{"<svg viewBox=\"0 0 1 1\"/>\n\n*after*\n", "<svg viewBox=\"0 0 1 1\"/>\n<p><em>after</em></p>\n"},
	}
	for _, c := range cases {
		if got, _ := mdBody(t, c.src); got != c.want {
			t.Errorf("%q renders\n%s\nwant\n%s", c.src, got, c.want)
		}
	}
	if got, _ := mdBody(t, "`<svg className=\"x\">`\n"); !strings.Contains(got, "<code>&lt;svg") {
		t.Errorf("a code span's tag became HTML: %s", got)
	}
}
