// Package cli is pm's command tree and the dispatch to each command's body, after the argument, session and config
// checks every command passes first. commands.go holds the tree, every command with its help text; the parse package
// builds it on cobra, and each leaf parses its own arguments with pflag (cobra's flag parsing is off on leaves):
// required arguments, mutually exclusive groups, choices, positional counts, and --option LABEL TEXT. Each noun's body
// has its own file (task.go, sprint.go, decision.go, …), the setup package holds pm init, doctor, upgrade and
// uninstall, the clean package pm clean, needform a decision need's parts, rules and layout, and fsutil the path
// helpers cli and setup share. store_commands.go holds the work-store commands' bodies, and the forms show ID and task
// add --task TASK, the one argv Execute routes before the tree (storeFormOf); their --help goes through the tree's
// renderer (storeHelpCommand). Every command pm runs is in the tree, so in pm --help --all (pm --help leaves out the
// Hidden machinery): commands_test.go fails otherwise.
// help_golden_test.go holds every command's --help to testdata/help.golden, and doc_test.go fails on a func whose doc
// comment does not start with its name.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/cli/clean"
	"github.com/Yeeef/pm/internal/cli/parse"
	"github.com/Yeeef/pm/internal/cli/setup"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/hooks"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/session"
)

type command = parse.Command[*storeCommand]
type arg = parse.Arg
type Parsed = parse.Parsed[*storeCommand]
type usageError = parse.UsageError[*storeCommand]

const (
	value, switchOn, switchTo, repeated = parse.Value, parse.SwitchOn, parse.SwitchTo, parse.Repeated
	optional, many, two                 = parse.Optional, parse.Many, parse.Two
	pairSep                             = parse.PairSep
)

// Nouns lists the commands pm --help lists, in its order: the machinery (Hidden) left out, as pm prime's noun list
// leaves it out.
func Nouns() []string {
	var out []string
	for _, s := range tree.Subs {
		if !s.Hidden {
			out = append(out, s.Name)
		}
	}
	return out
}

// refusal is a command's own error: "error: <message>" on stderr, exit 1.
type refusal struct{ msg string }

func (r *refusal) Error() string { return r.msg }

// removedForms are the forms the naming grammar (the pm-cli design page, Naming) removed.
var removedForms = []parse.Removed{
	{Path: "decision need", Flag: "--parent", Instead: "name the sprint with --sprint ID or the task with --task ID"},
	{Path: "action need", Flag: "--parent", Instead: "name the sprint with --sprint ID or the task with --task ID"},
	{Path: "decision add", Flag: "--level", Instead: "--project NAME or --sprint ID alone names the decision's level"},
	{Path: "task add", Flag: "--parent", Instead: "a sub-task names its task with --task TASK"},
	{Path: "decision close", Instead: "pm need close ID --reason \"…\", the answer as --text or --text-file"},
	{Path: "action done", Instead: "pm need close ID --reason \"…\" (a PR review: --reason \"merged as <sha>\")"},
	{Path: "need dismiss", Instead: "pm need close ID --dropped --reason \"…\" closes a need that became moot"},
}

// Execute runs pm with argv (without the program name) and returns the exit code.
func Execute(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if msg := parse.RefuseRemoved(argv, removedForms); msg != "" {
		fmt.Fprintf(stderr, "error: %s\n", msg)
		return 2
	}
	// every command that may act as a session, the self-parsing ones included, refuses an ambiguous one first; a hook
	// reads its session from its input (and fails open), and help and version act as none
	if len(argv) > 0 && argv[0] != "hook" && argv[0] != "prime" && argv[0] != "version" &&
		!slices.Contains(argv, "-h") && !slices.Contains(argv, "--help") {
		id, runtime, err := agentSession()
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if here, err := cwd(); err == nil && runtime == "codex" {
			session.WarnCodexHooksSkipped(here, id, stderr)
		}
	}
	if name, args, ok := storeFormOf(argv); ok {
		if err := runStoreCommand(name, args, openStore, stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		return 0
	}
	run := func(p *Parsed) error { return dispatch(p, stdin, stdout, stderr) }
	runStore := func(name string, args []string) error { return runStoreCommand(name, args, openStore, stdin, stdout) }
	root := parse.Build(tree, run, runStore, stdout)
	root.SetHelpCommand(&cobra.Command{Use: "no-help-command", Hidden: true})
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetArgs(argv)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	var ue *usageError
	var ec *exitCode
	var se setup.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ec):
		return ec.code
	case errors.As(err, &se):
		return se.Code
	case errors.As(err, &ue):
		fmt.Fprintf(stderr, "usage: %s\n%s: error: %s\n", ue.Cmd.Usage(), ue.Cmd.Prog(), ue.Msg)
		return 2
	default:
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
}

// cwd returns the kernel's working directory, independent of $PWD.
func cwd() (string, error) { return syscall.Getwd() }

