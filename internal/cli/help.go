package cli

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// The --help text, laid out as Python 3.13's argparse lays it out: the usage line wrapped to the terminal's width as
// argparse wraps it, then the sections and alignment, with no wrapping of the help texts. Python pm wraps those to the
// terminal too; whitespace aside, the two are identical.

const helpPosition = 24 // argparse's max_help_position

var asciiSpace = regexp.MustCompile(`[ \t\n\r\f\v]+`)

// collapse is argparse's whitespace handling of a help or description text: runs of ASCII whitespace become one space.
func collapse(s string) string { return strings.TrimSpace(asciiSpace.ReplaceAllString(s, " ")) }

// formatArgs is argparse's _format_args: the metavar part of an argument in usage and help.
func formatArgs(a arg, def string) string {
	m := a.metavar
	if m == nil {
		if a.choices != nil {
			m = []string{"{" + strings.Join(a.choices, ",") + "}"}
		} else {
			m = []string{def}
		}
	}
	switch a.nargs {
	case "":
		return m[0]
	case "?":
		return "[" + m[0] + "]"
	case "*":
		return "[" + m[0] + " ...]"
	case "+":
		return m[0] + " [" + m[0] + " ...]"
	case "2":
		if len(m) == 1 {
			m = []string{m[0], m[0]}
		}
		return strings.Join(m, " ")
	}
	panic(fmt.Sprintf("pm: nargs %q of %s has no usage form", a.nargs, a.dest))
}

// usagePart is one optional's part of the usage line, without the brackets of an optional or a group.
func usagePart(a arg) string {
	if a.kind == flagTrue || a.kind == flagConst {
		return a.flags[0]
	}
	return a.flags[0] + " " + formatArgs(a, strings.ToUpper(a.dest))
}

// choicesText is a subcommand list as argparse shows it: {a,b,c}.
func (c *command) choicesText() string {
	names := make([]string, len(c.subs))
	for i, s := range c.subs {
		names[i] = s.name
	}
	return "{" + strings.Join(names, ",") + "}"
}

// usageParts is the usage line's parts after the prog, as Python 3.13's argparse cuts them for wrapping: [-h], each
// optional (a mutually exclusive group's members each a part, all but the last ending in " |", the first opening the
// group's bracket and the last closing it), then the positionals and the subcommands; nOpt is the optionals' count.
func (c *command) usageParts() (parts []string, nOpt int) {
	parts = []string{"[-h]"}
	for i := 0; i < len(c.args); i++ {
		a := c.args[i]
		if a.positional() {
			continue
		}
		if a.group == 0 {
			if a.required {
				parts = append(parts, usagePart(a))
			} else {
				parts = append(parts, "["+usagePart(a)+"]")
			}
			continue
		}
		var members []string
		for ; i < len(c.args) && c.args[i].group == a.group; i++ {
			members = append(members, usagePart(c.args[i]))
		}
		i--
		open, close := "[", "]"
		if c.groups[a.group-1] {
			open, close = "(", ")"
			if len(members) == 1 {
				open, close = "", ""
			}
		}
		members[0] = open + members[0]
		members[len(members)-1] += close
		for j := range members[:len(members)-1] {
			members[j] += " |"
		}
		parts = append(parts, members...)
	}
	nOpt = len(parts)
	for _, a := range c.args {
		if a.positional() {
			parts = append(parts, formatArgs(a, a.dest))
		}
	}
	if c.subs != nil {
		parts = append(parts, c.choicesText()+" ...")
	}
	return parts, nOpt
}

