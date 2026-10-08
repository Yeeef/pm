// Package cli is pm's command tree: every command with its help, built on cobra from the table in commands.go, the
// argument checks argparse makes, the config check every command passes first, and the dispatch to each command's
// body. Python source: the argparse tree and main() in cli.py.
//
// Cobra routes to the command; each command then parses its own arguments with pflag (cobra's flag parsing is off on
// leaves) so that the checks and the messages follow argparse: required arguments, mutually exclusive groups, choices,
// positional counts, and options that take two values (--option LABEL TEXT), which pflag cannot express.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/hooks"
)

type kind int

const (
	value       kind = iota // --flag VALUE, or a positional
	flagTrue                // store_true
	flagConst               // store_const: sets dest to constant
	appendValue             // append: repeatable --flag VALUE
)

// arg is one argparse argument.
type arg struct {
	flags    []string // option strings; none for a positional
	dest     string
	metavar  []string // nil: argparse's default
	nargs    string   // "", "?", "*", "+" or "2"
	choices  []string
	required bool
	kind     kind
	constant string
	isInt    bool
	def      string
	group    int // 1-based index into command.groups, 0 for none
	help     string
}

func (a arg) positional() bool { return len(a.flags) == 0 }

// command is one argparse parser: a noun with subcommands, or a leaf.
type command struct {
	name, help, description, epilog string
	raw                             bool   // the description keeps its line breaks
	groups                          []bool // mutually exclusive groups: required or not
	args                            []arg
	subDest                         string
	subs                            []*command
	parent                          *command
}

func (c *command) prog() string {
	if c.parent == nil {
		return c.name
	}
	return c.parent.prog() + " " + c.name
}

// path is the command's name under pm: "task add".
func (c *command) path() string { return strings.TrimPrefix(c.prog(), "pm ") }

// usageError is argparse's error(): the usage line and "<prog>: error: <message>" on stderr, exit 2.
type usageError struct {
	cmd *command
	msg string
}

func (e *usageError) Error() string { return e.msg }

// Parsed is a command's arguments after the checks: each dest's values, as argparse's namespace holds them.
type Parsed struct {
	cmd    *command
	values map[string][]string
}

