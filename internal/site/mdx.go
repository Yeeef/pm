package site

import (
	"bytes"
	"regexp"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MDX-style raw HTML, as notes written for an MDX site carry it: a block whose opening tag has a className attribute,
// or an <svg> block, runs to the line that closes its tag, blank lines included (a CommonMark HTML block ends at the
// first blank line, and the indented lines after it would turn into a code block). Its HTML is written as HTML:
// className becomes class, a JSX presentation attribute such as strokeWidth its SVG name, and {/* … */} comments are
// dropped. JSX expressions, imports and components are not MDX this supports.

var kindMDX = ast.NewNodeKind("MDXBlock")

// mdxBlock is one such block; its lines hold the HTML as written, end is the source offset its last line ends at.
type mdxBlock struct {
	ast.BaseBlock
	end int
}

func (n *mdxBlock) Kind() ast.NodeKind { return kindMDX }

func (n *mdxBlock) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

func (n *mdxBlock) IsRaw() bool { return true }

type mdxParser struct{}

var mdxOpenRE = regexp.MustCompile(`^<([a-zA-Z][a-zA-Z0-9]*)[\s>/]`)

func (mdxParser) Trigger() []byte { return []byte{'<'} }

// Open opens a block on a line, within three spaces of indent, that starts with an <svg> tag or a tag whose opening
// tag has className; a block whose tag does not close is left to the HTML block parser.
func (mdxParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 {
		return nil, parser.NoChildren
	}
	m := mdxOpenRE.FindSubmatch(line[pos:])
	if len(m) < 2 {
		return nil, parser.NoChildren
	}
	head := line[pos:]
	if i := bytes.IndexByte(head, '>'); i >= 0 {
		head = head[:i]
	}
	if string(m[1]) != "svg" && !bytes.Contains(head, []byte(" className=")) {
		return nil, parser.NoChildren
	}
	end := closeOf(reader.Source(), seg.Start+pos, string(m[1]))
	if end < 0 {
		return nil, parser.NoChildren
	}
	n := &mdxBlock{end: end}
	n.Lines().Append(seg)
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

var (
	scanners   = map[string]*regexp.Regexp{} // by tag name: a fence line, a {/* comment start, or the tag
	scannersMu sync.Mutex
	commentEnd = []byte("*/}")
)

// tagScanner finds, from an offset, the next fence line, {/* comment or open, close or self-closing tag of the name.
func tagScanner(tag string) *regexp.Regexp {
	scannersMu.Lock()
	defer scannersMu.Unlock()
	re, ok := scanners[tag]
	if !ok {
		re = regexp.MustCompile("(?m)(^ {0,3}(?:```|~~~))|(\\{/\\*)|<(/?)" + regexp.QuoteMeta(tag) + `\b[^>]*?(/?)>`)
		scanners[tag] = re
	}
	return re
}

// closeOf is the offset of the end of the line on which the tag opened at from closes, counting the same tag nested
// inside it and skipping {/* … */} comments; -1 when a fence line or the end comes first. It scans forward once.
func closeOf(source []byte, from int, tag string) int {
	re := tagScanner(tag)
	depth := 0
	for off := from; off < len(source); {
		m := re.FindSubmatchIndex(source[off:])
		if m == nil {
			return -1
		}
		start, stop := off+m[0], off+m[1]
		switch {
		case m[2] >= 0: // a fence line; ^ also matches where the slice starts, mid-line
			if start == 0 || source[start-1] == '\n' {
				return -1
			}
		case m[4] >= 0:
			i := bytes.Index(source[stop:], commentEnd)
			if i < 0 {
				return -1
			}
			stop += i + len(commentEnd)
		case m[7] > m[6]: // </tag>
			depth--
		case m[9] == m[8]: // <tag …>, not <tag …/>
			depth++
		}
		if depth <= 0 && m[6] >= 0 {
			if nl := bytes.IndexByte(source[stop:], '\n'); nl >= 0 {
				return stop + nl + 1
			}
			return len(source)
		}
		off = max(stop, start+1)
	}
	return -1
}

func (mdxParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	n, ok := node.(*mdxBlock)
	_, seg := reader.PeekLine()
	if !ok || seg.Start >= n.end {
		return parser.Close
	}
	n.Lines().Append(seg)
	reader.AdvanceToEOL()
	if seg.Stop >= n.end {
		return parser.Close
	}
	return parser.Continue | parser.NoChildren
}

func (mdxParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (mdxParser) CanInterruptParagraph() bool { return false } // an HTML block of CommonMark type 7

func (mdxParser) CanAcceptIndentedLine() bool { return false }

var (
	jsxComment = regexp.MustCompile(`(?s)\{/\*.*?\*/\}`)
	// jsxAttr is the JSX names of the HTML and SVG attributes MDX diagrams use whose HTML names differ.
	jsxAttr = regexp.MustCompile(`(\s)(className|htmlFor|strokeWidth|strokeDasharray|strokeDashoffset|strokeLinecap|` +
		`strokeLinejoin|strokeMiterlimit|strokeOpacity|fillOpacity|fillRule|clipRule|clipPath|textAnchor|fontSize|` +
		`fontWeight|fontFamily|fontStyle|stopColor|stopOpacity|markerStart|markerMid|markerEnd|dominantBaseline|` +
		`alignmentBaseline|letterSpacing|textDecoration)=`)
	upper = regexp.MustCompile(`[A-Z]`)
)

// mdxHTML is an MDX block's text as HTML.
func mdxHTML(src string) string {
	src = jsxComment.ReplaceAllString(src, "")
	return jsxAttr.ReplaceAllStringFunc(src, func(m string) string {
		lead, name := m[:1], strings.TrimSuffix(m[1:], "=")
		switch name {
		case "className":
			return lead + "class="
		case "htmlFor":
			return lead + "for="
		}
		return lead + upper.ReplaceAllStringFunc(name, func(c string) string { return "-" + strings.ToLower(c) }) + "="
	})
}

func (r *recordRenderer) mdx(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	var b bytes.Buffer
	for i := 0; i < node.Lines().Len(); i++ {
		b.Write(segValue(node.Lines().At(i), source))
	}
	_, err := w.WriteString(mdxHTML(b.String()))
	return ast.WalkSkipChildren, err
}