// usage is the usage line after "usage: ", wrapped as argparse wraps it to the terminal's width (HelpFormatter: the
// width shutil.get_terminal_size gives, less 2): when "usage: " and the line are longer, the parts go on lines indented
// under the first part after the prog, the positionals starting a line of their own.
func (c *command) usage() string {
	prog := c.prog()
	parts, nOpt := c.usageParts()
	line := prog + " " + strings.Join(parts, " ")
	const prefix = "usage: "
	width := terminalColumns() - 2
	if len(prefix)+len(line) <= width {
		return line
	}
	getLines := func(parts []string, indent string, first bool) []string {
		var lines, cur []string
		n := len(indent) - 1
		if first {
			n = len(prefix) - 1
		}
		for _, p := range parts {
			if n+1+len(p) > width && cur != nil {
				lines = append(lines, indent+strings.Join(cur, " "))
				cur, n = nil, len(indent)-1
			}
			cur = append(cur, p)
			n += len(p) + 1
		}
		if cur != nil {
			lines = append(lines, indent+strings.Join(cur, " "))
		}
		if first {
			lines[0] = lines[0][len(indent):]
		}
		return lines
	}
	opts, pos := parts[:nOpt], parts[nOpt:]
	var lines []string
	if float64(len(prefix)+len(prog)) <= 0.75*float64(width) {
		indent := strings.Repeat(" ", len(prefix)+len(prog)+1)
		switch {
		case len(opts) > 0:
			lines = getLines(append([]string{prog}, opts...), indent, true)
			lines = append(lines, getLines(pos, indent, false)...)
		case len(pos) > 0:
			lines = getLines(append([]string{prog}, pos...), indent, true)
		default:
			lines = []string{prog}
		}
	} else {
		indent := strings.Repeat(" ", len(prefix))
		lines = getLines(parts, indent, false)
		if len(lines) > 1 {
			lines = append(getLines(opts, indent, false), getLines(pos, indent, false)...)
		}
		lines = append([]string{prog}, lines...)
	}
	return strings.Join(lines, "\n")
}

// terminalColumns is shutil.get_terminal_size().columns: $COLUMNS when a positive number, else the width of the
// terminal on stdout, else 80.
func terminalColumns() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

type helpItem struct {
	indent     int
	invocation string
	help       string
}

// section lays out items as argparse does: each invocation, then its help at the help column, or on the next line
// when the invocation is too wide for it.
func section(title string, items []helpItem, column int) string {
	var b strings.Builder
	b.WriteString(title + ":\n")
	for _, it := range items {
		width := column - it.indent - 2
		pad := strings.Repeat(" ", it.indent)
		switch {
		case it.help == "":
			b.WriteString(pad + it.invocation + "\n")
		case len(it.invocation) <= width:
			fmt.Fprintf(&b, "%s%-*s  %s\n", pad, width, it.invocation, it.help)
		default:
			fmt.Fprintf(&b, "%s%s\n%s%s\n", pad, it.invocation, strings.Repeat(" ", column), it.help)
		}
	}
	return b.String()
}

// help is the command's --help text.
func (c *command) helpText() string {
	var positionals, options []helpItem
	for _, a := range c.args {
		if a.positional() {
			inv := a.dest
			if a.metavar != nil {
				inv = a.metavar[0]
			} else if a.choices != nil {
				inv = "{" + strings.Join(a.choices, ",") + "}"
			}
			positionals = append(positionals, helpItem{2, inv, collapse(a.help)})
		}
	}
	if c.subs != nil {
		positionals = append(positionals, helpItem{2, c.choicesText(), ""})
		for _, s := range c.subs {
			positionals = append(positionals, helpItem{4, s.name, collapse(s.help)})
		}
	}
	options = append(options, helpItem{2, "-h, --help", "show this help message and exit"})
	for _, a := range c.args {
		if a.positional() {
			continue
		}
		inv := strings.Join(a.flags, ", ")
		if a.kind != flagTrue && a.kind != flagConst {
			inv += " " + formatArgs(a, strings.ToUpper(a.dest))
		}
		options = append(options, helpItem{2, inv, collapse(a.help)})
	}
	longest := 0 // argparse measures a subcommand at its section's indent, not its own
	for _, it := range append(append([]helpItem{}, positionals...), options...) {
		longest = max(longest, 2+len(it.invocation))
	}
	column := min(longest+2, helpPosition)

	parts := []string{"usage: " + c.usage() + "\n"}
	if c.description != "" {
		if c.raw {
			parts = append(parts, c.description+"\n")
		} else {
			parts = append(parts, collapse(c.description)+"\n")
		}
	}
	if len(positionals) > 0 {
		parts = append(parts, section("positional arguments", positionals, column))
	}
	parts = append(parts, section("options", options, column))
	if c.epilog != "" {
		parts = append(parts, collapse(c.epilog)+"\n")
	}
	return strings.Join(parts, "\n")
}