// Get is the dest's last value, or "" when it was not given.
func (p *Parsed) Get(dest string) string {
	v := p.values[dest]
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

// Nouns is pm's top-level commands in the order the tree defines them.
func Nouns() []string {
	out := make([]string, len(tree.subs))
	for i, s := range tree.subs {
		out[i] = s.name
	}
	return out
}

// Leaves is every command that runs (40), as "task add".
func Leaves() []string {
	var out []string
	var walk func(c *command)
	walk = func(c *command) {
		if c.subs == nil {
			out = append(out, c.path())
		}
		for _, s := range c.subs {
			walk(s)
		}
	}
	walk(tree)
	return out
}

// pairSep joins the two values of a two-value option inside pflag, which holds one string per occurrence.
const pairSep = "\x00"

// parse is argparse's parse_args for one leaf command: the options in command-line order, each value checked as it is
// consumed, then the required arguments and groups, then what nothing consumed, which the root parser reports.
func (c *command) parse(args []string, fs *pflag.FlagSet) (*Parsed, bool, error) {
	known, pairs := map[string]bool{"-h": true, "--help": true}, map[string]bool{}
	for _, a := range c.args {
		for _, f := range a.flags {
			known[f] = true
			if a.nargs == "2" {
				pairs[f] = true
			}
		}
	}
	var joined, unknown []string
	for i := 0; i < len(args); i++ {
		t := args[i]
		if t == "--" {
			joined = append(joined, args[i:]...)
			break
		}
		long := strings.HasPrefix(t, "--")
		if long { // argparse takes an unambiguous prefix of a long option (allow_abbrev)
			name, value, eq := strings.Cut(t, "=")
			if !known[name] {
				var matches []string
				for f := range known {
					if strings.HasPrefix(f, "--") && strings.HasPrefix(f, name) {
						matches = append(matches, f)
					}
				}
				sort.Slice(matches, func(x, y int) bool { return c.flagOrder(matches[x]) < c.flagOrder(matches[y]) })
				if len(matches) > 1 {
					return nil, false, &usageError{c, fmt.Sprintf("ambiguous option: %s could match %s", t,
						strings.Join(matches, ", "))}
				}
				if len(matches) == 1 {
					t = matches[0]
					if eq {
						t += "=" + value
					}
				}
			}
		}
		switch {
		case pairs[t]:
			if i+2 >= len(args) {
				return nil, false, &usageError{c, fmt.Sprintf("argument %s: expected 2 arguments", c.byFlag(t).name())}
			}
			joined = append(joined, t+"="+args[i+1]+pairSep+args[i+2])
			i += 2
		case len(t) > 1 && t[0] == '-' && (negativeNumber.MatchString(t) || strings.Contains(t, " ")) &&
			!known[strings.SplitN(t, "=", 2)[0]] && (long || !known[t[:2]]):
			joined = append(joined, notOption+t) // a value that starts with '-', as argparse reads it
		case long && !known[strings.SplitN(t, "=", 2)[0]], !long && len(t) > 1 && t[0] == '-' && !known[t[:2]]:
			unknown = append(unknown, t)
		default:
			joined = append(joined, t)
		}
	}
	p := &Parsed{cmd: c, values: map[string][]string{}}
	seen := map[int]*arg{} // group -> the member given first
	help := false
	err := fs.ParseAll(joined, func(f *pflag.Flag, v string) error {
		if f.Name == "help" {
			help = true
			return nil
		}
		a := c.byFlag("--" + f.Name)
		v = strings.TrimPrefix(v, notOption)
		if a.group > 0 {
			if other, ok := seen[a.group]; ok && other != a {
				return &usageError{c, fmt.Sprintf("argument %s: not allowed with argument %s", a.name(), other.name())}
			}
			seen[a.group] = a
		}
		switch a.kind {
		case flagTrue:
			p.values[a.dest] = []string{"true"}
		case flagConst:
			p.values[a.dest] = []string{a.constant}
		case appendValue:
			v, err := c.check(a, v)
			if err != nil {
				return err
			}
			p.values[a.dest] = append(p.values[a.dest], v)
		default:
			v, err := c.check(a, v)
			if err != nil {
				return err
			}
			p.values[a.dest] = []string{v}
		}
		return fs.Set(f.Name, v)
	})
	if help {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, c.flagError(err)
	}
	// positionals, in order: each takes what its nargs allows, the ones after it keeping what they need
	var rest []string
	for _, v := range fs.Args() {
		rest = append(rest, strings.TrimPrefix(v, notOption))
	}
	var missing []string
	var positionals []*arg
	for i := range c.args {
		if c.args[i].positional() {
			positionals = append(positionals, &c.args[i])
		}
	}
	for i, a := range positionals {
		need := 0
		for _, later := range positionals[i+1:] {
			if later.nargs == "" || later.nargs == "+" {
				need++
			}
		}
		take := 0
		switch a.nargs {
		case "":
			take = min(1, len(rest))
		case "?":
			take = min(1, max(0, len(rest)-need))
		case "*", "+":
			take = max(0, len(rest)-need)
		}
		if (a.nargs == "" || a.nargs == "+") && take == 0 {
			missing = append(missing, a.name())
			continue
		}
		var values []string
		for _, v := range rest[:take] {
			v, err := c.check(a, v)
			if err != nil {
				return nil, false, err
			}
			values = append(values, v)
		}
		p.values[a.dest] = values
		rest = rest[take:]
	}
	for _, a := range c.args {
		if a.required && !a.positional() && len(p.values[a.dest]) == 0 {
			missing = append(missing, a.name())
		}
	}
	if len(missing) > 0 {
		return nil, false, &usageError{c, "the following arguments are required: " + strings.Join(missing, ", ")}
	}
	for g, required := range c.groups {
		if _, ok := seen[g+1]; required && !ok {
			var names []string
			for _, a := range c.args {
				if a.group == g+1 {
					names = append(names, a.flags[0])
				}
			}
			return nil, false, &usageError{c, fmt.Sprintf("one of the arguments %s is required", strings.Join(names, " "))}
		}
	}
	if extra := append(unknown, rest...); len(extra) > 0 {
		return nil, false, &usageError{tree, "unrecognized arguments: " + strings.Join(extra, " ")}
	}
	return p, false, nil
}

// check is argparse's check of one value as it is consumed: its type (an int is converted, as int() reads " +01 "),
// then its choices. It returns the value as converted.
func (c *command) check(a *arg, v string) (string, error) {
	given := v
	if a.isInt {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return "", &usageError{c, fmt.Sprintf("argument %s: invalid int value: %s", a.name(), quote(given))}
		}
		v = strconv.Itoa(n)
	}
	if a.choices != nil && !contains(a.choices, v) {
		return "", &usageError{c, fmt.Sprintf("argument %s: invalid choice: %s (choose from %s)", a.name(),
			quote(given), quoteAll(a.choices))}
	}
	return v, nil
}

// flagOrder is where an option string comes in argparse's table: -h and --help first, then the arguments in order.
func (c *command) flagOrder(flag string) int {
	for i, a := range c.args {
		if contains(a.flags, flag) {
			return i
		}
	}
	return -1
}

// notOption marks a value that starts with '-' but that argparse reads as a value (a negative number, or text with a
// space), so that pflag does not take it for an option; parse removes it again.
const notOption = "\x01"

var negativeNumber = regexp.MustCompile(`^-\d+$|^-\d*\.\d+$`)

func quoteAll(list []string) string {
	q := make([]string, len(list))
	for i, s := range list {
		q[i] = quote(s)
	}
	return strings.Join(q, ", ")
}

func quote(s string) string { return "'" + s + "'" }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// name is argparse's _get_action_name: the option strings joined by '/', else the metavar, else the dest.
func (a arg) name() string {
	switch {
	case len(a.flags) > 0:
		return strings.Join(a.flags, "/")
	case a.metavar != nil:
		return a.metavar[0]
	}
	return a.dest
}

