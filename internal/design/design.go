// Package design keeps design pages in step with the code they cover (records.Record.Covers): pm check's coverage of
// the tracked source files, and the gate pm action need --pr and pm sprint close run on the files a sprint's PR
// changes. A page whose covers match a changed file passes when one of the PR's sprints updated it (a records commit
// to the page since the sprint opened, and a link to it in the sprint record's Design pages) or waived it (a sprint
// decision pm design unchanged records).
//
// The gate reads a PR's changed files with gh api --paginate repos/<o>/<r>/pulls/<n>/files, a rename's old path too.
package design

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
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
		for _, f := range lines(list) {
			out[f] = true
		}
	}
	return out, nil
}

// Check fails on a source pattern that pm cannot read or that matches no tracked file, naming each; otherwise it returns
// the tracked files a source pattern matches and no design page covers, sorted, and each covers pattern that matches no
// tracked file (Dead). Such a pattern waits on unmerged work: the records store is shared across branches, and the
// gate makes a sprint bring its page to the new state before its PR merges, so a page names a file before main has
// it. pm sprint close holds the patterns a sprint added to main instead (LiveOnMain).
func Check(recs []*records.Record, tracked map[string]bool, sources []string) (uncovered, waiting []string, err error) {
	var bad, covers []string
	for _, p := range sources {
		if err := records.CheckPattern(p); err != nil {
			bad = append(bad, fmt.Sprintf("design_sources pattern %q %v", p, err))
		} else if !matchesAny(p, tracked) {
			bad = append(bad, fmt.Sprintf("design_sources pattern %q matches no tracked file; fix or remove it", p))
		}
	}
	if bad != nil {
		return nil, nil, fmt.Errorf("%s", strings.Join(bad, "\n"))
	}
	var pages []*records.Record
	for _, r := range recs {
		if r.Type() == "design" {
			pages = append(pages, r)
			covers = append(covers, r.Covers()...)
		}
	}
	for f := range tracked {
		if records.MatchAny(sources, f) && !records.MatchAny(covers, f) {
			uncovered = append(uncovered, f)
		}
	}
	sort.Strings(uncovered)
	return uncovered, Dead(pages, tracked, (*records.Record).Covers), nil
}

// Report is what pm check prints after its render line, from Check: the tracked source files no page covers (the
// design_sources key of the config file at rel) or that a page covers each, then the covers patterns that wait on
// unmerged work.
func Report(recs []*records.Record, tracked map[string]bool, sources []string, rel string) (string, error) {
	uncovered, waiting, err := Check(recs, tracked, sources)
	if err != nil {
		return "", err
	}
	var out string
	switch {
	case uncovered != nil:
		out = fmt.Sprintf("; %d tracked source files (design_sources in %s) no design page covers; add each to the "+
			"covers: of the page about its area:\n  %s", len(uncovered), rel, strings.Join(uncovered, "\n  "))
	case sources != nil:
		out = "; a design page covers every tracked source file"
	}
	if waiting != nil {
		out += fmt.Sprintf("\n%d covers patterns match no tracked file, waiting on unmerged work; pm sprint close "+
			"refuses a sprint only on a pattern it added or changed while open that its merged PR leaves missing on "+
			"the remote's main:\n  %s",
			len(waiting), strings.Join(waiting, "\n  "))
	}
	return out, nil
}

// Dead is each pattern of the pages (covers gives a page's patterns) that matches none of files, as `<page>.md: covers
// pattern "<pattern>"`, in page then pattern order.
func Dead(pages []*records.Record, files map[string]bool, covers func(*records.Record) []string) []string {
	var out []string
	for _, r := range pages {
		for _, p := range covers(r) {
			if !matchesAny(p, files) {
				out = append(out, fmt.Sprintf("%s.md: covers pattern %q", r.Rel, p))
			}
		}
	}
	return out
}

// Own is the covers patterns of the committed page that the sprint opened at opened added or changed: those its covers
// did not hold at the newest records commit (dir is the store) authored before the sprint opened. Author dates, as a
// rebase of the records branch onto the remote keeps them; a clock skewed between clones can move the base. With no
// commit before then, or no page in it, every pattern is the sprint's. A records commit names no sprint, so another
// session's commit to the page while the sprint is open counts as the sprint's. The old page is read for its covers
// only, as a later pm's rules may refuse it.
func Own(dir string, page *records.Record, opened time.Time) ([]string, error) {
	log, err := store.Git(dir, "log", "--format=%H %at", "HEAD")
	if err != nil {
		return nil, err
	}
	base := ""
	for _, l := range lines(log) {
		sha, at, _ := strings.Cut(l, " ")
		if n, err := strconv.ParseInt(at, 10, 64); err != nil {
			return nil, fmt.Errorf("git log gave %q, not a sha and an author time", l)
		} else if n < opened.Unix() {
			base = sha
			break
		}
	}
	var before []string
	if base != "" {
		if there, err := store.Git(dir, "ls-tree", "--name-only", base, "--", page.Rel+".md"); err != nil {
			return nil, err
		} else if there != "" {
			text, err := store.Git(dir, "show", base+":"+page.Rel+".md")
			if err != nil {
				return nil, err
			}
			if before, err = records.CoversOf(text+"\n", page.Rel); err != nil {
				return nil, fmt.Errorf("the page as it was before the sprint opened (records commit %s): %w", base[:7], err)
			}
		}
	}
	return slices.DeleteFunc(page.Covers(), func(p string) bool { return slices.Contains(before, p) }), nil
}

