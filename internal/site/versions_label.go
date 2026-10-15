package site

import (
	"slices"
	"strings"

	"github.com/Yeeef/pm/internal/item"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/session"
	"github.com/Yeeef/pm/internal/store"
)

// LabelDrafts labels each records workspace's draft as the version bar names it: its worktree's branch, the sprints of
// the open tasks its worktree's live session holds, and that session (session.Uses, over every workspace's worktree).
func LabelDrafts(drafts []store.Draft, items []item.Item, main string) {
	var trees []string
	for _, d := range drafts {
		if d.Worktree != "" {
			trees = append(trees, d.Worktree)
		}
	}
	if trees == nil {
		return
	}
	uses, usesErr := session.Uses(main, trees, "")
	x := records.NewItems(items)
	for i := range drafts {
		d := &drafts[i]
		if d.Worktree == "" {
			continue
		}
		branch := d.Branch
		if branch == "" {
			branch = "detached HEAD"
		}
		parts := []string{"worktree " + d.Name + " on " + branch}
		use, live := uses[d.Worktree]
		var sprints []string
		for _, it := range x.All() {
			if !live || it.Holder == nil || it.Holder.Session != use.Session || it.Status == item.Closed {
				continue
			}
			for _, a := range x.Ancestors(it.ID) {
				if sp := x.Get(a); sp != nil && sp.Type == item.Sprint && !slices.Contains(sprints, a) {
					sprints = append(sprints, a)
				}
			}
		}
		if sprints != nil {
			parts = append(parts, "sprint "+strings.Join(sprints, ", "))
		}
		switch {
		case live:
			parts = append(parts, "session "+shortSession(use.Session)+" live")
		case usesErr != nil:
			parts = append(parts, "sessions unread: "+usesErr.Error())
		default:
			parts = append(parts, "no live session")
		}
		d.Label = strings.Join(parts, ", ")
	}
}

// shortSession is a session id's first 8 characters, as pm show names a holder.
func shortSession(id string) string {
	if r := []rune(id); len(r) > 8 {
		return string(r[:8])
	}
	return id
}
