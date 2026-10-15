package site

import (
	"os"
	"strings"
	"testing"
)

// Each JSX construct in raw HTML, and a blank line inside an <svg>, is one error naming its file line and the HTML fix.
func TestCheckRefusesJSXInRawHTML(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"className", `<div className="viz">x</div>`, "className= is JSX, not HTML; use class="},
		{"comment", "<svg viewBox=\"0 0 1 1\">\n  {/* a node */}\n</svg>",
			"{/* … */} is JSX, not HTML; remove the comment or use <!-- -->"},
		{"expression", `<rect width={w * 2} />`, "width={…} is JSX, not HTML; write the computed value in quotes"},
		{"camelCase", `<text textAnchor="middle">a</text>`, "textAnchor is JSX, not HTML; use text-anchor"},
		{"strokeWidth", `<path d="M0 0" strokeWidth="2" />`, "strokeWidth is JSX, not HTML; use stroke-width"},
		{"blank line", "<svg viewBox=\"0 0 1 1\">\n  <rect />\n\n  <rect />\n</svg>",
			"a blank line between <svg and </svg>; remove the blank line: a blank line ends an HTML block"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := "## Diagram\n\n" + c.body + "\n"
			got := jsxErrors("docs/x", body, 5)
			if len(got) != 1 || !strings.HasSuffix(got[0], ": "+c.want) || !strings.HasPrefix(got[0], "docs/x.md:") {
				t.Fatalf("errors %q, want one ending %q", got, c.want)
			}
		})
	}
	// the line is the file's: the body's first line is line 5, so its third line is line 7
	if got := jsxErrors("docs/x", "a\n\n<p className=\"k\">x</p>\n", 5); len(got) != 1 ||
		!strings.HasPrefix(got[0], "docs/x.md:7: ") {
		t.Errorf("errors %q, want one on line 7", got)
	}
	// JSX after the blank line of an open <svg> is still in it, and is reported too
	got := jsxErrors("docs/x", "<svg viewBox=\"0 0 1 1\">\n\n    <rect strokeWidth=\"1\" />\n</svg>\n", 1)
	if len(got) != 2 {
		t.Errorf("errors %q, want the blank line and strokeWidth", got)
	}
}

// Plain HTML SVG passes, and so do the same words in fenced or indented code, in a code span and in prose.
func TestCheckPassesPlainHTMLAndJSXInCode(t *testing.T) {
	body := strings.Join([]string{
		`<div class="viz">`,
		`  <svg viewBox="0 0 820 360" role="img" aria-label="a">`,
		`    <rect x="1" y="2" width="3" height="4" stroke-width="1.5" fill-opacity=".5" />`,
		`    <text x="300" y="68" text-anchor="middle" font-size="12">h</text>`,
		`    <!-- className={x} strokeWidth="2" -->`,
		`    <linearGradient id="g" gradientUnits="userSpaceOnUse"><stop stop-color="#fff" /></linearGradient>`,
		`  </svg>`,
		`</div>`,
		``,
		"Prose: `className=\"a\"`, `{/* c */}`, `strokeWidth={2}`, and className= in plain words.",
		``,
		"```jsx",
		`<svg className="a" strokeWidth={2}>`,
		``,
		`  {/* c */}`,
		`</svg>`,
		"```",
		"````md",
		"```",
		`<p className="still in the longer fence">`,
		"````",
		``,
		`    <div className="indented code" />`,
		``,
		"<p>Inline `className=` in a code span</p>",
	}, "\n")
	if got := jsxErrors("docs/x", body, 1); got != nil {
		t.Errorf("errors %q, want none", got)
	}
}

// The constructs fixture's diagram is plain HTML: it passes.
func TestTheFixtureDiagramIsPlainHTML(t *testing.T) {
	b, err := os.ReadFile("testdata/constructs/records/docs/2026-10-04-regret-note.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<svg") {
		t.Fatal("the fixture has no <svg>")
	}
	if got := jsxErrors("docs/2026-10-04-regret-note", string(b), 1); got != nil {
		t.Errorf("errors %q, want none", got)
	}
}