func (c *command) byFlag(flag string) *arg {
	for i := range c.args {
		if contains(c.args[i].flags, flag) {
			return &c.args[i]
		}
	}
	panic(fmt.Sprintf("pm %s has no option %s", c.path(), flag))
}

// flagError is a pflag parse error in argparse's words.
func (c *command) flagError(err error) error {
	var ue *usageError
	var missing *pflag.ValueRequiredError
	var unknown *pflag.NotExistError
	switch {
	case errors.As(err, &ue):
		return ue
	case errors.As(err, &missing):
		return &usageError{c, fmt.Sprintf("argument %s: expected one argument", c.byFlag("--"+missing.GetFlag().Name).name())}
	case errors.As(err, &unknown):
		return &usageError{c, "unrecognized arguments: " + unknown.GetSpecifiedName()}
	}
	return &usageError{c, err.Error()}
}

// build makes the cobra command for c and its subcommands.
func build(c *command, run runner, out io.Writer) *cobra.Command {
	cc := &cobra.Command{Use: c.name, Short: c.help, Long: c.description, SilenceErrors: true, SilenceUsage: true}
	cc.SetHelpFunc(func(*cobra.Command, []string) { fmt.Fprint(out, c.helpText()) })
	if c.subs != nil {
		cc.Args = cobra.ArbitraryArgs // argparse's messages, not cobra's "unknown command"
		cc.RunE = func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &usageError{c, "the following arguments are required: " + c.subDest}
			}
			names := make([]string, len(c.subs))
			for i, s := range c.subs {
				names[i] = s.name
			}
			return &usageError{c, fmt.Sprintf("argument %s: invalid choice: %s (choose from %s)",
				c.subDest, quote(args[0]), quoteAll(names))}
		}
		for _, s := range c.subs {
			s.parent = c
			cc.AddCommand(build(s, run, out))
		}
		return cc
	}
	cc.DisableFlagParsing = true
	fs := cc.Flags()
	fs.SortFlags = false
	for _, a := range c.args {
		if a.positional() {
			continue
		}
		long, short := "", ""
		for _, f := range a.flags {
			if strings.HasPrefix(f, "--") {
				long = f[2:]
			} else {
				short = f[1:]
			}
		}
		if a.kind == flagTrue || a.kind == flagConst {
			fs.BoolP(long, short, false, a.help)
		} else {
			fs.StringArrayP(long, short, nil, a.help)
		}
	}
	cc.RunE = func(_ *cobra.Command, args []string) error { // cobra has added its -h/--help to fs by now
		p, help, err := c.parse(args, fs)
		if err != nil {
			return err
		}
		if help {
			fmt.Fprint(out, c.helpText())
			return nil
		}
		return run(p)
	}
	return cc
}

// runner runs a parsed command.
type runner func(*Parsed) error

// refusal is a command's own error: "error: <message>" on stderr, exit 1.
type refusal struct{ msg string }

func (r *refusal) Error() string { return r.msg }

// Execute runs pm with argv (without the program name) and returns the exit code.
func Execute(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	run := func(p *Parsed) error { return dispatch(p, stdin, stdout, stderr) }
	root := build(tree, run, stdout)
	root.SetHelpCommand(&cobra.Command{Use: "no-help-command", Hidden: true})
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetArgs(argv)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	var ue *usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ue):
		fmt.Fprintf(stderr, "usage: %s\n%s: error: %s\n", ue.cmd.usage(), ue.cmd.prog(), ue.msg)
		return 2
	default:
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
}

// cwd is the process's directory as Python's Path.cwd() gives it: the kernel's path, not $PWD.
func cwd() (string, error) { return syscall.Getwd() }

// dispatch runs a parsed command: the config check first, as for every Python command, then its body.
func dispatch(p *Parsed, stdin io.Reader, stdout, stderr io.Writer) error {
	here, err := cwd()
	if err != nil {
		return err
	}
	name := p.cmd.path()
	check := name != "upgrade"
	if name == "init" { // pm init writes a missing config
		root, err := config.Root(here)
		if err != nil {
			return err
		}
		if st, err := os.Stat(filepath.Join(root, config.Rel)); err != nil || !st.Mode().IsRegular() {
			check = false
		}
	}
	if check {
		if _, err := config.Load(here); err != nil {
			return err
		}
	}
	switch name {
	case "prime":
		part := hooks.Part{}
		switch v := p.Get("part"); v {
		case "":
		case "state":
			part.State = true
		case "subagent":
			part.Subagent = true
		default:
			part.Chunk, _ = strconv.Atoi(v) // parse checked it is one of the chunk numbers
		}
		return hooks.CmdPrime(part, p.Get("hook_json") == "true", Nouns(), stdin, stdout)
	case "hook stop":
		return hooks.HookStop(stdin, stdout, stderr)
	}
	return &refusal{fmt.Sprintf("pm %s is not in Go pm yet; Python pm runs it until the cut-over", name)}
}
