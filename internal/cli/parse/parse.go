// Package parse builds pm's command tree and validates its arguments with pflag.
package parse

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"io"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Kind is what an option does with what follows it on the command line.
type Kind int

const (
	Value    Kind = iota // --flag VALUE, or a positional: the last one given is the value
	SwitchOn             // --flag alone: its dest is "true"
	SwitchTo             // --flag alone: its dest is the Arg's Constant, so several switches can name one choice
	Repeated             // --flag VALUE, repeatable: every value given, in order
)

// Arity is how many values one positional or option takes.
type Arity int

const (
	One      Arity = iota // exactly one
	Optional              // a positional that may be left out
	Many                  // a positional that takes every value left, none included
	Two                   // an option followed by two values, such as --option LABEL TEXT
)

// Arg defines one positional or option and its validation.
type Arg struct {
	Flags    []string // option strings; none for a positional
	Dest     string
	Metavar  []string // nil: generated from choices or destination
	Arity    Arity
	Choices  []string
	Required bool
	Kind     Kind
	Constant string // the value a SwitchTo option sets
	IsInt    bool
	Def      string
	Group    int // 1-based index into Command.Groups, 0 for none
	Help     string
}

func (a Arg) Positional() bool { return len(a.Flags) == 0 }

// Command defines a noun or leaf; T holds the application-specific command handler.
type Command[T comparable] struct {
	Name, Help, Description, Epilog string
	Raw                             bool   // the description keeps its line breaks
	Groups                          []bool // mutually exclusive Groups: required or not
	Args                            []Arg
	SubDest                         string
	Subs                            []*Command[T]
	Parent                          *Command[T]
	Store                           T // nonzero for a handler that parses its own arguments
}

func (c *Command[T]) Prog() string {
	if c.Parent == nil {
		return c.Name
	}
	return c.Parent.Prog() + " " + c.Name
}

// Path returns the command name under pm, such as "task add".
func (c *Command[T]) Path() string { return strings.TrimPrefix(c.Prog(), "pm ") }

// UsageError describes invalid arguments, printed with the command's usage and exit code 2.
type UsageError[T comparable] struct {
	Cmd *Command[T]
	Msg string
}

func (e *UsageError[T]) Error() string { return e.Msg }

// Parsed holds validated arguments by destination.
type Parsed[T comparable] struct {
	Cmd    *Command[T]
	Values map[string][]string
}

// Get returns the last value, or "" when the destination was not given.
func (p *Parsed[T]) Get(dest string) string {
	v := p.Values[dest]
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

// Given is the first of these dests' arguments that was given, or nil.
func (p *Parsed[T]) Given(dests ...string) *Arg {
	for _, d := range dests {
		if _, ok := p.Values[d]; ok {
			for i := range p.Cmd.Args {
				if p.Cmd.Args[i].Dest == d {
					return &p.Cmd.Args[i]
				}
			}
		}
	}
	return nil
}

func (p *Parsed[T]) ValuesOf(dest string) []string { return p.Values[dest] }

// PairSep joins a two-value option inside pflag, which holds one string per occurrence.
const PairSep = "\x00"

// Parse validates options in command-line order, then positionals and required groups.
// Unknown arguments are reported by the root command.
func (c *Command[T]) Parse(args []string, fs *pflag.FlagSet) (*Parsed[T], bool, error) {
	joined, unknown, err := c.normalize(args)
	if err != nil {
		return nil, false, err
	}
	p, seen, help, err := c.parseFlags(joined, fs)
	if help {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, c.flagError(err)
	}
	rest, missing, err := c.parsePositionals(p, fs.Args())
	if err != nil {
		return nil, false, err
	}
	if err := c.require(p, missing, seen); err != nil {
		return nil, false, err
	}
	if extra := slices.Concat(unknown, rest); len(extra) > 0 {
		return nil, false, &UsageError[T]{c.root(), "unrecognized arguments: " + strings.Join(extra, " ")}
	}
	return p, false, nil
}

// check converts integers (including surrounding whitespace) before validating choices.
func (c *Command[T]) check(a *Arg, v string) (string, error) {
	given := v
	if a.IsInt {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return "", &UsageError[T]{c, fmt.Sprintf("argument %s: invalid int value: %s", a.Name(), quote(given))}
		}
		v = strconv.Itoa(n)
	}
	if a.Choices != nil && !slices.Contains(a.Choices, v) {
		return "", &UsageError[T]{c, fmt.Sprintf("argument %s: invalid choice: %s (choose from %s)", a.Name(),
			quote(given), quoteAll(a.Choices))}
	}
	return v, nil
}

// flagOrder puts help first, followed by declared options in order.
func (c *Command[T]) flagOrder(flag string) int {
	for i, a := range c.Args {
		if slices.Contains(a.Flags, flag) {
			return i
		}
	}
	return -1
}

