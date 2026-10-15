package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// help is pm <argv> --help's output, failing the test unless it exits 0.
func help(t *testing.T, argv ...string) string {
	t.Helper()
	var out, errb bytes.Buffer
	if code := Execute(append(append([]string{}, argv...), "--help"), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("pm %s --help exits %d: %s", strings.Join(argv, " "), code, errb.String())
	}
	return out.String()
}

// lists is whether a --help text lists the command name among its subcommands.
func lists(text, name string) bool {
	return regexp.MustCompile(`(?m)^    ` + regexp.QuoteMeta(name) + `( |$)`).MatchString(text)
}

// leaf is the tree's command at path ("task add"), or nil.
func leaf(path string) *command {
	c := tree
	for _, name := range strings.Fields(path) {
		var next *command
		for _, s := range c.Subs {
			if s.Name == name {
				next = s
			}
		}
		if next == nil {
			return nil
		}
		c = next
	}
	return c
}

// Every command pm dispatches is in pm --help --all: the tree is the dispatcher, so pm --help --all and each
// pm <noun> --help --all must list every command under them, and pm --help each but the hidden machinery; every work-store command is a leaf of the tree, or one of the two forms that argv
// reaches before the tree, which its tree command's help names; and every agent command's body is a leaf's.
func TestEveryCommandPmRunsIsListedInHelp(t *testing.T) {
	var walk func(c *command, path []string)
	walk = func(c *command, path []string) {
		if c.Subs == nil {
			return
		}
		text, all := help(t, path...), help(t, append(append([]string{}, path...), "--all")...)
		for _, s := range c.Subs {
			if !lists(all, s.Name) {
				t.Errorf("pm %s --help --all does not list %s", strings.Join(path, " "), s.Name)
			}
			if lists(text, s.Name) == s.Hidden { // a hidden one is machinery: only --help --all lists it
				t.Errorf("pm %s --help lists %s: %v, but it is hidden: %v", strings.Join(path, " "), s.Name,
					!s.Hidden, s.Hidden)
			}
			walk(s, append(append([]string{}, path...), s.Name))
		}
	}
	walk(tree, nil)
	for name, sc := range storeCommands {
		// its help is the shared renderer's: the usage line with [-h], each flag with its usage's metavar
		fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
		if sc.flags != nil {
			sc.flags(fs)
		}
		if text := storeHelpCommand(sc, fs).HelpText(); !strings.HasPrefix(text, "usage: pm "+strings.Fields(name)[0]) ||
			!strings.Contains(text, " [-h]") {
			t.Errorf("pm %s --help is not the shared renderer's:\n%s", name, text)
		}
		if c := leaf(name); c != nil {
			if c.Store != sc || c.Subs != nil {
				t.Errorf("pm %s is in the tree but not as its work-store command", name)
			}
			continue
		}
		if storeForms[name] != sc {
			t.Errorf("pm %s is a work-store command that is neither in the tree nor a form", name)
			continue
		}
		words := strings.Fields(name)
		of := leaf(strings.Join(words[:len(words)-1], " ")) // pm show ID's is pm show, pm task add --parent's pm task add
		if of == nil || !strings.Contains(strings.Join(strings.Fields(of.HelpText()), " "), "pm "+name) {
			t.Errorf("the form pm %s is not named in its tree command's --help", name)
		}
	}
	for name := range agentCommands {
		if c := leaf(name); c == nil || c.Subs != nil || c.Store != nil {
			t.Errorf("agent command pm %s is no leaf of the tree", name)
		}
	}
}

// TestAnAbbreviatedOptionIsRefused: an option is named whole; a prefix of one is an unrecognized argument, refused
// before the required option it left out is named.
func TestAnAbbreviatedOptionIsRefused(t *testing.T) {
	t.Chdir(t.TempDir()) // no repo: a command that got past the refusal fails on that, never on a real store
	t.Setenv("CLAUDE_CODE_SESSION_ID", "cl")
	t.Setenv("CODEX_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
	for _, tt := range []struct {
		argv []string
		msg  string
	}{
		{[]string{"task", "add", "--tit", "x"}, "unrecognized arguments: --tit x"},
		{[]string{"task", "add", "--sprint", "s-1", "--tit=x"}, "unrecognized arguments: --tit=x"},
		{[]string{"task", "close", "x-1", "--drop"}, "unrecognized arguments: --drop"},
	} {
		var out, errb bytes.Buffer
		code := Execute(tt.argv, strings.NewReader(""), &out, &errb)
		if want := "pm: error: " + tt.msg + "\n"; code != 2 || !strings.HasPrefix(errb.String(), "usage: pm [-h]") ||
			!strings.HasSuffix(errb.String(), want) {
			t.Errorf("pm %s: exit %d, stderr %q; want exit 2 ending %q", strings.Join(tt.argv, " "), code, errb.String(), want)
		}
	}
}
