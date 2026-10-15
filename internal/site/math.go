package site

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// TeX math: $…$ inline and $$…$$ display, kept verbatim (no backslash escape, entity or emphasis applies inside) in a
// span or div of class math, which KaTeX typesets in the browser. The inline rule is Pandoc's tex_math_dollars: the
// opening $ is followed by a non-space, the closing $ is preceded by a non-space and not followed by a digit, so a
// price such as $5 stays text. A $ in a code span or block is code: code parses first, and a formula never runs into a
// code span.

var kindMath = ast.NewNodeKind("Math")

// mathNode is one formula: its TeX source and whether it is display math.
type mathNode struct {
	ast.BaseInline
	tex     string
	display bool
}

func (n *mathNode) Kind() ast.NodeKind { return kindMath }

func (n *mathNode) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type mathInlineParser struct{}

func (mathInlineParser) Trigger() []byte { return []byte{'$'} }

func isSpaceByte(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// Parse reads $$…$$ or $…$ within the current line; nil leaves the $ as text.
func (mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) > 1 && line[1] == '$' {
		end := bytes.Index(line[2:], []byte("$$"))
		if end < 0 || strings.TrimSpace(string(line[2:2+end])) == "" || bytes.IndexByte(line[2:2+end], '`') >= 0 {
			return nil
		}
		block.Advance(2 + end + 2)
		return &mathNode{tex: string(line[2 : 2+end]), display: true}
	}
	if len(line) < 3 || isSpaceByte(line[1]) {
		return nil
	}
	for j := 2; j < len(line); j++ {
		if line[j] == '`' { // a code span binds tighter: a $ inside one closes nothing
			return nil
		}
		if line[j] != '$' || isSpaceByte(line[j-1]) || line[j-1] == '\\' {
			continue
		}
		if j+1 < len(line) && line[j+1] >= '0' && line[j+1] <= '9' {
			continue
		}
		block.Advance(j + 1)
		return &mathNode{tex: string(line[1:j])}
	}
	return nil
}

var kindMathBlock = ast.NewNodeKind("MathBlock")

// mathBlock is a $$ … $$ display formula on lines of its own; its lines hold the TeX.
type mathBlock struct {
	ast.BaseBlock
	closed bool
}

func (n *mathBlock) Kind() ast.NodeKind { return kindMathBlock }

func (n *mathBlock) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

func (n *mathBlock) IsRaw() bool { return true }

type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }

// closing is where a line's TeX ends when the line closes the formula (it ends in $$), else -1.
func closing(line []byte) int {
	t := bytes.TrimRight(line, " \t\r\n")
	if !bytes.HasSuffix(t, []byte("$$")) {
		return -1
	}
	return len(t) - 2
}

// Open opens a formula on a line that starts with $$, within three spaces of indent; one that ends in $$ too is the
// whole formula.
func (mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 || !bytes.HasPrefix(line[pos:], []byte("$$")) {
		return nil, parser.NoChildren
	}
	n := &mathBlock{}
	rest := line[pos+2:]
	start := seg.Start + pos + 2
	if end := closing(rest); end >= 0 {
		n.Lines().Append(text.NewSegment(start, start+end))
		n.closed = true
	} else if strings.TrimSpace(string(rest)) != "" {
		n.Lines().Append(text.NewSegment(start, seg.Stop))
	}
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

func (mathBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	n, ok := node.(*mathBlock)
	if !ok || n.closed {
		return parser.Close
	}
	line, seg := reader.PeekLine()
	if end := closing(line); end >= 0 {
		n.Lines().Append(text.NewSegment(seg.Start, seg.Start+end))
		reader.AdvanceToEOL()
		return parser.Close
	}
	n.Lines().Append(seg)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

func (mathBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (mathBlockParser) CanInterruptParagraph() bool { return true }

func (mathBlockParser) CanAcceptIndentedLine() bool { return false }

func (r *recordRenderer) math(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n, ok := node.(*mathNode)
	if !entering || !ok {
		return ast.WalkSkipChildren, nil
	}
	r.c.math = true
	class := "math inline"
	if n.display {
		class = "math display"
	}
	_, err := w.WriteString(`<span class="` + class + `">` + esc(n.tex) + "</span>")
	return ast.WalkSkipChildren, err
}

func (r *recordRenderer) mathBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	r.c.math = true
	var tex bytes.Buffer
	for i := 0; i < node.Lines().Len(); i++ {
		tex.Write(segValue(node.Lines().At(i), source))
	}
	_, err := w.WriteString(`<div class="math display">` + esc(strings.TrimSpace(tex.String())) + "</div>\n")
	return ast.WalkSkipChildren, err
}

// KatexVersion is the exact KaTeX release the pages with math load, pinned as MermaidVersion is: a pinned URL is
// immutable, and a second CDN serves it when the first fails.
const KatexVersion = "0.19.0"

// katexScript typesets each .math element in place from its text, as display math when it has class display; a
// formula KaTeX cannot parse shows its source in red rather than stopping the rest. The stylesheet comes from the CDN
// the module came from.
var katexScript = strings.ReplaceAll(`
<script type="module">
const at = ["https://cdn.jsdelivr.net/npm/katex@VERSION/dist/",
  "https://unpkg.com/katex@VERSION/dist/"];
let katex, base = at[0];
try { katex = (await import(at[0] + "katex.mjs")).default; } catch { base = at[1]; katex = (await import(at[1] + "katex.mjs")).default; }
const css = document.createElement("link");
css.rel = "stylesheet";
css.href = base + "katex.min.css";
document.head.append(css);
for (const n of document.querySelectorAll(".math")) {
  katex.render(n.textContent, n, { displayMode: n.classList.contains("display"), throwOnError: false });
}
</script>
`, "VERSION", KatexVersion)
