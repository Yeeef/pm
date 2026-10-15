package site

import (
	"fmt"
	"regexp"
	"strings"
)

// JSX in raw HTML. pm renders CommonMark, not MDX: a note written for an MDX site carries JSX in its HTML, which a
// browser does not read (className, strokeWidth and {/* */} are not HTML) and a blank line inside an <svg> ends the
// HTML block, so the rest of the drawing turns into text or a code block. jsxErrors finds both with a line scan:
// fenced and indented code are skipped, an HTML block runs from a line that starts with a tag to a blank line, and an
// open <svg> keeps the lines after its blank lines in HTML up to its </svg> or a fence. Code spans and <!-- -->
// comments are cut from a line before it is read.

var (
	fenceRE     = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	htmlStartRE = regexp.MustCompile(`^ {0,3}</?[a-zA-Z]`)
	codeSpanRE  = regexp.MustCompile("(`+)[^`]*?(`+)")
	htmlCommRE  = regexp.MustCompile(`<!--.*?-->`)
	svgOpenRE   = regexp.MustCompile(`<svg[\s>]`)
	svgCloseRE  = regexp.MustCompile(`</svg\s*>`)
	classNameRE = regexp.MustCompile(`\bclassName\s*=`)
	jsxCommRE   = regexp.MustCompile(`\{/\*`)
	jsxValueRE  = regexp.MustCompile(`(?:^|\s)([a-zA-Z_:][-a-zA-Z0-9_:.]*)=\{`)
	// camelRE is the JSX names of the SVG presentation attributes whose SVG names are kebab-case.
	camelRE = regexp.MustCompile(`\b(alignmentBaseline|baselineShift|clipPath|clipRule|colorInterpolation|` +
		`colorInterpolationFilters|dominantBaseline|fillOpacity|fillRule|floodColor|floodOpacity|fontFamily|fontSize|` +
		`fontSizeAdjust|fontStretch|fontStyle|fontVariant|fontWeight|imageRendering|letterSpacing|lightingColor|` +
		`markerEnd|markerMid|markerStart|paintOrder|pointerEvents|shapeRendering|stopColor|stopOpacity|` +
		`strokeDasharray|strokeDashoffset|strokeLinecap|strokeLinejoin|strokeMiterlimit|strokeOpacity|strokeWidth|` +
		`textAnchor|textDecoration|textRendering|vectorEffect|wordSpacing|writingMode)\s*=`)
	upperRE = regexp.MustCompile(`[A-Z]`)
)

// jsxErrors is each JSX construct, and each blank line inside an <svg>, in the raw HTML of a record's text, one line
// each: <file>.md:<line>: <what> is JSX, not HTML; <fix>. first is the file line of the text's first line.
func jsxErrors(rel, body string, first int) []string {
	var errs []string
	report := func(i int, what, fix string) {
		errs = append(errs, fmt.Sprintf("%s.md:%d: %s; %s", rel, first+i, what, fix))
	}
	fence := ""                    // the open fence's marker run, else ""
	html, svg := false, 0          // in an HTML block; the <svg> elements open
	comment := false               // in a <!-- --> comment that spans lines
	prevBlank, code := true, false // the line before was blank; in an indented code block
	for i, line := range strings.Split(body, "\n") {
		blank := strings.TrimSpace(line) == ""
		if fence != "" {
			// a closing fence: within three spaces of indent, a run of the fence's character as long as it, alone
			t := strings.TrimSpace(line)
			if len(line)-len(strings.TrimLeft(line, " ")) <= 3 && len(t) >= len(fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if m := fenceRE.FindStringSubmatch(line); m != nil && !(html && svg == 0) {
			fence, html, svg, comment, code = m[1], false, 0, false, false
			continue
		}
		switch {
		case blank && svg > 0:
			report(i, "a blank line between <svg and </svg>", "remove the blank line: a blank line ends an HTML block")
			prevBlank = true
			continue
		case blank:
			html, code, prevBlank = false, false, true
			continue
		case !html && svg == 0 && (code || prevBlank) && strings.HasPrefix(strings.ReplaceAll(line, "\t", "    "), "    "):
			code, prevBlank = true, false
			continue
		case !html && svg == 0 && htmlStartRE.MatchString(line):
			html = true
		}
		code, prevBlank = false, false
		if !html && svg == 0 {
			continue
		}
		html = true
		s := codeSpanRE.ReplaceAllString(line, "")
		if comment {
			end := strings.Index(s, "-->")
			if end < 0 {
				continue
			}
			s, comment = s[end+3:], false
		}
		s = htmlCommRE.ReplaceAllString(s, "")
		if k := strings.Index(s, "<!--"); k >= 0 {
			s, comment = s[:k], true
		}
		if classNameRE.MatchString(s) {
			report(i, "className= is JSX, not HTML", "use class=")
		}
		if jsxCommRE.MatchString(s) {
			report(i, "{/* … */} is JSX, not HTML", "remove the comment or use <!-- -->")
		}
		for _, m := range jsxValueRE.FindAllStringSubmatch(s, -1) {
			report(i, m[1]+"={…} is JSX, not HTML", "write the computed value in quotes")
		}
		for _, m := range camelRE.FindAllStringSubmatch(s, -1) {
			kebab := upperRE.ReplaceAllStringFunc(m[1], func(c string) string { return "-" + strings.ToLower(c) })
			report(i, m[1]+" is JSX, not HTML", "use "+kebab)
		}
		svg += len(svgOpenRE.FindAllString(s, -1)) - len(svgCloseRE.FindAllString(s, -1))
		svg = max(svg, 0)
	}
	return errs
}
