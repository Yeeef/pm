package launch

// A pin move crosses every release after the old pin up to the new one, and each of them may ask a repo for a step of
// its own (a restart, a config key, a re-trust). pm upgrade --to X runs pm X, which carries every earlier release's
// notes (pm.Notes: CHANGELOG.md and changelog.d/ as X was built), so Guides prints each crossed release's upgrade
// guide from them, with no network. X's own guide is its section of CHANGELOG.md when the build has one, else the
// one its entry files assemble into, as release/changelog.py writes it: a release builds before its release commit
// writes the section.

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/Yeeef/pm/internal/config"
)

const guideHeading = "### Upgrade guide"

var (
	releaseHeading = regexp.MustCompile(`^## \[(\d+\.\d+\.\d+)\] - \d{4}-\d{2}-\d{2}$`)
	stepLine       = regexp.MustCompile(`^(\d+)\. \S`)
)

// guide is one release's upgrade guide: its version and the lines of its numbered steps.
type guide struct {
	version string
	lines   []string
}

// Guides is the upgrade guide of each release a pin move from `from` to `to` crosses, oldest first, read from notes
// (pm.Notes); "" when the move crosses none (to is not newer than from, nor another candidate of its release) or to
// is not a version (an untagged build). A
// crossed release whose guide notes do not hold fails hard: a missing step is what this text is for.
func Guides(notes fs.FS, from, to string) (string, error) {
	after := from
	if _, ok := Key(from); !ok {
		after = "0" // a pin that names no version comes before every release: the move may cross each one
	}
	if !Newer(to, after) && !candidates(after, to) {
		return "", nil
	}
	changelog, err := fs.ReadFile(notes, "CHANGELOG.md")
	if err != nil {
		return "", fmt.Errorf("pm %s cannot read the release notes it was built with: %v", to, err)
	}
	base := strings.SplitN(to, "-", 2)[0]
	var crossed []guide
	var own *guide
	for _, g := range released(string(changelog)) {
		switch {
		case g.version == base:
			own = &g
		case Newer(g.version, after) && Newer(base, g.version):
			crossed = append(crossed, g)
		}
	}
	if own == nil {
		g, err := assembled(notes, to)
		if err != nil {
			return "", err
		}
		own = &g
	}
	sort.SliceStable(crossed, func(i, j int) bool { return Newer(crossed[j].version, crossed[i].version) })
	crossed = append(crossed, *own)
	for _, g := range crossed {
		if len(g.lines) == 0 {
			return "", fmt.Errorf("the release notes pm %s was built with give release %s no upgrade guide, so the "+
				"move from %s cannot name its steps; read them at %s/releases/tag/pm-v%s", to, g.version, from,
				config.Repo, g.version)
		}
	}
	n := fmt.Sprintf("%d releases; each one's upgrade guide follows", len(crossed))
	if len(crossed) == 1 {
		n = "1 release; its upgrade guide follows"
	}
	out := []string{fmt.Sprintf("pm %s to %s crosses %s, oldest first, from pm %s's release notes. Do every step "+
		"in order; installing a release and pm upgrade --to it are done once, for %s.", from, to, n, to, to)}
	for _, g := range crossed {
		out = append(out, "", "pm "+g.version+":")
		out = append(out, g.lines...)
	}
	return strings.Join(out, "\n") + "\n\n", nil
}

// candidates is whether a and b are two pre-releases of one release (0.9.0-rc.1 and 0.9.0-rc.2): a move between them
// crosses that release's own guide, which its entries give anew for each candidate.
func candidates(a, b string) bool {
	ka, okA := Key(a)
	kb, okB := Key(b)
	return okA && okB && a != b && !Less(ka, kb) && !Less(kb, ka) && strings.Contains(a, "-") &&
		strings.Contains(b, "-")
}

// released is each release section's upgrade guide in CHANGELOG.md, in the file's order.
func released(changelog string) []guide {
	var out []guide
	in := false
	for _, line := range strings.Split(changelog, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if m := releaseHeading.FindStringSubmatch(line); m != nil {
			out = append(out, guide{version: m[1]})
			in = false
			continue
		}
		switch {
		case len(out) == 0:
		case line == guideHeading:
			in = true
		case strings.HasPrefix(line, "#") || strings.HasPrefix(line, "["): // another category, or the link references
			in = false
		case in:
			out[len(out)-1].lines = append(out[len(out)-1].lines, line)
		}
	}
	for i := range out {
		out[i].lines = trim(out[i].lines)
	}
	return out
}

// assembled is release version's upgrade guide as its entry files give it (release/changelog.py's assemble): the
// two steps every release opens with, then each entry's own steps in file name order, renumbered after them.
func assembled(notes fs.FS, version string) (guide, error) {
	lines := []string{
		"1. On each machine, install the release:",
		fmt.Sprintf("   `curl -fsSL %s/releases/download/pm-v%s/install.sh | sh`.", config.Repo, version),
		fmt.Sprintf("2. In the repo, run `pm upgrade --to %s`, then commit what it changes and merge it, as an "+
			"ordinary PR.", version),
	}
	names, err := fs.Glob(notes, "changelog.d/*.md")
	if err != nil {
		return guide{}, err
	}
	step := 2
	for _, name := range names { // fs.Glob gives them in name order
		data, err := fs.ReadFile(notes, name)
		if err != nil {
			return guide{}, fmt.Errorf("pm %s cannot read its release notes' entry %s: %v", version, name, err)
		}
		var own []string
		in := false
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimRight(line, " \t\r")
			switch {
			case line == guideHeading:
				in = true
			case strings.HasPrefix(line, "#"):
				in = false
			case in:
				if m := stepLine.FindStringSubmatch(line); m != nil {
					step++
					line = fmt.Sprintf("%d%s", step, line[len(m[1]):])
				}
				own = append(own, line)
			}
		}
		lines = append(lines, trim(own)...)
	}
	return guide{version: version, lines: lines}, nil
}

// trim is lines without the blank lines at either end.
func trim(lines []string) []string {
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
