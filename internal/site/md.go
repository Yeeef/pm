package site

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Yeeef/pm/internal/records"
)

// The Markdown renderers. Python pm renders with markdown-it-py's commonmark preset; goldmark is CommonMark too, and
// the parts below give it what pm adds: GFM tables wrapped in <div class="tbl"> with style alignment, ids on h2 and h3
// with mdit-py-plugins' anchors slug, the ::: note, decision and result containers, the mermaid fence, the link rewrite
// .md(#…) to .html(#…), and front matter skipped.

// ctx is one page's render: the record it is for, for errors, and whether a mermaid diagram is on it.
type ctx struct {
	rel     string
	mermaid bool
}

// recordMD is the renderer of record bodies and request descriptions: raw HTML kept, tables, anchors, containers.
func recordMD(c *ctx) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignStyle))),
		goldmark.WithParserOptions(
			parser.WithBlockParsers(util.Prioritized(&containerParser{}, 690)),
			parser.WithASTTransformers(util.Prioritized(anchors{}, 100), util.Prioritized(linkRewrite{}, 200),
				util.Prioritized(cellAlign{}, 300)),
		),
		goldmark.WithRendererOptions(ghtml.WithUnsafe(), renderer.WithNodeRenderers(util.Prioritized(&recordRenderer{c}, 100))),
	)
}

// plainMD is markdown-it's commonmark preset with raw HTML: what the overview renders request descriptions with.
var plainMD = goldmark.New(goldmark.WithRendererOptions(ghtml.WithUnsafe()))

// commentMD renders comments and model summaries: raw HTML is not parsed, so it shows as text (markdown-it's
// html=False).
var commentMD = goldmark.New(goldmark.WithParser(parser.NewParser(
	parser.WithBlockParsers(without(parser.DefaultBlockParsers(), parser.NewHTMLBlockParser())...),
	parser.WithInlineParsers(without(parser.DefaultInlineParsers(), parser.NewRawHTMLParser())...),
	parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
)))

// inlineHTML and inlineText render one line as markdown-it's renderInline does: inline syntax only, no block, so the
// parser knows only paragraphs; with and without raw HTML.
var (
	inlineHTML = goldmark.New(goldmark.WithParser(parser.NewParser(
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
	)), goldmark.WithRendererOptions(ghtml.WithUnsafe()))
	inlineText = goldmark.New(goldmark.WithParser(parser.NewParser(
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
		parser.WithInlineParsers(without(parser.DefaultInlineParsers(), parser.NewRawHTMLParser())...),
	)))
)

func without(list []util.PrioritizedValue, drop any) []util.PrioritizedValue {
	var out []util.PrioritizedValue
	for _, v := range list {
		if fmt.Sprintf("%T", v.Value) != fmt.Sprintf("%T", drop) {
			out = append(out, v)
		}
	}
	return out
}

func convert(md goldmark.Markdown, src string) (string, error) {
	var b bytes.Buffer
	if err := md.Convert([]byte(src), &b); err != nil {
		return "", err
	}
	return b.String(), nil
}

// render is a whole Markdown text as HTML.
func render(md goldmark.Markdown, src string) (string, error) {
	return convert(md, skipFrontMatter(src))
}

func mustRender(md goldmark.Markdown, src string) string {
	out, err := render(md, src)
	if err != nil {
		panic(err) // only the record renderer fails, on a container's attributes; the others cannot
	}
	return out
}

// renderInline is one line of Markdown rendered inline, without a paragraph around it.
func renderInline(md goldmark.Markdown, src string) string {
	out, err := convert(md, src)
	if err != nil {
		panic(err)
	}
	out = strings.TrimSuffix(out, "\n")
	if strings.HasPrefix(out, "<p>") && strings.HasSuffix(out, "</p>") {
		return out[3 : len(out)-4]
	}
	return out
}

// skipFrontMatter drops a front matter block that opens the text, as mdit-py-plugins' front_matter does: a first line
// starting with three or more dashes, up to a later line of at least as many dashes and nothing but spaces after them.
// An unclosed one is no front matter.
func skipFrontMatter(src string) string {
	if !strings.HasPrefix(src, "---") {
		return src
	}
	lines := strings.SplitAfter(src, "\n")
	first := strings.TrimRight(lines[0], "\n")
	n := len(first) - len(strings.TrimLeft(first, "-"))
	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\n")
		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) >= 4 || !strings.HasPrefix(trimmed, "-") {
			continue
		}
		dashes := len(trimmed) - len(strings.TrimLeft(trimmed, "-"))
		if dashes >= n && strings.Trim(trimmed[dashes:], " ") == "" {
			return strings.Join(lines[i+1:], "")
		}
	}
	return src
}