// notOption marks a word that starts with '-' but is a value, not an option: a negative number, or text with a space.
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

// Name returns the option strings, metavar or destination used in errors.
func (a Arg) Name() string {
	switch {
	case len(a.Flags) > 0:
		return strings.Join(a.Flags, "/")
	case a.Metavar != nil:
		return a.Metavar[0]
	}
	return a.Dest
}

func (c *Command[T]) byFlag(flag string) *Arg {
	for i := range c.Args {
		if slices.Contains(c.Args[i].Flags, flag) {
			return &c.Args[i]
		}
	}
	panic(fmt.Sprintf("pm %s has no option %s", c.Path(), flag))
}

// flagError translates pflag errors into pm's usage errors.
func (c *Command[T]) flagError(err error) error {
	var ue *UsageError[T]
	var missing *pflag.ValueRequiredError
	var unknown *pflag.NotExistError
	switch {
	case errors.As(err, &ue):
		return ue
	case errors.As(err, &missing):
		return &UsageError[T]{c, fmt.Sprintf("argument %s: expected one argument", c.byFlag("--"+missing.GetFlag().Name).Name())}
	case errors.As(err, &unknown):
		return &UsageError[T]{c, "unrecognized arguments: " + unknown.GetSpecifiedName()}
	}
	return &UsageError[T]{c, err.Error()}
}

// Build constructs the cobra tree; runStore handles leaves with their own flag parser.
func Build[T comparable](c *Command[T], run func(*Parsed[T]) error, runStore func(name string, args []string) error, out io.Writer) *cobra.Command {
	cc := &cobra.Command{Use: c.Name, Short: c.Help, Long: c.Description, SilenceErrors: true, SilenceUsage: true}
	cc.SetHelpFunc(func(*cobra.Command, []string) { fmt.Fprint(out, c.HelpText()) })
	if c.Subs != nil {
		cc.Args = cobra.ArbitraryArgs // validate subcommand names with the same usage errors as arguments
		cc.RunE = func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &UsageError[T]{c, "the following arguments are required: " + c.SubDest}
			}
			names := make([]string, len(c.Subs))
			for i, s := range c.Subs {
				names[i] = s.Name
			}
			return &UsageError[T]{c, fmt.Sprintf("argument %s: invalid choice: %s (choose from %s)",
				c.SubDest, quote(args[0]), quoteAll(names))}
		}
		for _, s := range c.Subs {
			s.Parent = c
			cc.AddCommand(Build(s, run, runStore, out))
		}
		return cc
	}
	cc.DisableFlagParsing = true
	var zero T
	if c.Store != zero { // its own pflag parse and help (store_commands.go), after argv reached it through the tree
		cc.RunE = func(_ *cobra.Command, args []string) error { return runStore(c.Path(), args) }
		return cc
	}
	fs := cc.Flags()
	fs.SortFlags = false
	for _, a := range c.Args {
		if a.Positional() {
			continue
		}
		long, short := "", ""
		for _, f := range a.Flags {
			if strings.HasPrefix(f, "--") {
				long = f[2:]
			} else {
				short = f[1:]
			}
		}
		if a.Kind == SwitchOn || a.Kind == SwitchTo {
			fs.BoolP(long, short, false, a.Help)
		} else {
			fs.StringArrayP(long, short, nil, a.Help)
		}
	}
	cc.RunE = func(_ *cobra.Command, args []string) error { // cobra has added its -h/--help to fs by now
		p, help, err := c.Parse(args, fs)
		if err != nil {
			return err
		}
		if help {
			fmt.Fprint(out, c.HelpText())
			return nil
		}
		return run(p)
	}
	return cc
}

func (c *Command[T]) root() *Command[T] {
	for c.Parent != nil {
		c = c.Parent
	}
	return c
}

