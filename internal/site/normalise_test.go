package site

import (
	"strings"
	"testing"
)

// Pairs the parity check must see as equal: they differ only in entity form, attribute order or whitespace that
// HTML does not render.
var knownEqual = [][2]string{
	{`<p>a &quot;b&quot; &#x27;c&#x27; &amp;</p>`, `<p>a "b" 'c' &amp;</p>`},
	{`<a href="x" class="y">t</a>`, `<a class="y" href="x">t</a>`},
	{"<ul>\n<li>a</li>\n</ul>\n", "<ul><li>a</li></ul>"},
	{"<p>a\n  b</p>", "<p>a b</p>"},
	{"<li>\n<p>a</p>\n</li>", "<li><p>a</p></li>"},
	{"<p>line<br />\nnext</p>", "<p>line<br>\nnext</p>"},
	{"<div class=\"tbl\"><table>\n<thead>\n<tr>\n<th>a</th>\n</tr>\n</thead>\n</table></div>",
		"<div class=\"tbl\"><table><thead><tr><th>a</th></tr></thead></table></div>"},
}

// Pairs it must see as different: text, attributes, nesting, whitespace inside <pre> and between inline elements.
var knownDifferent = [][2]string{
	{"<p>a</p>", "<p>b</p>"},
	{`<a href="x">t</a>`, `<a href="y">t</a>`},
	{`<h2 id="a">t</h2>`, `<h2>t</h2>`},
	{"<p><em>a</em></p>", "<p><strong>a</strong></p>"},
	{"<div><p>a</p></div>", "<div></div><p>a</p>"},
	{"<pre>a  b</pre>", "<pre>a b</pre>"},
	{"<pre>a\n</pre>", "<pre>a</pre>"},
	{"<p><em>a</em> <em>b</em></p>", "<p><em>a</em><em>b</em></p>"},
	{"<p>a<!--x--></p>", "<p>a<!--y--></p>"},
	{"<table><tr><td>x</td></tr></table>", `<table><tr><td style="text-align:left">x</td></tr></table>`},
}

func TestNormaliseKnownPairs(t *testing.T) {
	for _, p := range knownEqual {
		a, _ := Normalise(p[0])
		b, _ := Normalise(p[1])
		if a != b {
			t.Errorf("%q and %q normalise differently:\n%s", p[0], p[1], Diff(a, b))
		}
	}
	for _, p := range knownDifferent {
		a, _ := Normalise(p[0])
		b, _ := Normalise(p[1])
		if a == b {
			t.Errorf("%q and %q normalise alike: %q", p[0], p[1], a)
		}
	}
}

func TestDiffShowsTheChangedLinesWithContext(t *testing.T) {
	got := Diff("a\nb\nc\nd", "a\nb\nx\nd")
	if want := "  b\n+ x\n- c"; got != want {
		t.Errorf("Diff = %q", got)
	}
	if !strings.Contains(Diff("a", "b"), "+ b") {
		t.Error("Diff misses an added line")
	}
}
