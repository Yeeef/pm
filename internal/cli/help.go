package cli

import (
	"fmt"
	"regexp"
	"strings"
)

// The --help text, laid out as Python 3.13's argparse lays it out at an unbounded width: the same usage line, sections
// and alignment, with no line wrapping. Python pm wraps to the terminal; whitespace aside, the two are identical.

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

// usage is the usage line after "usage: ": the prog, [-h], the optionals (a mutually exclusive group as one part),
// then the positionals and the subcommands.
func (c *command) usage() string {
	parts := []string{c.prog(), "[-h]"}
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
		if c.groups[a.group-1] {
			parts = append(parts, "("+strings.Join(members, " | ")+")")
		} else {
			parts = append(parts, "["+strings.Join(members, " | ")+"]")
		}
	}
	for _, a := range c.args {
		if a.positional() {
			parts = append(parts, formatArgs(a, a.dest))
		}
	}
	if c.subs != nil {
		parts = append(parts, c.choicesText()+" ...")
	}
	return strings.Join(parts, " ")
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