func (c *Command[T]) normalize(args []string) ([]string, []string, error) {
	known, pairs := map[string]bool{"-h": true, "--help": true}, map[string]bool{}
	for _, a := range c.Args {
		for _, f := range a.Flags {
			known[f] = true
			if a.Arity == Two {
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
		if long {
			var err error
			t, err = c.longOption(t, known)
			if err != nil {
				return nil, nil, err
			}
		}
		switch {
		case pairs[t]:
			if i+2 >= len(args) {
				return nil, nil, &UsageError[T]{c, fmt.Sprintf("argument %s: expected 2 arguments", c.byFlag(t).Name())}
			}
			joined = append(joined, t+"="+args[i+1]+PairSep+args[i+2])
			i += 2
		case len(t) > 1 && t[0] == '-' && (negativeNumber.MatchString(t) || strings.Contains(t, " ")) &&
			!known[strings.SplitN(t, "=", 2)[0]] && (long || !known[t[:2]]):
			joined = append(joined, notOption+t) // positional text starting with '-'
		case long && !known[strings.SplitN(t, "=", 2)[0]], !long && len(t) > 1 && t[0] == '-' && !known[t[:2]]:
			unknown = append(unknown, t)
		default:
			joined = append(joined, t)
		}
	}
	return joined, unknown, nil
}

func (c *Command[T]) parseFlags(joined []string, fs *pflag.FlagSet) (*Parsed[T], map[int]*Arg, bool, error) {
	p := &Parsed[T]{Cmd: c, Values: map[string][]string{}}
	seen := map[int]*Arg{} // group -> the member given first
	help := false
	err := fs.ParseAll(joined, func(f *pflag.Flag, v string) error {
		if f.Name == "help" {
			help = true
			return nil
		}
		a := c.byFlag("--" + f.Name)
		v = strings.TrimPrefix(v, notOption)
		if a.Group > 0 {
			if other, ok := seen[a.Group]; ok && other != a {
				return &UsageError[T]{c, fmt.Sprintf("argument %s: not allowed with argument %s", a.Name(), other.Name())}
			}
			seen[a.Group] = a
		}
		switch a.Kind {
		case SwitchOn:
			p.Values[a.Dest] = []string{"true"}
		case SwitchTo:
			p.Values[a.Dest] = []string{a.Constant}
		case Repeated:
			v, err := c.check(a, v)
			if err != nil {
				return err
			}
			p.Values[a.Dest] = append(p.Values[a.Dest], v)
		default:
			v, err := c.check(a, v)
			if err != nil {
				return err
			}
			p.Values[a.Dest] = []string{v}
		}
		return fs.Set(f.Name, v)
	})
	return p, seen, help, err
}

func (c *Command[T]) parsePositionals(p *Parsed[T], args []string) ([]string, []string, error) {
	// positionals, in order: each takes what its arity allows, the ones after it keeping what they need
	var rest []string
	for _, v := range args {
		rest = append(rest, strings.TrimPrefix(v, notOption))
	}
	var missing []string
	var positionals []*Arg
	for i := range c.Args {
		if c.Args[i].Positional() {
			positionals = append(positionals, &c.Args[i])
		}
	}
	for i, a := range positionals {
		need := 0
		for _, later := range positionals[i+1:] {
			if later.Arity == One {
				need++
			}
		}
		var take int
		switch a.Arity {
		case One:
			take = min(1, len(rest))
		case Optional:
			take = min(1, max(0, len(rest)-need))
		case Many:
			take = max(0, len(rest)-need)
		default:
			panic(fmt.Sprintf("pm %s: positional %s cannot take arity %d", c.Path(), a.Dest, a.Arity))
		}
		if a.Arity == One && take == 0 {
			missing = append(missing, a.Name())
			continue
		}
		var values []string
		for _, v := range rest[:take] {
			v, err := c.check(a, v)
			if err != nil {
				return nil, nil, err
			}
			values = append(values, v)
		}
		p.Values[a.Dest] = values
		rest = rest[take:]
	}
	return rest, missing, nil
}

func (c *Command[T]) require(p *Parsed[T], missing []string, seen map[int]*Arg) error {
	for _, a := range c.Args {
		if a.Required && !a.Positional() && len(p.Values[a.Dest]) == 0 {
			missing = append(missing, a.Name())
		}
	}
	if len(missing) > 0 {
		return &UsageError[T]{c, "the following arguments are required: " + strings.Join(missing, ", ")}
	}
	for g, required := range c.Groups {
		if _, ok := seen[g+1]; required && !ok {
			var names []string
			for _, a := range c.Args {
				if a.Group == g+1 {
					names = append(names, a.Flags[0])
				}
			}
			return &UsageError[T]{c, fmt.Sprintf("one of the arguments %s is required", strings.Join(names, " "))}
		}
	}
	return nil
}

func (c *Command[T]) longOption(t string, known map[string]bool) (string, error) {
	name, optionValue, eq := strings.Cut(t, "=")
	if !known[name] {
		var matches []string
		for f := range known {
			if strings.HasPrefix(f, "--") && strings.HasPrefix(f, name) {
				matches = append(matches, f)
			}
		}
		sort.Slice(matches, func(x, y int) bool { return c.flagOrder(matches[x]) < c.flagOrder(matches[y]) })
		if len(matches) > 1 {
			return "", &UsageError[T]{c, fmt.Sprintf("ambiguous option: %s could match %s", t,
				strings.Join(matches, ", "))}
		}
		if len(matches) == 1 {
			t = matches[0]
			if eq {
				t += "=" + optionValue
			}
		}
	}
	return t, nil
}
