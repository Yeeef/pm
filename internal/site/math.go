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

// Parse reads $$…$$ or $…$ from the current line, or onto the paragraph's next line (records hard-wrap; a paragraph
// holds no blank line); nil leaves the $ as text. A formula never runs into a code span or across a link's "](", so
// a $ in a link's text and its destination leaves the link whole.
func (mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	src, _ := block.PeekLine()
	if bytes.HasSuffix(src, []byte("\n")) {
		l, pos := block.Position()
		block.AdvanceLine()
		if next, _ := block.PeekLine(); next != nil {
			src = append(append([]byte{}, src...), next...)
		}
		block.SetPosition(l, pos)
	}
	stops := func(tex []byte) bool { return bytes.IndexByte(tex, '`') >= 0 || bytes.Contains(tex, []byte("](")) }
	if len(src) > 1 && src[1] == '$' {
		end := bytes.Index(src[2:], []byte("$$"))
		if end < 0 || strings.TrimSpace(string(src[2:2+end])) == "" || stops(src[2:2+end]) {
			return nil
		}
		block.Advance(2 + end + 2)
		return &mathNode{tex: string(src[2 : 2+end]), display: true}
	}
	if len(src) < 3 || isSpaceByte(src[1]) {
		return nil
	}
	for j := 2; j < len(src); j++ {
		if stops(src[j:j+1]) || src[j] == ']' && j+1 < len(src) && src[j+1] == '(' {
			return nil
		}
		if src[j] != '$' || isSpaceByte(src[j-1]) || escaped(src, j) {
			continue
		}
		if j+1 < len(src) && src[j+1] >= '0' && src[j+1] <= '9' {
			continue
		}
		block.Advance(j + 1)
		return &mathNode{tex: string(src[1:j])}
	}
	return nil
}

// escaped says whether the byte at i follows an odd run of backslashes: after \\ a $ closes a formula, after \ not.
func escaped(src []byte, i int) bool {
	n := 0
	for i-n-1 >= 0 && src[i-n-1] == '\\' {
		n++
	}
	return n%2 == 1
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

// Open opens a formula, within three spaces of indent, on a line that is $$ alone, closed by a later line that ends in
// $$ before any blank line, or on a line that is $$…$$ and nothing more. Any other $$ is the inline parser's.
func (mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	t := bytes.TrimRight(line[pos:], " \t\r\n")
	if w > 3 || !bytes.HasPrefix(t, []byte("$$")) {
		return nil, parser.NoChildren
	}
	n := &mathBlock{}
	start := seg.Start + pos + 2
	switch {
	case len(t) == 2:
		if !closesBeforeABlankLine(reader.Source()[seg.Stop:]) {
			return nil, parser.NoChildren
		}
	case len(t) > 4 && bytes.HasSuffix(t, []byte("$$")) && !bytes.Contains(t[2:len(t)-2], []byte("$$")):
		n.Lines().Append(text.NewSegment(start, start+len(t)-4))
		n.closed = true
	default:
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

// closesBeforeABlankLine says whether a line of rest ends in $$ before a blank line or the end.
func closesBeforeABlankLine(rest []byte) bool {
	for len(rest) > 0 {
		line := rest
		if i := bytes.IndexByte(rest, '\n'); i >= 0 {
			line, rest = rest[:i+1], rest[i+1:]
		} else {
			rest = nil
		}
		if strings.TrimSpace(string(line)) == "" {
			return false
		}
		if closing(line) >= 0 {
			return true
		}
	}
	return false
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
