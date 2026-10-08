package site

import (
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Normalise is a page as the parity check compares it: parsed as HTML5, which decodes every entity, then written
// back with one escaper, attributes sorted by name, one block element per line. Runs of whitespace in text collapse to
// one space; text loses its whitespace where it meets a block boundary (the start or end of a block element, or a
// block sibling), where HTML renders none, and drops out when nothing is left; text in <pre>, <script>, <style> and
// <textarea> is kept as is. Elements, nesting, attribute values, comments and text are compared as they are.
func Normalise(page string) (string, error) {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return "", err
	}
	w := &writer{}
	w.children(doc, false)
	w.flush()
	return strings.Join(w.lines, "\n") + "\n", nil
}

var blockElements = map[atom.Atom]bool{}

func init() {
	for _, a := range []atom.Atom{atom.Html, atom.Head, atom.Body, atom.Title, atom.Meta, atom.Link, atom.Script,
		atom.Style, atom.Div, atom.P, atom.Ul, atom.Ol, atom.Li, atom.Table, atom.Thead, atom.Tbody, atom.Tfoot, atom.Tr,
		atom.Th, atom.Td, atom.Caption, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Pre, atom.Blockquote,
		atom.Nav, atom.Section, atom.Article, atom.Aside, atom.Header, atom.Footer, atom.Main, atom.Hr, atom.Form,
		atom.Dl, atom.Dt, atom.Dd, atom.Details, atom.Summary, atom.Figure, atom.Figcaption, atom.Textarea,
		atom.Noscript} {
		blockElements[a] = true
	}
}

func isBlock(n *html.Node) bool {
	return n != nil && n.Type == html.ElementNode && blockElements[n.DataAtom]
}

var rawText = map[atom.Atom]bool{atom.Pre: true, atom.Script: true, atom.Style: true, atom.Textarea: true}

type writer struct {
	lines []string
	line  strings.Builder
}

func (w *writer) flush() {
	if w.line.Len() > 0 {
		w.lines = append(w.lines, w.line.String())
		w.line.Reset()
	}
}

func (w *writer) block(s string) {
	w.flush()
	w.lines = append(w.lines, s)
}

func (w *writer) children(n *html.Node, raw bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.node(c, raw)
	}
}

func collapse(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\f' || r == '\r' {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}

func (w *writer) node(n *html.Node, raw bool) {
	switch n.Type {
	case html.TextNode:
		if raw {
			w.line.WriteString(html.EscapeString(n.Data))
			return
		}
		t := collapse(n.Data)
		if (n.PrevSibling == nil && isBlock(n.Parent)) || isBlock(n.PrevSibling) || n.PrevSibling == nil && n.Parent.Type == html.DocumentNode {
			t = strings.TrimLeft(t, " ")
		}
		if (n.NextSibling == nil && isBlock(n.Parent)) || isBlock(n.NextSibling) || n.NextSibling == nil && n.Parent.Type == html.DocumentNode {
			t = strings.TrimRight(t, " ")
		}
		w.line.WriteString(html.EscapeString(t))
	case html.CommentNode:
		w.line.WriteString("<!--" + n.Data + "-->")
	case html.DoctypeNode:
		w.block("<!doctype " + n.Data + ">")
	case html.DocumentNode:
		w.children(n, raw)
	case html.ElementNode:
		attrs := append([]html.Attribute{}, n.Attr...)
		sort.SliceStable(attrs, func(i, j int) bool {
			if attrs[i].Namespace != attrs[j].Namespace {
				return attrs[i].Namespace < attrs[j].Namespace
			}
			return attrs[i].Key < attrs[j].Key
		})
		var open strings.Builder
		open.WriteString("<" + n.Data)
		for _, a := range attrs {
			key := a.Key
			if a.Namespace != "" {
				key = a.Namespace + ":" + key
			}
			open.WriteString(" " + key + `="` + html.EscapeString(a.Val) + `"`)
		}
		open.WriteString(">")
		void := voidElements[n.DataAtom]
		inner := raw || rawText[n.DataAtom]
		if isBlock(n) {
			w.flush()
			if inner {
				w.line.WriteString(open.String())
				w.children(n, true)
				if !void {
					w.line.WriteString("</" + n.Data + ">")
				}
				w.flush()
				return
			}
			w.block(open.String())
			w.children(n, false)
			if !void {
				w.block("</" + n.Data + ">")
			}
			return
		}
		w.line.WriteString(open.String())
		w.children(n, inner)
		if !void {
			w.line.WriteString("</" + n.Data + ">")
		}
	}
}

var voidElements = map[atom.Atom]bool{atom.Area: true, atom.Base: true, atom.Br: true, atom.Col: true,
	atom.Embed: true, atom.Hr: true, atom.Img: true, atom.Input: true, atom.Link: true, atom.Meta: true,
	atom.Source: true, atom.Track: true, atom.Wbr: true}
