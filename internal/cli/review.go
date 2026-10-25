package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/proc"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
	"slices"
	"strconv"
	"strings"
)

// dedupe is list with each value once, in first-seen order.
func dedupe(list []string) []string {
	var out []string
	for _, v := range list {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// raiseReview raises a PR review: a need of kind review carrying what the owner reads first, the PR, its sprints, the
// design pages behind it and the focus, so the site links each to its page.
func raiseReview(e *env, p *Parsed) (string, error) {
	pr := strip(p.Get("pr"))
	if !urlRE.MatchString(pr) {
		return "", refuse("--pr %s is not a URL; give the pull request's link", strconv.Quote(pr))
	}
	focus := strip(p.Get("focus"))
	if focus == "" {
		return "", refuse("--focus is empty; say what to look at first: the risky changes and the open choices")
	}
	extra := p.Get("text")
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	sprints := dedupe(p.Values["sprint"])
	// A sprint closes only once its PR merges, so every sprint a PR under review delivers is open, and its delivery
	// report is written, for the owner to read next to the diff. The review goes under the first sprint and blocks its
	// close.
	if err := r.reviewReports(sprints); err != nil {
		return "", err
	}
	if err := prReachesMain(e.here, r.root, pr); err != nil {
		return "", err
	}
	parent := sprints[0]
	designs := dedupe(p.Values["design"])
	known := r.designSlugs()
	for _, slug := range designs {
		if !slices.Contains(known, slug) {
			list := strings.Join(known, ", ")
			if list == "" {
				list = "none"
			}
			return "", refuse("--design %s has no design page; known: %s", slug, list)
		}
	}
	verdicts, err := r.designGate(pr, sprints, design.PRFiles(r.root, pr))
	if err != nil {
		return "", err
	}
	title := strip(p.Get("title"))
	if title == "" {
		title = "Review " + pr
		if m := pullNumber.FindStringSubmatch(pr); m != nil {
			title = "Review PR #" + m[1]
		}
	}
	desc := fmt.Sprintf("Review %s, which delivers %s.\n\nFocus: %s", pr, strings.Join(sprints, ", "), focus)
	if designs != nil {
		desc += "\n\nDesign pages: " + strings.Join(designs, ", ")
	}
	if verdicts != nil {
		site, err := showSiteURL(e.here, store.MainOf(r.records))
		if err != nil {
			return "", err
		}
		desc += "\n\n" + design.Covering(site, verdicts)
	}
	if extra != "" {
		desc += "\n\n" + extra
	}
	rb, err := raisedBy()
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	made, err := ws.Create(work.New{Type: work.Need, Parent: parent, Title: title, Description: desc,
		Need: &work.NeedInfo{Kind: work.Review, RaisedBy: rb,
			Review: &work.ReviewInfo{PR: pr, Sprints: sprints, Designs: designs, Focus: focus}}})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("raised review %s under %s; %s; the pm service pushes the PR's merge to main the same way; once "+
		"the PR is on main: pm need close %s --reason \"merged as <sha>\"", made.ID, parent, deliveryHint(made.ID),
		made.ID), nil
}

func (r *repo) reviewReports(sprints []string) error {
	for _, sid := range sprints {
		rec, err := r.sprint(sid)
		if err != nil {
			return err
		}
		if it := r.item(sid); it != nil && it.Status == work.Closed {
			return refuse("sprint %s is closed, but a sprint closes only after its PR merges; a PR under review "+
				"delivers only open sprints", sid)
		}
		line, err := records.Outcome(rec)
		if err != nil {
			return err
		}
		var unwritten []string
		for _, part := range []struct{ name, text string }{{"Outcome", line},
			{`Against "Done when"`, records.ReportPart(rec, `Against "Done when"`)}} {
			if part.text == "" || part.text == records.NotClosed {
				unwritten = append(unwritten, "'"+part.name+"'")
			}
		}
		if unwritten != nil {
			verb := "is"
			if len(unwritten) > 1 {
				verb = "are"
			}
			return refuse("%s: the committed Delivery report %s %s still '%s'; write the Outcome (done, partial or "+
				"voided, plus one sentence, then optional bullets) and each 'Done when' item with its evidence, and "+
				"commit them with pm commit, so the owner reads the report while reviewing the PR", r.rel(rec.Path),
				strings.Join(unwritten, " and "), verb, records.NotClosed)
		}
	}
	return nil
}

// prReachesMain refuses a GitHub PR that does not merge into the main branch, or another repo's PR that does not
// merge into that repo's default branch (proc.PRReachesMain).
func prReachesMain(here, root, pr string) error {
	cfg, err := config.Load(here)
	if err != nil {
		return err
	}
	return proc.PRReachesMain(root, cfg.Remote, cfg.MainBranch, pr)
}

// otherRepo is the OWNER/REPO of pr when it is another repo's GitHub PR (proc.OtherRepoPR, on the config's remote).
func otherRepo(root, pr string) (string, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return "", err
	}
	return proc.OtherRepoPR(root, cfg.Remote, pr)
}

// mergeOf is the full sha that sha (given names it in a refusal) names as pr's merge: for another repo's PR, its merge
// commit as gh gives it (proc.OtherMerge); for this repo's, a commit on the remote's main (onMain), checked only with
// main.
func mergeOf(e *env, pr, given, sha string, main bool) (string, error) {
	root, err := store.CodeRoot(e.here, e.records)
	if err != nil {
		return "", err
	}
	switch other, err := otherRepo(root, pr); {
	case err != nil:
		return "", err
	case other != "":
		return proc.OtherMerge(root, other, pr, given, sha)
	case main:
		return onMain(e, given, sha)
	}
	return sha, nil
}
