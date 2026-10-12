// Package design keeps design pages in step with the code they cover (records.Record.Covers): pm check's coverage of
// the tracked source files, and the gate pm action need --pr and pm sprint close run on the files a sprint's PR
// changes. A page whose covers match a changed file passes when one of the PR's sprints updated it (a records commit
// to the page since the sprint opened, and a link to it in the sprint record's Design pages) or waived it (a sprint
// decision pm design unchanged records).
package design

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// Tracked is the files git tracks in the worktree of each dir (git ls-files), as slash paths relative to its top.
func Tracked(dirs ...string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, d := range dirs {
		list, err := store.Git(d, "ls-files", "--full-name", "--", ":/")
		if err != nil {
			return nil, err
		}
		for _, f := range strings.Split(list, "\n") {
			if f != "" {
				out[f] = true
			}
		}
	}
	return out, nil
}

// Check fails on a covers pattern that matches no tracked file and on a source pattern pm cannot read, naming each;
// otherwise it returns the tracked files a source pattern matches and no design page covers, sorted.
func Check(recs []*records.Record, tracked map[string]bool, sources []string) ([]string, error) {
	var bad, covers []string
	for _, p := range sources {
		if err := records.CheckPattern(p); err != nil {
			bad = append(bad, fmt.Sprintf("design_sources pattern %q %v", p, err))
		}
	}
	for _, r := range recs {
		if r.Type() != "design" {
			continue
		}
		for _, p := range r.Covers() {
			covers = append(covers, p)
			if !matchesAny(p, tracked) {
				bad = append(bad, fmt.Sprintf("%s: covers pattern %q matches no tracked file; fix or remove it "+
					"(a directory's files are dir/**)", r.Rel, p))
			}
		}
	}
	if bad != nil {
		return nil, fmt.Errorf("%s", strings.Join(bad, "\n"))
	}
	var uncovered []string
	for f := range tracked {
		if records.MatchAny(sources, f) && !records.MatchAny(covers, f) {
			uncovered = append(uncovered, f)
		}
	}
	sort.Strings(uncovered)
	return uncovered, nil
}

func matchesAny(pattern string, files map[string]bool) bool {
	for f := range files {
		if records.Match(pattern, f) {
			return true
		}
	}
	return false
}

// waiverRE is the first line of a waiver's decision: the fixed form WaiverText writes and Waivers reads.
var waiverRE = regexp.MustCompile(`^The design page ([a-z0-9-]+) stays as it is for this sprint's code changes\.$`)

// WaiverText is the decision body pm design unchanged records in the sprint record: the fixed line, then the reason.
func WaiverText(slug, reason string) string {
	return "The design page " + slug + " stays as it is for this sprint's code changes.\n" + reason
}

// Waivers is the reason of each waiver among the sprint record's decisions, by page slug.
func Waivers(sprint *records.Record) map[string]string {
	out := map[string]string{}
	for _, d := range records.Decisions(sprint.Text) {
		first, rest, _ := strings.Cut(strings.TrimSpace(d.Body), "\n")
		if m := waiverRE.FindStringSubmatch(first); m != nil {
			out[m[1]] = records.FirstPara(rest)
		}
	}
	return out
}

// Sprint is a sprint a PR delivers: its record and when it opened.
type Sprint struct {
	Record *records.Record
	Opened time.Time
}

// Verdict is how one design page covering a changed file stands.
type Verdict struct {
	Page   *records.Record
	Files  []string // the changed files its covers match
	Edited bool     // one of the sprints committed the page since it opened and links it in its Design pages
	Waiver string   // a sprint's waiver reason; "" when none
}

// OK says whether the page passes the gate.
func (v Verdict) OK() bool { return v.Edited || v.Waiver != "" }

// Covered says whether any design page has covers; with none there is nothing to gate.
func Covered(recs []*records.Record) bool {
	for _, r := range recs {
		if len(r.Covers()) > 0 {
			return true
		}
	}
	return false
}

