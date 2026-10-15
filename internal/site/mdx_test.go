package site

import (
	"strings"
	"testing"
	"time"
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

// An MDX block's tag closes only outside a code fence and a {/* */} comment; a fence line ends the search, and the
// block falls back to CommonMark.
func TestAnMDXBlockClosesOutsideFencesAndComments(t *testing.T) {
	cases := []struct{ src, want string }{
		{"<div className=\"a\">open\n\n# H\n\n```\n</div>\n```\n\ntail\n",
			"<div className=\"a\">open\n<h1>H</h1>\n<pre><code>&lt;/div&gt;\n</code></pre>\n<p>tail</p>\n"},
		{"<div className=\"a\">\n{/* </div> */}\n\ninside\n</div>\n\n*after*\n",
			"<div class=\"a\">\n\n\ninside\n</div>\n<p><em>after</em></p>\n"},
		{"<div className=\"a\">\n{/* <div> */}\n\ninside\n</div>\n\n*after*\n",
			"<div class=\"a\">\n\n\ninside\n</div>\n<p><em>after</em></p>\n"},
		// className only counts inside the opening tag; the block interrupts no paragraph (CommonMark type 7), so after a
		// paragraph line the tag is CommonMark's type 6 block
		{"<div data-x=\"1\">see className= here\n\n*z*\n</div>\n",
			"<div data-x=\"1\">see className= here\n<p><em>z</em></p>\n</div>\n"},
		{"para\n<div className=\"y\">\n\n*z*\n</div>\n", "<p>para</p>\n<div className=\"y\">\n<p><em>z</em></p>\n</div>\n"},
	}
	for _, c := range cases {
		if got, _ := mdBody(t, c.src); got != c.want {
			t.Errorf("%q renders\n%s\nwant\n%s", c.src, got, c.want)
		}
	}
}

// Finding where a block closes scans forward once: thousands of blocks render in linear time (it took 23 s for
// 4000 when each block matched every later tag).
func TestManyMDXBlocksRenderFast(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 4000; i++ {
		b.WriteString("<div className=\"viz\">\n  <svg viewBox=\"0 0 1 1\">\n\n    {/* c */}\n    <rect x=\"1\" />\n  </svg>\n</div>\n\n")
	}
	start := time.Now()
	got, _ := mdBody(t, b.String())
	if n := strings.Count(got, `<div class="viz">`); n != 4000 {
		t.Fatalf("%d blocks rendered, want 4000", n)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("4000 blocks (%d KB) took %v", b.Len()/1024, d)
	}
}
