// Package design keeps design pages in step with the code they cover (records.Record.Covers): pm check's coverage of
// the tracked source files and its hold of each page's covers to the code, and the gate pm action need --pr and pm
// sprint close run on the files a sprint's PR changes. A page whose covers match a changed file passes when one of the
// PR's sprints updated it (its draft of the page, or a records commit to the page since the sprint opened, and a link
// to it in the sprint record's Design pages) or waived it (a sprint decision pm design unchanged or pm sprint close
// --discard records).
//
// A design page publishes at its sprint's close, once its PRs are on main (the record-drafts page), so every covers
// pattern of a published page must match a file on the remote's main (MissingOnMain): pm check holds them there, and
// pm sprint close holds every pattern of a page it publishes. A draft page names the files of its own
// worktree's branch, which pm check holds it to in that worktree.
//
// The gate reads a PR's changed files with gh api --paginate repos/<o>/<r>/pulls/<n>/files, a rename's old path too.
package design

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/item"
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
// the tracked files a source pattern matches and no design page covers, sorted.
func Check(recs []*records.Record, tracked map[string]bool, sources []string) (uncovered []string, err error) {
	var bad, covers []string
	for _, p := range sources {
		if err := records.CheckPattern(p); err != nil {
			bad = append(bad, fmt.Sprintf("design_sources pattern %q %v", p, err))
		} else if !matchesAny(p, tracked) {
			bad = append(bad, fmt.Sprintf("design_sources pattern %q matches no tracked file; fix or remove it", p))
		}
	}
	if bad != nil {
		return nil, fmt.Errorf("%s", strings.Join(bad, "\n"))
	}
	for _, r := range recs {
		if r.Type() == "design" {
			covers = append(covers, r.Covers()...)
		}
	}
	for f := range tracked {
		if records.MatchAny(sources, f) && !records.MatchAny(covers, f) {
			uncovered = append(uncovered, f)
		}
	}
	sort.Strings(uncovered)
	return uncovered, nil
}

// Report is what pm check prints after its render line, from Check: the tracked source files no page covers (the
// design_sources key of the config file at rel), or that a page covers each.
func Report(recs []*records.Record, tracked map[string]bool, sources []string, rel string) (string, error) {
	uncovered, err := Check(recs, tracked, sources)
	if err != nil {
		return "", err
	}
	switch {
	case uncovered != nil:
		return fmt.Sprintf("; %d tracked source files (design_sources in %s) no design page covers; add each to the "+
			"covers: of the page about its area:\n  %s", len(uncovered), rel, strings.Join(uncovered, "\n  ")), nil
	case sources != nil:
		return "; a design page covers every tracked source file", nil
	}
	return "", nil
}

// Missing is each of patterns (by page, as its store path without .md: design/x) that matches none of files, as
// `<page>.md: covers pattern "<pattern>"`, in page then pattern order.
func Missing(patterns map[string][]string, files map[string]bool) []string {
	var out []string
	for _, page := range slices.Sorted(maps.Keys(patterns)) {
		for _, p := range patterns[page] {
			if !matchesAny(p, files) {
				out = append(out, fmt.Sprintf("%s.md: covers pattern %q", page, p))
			}
		}
	}
	return out
}

// MissingOnMain is Missing against the files of the remote's main, read in the checkout root from its remote-tracking
// ref, which is fetched first when it is missing or when a pattern matches nothing there, as it may be behind. With no
// pattern, nothing is read.
func MissingOnMain(root, remote, branch string, patterns map[string][]string) ([]string, error) {
	n := 0
	for _, list := range patterns {
		n += len(list)
	}
	if n == 0 {
		return nil, nil
	}
	main := remote + "/" + branch
	var missing []string
	if files, err := MainFiles(root, "refs/remotes/"+main); err == nil {
		if missing = Missing(patterns, files); missing == nil {
			return nil, nil
		}
	}
	ref, err := store.FetchMain(root, remote, branch)
	if err != nil {
		return nil, fmt.Errorf("pm cannot see %s, so it cannot check design pages' covers against it: %v", main, err)
	}
	files, err := MainFiles(root, ref)
	if err != nil {
		return nil, err
	}
	return Missing(patterns, files), nil
}

