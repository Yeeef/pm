package site

import (
	"regexp"
	"strings"
	"testing"
)

// mdBody is src rendered as a record body, and whether math is on it.
func mdBody(t *testing.T, src string) (string, bool) {
	t.Helper()
	c := &ctx{rel: "docs/2026-10-15-x"}
	out, err := render(recordMD(c), src)
	if err != nil {
		t.Fatal(err)
	}
	return out, c.math
}

// TeX reaches KaTeX as written: no backslash escape, entity or emphasis applies inside $…$ or $$…$$; a $ in code, a
// price and an escaped \$ stay text.
func TestMathKeepsItsTeXVerbatim(t *testing.T) {
	cases := []struct{ src, want string }{
		{`sample $a_t \sim \sigma_t$ now`, `sample <span class="math inline">a_t \sim \sigma_t</span> now`},
		{`set $\{x\_1, x\_2\}$ and $a_b c_d$`, `set <span class="math inline">\{x\_1, x\_2\}</span> and ` +
			`<span class="math inline">a_b c_d</span>`},
		{`less $a<b$ &amp; more`, `less <span class="math inline">a&lt;b</span> &amp; more`},
		{`inline display $$\bar{\sigma}_T$$ end`, `inline display <span class="math display">\bar{\sigma}_T</span> end`},
		{"$$\nR_T(a) = \\sum_{t=1}^{T} r_t\n$$", `<div class="math display">R_T(a) = \sum_{t=1}^{T} r_t</div>`},
		{`$$\max\bigl(R, 0\bigr)$$`, `<div class="math display">\max\bigl(R, 0\bigr)</div>`},
		{"text\n$$\nx_1\n$$\nafter", "<p>text</p>\n<div class=\"math display\">x_1</div>\n<p>after</p>"},
		{`a price of $5 and $10`, `a price of $5 and $10`},
		{`from $5–$10 total`, `from $5–$10 total`},
		{"`$a_t$` and `$x`", "<code>$a_t$</code> and <code>$x</code>"},
		{"$5 or `$x$`", "$5 or <code>$x$</code>"},
		{`escaped \$x\$ stays`, `escaped $x$ stays`},
		{`$ x$ and $x $`, `$ x$ and $x $`},
		// $$ opens a block only alone on its line, or as $$…$$ with nothing after; the inline parser has the rest
		{"$$E=mc^2$$ is famous.\n\nNext.\n\n## H\n", "<p><span class=\"math display\">E=mc^2</span> is famous.</p>\n" +
			"<p>Next.</p>\n<h2 id=\"h\">H</h2>"},
		{"$$ is the shell's pid.\n\nLater.", "<p>$$ is the shell's pid.</p>\n<p>Later.</p>"},
		{"text\n$$x$$ more\n\nnext", "<p>text\n<span class=\"math display\">x</span> more</p>\n<p>next</p>"},
		{"$$a$$ and $$b$$", `<span class="math display">a</span> and <span class="math display">b</span>`},
		{"$$\nx\n\ny\n$$\n", "<p>$$\nx</p>\n<p>y\n$$</p>"}, // unclosed before a blank line: no math
		{`[cost $5](http://x/$y)`, `<a href="http://x/$y">cost $5</a>`},
		{`[a $b$](http://x/$y$)`, `<a href="http://x/$y$">a <span class="math inline">b</span></a>`},
		{"wrapped $a +\nb$ formula", "wrapped <span class=\"math inline\">a +\nb</span> formula"},
		{"one $a\n\nb$ two", "<p>one $a</p>\n<p>b$ two</p>"},
		{`$a\\$ b`, `<span class="math inline">a\\</span> b`},
		{`$a\$ b$`, `<span class="math inline">a\$ b</span>`},
	}
	for _, c := range cases {
		got, _ := mdBody(t, c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q renders\n%s\nwant it to hold\n%s", c.src, got, c.want)
		}
	}
	got, _ := mdBody(t, "```\n$a_t$\n```\n")
	if strings.Contains(got, "math") {
		t.Errorf("a fence's $ became math: %s", got)
	}
}

// A page loads KaTeX only when math is on it.
func TestOnlyAPageWithMathLoadsKatex(t *testing.T) {
	if _, math := mdBody(t, "no math, $5 only, `$x$`"); math {
		t.Error("a body without math is marked as having math")
	}
	for _, src := range []string{"$x$", "$$\nx\n$$"} {
		if _, math := mdBody(t, src); !math {
			t.Errorf("%q is not marked as having math", src)
		}
	}
	for _, math := range []bool{false, true} {
		p, err := page(nil, "doc", "T", "", &ctx{math: math}, "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(p, "katex") != math {
			t.Errorf("math %v: the page loads KaTeX %v", math, !math)
		}
	}
}

// KaTeX loads as Mermaid does: one exact release, from a CDN and a fallback, its stylesheet from the CDN that served
// the module.
func TestKatexLoadsOneExactVersion(t *testing.T) {
	urls := regexp.MustCompile(`https://[^"]*`).FindAllString(katexScript, -1)
	if len(urls) < 2 {
		t.Fatalf("want a CDN and a fallback, got %q", urls)
	}
	exact := regexp.MustCompile(`/katex@(\d+\.\d+\.\d+)/dist/$`)
	for _, u := range urls {
		if m := exact.FindStringSubmatch(u); len(m) < 2 || m[1] != KatexVersion {
			t.Errorf("%s does not name katex@%s exactly", u, KatexVersion)
		}
	}
	for _, want := range []string{`"katex.mjs"`, `"katex.min.css"`, `querySelectorAll(".math")`,
		`displayMode: n.classList.contains("display")`, "throwOnError: false"} {
		if !strings.Contains(katexScript, want) {
			t.Errorf("the script lacks %q", want)
		}
	}
}
