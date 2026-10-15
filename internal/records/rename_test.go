package records

import "testing"

func TestRenameLinksRewritesEveryFormOfALinkToTheRecord(t *testing.T) {
	cases := []struct{ dir, text, want string }{
		{"sprints", "- [The parser](../design/parser.md)\n", "- [The lexer](../design/lexer.md)\n"},
		{"sprints", "See [it](../design/parser.md#tables).", "See [it](../design/lexer.md#tables)."},
		{"design", "[x](parser.md) [y](./parser.md#a) [z](../design/parser.md)",
			"[x](lexer.md) [y](./lexer.md#a) [z](../design/lexer.md)"},
		// a bare name means the page only beside it; other pages and other kinds stay
		{"sprints", "[x](parser.md) [y](../design/parser-old.md) [z](../docs/parser.md) [w](../design/my-parser.md)",
			"[x](parser.md) [y](../design/parser-old.md) [z](../docs/parser.md) [w](../design/my-parser.md)"},
	}
	for _, c := range cases {
		got, changed := RenameLinks(c.text, c.dir, "design/parser", "design/lexer", "The parser", "The lexer")
		if got != c.want || changed != (c.text != c.want) {
			t.Errorf("RenameLinks(%q, %s) = %q, %v; want %q", c.text, c.dir, got, changed, c.want)
		}
	}
	// no new title: link texts stay
	if got, _ := RenameLinks("[The parser](../design/parser.md)", "projects", "design/parser", "design/lexer",
		"The parser", ""); got != "[The parser](../design/lexer.md)" {
		t.Errorf("without a new title: %q", got)
	}
	// a doc keeps its date prefix in the name pm passes
	if got, _ := RenameLinks("[d](2026-10-03-old.md)", "docs", "docs/2026-10-03-old", "docs/2026-10-03-new", "D",
		""); got != "[d](2026-10-03-new.md)" {
		t.Errorf("a doc beside it: %q", got)
	}
}