// Held is the design pages with covers that pm sprint close holds to the remote's main: those the sprint record's
// Design pages list, then those the gate's verdicts for its PRs name, each once.
func Held(sprint *records.Record, recs []*records.Record, verdicts []Verdict) []*records.Record {
	var out []*records.Record
	for _, page := range recs {
		if page.Type() == "design" && slices.Contains(records.DesignLinks(sprint), page.Name()) {
			out = append(out, page)
		}
	}
	for _, v := range verdicts {
		if !slices.Contains(out, v.Page) {
			out = append(out, v.Page)
		}
	}
	return slices.DeleteFunc(out, func(p *records.Record) bool { return len(p.Covers()) == 0 })
}

// LiveOnMain fetches the remote's main in the checkout root and fails while one of the pages has a covers pattern the
// sprint added or changed (Own; dir is the records store) that matches no file there, naming page, pattern and the
// fix: a page may name a file before main has it only while the PR that adds it is open, and the sprint closes once
// its PR is merged. A pattern on the page before the sprint opened is another sprint's to keep live, and is not held.
// With no pattern of the sprint's, nothing is fetched.
func LiveOnMain(root, remote, branch, dir string, s Sprint, pages []*records.Record) error {
	id, own, n := s.Record.Bead(), map[*records.Record][]string{}, 0
	for _, p := range pages {
		patterns, err := Own(dir, p, s.Opened)
		if err != nil {
			return err
		}
		own[p], n = patterns, n+len(patterns)
	}
	if n == 0 {
		return nil
	}
	main := remote + "/" + branch
	ref, err := store.FetchMain(root, remote, branch)
	if err != nil {
		return fmt.Errorf("sprint %s: pm cannot see %s, so it cannot check its design pages' covers against it: %v",
			id, main, err)
	}
	files, err := MainFiles(root, ref)
	if err != nil {
		return err
	}
	if dead := Dead(pages, files, func(p *records.Record) []string { return own[p] }); dead != nil {
		return fmt.Errorf("sprint %s: covers patterns it added or changed since it opened name files %s does not have: "+
			"%s. Fix each pattern to the file main has and commit the page with pm commit, or merge the PR that adds the "+
			"file first, which may be another sprint's that edited the page while this one was open; then run pm sprint "+
			"close %s again", id, main, strings.Join(dead, ", "), id)
	}
	return nil
}

// MainFiles is the files of the tree at ref (the remote's main, fetched), as slash paths from the repo's top.
func MainFiles(root, ref string) (map[string]bool, error) {
	list, err := store.Git(root, "ls-tree", "-r", "--name-only", "--full-tree", ref)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, f := range lines(list) {
		out[f] = true
	}
	return out, nil
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
var waiverRE = regexp.MustCompile(`^The design page (\S+) stays as it is for this sprint's code changes\.$`)

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

var pullURL = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/pull/([0-9]+)$`)

// PRFiles is the files the PR changes, as GitHub's pull request files API lists them (every page of it): each file's
// path, and a renamed file's old path too, for Gate.
func PRFiles(root, pr string) func() ([]string, error) {
	return func() ([]string, error) {
		m := pullURL.FindStringSubmatch(pr)
		if m == nil {
			return nil, fmt.Errorf("the design-page check reads the files a PR changes from GitHub, and %s is no "+
				"GitHub pull request URL (https://github.com/OWNER/REPO/pull/N)", pr)
		}
		out, err := proc.GH(root, "api", "--paginate", "repos/"+m[1]+"/pulls/"+m[2]+"/files", "--jq",
			".[] | .filename, (.previous_filename // empty)")
		if err != nil {
			return nil, fmt.Errorf("the design-page check needs the files %s changes: %v", pr, err)
		}
		return lines(out), nil
	}
}

// MergeFiles is the files the merge commit sha brought, against its first parent, for Gate. Only a merge commit tells
// where a delivery starts: a squash merge, a rebase merge's last commit and a direct commit each have one parent and
// look alike, and a rebase merge's earlier commits would go unseen, so a commit with one parent is refused.
func MergeFiles(root, sha string) func() ([]string, error) {
	return func() ([]string, error) {
		parents, err := store.Git(root, "rev-list", "--parents", "-n", "1", sha)
		if err != nil {
			return nil, err
		}
		if len(strings.Fields(parents)) < 3 {
			return nil, fmt.Errorf("--merged %s is not a merge commit, so the design-page check cannot tell which "+
				"commits the delivery holds (a squash or a rebase merge leaves one commit per change); give the PR "+
				"with --pr URL, whose files GitHub lists", sha[:7])
		}
		out, err := store.Git(root, "diff", "--name-only", "--no-renames", sha+"^1", sha)
		return lines(out), err
	}
}

// lines is out's non-empty lines.
func lines(out string) []string {
	var files []string
	for _, f := range strings.Split(out, "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
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