// Hold fails on a covers pattern of recs that matches no file where its page holds it: a draft page's (drafts, by
// store path: design/x.md) in the files the worktree root tracks, every other page's on the remote's main
// (MissingOnMain). A published page's pattern main lacks while a draft of the page in the clone (dir is the records
// store) no longer holds it is not a failure but waits on that sprint's close, which publishes the new covers: Hold
// gives those as pm check's waiting lines, and a warning for each drafts/ branch it cannot read. pm check runs it.
func Hold(recs []*records.Record, drafts map[string]bool, dir, root, remote, branch string) (string, error) {
	onMain, here := map[string][]string{}, map[string][]string{}
	for _, r := range recs {
		if r.Type() != "design" || len(r.Covers()) == 0 {
			continue
		}
		if drafts[r.Rel+".md"] {
			here[r.Rel] = r.Covers()
		} else {
			onMain[r.Rel] = r.Covers()
		}
	}
	var why []string
	if len(here) > 0 {
		files, err := Tracked(root)
		if err != nil {
			return "", err
		}
		if missing := Missing(here, files); missing != nil {
			why = append(why, fmt.Sprintf("covers patterns of this workspace's draft pages match no file this worktree "+
				"tracks: %s; fix each to a file of this branch, or git add the file", strings.Join(missing, ", ")))
		}
	}
	missing, err := MissingOnMain(root, remote, branch, onMain)
	if err != nil {
		return "", err
	}
	var pending func(page, pattern string) []string
	var warnings []string
	if missing != nil { // the drafts are read only for a pattern main lacks
		if pending, warnings, err = pendingCovers(dir); err != nil {
			return "", err
		}
	}
	var dead, waiting []string
	for _, m := range missing {
		page, quoted, _ := strings.Cut(m, ".md: covers pattern ")
		pattern, err := strconv.Unquote(quoted)
		if err != nil {
			return "", fmt.Errorf("design.Missing gave %q", m)
		}
		if sprints := pending(page, pattern); sprints != nil {
			waiting = append(waiting, fmt.Sprintf("%s (sprint %s)", m, strings.Join(sprints, ", ")))
		} else {
			dead = append(dead, m)
		}
	}
	if dead != nil {
		why = append(why, fmt.Sprintf("covers patterns of published design pages match no file on %s/%s: %s; a page "+
			"publishes at its sprint's close, once its code is on main: fix each pattern to a file main has, in a "+
			"worktree's records workspace, with the page linked in a sprint's Design pages, and pm commit it there",
			remote, branch, strings.Join(dead, ", ")))
	}
	if why != nil {
		return "", errors.New(strings.Join(append(why, warnings...), "\n"))
	}
	said := ""
	for _, w := range warnings {
		said += "\n" + w
	}
	if waiting == nil {
		return said, nil
	}
	return said + fmt.Sprintf("\n%d covers patterns of published pages match no file on %s/%s, waiting on the close of the "+
		"sprint whose draft of the page drops the pattern:\n  %s", len(waiting), remote, branch,
		strings.Join(waiting, "\n  ")), nil
}

// pendingCovers says, of a published page's pattern main lacks, the sprints whose drafts of the page in the clone
// (dir is the records store) no longer hold that pattern, or whose draft header does not parse: the pattern waits on
// their close. Each drafts/ branch pm cannot read is left out, with a warning naming it and its fix.
func pendingCovers(dir string) (func(page, pattern string) []string, []string, error) {
	drafts, warnings, err := store.AllDrafts(dir)
	if err != nil {
		return nil, nil, err
	}
	return func(page, pattern string) []string {
		var sprints []string
		for _, b := range drafts {
			path := page + ".md"
			text, ok := b.Pages[path]
			if !ok {
				continue
			}
			if text != nil {
				if covers, err := records.CoversOf(*text, path); err == nil && slices.Contains(covers, pattern) {
					continue
				}
			}
			if !slices.Contains(sprints, b.Tags[path]) {
				sprints = append(sprints, b.Tags[path])
			}
		}
		return sprints
	}, warnings, nil
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

// RenameCheck is pm design rename's check of a rename of the page slug to the slug to ("" when only its title
// changes): it refuses while an open PR review names the page, since the work store holds the review's targets, which a
// rename does not rewrite; else it gives the rewrite of the page's waivers (RenameWaivers), or nil.
func RenameCheck(items []item.Item, slug, to string) (func(string) string, error) {
	for _, it := range items {
		if r := it.Need; it.Status != item.Closed && r != nil && r.Review != nil && slices.Contains(r.Review.Designs, slug) {
			return nil, fmt.Errorf("the open PR review %s names design page %s (--design); close it with pm action "+
				"done, or dismiss it, before the page is renamed", it.ID, slug)
		}
	}
	if to == "" {
		return nil, nil
	}
	return func(text string) string { return RenameWaivers(text, slug, to) }, nil
}

// RenameWaivers is a record's text with each waiver of the design page from waiving the page to instead, as pm design
// rename moves it: the fixed first line, which Waivers reads, rewritten in place.
func RenameWaivers(text, from, to string) string {
	line := regexp.MustCompile(`(?m)^([ \t]*)The design page ` + regexp.QuoteMeta(from) +
		` stays as it is for this sprint's code changes\.$`)
	return line.ReplaceAllString(text, "${1}The design page "+to+" stays as it is for this sprint's code changes.")
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

// Sprint is a sprint a PR delivers: its record, when it opened, and the pages the clone's drafts hold for it, by store
// path (design/x.md).
type Sprint struct {
	Record  *records.Record
	Opened  time.Time
	Drafted map[string]bool
}

// Verdict is how one design page covering a changed file stands.
type Verdict struct {
	Page   *records.Record
	Files  []string // the changed files its covers match
	Edited bool     // one of the sprints drafted or committed the page since it opened and links it in its Design pages
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
			if s.Drafted[page.Rel+".md"] {
				v.Edited = true
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