// ---------------------------------------------------------------- containers

var kindContainer = ast.NewNodeKind("Container")

// container is a ::: note, decision or result block.
type container struct {
	ast.BaseBlock
	name, info string
	count      int // the opening line's colons; the closing line has at least as many
}

func (n *container) Kind() ast.NodeKind { return kindContainer }

func (n *container) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

var containerNames = []string{"note", "decision", "result"}

type containerParser struct{}

func (p *containerParser) Trigger() []byte { return []byte{':'} }

// colons is the run of ':' at the line's first non-space character, within three spaces of indent; -1 when none.
func colons(line []byte, offset int) (pos, count int) {
	w, pos := util.IndentWidth(line, offset)
	if w > 3 {
		return -1, 0
	}
	i := pos
	for i < len(line) && line[i] == ':' {
		i++
	}
	return pos, i - pos
}

// Open opens a container on a line of three or more colons whose first word after them names one, as
// mdit-py-plugins' container validates it.
func (p *containerParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos, count := colons(line, reader.LineOffset())
	if pos < 0 || count < 3 {
		return nil, parser.NoChildren
	}
	params := strings.TrimRight(string(line[pos+count:]), "\r\n")
	name := strings.SplitN(records.Strip(params), " ", 3)[0]
	if !contains(containerNames, name) {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return &container{name: name, info: params, count: count}, parser.HasChildren
}

// Continue closes the container on a line of at least as many colons followed by spaces only; an unclosed one runs to
// the end of its parent.
func (p *containerParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if pos, count := colons(line, reader.LineOffset()); pos >= 0 && count >= node.(*container).count &&
		strings.Trim(string(line[pos+count:]), " \r\n") == "" {
		reader.AdvanceToEOL()
		return parser.Close
	}
	return parser.Continue | parser.HasChildren
}

func (p *containerParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (p *containerParser) CanInterruptParagraph() bool { return true }

func (p *containerParser) CanAcceptIndentedLine() bool { return false }

// ---------------------------------------------------------------- anchors

// anchors gives every h2 and h3 an id: mdit-py-plugins' slug of its text, made unique within the document.
type anchors struct{}

var slugDrop = regexp.MustCompile(`[^\p{L}\p{N}_\x{4e00}-\x{9fff}\- ]`)

// Slug is mdit-py-plugins' slugify: the title stripped and lowercased, spaces to dashes, every character dropped but
// word characters, CJK ideographs, dashes and spaces.
func Slug(title string) string {
	return slugDrop.ReplaceAllString(strings.ReplaceAll(strings.ToLower(records.Strip(title)), " ", "-"), "")
}

func (anchors) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	slugs := map[string]bool{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !entering || !ok || h.Level < 2 || h.Level > 3 {
			return ast.WalkContinue, nil
		}
		base := Slug(headingText(h, source))
		slug := base
		for i := 1; slugs[slug]; i++ {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		slugs[slug] = true
		h.SetAttributeString("id", []byte(slug))
		return ast.WalkSkipChildren, nil
	})
}

// headingText is what markdown-it's anchors read from a heading: the text of its text and code spans, emphasis and
// link labels included, raw HTML and images left out, escapes and entities decoded.
func headingText(h ast.Node, source []byte) string {
	var b strings.Builder
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				b.WriteString(decoded(t.Segment.Value(source)))
			case *ast.String:
				b.WriteString(string(t.Value))
			case *ast.CodeSpan:
				for cc := t.FirstChild(); cc != nil; cc = cc.NextSibling() {
					if tx, ok := cc.(*ast.Text); ok {
						b.Write(tx.Segment.Value(source))
					}
				}
			case *ast.AutoLink:
				b.Write(t.Label(source))
			case *ast.RawHTML, *ast.Image:
			default:
				walk(c)
			}
		}
	}
	walk(h)
	return b.String()
}

// decoded is a text segment as goldmark writes it, backslash escapes and entities resolved, then unescaped.
func decoded(seg []byte) string {
	var b bytes.Buffer
	w := bufWriter{&b}
	ghtml.DefaultWriter.Write(w, seg)
	return html.UnescapeString(b.String())
}