// Gate is a verdict for each design page whose covers match a file the PR (pr names it) changes, in path order, for
// the sprints it delivers; an error naming each page that fails and the two ways out (Refusal). dir is the records
// store, whose HEAD is the records branch.
func Gate(dir, pr string, sprints []Sprint, recs []*records.Record, changed func() ([]string, error)) ([]Verdict, error) {
	files, err := changed()
	if err != nil {
		return nil, err
	}
	var out []Verdict
	for _, page := range recs {
		if page.Type() != "design" {
			continue
		}
		v := Verdict{Page: page}
		for _, f := range files {
			if records.MatchAny(page.Covers(), f) {
				v.Files = append(v.Files, f)
			}
		}
		if v.Files == nil {
			continue
		}
		for _, s := range sprints {
			if reason, ok := Waivers(s.Record)[page.Name()]; ok && v.Waiver == "" {
				v.Waiver = reason
			}
			if v.Edited || !slices.Contains(records.DesignLinks(s.Record), page.Name()) {
				continue
			}
			log, err := store.Git(dir, "log", "-1", "--format=%H", "--since="+s.Opened.UTC().Format(time.RFC3339),
				"HEAD", "--", page.Rel+".md")
			if err != nil {
				return nil, err
			}
			v.Edited = log != ""
		}
		out = append(out, v)
	}
	if why := refusal(pr, sprints, out); why != "" {
		return nil, errors.New(why)
	}
	return out, nil
}

// refusal is why the gate refuses the PR, naming each page that fails and the two ways out; "" when every page passes.
func refusal(pr string, sprints []Sprint, verdicts []Verdict) string {
	var pages []string
	for _, v := range verdicts {
		if v.OK() {
			continue
		}
		files := v.Files[0]
		if len(v.Files) > 1 {
			files += fmt.Sprintf(" and %d more", len(v.Files)-1)
		}
		pages = append(pages, fmt.Sprintf("%s (covers %s)", v.Page.Rel+".md", files))
	}
	if pages == nil {
		return ""
	}
	ids := make([]string, len(sprints))
	for i, s := range sprints {
		ids[i] = s.Record.Bead()
	}
	return fmt.Sprintf("%s changes code that design pages cover, and sprint %s neither updated nor waived them: %s. "+
		"For each page, bring it to the new state, commit it with pm commit and link it in the sprint record's Design "+
		"pages section, or record why it stays as it is: pm design unchanged <slug> --sprint %s --reason \"…\"",
		pr, strings.Join(ids, ", "), strings.Join(pages, ", "), ids[0])
}

// PRFiles is the files the PR changes, as gh lists them, for Gate.
func PRFiles(root, pr string) func() ([]string, error) {
	return func() ([]string, error) {
		var info struct {
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
			ChangedFiles int `json:"changedFiles"`
		}
		if err := proc.GHJSON(root, &info, "pr", "view", pr, "--json", "files,changedFiles"); err != nil {
			return nil, fmt.Errorf("the design-page check needs the files %s changes: %v", pr, err)
		}
		if len(info.Files) != info.ChangedFiles {
			return nil, fmt.Errorf("gh lists %d of the %d files %s changes, so the design-page check cannot see them all",
				len(info.Files), info.ChangedFiles, pr)
		}
		out := make([]string, len(info.Files))
		for i, f := range info.Files {
			out[i] = f.Path
		}
		return out, nil
	}
}

// MergeFiles is the files the merge commit sha brought to its first parent, for Gate.
func MergeFiles(root, sha string) func() ([]string, error) {
	return func() ([]string, error) {
		out, err := store.Git(root, "diff", "--name-only", "--no-renames", sha+"^1", sha)
		return strings.Fields(out), err
	}
}

// Covering is a PR review's paragraph on the pages covering the changed code, each linked under the site's URL, and
// updated or why not.
func Covering(site string, verdicts []Verdict) string {
	lines := []string{"Design pages covering the changed code:"}
	for _, v := range verdicts {
		state := "updated in the sprint"
		if !v.Edited {
			state = "unchanged: " + v.Waiver
		}
		lines = append(lines, fmt.Sprintf("- [%s](%s/%s): %s", v.Page.Title(), site, v.Page.Out(), state))
	}
	return strings.Join(lines, "\n")
}