// dispatch checks configuration before running a parsed command's body.
func dispatch(p *Parsed, stdin io.Reader, stdout, stderr io.Writer) error {
	here, err := cwd()
	if err != nil {
		return err
	}
	name := p.Cmd.Path()
	if err := checkConfig(name, here); err != nil {
		return err
	}
	switch name {
	case "prime":
		return runPrime(p, stdin, stdout)
	case "hook stop":
		return hooks.HookStop(stdin, stdout, stderr)
	case "hook ask":
		return hooks.HookAsk(here, ask.Runtime(p.Get("runtime")), hooks.DialStore, stdin, stdout, stderr)
	case "hook session-end":
		return hooks.HookSessionEnd(here, stdin, stderr)
	case "hook owner-request":
		return runOwnerHook(p, here, stdin, stdout, stderr)
	case "service install", "service status", "service restart", "service stop", "service logs", "service run":
		return runService(strings.TrimPrefix(name, "service "), p, here, stdout, stderr)
	case "push":
		cfg, err := config.Load(here)
		if err != nil {
			return err
		}
		return cmdPush(cfg, here, stdout)
	case "init":
		return runInit(p, here, stdout)
	case "version": // this build's version, outside any repo too; dev when untagged
		_, err := fmt.Fprintln(stdout, buildinfo.Version)
		return err
	case "export":
		if dir, ok := p.Values["store"]; ok {
			return exportStore(dir[0], stdout)
		}
		return cmdExport(stdout)
	case "doctor":
		return setup.Doctor(here, stdout)
	case "upgrade":
		return setup.Upgrade(p, here, stdout)
	case "uninstall":
		return setup.Uninstall(here, stdout)
	case "check":
		return cmdCheck(here, stdout)
	case "where":
		return cmdWhere(p, here, stdout)
	case "clean":
		return clean.Run(here, p.Get("apply") == "true", stdout)
	case "interactive", "autopilot":
		return cmdMode(here, name == "interactive", stdout)
	}
	if cmd, ok := agentCommands[name]; ok {
		return runAgent(name, cmd, p, here, stdin, stdout, stderr)
	}
	panic(fmt.Sprintf("pm command %s has no handler", name))
}

func runPrime(p *Parsed, stdin io.Reader, stdout io.Writer) error {
	part := hooks.Part{}
	part.RequestRule = p.Get("part") == "request-rule"
	switch v := p.Get("part"); v {
	case "", "request-rule":
	case "state":
		part.State = true
	case "subagent":
		part.Subagent = true
	default:
		var err error
		part.Chunk, err = strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid rules chunk %s: %w", v, err)
		}
	}
	if session.Innermost() == "codex" { // the state hook of a Codex session: whether Codex skips any of pm's hooks
		part.Trust = install.SessionTrust
	}
	return hooks.CmdPrime(part, p.Get("hook_json") == "true", Nouns(), stdin, stdout)
}

func runOwnerHook(p *Parsed, here string, stdin io.Reader, stdout, stderr io.Writer) error {
	runtime := ask.Runtime(p.Get("runtime")) // parse checked it is one of ask.Runtimes
	if runtime == "" {
		runtime = ask.Claude // an entry written before --runtime existed ran the claude judge
	}
	if code, err := hooks.HookOwnerRequest(here, runtime, hooks.OpenNeeds, stdin, stdout, stderr); err != nil || code == 0 {
		return err
	} else {
		return &exitCode{code}
	}
}

// cmdMode is pm interactive (on) and pm autopilot, for this command's session (hooks.SetMode).
func cmdMode(here string, on bool, stdout io.Writer) error {
	id := currentSession()
	if id == "" {
		return refuse("no agent session: neither %s nor %s is set, so there is no session to set the mode of",
			sessionEnv, codexSessionEnv)
	}
	text, err := hooks.SetMode(here, id, on)
	if err != nil {
		return refuse("the mode of session %s is unchanged: %v", id, err)
	}
	_, err = fmt.Fprintln(stdout, text)
	return err
}

func runInit(p *Parsed, here string, stdout io.Writer) error {
	if imp := p.Given("import_bd", "import"); imp != nil { // the import alone, into the work store
		if other := p.Given("session_start", "site_url"); other != nil {
			return &usageError{Cmd: p.Cmd, Msg: fmt.Sprintf("argument %s: not allowed with argument %s", imp.Name(), other.Name())}
		}
		if imp.Dest == "import_bd" {
			return cmdImportBD(p.Get("import_bd"), stdout)
		}
		return cmdImport(p.Get("import"), stdout)
	}
	return setup.Init(p, here, stdout)
}

func checkConfig(name, here string) error {
	check := name != "upgrade" && name != "version" && name != "export" // pm export checks it, but not with --store
	if name == "init" {                                                 // pm init writes a missing config
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
	return nil
}
