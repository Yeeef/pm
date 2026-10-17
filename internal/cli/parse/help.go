package parse

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// The --help text: the usage line, wrapped to the terminal's width, then the description, the positional arguments and
// the options, each item's help aligned at one column, then the epilog. Help texts are not wrapped.

const helpPosition = 24 // the help column's widest position; a wider invocation puts its help on the next line

var asciiSpace = regexp.MustCompile(`[ \t\n\r\f\v]+`)

// collapse is a help or description text as --help prints it: runs of ASCII whitespace become one space.
func collapse(s string) string { return strings.TrimSpace(asciiSpace.ReplaceAllString(s, " ")) }

// formatArgs is the metavar part of an argument in usage and help, as its arity shows it.
func formatArgs(a Arg, def string) string {
	m := a.Metavar
	if m == nil {
		if a.Choices != nil {
			m = []string{"{" + strings.Join(a.Choices, ",") + "}"}
		} else {
			m = []string{def}
		}
	}
	switch a.Arity {
	case One:
		return m[0]
	case Optional:
		return "[" + m[0] + "]"
	case Many:
		return "[" + m[0] + " ...]"
	case Two:
		if len(m) == 1 {
			m = []string{m[0], m[0]}
		}
		return strings.Join(m, " ")
	}
	panic(fmt.Sprintf("pm: arity %d of %s has no usage form", a.Arity, a.Dest))
}

// usagePart is one optional's part of the usage line, without the brackets of an optional or a group.
func usagePart(a Arg) string {
	if a.Kind == SwitchOn || a.Kind == SwitchTo {
		return a.Flags[0]
	}
	return a.Flags[0] + " " + formatArgs(a, strings.ToUpper(a.Dest))
}

// subs is the subcommands help lists: the visible ones, or with all the hidden ones too.
func (c *Command[T]) subs(all bool) []*Command[T] {
	var out []*Command[T]
	for _, s := range c.Subs {
		if all || !s.Hidden {
			out = append(out, s)
		}
	}
	return out
}

// choicesText is a subcommand list as usage and help show it: {a,b,c}.
func (c *Command[T]) choicesText(all bool) string {
	var names []string
	for _, s := range c.subs(all) {
		names = append(names, s.Name)
	}
	return "{" + strings.Join(names, ",") + "}"
}

// usageParts is the usage line's parts after the prog, the units it wraps at: [-h], each
// optional (a mutually exclusive group's members each a part, all but the last ending in " |", the first opening the
// group's bracket and the last closing it), then the positionals and the subcommands; nOpt is the optionals' count.
func (c *Command[T]) usageParts(all bool) (parts []string, nOpt int) {
	parts = []string{"[-h]"}
	for i := 0; i < len(c.Args); i++ {
		a := c.Args[i]
		if a.Positional() {
			continue
		}
		if a.Group == 0 {
			if a.Required {
				parts = append(parts, usagePart(a))
			} else {
				parts = append(parts, "["+usagePart(a)+"]")
			}
			continue
		}
		var members []string
		for ; i < len(c.Args) && c.Args[i].Group == a.Group; i++ {
			members = append(members, usagePart(c.Args[i]))
		}
		i--
		opening, closing := "[", "]"
		if c.Groups[a.Group-1] {
			opening, closing = "(", ")"
			if len(members) == 1 {
				opening, closing = "", ""
			}
		}
		members[0] = opening + members[0]
		members[len(members)-1] += closing
		for j := range members[:len(members)-1] {
			members[j] += " |"
		}
		parts = append(parts, members...)
	}
	nOpt = len(parts)
	for _, a := range c.Args {
		if a.Positional() {
			parts = append(parts, formatArgs(a, a.Dest))
		}
	}
	if c.Subs != nil {
		parts = append(parts, c.choicesText(all)+" ...")
	}
	return parts, nOpt
}