// headingSource is a heading's raw text, as markdown-it's inline token content holds it: what the table of contents
// shows.
func headingSource(h *ast.Heading, source []byte) string {
	var parts []string
	for i := 0; i < h.Lines().Len(); i++ {
		parts = append(parts, string(segValue(h.Lines().At(i), source)))
	}
	return records.Strip(strings.Join(parts, ""))
}

// ---------------------------------------------------------------- links

// linkRewrite points a relative link at a record's page: .md, or .md#anchor, becomes .html.
type linkRewrite struct{}

var (
	schemeRE = regexp.MustCompile(`^[a-z]+:`)
	mdLinkRE = regexp.MustCompile(`\.md(#|$)`)
)

func (linkRewrite) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.Link); ok && entering && !schemeRE.Match(l.Destination) {
			l.Destination = mdLinkRE.ReplaceAll(l.Destination, []byte(".html$1"))
		}
		return ast.WalkContinue, nil
	})
}

// cellAlign gives a cell that pads a short row its column's alignment, as markdown-it does; goldmark leaves it none.
type cellAlign struct{}

func (cellAlign) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		t, ok := n.(*east.Table)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		for row := t.FirstChild(); row != nil; row = row.NextSibling() {
			i := 0
			for c := row.FirstChild(); c != nil; c = c.NextSibling() {
				if cell, ok := c.(*east.TableCell); ok && i < len(t.Alignments) {
					cell.Alignment = t.Alignments[i]
				}
				i++
			}
		}
		return ast.WalkSkipChildren, nil
	})
}

// ---------------------------------------------------------------- rendering

type recordRenderer struct{ c *ctx }

func (r *recordRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindContainer, r.container)
	reg.Register(ast.KindFencedCodeBlock, r.fence)
	reg.Register(east.KindTable, r.table)
}

func (r *recordRenderer) container(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*container)
	if !entering {
		_, _ = w.WriteString("</div>\n")
		return ast.WalkContinue, nil
	}
	a := records.Attrs(n.info)
	for _, req := range records.BlockAttrs[n.name] {
		if _, ok := a[req]; !ok {
			return ast.WalkStop, &records.Error{Msg: fmt.Sprintf("%s: ::: %s needs '%s'", r.c.rel, n.name, req)}
		}
	}
	switch n.name {
	case "note":
		_, _ = w.WriteString(`<div class="note">` + "\n")
	case "result":
		_, _ = w.WriteString(`<div class="result"><p class="m-sec">` + esc(a["title"]) + "</p>\n")
	case "decision":
		meta := "<span>" + esc(a["source"]) + "</span><span>" + esc(a["date"]) + "</span>"
		if until, ok := a["until"]; ok {
			meta += `<span class="until">until ` + esc(until) + "</span>"
		}
		_, _ = w.WriteString(`<div class="decision"><div class="meta">` + meta + "</div>\n")
	}
	return ast.WalkContinue, nil
}

func (r *recordRenderer) fence(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	var content bytes.Buffer
	for i := 0; i < n.Lines().Len(); i++ {
		content.Write(segValue(n.Lines().At(i), source))
	}
	info := ""
	if n.Info != nil {
		info = string(n.Info.Segment.Value(source))
	}
	if records.Strip(info) == "mermaid" {
		if entering {
			r.c.mermaid = true
			_, _ = w.WriteString(`<pre class="mermaid">` + esc(content.String()) + "</pre>\n")
		}
		return ast.WalkSkipChildren, nil
	}
	if !entering {
		_, _ = w.WriteString("</code></pre>\n")
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("<pre><code")
	if lang := n.Language(source); lang != nil {
		_, _ = w.WriteString(` class="language-`)
		ghtml.DefaultWriter.Write(w, lang)
		_, _ = w.WriteString(`"`)
	}
	_, _ = w.WriteString(">" + esc(content.String()))
	return ast.WalkContinue, nil
}

func (r *recordRenderer) table(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<div class="tbl"><table>` + "\n")
	} else {
		_, _ = w.WriteString("</table></div>\n")
	}
	return ast.WalkContinue, nil
}

// bufWriter makes a bytes.Buffer a util.BufWriter.
type bufWriter struct{ *bytes.Buffer }

func (bufWriter) Available() int { return 4096 }
func (bufWriter) Buffered() int  { return 0 }
func (bufWriter) Flush() error   { return nil }

// esc is the one HTML escaper of every fragment pm builds: the standard library's.
func esc(s string) string { return html.EscapeString(s) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func segValue(s text.Segment, source []byte) []byte { return s.Value(source) }
