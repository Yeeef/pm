package setup

import (
	"fmt"
	"github.com/Yeeef/pm"
	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/install"
	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/store"
	"io"
	"strings"
)

// Upgrade is pm upgrade: print the upgrade guide of each release the move crosses, then move the pin to the running
// pm, rewrite every managed piece as it writes them, take out Beads' pieces and the retired ones, point core.hooksPath
// at pm's hooks and prune the pins no checkout on the machine pins; commits nothing, and names the untracking of a
// records/ copy the branch still tracks. Without --to it never moves a pin down.
func Upgrade(p Arguments, here string, stdout io.Writer) error {
	to := p.Get("to")
	if to == "" {
		to = buildinfo.Version
	}
	if to != buildinfo.Version {
		return fmt.Errorf("pm upgrade --to %s must run pm %s, but pm %s is running; run it as the pm on PATH, which "+
			"launches pm %s: install the latest with %s", to, to, buildinfo.Version, to, latest)
	}
	records, err := store.PathOf(here)
	if err != nil {
		return err
	}
	top, err := codeTop(here, records, "upgrade")
	if err != nil {
		return err
	}
	c, err := config.Read(top)
	if err != nil {
		return err
	}
	have, okHave := launch.Key(c.Version)
	run, okRun := launch.Key(buildinfo.Version)
	if p.Get("to") == "" && okHave && okRun && launch.Less(run, have) {
		return fmt.Errorf("this repo pins pm %s, newer than the running pm %s, and pm upgrade moves a pin down only when "+
			"--to names the version; run pm upgrade --to %s to rewrite pm's pieces at the pin, or install the latest "+
			"pm with %s, then pm upgrade", c.Version, buildinfo.Version, c.Version, latest)
	}
	main := store.MainOf(records)
	if err := install.RefuseLegacy(top, main, true); err != nil {
		return err
	}
	if err := install.CheckHooksPath(top, main); err != nil {
		return err
	}
	// read before any write, so a worktree git cannot read refuses the upgrade before it changes anything
	stranded, err := strandedWorktrees(top, main, records, c.MainBranch)
	if err != nil {
		return err
	}
	guide, err := launch.Guides(pm.Notes, c.Version, buildinfo.Version) // before any write, as the move needs them
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, guide)
	planned, err := install.Rewrite(top, install.SettingsOf(c))
	if err != nil {
		return err
	}
	written, err := install.Write(planned)
	if err != nil {
		return err
	}
	// pm's hook files live in .pm/hooks: a clone Python pm set up points core.hooksPath at Beads' .beads/hooks
	hooksPath, err := install.SetupHooksPath(main)
	if err != nil {
		return err
	}
	// the main branch's records/ copy an earlier pm kept: no branch tracks records/ now
	tracks, err := install.TracksRecords(top)
	if err != nil {
		return err
	}
	stranded = append(stranded, install.PrunePins(main)...) // after the move, which may leave the old pin unused
	return reportUpgrade(stdout, top, c, written, tracks, hooksPath, planned, stranded)
}

// strandedWorktrees is pm upgrade's lines on the clone's other worktrees whose branch pins another pm than this one:
// once the main checkout pins this pm, its pm service runs it, and the version handshake refuses every pm command there
// that reaches the work store. The main checkout, whose pin the service follows, and the records store are not listed;
// nor is a worktree whose branch has no readable pin, where pm refuses every command already.
func strandedWorktrees(top, main, records, mainBranch string) ([]string, error) {
	trees, err := install.Worktrees(main)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, t := range trees {
		if r := fsutil.Resolved(t); r == fsutil.Resolved(top) || r == fsutil.Resolved(main) || r == fsutil.Resolved(records) {
			continue
		}
		c, err := config.Read(t)
		if err != nil || c.Version == buildinfo.Version {
			continue
		}
		branch, err := install.Git(t, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return nil, err
		}
		if branch == "HEAD" {
			branch = "detached"
		} else {
			branch = "branch " + branch
		}
		fix := "merge or rebase it onto the pin move"
		if launch.Newer(c.Version, buildinfo.Version) {
			fix = "it moves the pin further ahead, and pm runs there once that move is on " + mainBranch
		}
		out = append(out, fmt.Sprintf("  %s  %s pins pm %s: %s", t, branch, c.Version, fix))
	}
	if len(out) == 0 {
		return nil, nil
	}
	when := fmt.Sprintf("once the pin move to pm %s is on %s and the main checkout pulls it, the pm service runs it",
		buildinfo.Version, mainBranch)
	if mc, err := config.Read(main); err == nil && mc.Version == buildinfo.Version {
		when = fmt.Sprintf("the main checkout pins pm %s, so the pm service runs it", buildinfo.Version)
	}
	head := when + ", and pm refuses every work-store command in these worktrees, whose branches pin another pm:"
	return append([]string{head}, out...), nil
}

func reportUpgrade(stdout io.Writer, top string, c config.Config, written []string, tracks bool, hooksPath string, planned []install.Planned, stranded []string) error {
	if len(written) == 0 && !tracks {
		if hooksPath != "" {
			fmt.Fprintln(stdout, hooksPath)
		}
		lines := append([]string{fmt.Sprintf("pm %s: every managed piece is current; nothing to commit",
			buildinfo.Version)}, stranded...)
		_, err := fmt.Fprintln(stdout, strings.Join(lines, "\n"))
		return err
	}
	branch, err := install.Git(top, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	moved := "pin stays " + buildinfo.Version
	if c.Version != buildinfo.Version {
		moved = fmt.Sprintf("moved the pin from %s to %s", c.Version, buildinfo.Version)
	}
	lines := append([]string{moved}, install.Said(planned)...)
	if hooksPath != "" {
		lines = append(lines, hooksPath)
	}
	var steps []string
	if len(written) > 0 {
		steps = append(steps, "git add -- "+strings.Join(written, " "))
	}
	if tracks {
		lines = append(lines, "this branch tracks records/, a copy no branch keeps now; the commit below untracks it")
		steps = append(steps, "git rm -r -q --cached --sparse --ignore-unmatch records")
	}
	lines = append(lines, fmt.Sprintf("pm commits nothing on %s; commit pm's files there: %s && git commit -m \"Upgrade "+
		"pm to %s\"", branch, strings.Join(steps, " && "), buildinfo.Version))
	_, err = fmt.Fprintln(stdout, strings.Join(append(lines, stranded...), "\n"))
	return err
}