// Usage is the usage line after "usage: ", wrapped to the terminal's width less 2: when "usage: " and the line are
// longer, the parts go on lines indented under the first part after the prog, the positionals starting a line of their
// own; a prog wider than three quarters of the width puts every part on the lines under it.
func (c *Command[T]) Usage() string { return c.usage(false) }

// usage is Usage, its subcommands with the hidden ones when all.
func (c *Command[T]) usage(all bool) string {
	prog := c.Prog()
	parts, nOpt := c.usageParts(all)
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

// terminalColumns is the terminal's width: $COLUMNS when a positive number, else the width of the terminal on stdout,
// else 80.
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
	Help       string
}

// section lays out items: each invocation, then its help at the help column, or on the next line
// when the invocation is too wide for it.
func section(title string, items []helpItem, column int) string {
	var b strings.Builder
	b.WriteString(title + ":\n")
	for _, it := range items {
		width := column - it.indent - 2
		pad := strings.Repeat(" ", it.indent)
		switch {
		case it.Help == "":
			b.WriteString(pad + it.invocation + "\n")
		case len(it.invocation) <= width:
			fmt.Fprintf(&b, "%s%-*s  %s\n", pad, width, it.invocation, it.Help)
		default:
			fmt.Fprintf(&b, "%s%s\n%s%s\n", pad, it.invocation, strings.Repeat(" ", column), it.Help)
		}
	}
	return b.String()
}

// HelpText is the command's --help text, without its hidden subcommands.
func (c *Command[T]) HelpText() string { return c.helpText(false) }

// HelpTextAll is the command's --help --all text: its hidden subcommands listed too.
func (c *Command[T]) HelpTextAll() string { return c.helpText(true) }

// helpText is the --help text; a noun with hidden subcommands lists them only with all, and says so without it.
func (c *Command[T]) helpText(all bool) string {
	var positionals, options []helpItem
	for _, a := range c.Args {
		if a.Positional() {
			inv := a.Dest
			if a.Metavar != nil {
				inv = a.Metavar[0]
			} else if a.Choices != nil {
				inv = "{" + strings.Join(a.Choices, ",") + "}"
			}
			positionals = append(positionals, helpItem{2, inv, collapse(a.Help)})
		}
	}
	if c.Subs != nil {
		positionals = append(positionals, helpItem{2, c.choicesText(all), ""})
		for _, s := range c.subs(all) {
			positionals = append(positionals, helpItem{4, s.Name, collapse(s.Help)})
		}
	}
	options = append(options, helpItem{2, "-h, --help", "show this help message and exit"})
	for _, a := range c.Args {
		if a.Positional() {
			continue
		}
		inv := strings.Join(a.Flags, ", ")
		if a.Kind != SwitchOn && a.Kind != SwitchTo {
			inv += " " + formatArgs(a, strings.ToUpper(a.Dest))
		}
		options = append(options, helpItem{2, inv, collapse(a.Help)})
	}
	longest := 0 // a subcommand is measured at its section's indent, not its own
	for _, it := range append(append([]helpItem{}, positionals...), options...) {
		longest = max(longest, 2+len(it.invocation))
	}
	column := min(longest+2, helpPosition)

	parts := []string{"usage: " + c.usage(all) + "\n"}
	if c.Description != "" {
		if c.Raw {
			parts = append(parts, c.Description+"\n")
		} else {
			parts = append(parts, collapse(c.Description)+"\n")
		}
	}
	if len(positionals) > 0 {
		parts = append(parts, section("positional arguments", positionals, column))
	}
	parts = append(parts, section("options", options, column))
	if c.Epilog != "" {
		parts = append(parts, collapse(c.Epilog)+"\n")
	}
	if !all && len(c.subs(true)) > len(c.subs(false)) {
		parts = append(parts, c.Prog()+" --help --all also lists the machinery commands: what the runtimes' hooks, "+
			"the scheduler and the pm service run, not agents.\n")
	}
	return strings.Join(parts, "\n")
}
