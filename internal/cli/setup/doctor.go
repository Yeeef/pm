package setup

import (
	"errors"
	"fmt"
	"github.com/Yeeef/pm/internal/ask"
	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
	"io"
	"os/exec"
	"strings"
)

// Doctor is pm doctor: every managed piece against what this pm writes, and the clone and worktree against what pm
// init makes; non-zero on any difference.
func Doctor(here string, stdout io.Writer) error {
	c, err := config.Load(here)
	if err != nil {
		return err
	}
	records, err := store.PathOf(here)
	if err != nil {
		return err
	}
	main := store.MainOf(records)
	top, err := codeTop(here, records, "doctor")
	if err != nil {
		return err
	}
	fix := "pm upgrade --to " + buildinfo.Version // the pin: a bare pm upgrade runs the launcher's version
	repo, err := install.Drift(top, install.SettingsOf(c))
	if err != nil {
		return err
	}
	var diffs []string
	for _, d := range repo {
		diffs = append(diffs, fmt.Sprintf("repo: %s; run %s to rewrite it", d, fix))
	}
	if missing := c.AccessMissing(); len(missing) > 0 {
		diffs = append(diffs, fmt.Sprintf("config: site_url is set, but %s lacks %s, so the site refuses every request "+
			"to its host; set what is missing there (pm init --help)", config.Rel, strings.Join(missing, ", ")))
	}
	port, err := service.PortFor(main, int(c.Port))
	if err != nil {
		return err
	}
	setup, err := install.DoctorSetup(top, main, records, c.Remote, port)
	if err != nil {
		return err
	}
	diffs = append(diffs, setup...)
	found, err := install.LegacyRepo(top)
	var ie *install.Error
	if errors.As(err, &ie) { // a file pm cannot read: no legacy piece is known, the file needs a hand fix
		diffs = append(diffs, "repo: cannot look for the pre-package harness's pieces: "+ie.Msg)
	} else if err != nil {
		return err
	}
	for _, d := range append(found, install.LegacyClone(main)...) {
		diffs = append(diffs, "legacy: "+d+"; "+install.LegacyFix())
	}
	// Only the summary's runtime must be here: the pm service asks it every 10 minutes. Each owner-request judge runs on
	// the runtime of the session that stops, which runs only where that runtime is, so neither binary is missing for it.
	rt := ask.Runtime(c.Summary())
	if _, err := exec.LookPath(string(rt)); err != nil {
		diffs = append(diffs, fmt.Sprintf("%s: not on PATH; summary_runtime in %s is %s, so pm day summarize, which "+
			"the pm service runs every 10 minutes, fails; install %s and log in, or set summary_runtime to the runtime "+
			"this machine has", rt, config.Rel, rt, map[ask.Runtime]string{ask.Claude: "Claude Code", ask.Codex: "Codex"}[rt]))
	}
	skips, note, err := install.CodexTrust(main, install.CodexTrustTimeout)
	if err != nil {
		return err
	}
	diffs = append(diffs, skips...)
	sandbox, sandboxNote := install.CodexSandbox(install.CodexSandboxTimeout)
	if sandbox != "" {
		diffs = append(diffs, sandbox)
	}
	notes := append(launch.ReleaseLine(c.Version), install.UnusedPins(main)...) // lines that inform, exit code aside
	for _, n := range []string{sandboxNote, note} {
		if n != "" {
			notes = append([]string{n}, notes...)
		}
	}
	if len(diffs) > 0 {
		fmt.Fprintln(stdout, strings.Join(append(diffs, notes...), "\n"))
		return ExitError{1}
	}
	_, err = fmt.Fprintf(stdout, "pm %s (%s): every managed piece and the clone's setup match what pm init makes\n",
		buildinfo.Version, launch.How())
	if err == nil && len(notes) > 0 {
		_, err = fmt.Fprintln(stdout, strings.Join(notes, "\n"))
	}
	return err
}
