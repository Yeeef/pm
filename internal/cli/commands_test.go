package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
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
		for _, s := range c.subs {
			if s.name == name {
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

// Every command pm dispatches is in pm --help: the tree is the dispatcher, so pm --help and each pm <noun> --help must
// list every command under them; every work-store command is a leaf of the tree, or one of the two forms that argv
// reaches before the tree, which its tree command's help names; and every agent command's body is a leaf's.
func TestEveryCommandPmRunsIsListedInHelp(t *testing.T) {
	var walk func(c *command, path []string)
	walk = func(c *command, path []string) {
		if c.subs == nil {
			return
		}
		text := help(t, path...)
		for _, s := range c.subs {
			if !lists(text, s.name) {
				t.Errorf("pm %s --help does not list %s", strings.Join(path, " "), s.name)
			}
			walk(s, append(append([]string{}, path...), s.name))
		}
	}
	walk(tree, nil)
	for name, sc := range storeCommands {
		if c := leaf(name); c != nil {
			if c.store != sc || c.subs != nil {
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
		if of == nil || !strings.Contains(collapse(of.helpText()), "pm "+name) {
			t.Errorf("the form pm %s is not named in its tree command's --help", name)
		}
	}
	for name := range agentCommands {
		if c := leaf(name); c == nil || c.subs != nil || c.store != nil {
			t.Errorf("agent command pm %s is no leaf of the tree", name)
		}
	}
}

// The commands pm ran outside the tree until pm-quality sprint 7, so that pm --help listed none of them (pm dep
// answered "invalid choice: 'dep'"), each in its noun's --help now, its own --help exiting 0.
func TestTheCommandsOnceOutsideTheTreeAreInIt(t *testing.T) {
	for _, path := range []string{"task ready", "task edit", "task release", "dep add", "dep rm", "comment add",
		"need dismiss", "reply add", "sync", "version", "export"} {
		words := strings.Fields(path)
		if !lists(help(t, words[:len(words)-1]...), words[len(words)-1]) {
			t.Errorf("pm %s is not listed in its noun's --help", path)
		}
		help(t, words...)
	}
	for _, text := range []struct{ argv, want string }{{"show", "pm show ID"}, {"task add", "pm task add --parent"},
		{"init", "--import-bd FILE"}, {"init", "--import FILE"}, {"export", "--store DIR"}} {
		if got := collapse(help(t, strings.Fields(text.argv)...)); !strings.Contains(got, text.want) {
			t.Errorf("pm %s --help does not name %s", text.argv, text.want)
		}
	}
}
