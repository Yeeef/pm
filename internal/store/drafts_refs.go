package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Draft refs (the record-drafts page): the pm service's sync pushes each drafts/<name> branch's draft commit to the
// remote's refs/pm/drafts/<clone>/<name>, where <clone> is the id pm init made for this clone (CloneID), so each clone
// is the only writer of its namespace; no clone ever writes another's. A follow rewrites the draft commit, so the push
// is forced, with --force-with-lease against the sha this clone last pushed there, kept in its
// refs/pm/pushed/<clone>/<name> (keyed by the id, so a clone given a new id pushes every draft anew): a lease that fails
// is reported and never forced, and so is a ref of this clone's that the remote holds at another sha than the one this
// clone last pushed, even when its draft did not move (git ls-remote, once per sync). A branch gone, or holding no unpublished page and no conflict mark,
// deletes its remote ref. The sync and pm sprint close fetch the other clones' refs into refs/pm/fetched/<clone>/<name>
// (FetchDrafts), which the site lists as committed drafts with no session (Clone) and the close publishes from
// (PlanPublish). A page another clone's close published stays in this clone's draft until its next follow onto a
// records tip holding that close, as the merge then finds nothing left of it; its remote ref goes at the next sync once
// the remote's records branch names the page published.

const (
	cloneKey    = "pm.clone"
	remoteRefs  = "refs/pm/drafts/"  // on the remote: refs/pm/drafts/<clone>/<name>
	fetchedRefs = "refs/pm/fetched/" // here, the other clones' as last fetched: refs/pm/fetched/<clone>/<name>
	pushedRefs  = "refs/pm/pushed/"  // here, what this clone last pushed: refs/pm/pushed/<clone>/<name>
)

var cloneID = regexp.MustCompile(`^[0-9a-f]{12}$`)

// CloneID is the clone's id from its own git config (pm.clone), as MakeCloneID made it; "" when it has none.
func CloneID(dir string) (string, error) {
	id, err := Git(dir, "config", "--local", "--get", "--default=", cloneKey)
	if err != nil || id == "" {
		return "", err
	}
	if !cloneID.MatchString(id) {
		return "", errorf("git config %s is %q, not the 12 hex digits pm init makes; unset it (git config --unset %s) "+
			"and run %s", cloneKey, id, cloneKey, Setup)
	}
	return id, nil
}

// MakeCloneID gives the clone an id when it has none: 12 random hex digits in its own git config, so it is the
// clone's alone and never tracked. What it did, or "" when the clone has one.
func MakeCloneID(dir string) (string, error) {
	if id, err := CloneID(dir); err != nil || id != "" {
		return "", err
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	if _, err := Git(dir, "config", "--local", cloneKey, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("gave this clone the id %s (git config %s): its drafts push to %s%s/ on the remote", id, cloneKey,
		remoteRefs, id), nil
}

// DescribeClone is pm where's word on the clone's id: the id and where its drafts push, or why there is none.
func DescribeClone(dir, remote string) string {
	id, err := CloneID(dir)
	switch {
	case err != nil:
		return err.Error()
	case id == "":
		return "no id; run " + Setup
	}
	return fmt.Sprintf("%s  its drafts push to %s %s%s/", id, remote, remoteRefs, id)
}

// FetchOtherDrafts is pm sprint close's fetch of the other clones' draft refs (FetchDrafts), before it reads them: it
// refuses a clone with no id and a fetch that fails; a clone whose remote is not set up has no other clone.
func FetchOtherDrafts(store, remote string) error {
	if _, err := Git(store, "remote", "get-url", remote); err != nil {
		return nil //nolint:nilerr // no remote: no other clone has drafts to fetch
	}
	id, err := CloneID(store)
	if err != nil {
		return err
	}
	if id == "" {
		return errorf("this clone has no id (git config %s), which pm sprint close needs to fetch the other clones' "+
			"drafts; run %s", cloneKey, Setup)
	}
	if err := FetchDrafts(store, remote, id); err != nil {
		return errorf("pm sprint close fetches the other clones' drafts from %s first, as it publishes them too, and "+
			"the fetch failed: %v; run it again (a fetch the pm service ran at the same moment fails it too)", remote, err)
	}
	return nil
}

// FetchDrafts fetches the other clones' draft refs from remote into refs/pm/fetched/<clone>/<name>, removing each
// the remote no longer has; id's own are left out.
func FetchDrafts(store, remote, id string) error {
	if id == "" {
		return errorf("this clone has no id (git config %s), which its draft refs need; run %s", cloneKey, Setup)
	}
	_, errOut, err := netGit(store, "fetch", "--quiet", "--prune", "--no-tags", "--no-write-fetch-head", remote,
		"+"+remoteRefs+"*:"+fetchedRefs+"*", "^"+remoteRefs+id+"/*")
	if err != nil {
		return errorf("git fetch of the draft refs from %s failed: %s", remote, strings.Join(strings.Fields(errOut), " "))
	}
	return nil
}

// FetchedStamp is the loose ref files of the other clones' drafts under the clone's common git dir, which a look at
// the drafts reads without git (packed ones are in packed-refs).
func FetchedStamp(common string) []string {
	var out []string
	filepath.WalkDir(filepath.Join(common, filepath.FromSlash(fetchedRefs)), //nolint:errcheck // none yet: no file
		func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				out = append(out, p)
			}
			return nil
		})
	return out
}

// Fetched is another clone's draft commit as this clone last fetched it.
type Fetched struct {
	Clone, Name string // the clone's id, and the admin name of the worktree whose draft it is there
	d           draftState
	message     string
	Err         error // why it is no draft commit pm reads: it is left out, with a warning
}

// Ref is where this clone keeps it.
func (f Fetched) Ref() string { return fetchedRefs + f.Clone + "/" + f.Name }

// Label names it in the close's and the gate's lines.
func (f Fetched) Label() string { return fmt.Sprintf("clone %s's drafts/%s", f.Clone, f.Name) }

// FetchedDrafts is the other clones' draft commits as last fetched, by ref; each that is no draft commit (one commit on
// one parent, its base, with pm's draft subject) carries its Err.
func FetchedDrafts(store string) ([]Fetched, error) {
	id, err := CloneID(store)
	if err != nil {
		return nil, err
	}
	list, err := Git(store, "for-each-ref", "--format=%(refname)", fetchedRefs)
	if err != nil {
		return nil, err
	}
	var out []Fetched
	for _, ref := range strings.Split(list, "\n") {
		clone, name, ok := strings.Cut(strings.TrimPrefix(ref, fetchedRefs), "/")
		if ref == "" || clone == id { // this clone's own, fetched before it had its id: its branches are the drafts
			continue
		}
		f := Fetched{Clone: clone, Name: name}
		if !ok || strings.Contains(name, "/") {
			f.Err = errorf("%s is not %s<clone>/<name>", ref, fetchedRefs)
		} else {
			f.d, f.message, f.Err = readFetched(store, ref)
		}
		out = append(out, f)
	}
	return out, nil
}

func readFetched(store, ref string) (draftState, string, error) {
	out, err := Git(store, "log", "-1", "--format=%H%x00%P%x00%B", ref)
	if err != nil {
		return draftState{}, "", err
	}
	parts := strings.SplitN(out, "\x00", 3)
	if len(parts) != 3 {
		return draftState{}, "", fmt.Errorf("git log -1 %s printed %q", ref, out)
	}
	if parents := strings.Fields(parts[1]); len(parents) != 1 || !strings.HasPrefix(parts[2], draftSubject) {
		return draftState{}, parts[2], errorf("%s is at %s, which is no draft commit pm makes (one commit on its base "+
			"with the subject %q)", ref, parts[0], draftSubject)
	}
	return draftOf(parts[0], strings.TrimSpace(parts[1]), parts[2]), parts[2], nil
}

// Pages is the draft's unpublished pages, each with its text (nil: it removes the page): each page it tags with a
// sprint whose close has not published it since the draft's base on this clone's records branch.
func (f Fetched) Pages(store string) (map[string]*string, error) {
	changed, err := changedPaths(store, f.d.base, f.d.head)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range changed {
		sprint, ok := f.d.tags[p]
		if !ok {
			continue
		}
		done, err := publishedSince(store, f.d.base, p, sprint)
		if err != nil {
			return nil, err
		}
		if !done {
			paths = append(paths, p)
		}
	}
	return blobs(store, f.d.head, paths)
}

// Tags is each page's sprint, and Conflicts each page its clone's follow left in conflict, with its sprint.
func (f Fetched) Tags() map[string]string      { return f.d.tags }
func (f Fetched) Conflicts() map[string]string { return f.d.conflicts }

// draft is the fetched draft as the site lists it: a committed draft, labelled by its clone, its worktree there and
// its sprints, with no session.
func (f Fetched) draft(store string) Draft {
	d := Draft{Name: f.Clone + "~" + f.Name, State: "committed", Err: f.Err} // ~ is in no ref name, so no worktree's
	var sprints []string
	if f.Err == nil {
		d.Files, d.Err = f.Pages(store)
		for p := range d.Files {
			sprints = append(sprints, f.d.tags[p])
		}
		for _, s := range f.d.conflicts {
			sprints = append(sprints, s)
		}
		if len(f.d.conflicts) > 0 {
			d.State += ", conflict"
		}
	}
	sort.Strings(sprints)
	d.Label = fmt.Sprintf("clone %s, worktree %s", f.Clone, f.Name)
	if sprints = slices.Compact(sprints); sprints != nil {
		d.Label += ", sprint " + strings.Join(sprints, ", ")
	}
	d.Label += ", no session"
	return d
}

// takes is the pages the close of sprint takes from the fetched draft, sorted: those it tags with the sprint, less
// discard and those a close of the sprint published since the draft's base.
func (f Fetched) takes(store, sprint string, discard []string) ([]string, error) {
	var take []string
	for page, s := range f.d.tags {
		if s != sprint || slices.Contains(discard, page) {
			continue
		}
		done, err := publishedSince(store, f.d.base, page, sprint)
		if err != nil {
			return nil, err
		}
		if !done {
			take = append(take, page)
		}
	}
	sort.Strings(take)
	return take, nil
}

// mayHold is whether an unreadable fetched draft's message names sprint in a "Sprint:" or "Conflict:" line.
func (f Fetched) mayHold(sprint string) bool {
	for _, tag := range []*regexp.Regexp{sprintTag, conflictTag} {
		for _, m := range tag.FindAllStringSubmatch(f.message, -1) {
			if m[2] == sprint {
				return true
			}
		}
	}
	return false
}

// conflicting is why each page the fetched draft marks as the sprint's, in conflict, holds the close, unless
// discarded.
func (f Fetched) conflicting(sprint string, discard []string) []string {
	var out []string
	for _, page := range slices.Sorted(maps.Keys(f.d.conflicts)) {
		if f.d.conflicts[page] != sprint || slices.Contains(discard, page) {
			continue
		}
		out = append(out, fmt.Sprintf("records/%s in clone %s's worktree %s holds the sprint's edit with conflict "+
			"markers a follow left, uncommitted: resolve them there, pm commit --sprint %s it and pm push there, or drop "+
			"it with --discard %s --reason \"…\"", page, f.Clone, f.Name, sprint,
			strings.TrimSuffix(path.Base(page), ".md")))
	}
	return out
}

// overlap is why the fetched draft cannot publish p, and the way out: the records commits that changed it since the
// draft's base, or the sprint's earlier drafts (others) that changed it in this merge.
func (f Fetched) overlap(store, tip, p, sprint string, others []string) (string, error) {
	commits, err := Git(store, "log", "--reverse", "--cherry-pick", "--right-only", "--format=%h %s",
		f.d.base+"..."+tip, "--", p)
	if err != nil {
		return "", err
	}
	page := fmt.Sprintf("records/%s on %s", p, f.Label())
	if _, _, err := run(store, "merge-base", "--is-ancestor", f.d.base, tip); err != nil {
		//nolint:nilerr // exit 1: the base is no ancestor of the tip, the case this names
		return fmt.Sprintf("%s is drafted on records commits this clone's records branch does not have yet (its base "+
			"%.12s): bring this clone's records branch up to the remote's with pm push; if it is up to date, that "+
			"clone's draft sits on records commits it has since rebased, and a pm write in its worktree %s then pm push "+
			"there move it onto the remote's; then pm sprint close %s again", page, f.d.base, f.Name, sprint), nil
	}
	if commits != "" {
		return fmt.Sprintf("%s overlaps what the records branch published since its base, in %s: in clone %s, once "+
			"its records branch has these commits (its pm push rebases onto the remote's once it has a records commit "+
			"of its own), any pm write in worktree %s brings them in as conflict markers; resolve them, pm commit "+
			"--sprint %s it and pm push there, then pm sprint close %s again here", page,
			strings.Join(strings.Split(commits, "\n"), "; "), f.Clone, f.Name, sprint, sprint), nil
	}
	return fmt.Sprintf("%s overlaps the sprint's draft of it on %s: bring both edits into one draft's records/%s (git "+
		"-C %s show %s:%s is this one's text) and pm commit --sprint %s it there; put the page back to its published "+
		"text in the other and pm commit --sprint %s it there, which drops it from that draft (in clone %s, then pm "+
		"push there); then pm sprint close %s again", page, strings.Join(others, ", "), p, store, f.Ref(), p, sprint,
		sprint, f.Clone, sprint), nil
}

// live is whether a draft commit still holds what other clones need: a conflict mark, or a page that no close of its
// sprint published since the draft's base, on this clone's records branch or on the remote's as last fetched.
func (d draftState) live(store, remote string) (bool, error) {
	if len(d.conflicts) > 0 {
		return true, nil
	}
	revs := []string{"refs/heads/" + Branch}
	if _, err := Git(store, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+Branch); err == nil {
		revs = append(revs, "refs/remotes/"+remote+"/"+Branch)
	}
	for page, sprint := range d.tags {
		done := false
		for _, rev := range revs {
			on, err := publishedOn(store, rev, d.base, page, sprint)
			if err != nil {
				return false, err
			}
			done = done || on
		}
		if !done {
			return true, nil
		}
	}
	return false, nil
}

// refPlan is what PushDrafts makes each remote ref of this clone: by name, the draft commit to hold, "" for none.
type refPlan struct {
	want, pushed map[string]string
	notes        []string
}

// planRefs reads, under the store lock, which draft commit each remote ref should hold and which it last pushed. A
// branch pm cannot read keeps its remote ref as it is.
func planRefs(store, remote, id string) (refPlan, error) {
	unlock, err := Lock(store)
	if err != nil {
		return refPlan{}, err
	}
	defer unlock()
	plan := refPlan{want: map[string]string{}, pushed: map[string]string{}}
	list, err := Git(store, "for-each-ref", "--format=%(refname) %(objectname)", pushedRefs+id+"/")
	if err != nil {
		return refPlan{}, err
	}
	for _, l := range strings.Split(list, "\n") {
		if ref, sha, ok := strings.Cut(l, " "); ok {
			plan.pushed[strings.TrimPrefix(ref, pushedRefs+id+"/")] = sha
		}
	}
	branches, err := DraftBranches(store)
	if err != nil {
		return refPlan{}, err
	}
	for _, w := range branches {
		d, err := readDraft(store, w)
		if err != nil {
			plan.want[w.Name] = plan.pushed[w.Name]
			plan.notes = append(plan.notes, fmt.Sprintf("left the remote ref of %s as it was: %v", w.Branch(), err))
			continue
		}
		if !d.draft {
			continue
		}
		live, err := d.live(store, remote)
		if err != nil {
			return refPlan{}, err
		}
		if live {
			plan.want[w.Name] = d.head
		}
	}
	return plan, nil
}

// PushDrafts brings the remote's refs/pm/drafts/<id>/<name> to each drafts/ branch's draft commit: pushed when it moved
// since this clone last pushed it, deleted when the branch is gone or holds no unpublished page and no conflict mark,
// each with --force-with-lease against the sha last pushed. It reads the branches under the store lock and pushes
// outside it. What it did, and an error naming each ref it did not move: a lease that failed is never forced.
func PushDrafts(store, remote, id string) (string, error) {
	if id == "" {
		return "", errorf("this clone has no id (git config %s), which its draft refs need; run %s", cloneKey, Setup)
	}
	plan, err := planRefs(store, remote, id)
	if err != nil {
		return "", err
	}
	out, errOut, err := netGit(store, "ls-remote", remote, remoteRefs+id+"/*")
	if err != nil {
		return "", errorf("git ls-remote %s failed: %s", remote, strings.Join(strings.Fields(errOut), " "))
	}
	there := map[string]string{} // each ref of this clone's namespace on the remote, by name: its sha
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if sha, ref, ok := strings.Cut(l, "\t"); ok {
			there[strings.TrimPrefix(ref, remoteRefs+id+"/")] = sha
		}
	}
	var pushed, deleted int
	var failed []string
	for name := range there {
		if _, ok := plan.pushed[name]; !ok {
			plan.pushed[name] = "" // never pushed from here: another writer's, which the check below names
		}
	}
	all := maps.Clone(plan.want) // every branch, and every name pushed before: a branch gone deletes its ref
	for n := range plan.pushed {
		if _, ok := all[n]; !ok {
			all[n] = ""
		}
	}
	for _, name := range slices.Sorted(maps.Keys(all)) {
		want, have := plan.want[name], plan.pushed[name]
		ref := remoteRefs + id + "/" + name
		if there[name] == want && want != have { // pushed, but the run stopped before it noted so
			if err := notePushed(store, pushedRefs+id+"/"+name, want); err != nil {
				return "", err
			}
			continue
		}
		if there[name] != have {
			failed = append(failed, leaseFailed(remote, ref, have))
			continue
		}
		if want == have {
			continue
		}
		if err := pushRef(store, remote, ref, pushedRefs+id+"/"+name, want, have); err != nil {
			failed = append(failed, err.Error())
			continue
		}
		if want == "" {
			deleted++
		} else {
			pushed++
		}
	}
	said := fmt.Sprintf("pushed %d and deleted %d draft refs", pushed, deleted)
	for _, n := range plan.notes {
		said += "; " + n
	}
	if failed != nil {
		return "", errorf("%s; %s", said, strings.Join(failed, "; "))
	}
	return said, nil
}

// leaseFailed says that the remote ref is not where this clone last pushed it (have, "" for absent), and the way out.
func leaseFailed(remote, ref, have string) string {
	if have == "" {
		have = "absent"
	}
	return fmt.Sprintf("%s on %s is not at %s, where this clone last pushed it, so pm did not force it: another writer "+
		"moved it, a copy of this clone with its id or a push by hand. See it with git ls-remote %s %s; a copied clone "+
		"gets its own id with git config --unset %s and %s there", ref, remote, have, remote, ref, cloneKey, Setup)
}

// pushRef moves the remote ref from have, the sha this clone last pushed ("" none), to want ("" deletes it), with
// --force-with-lease, then records want in local.
func pushRef(store, remote, ref, local, want, have string) error {
	out, errOut, err := netGit(store, "push", "--porcelain", remote, "--force-with-lease="+ref+":"+have, want+":"+ref)
	if err != nil {
		if strings.Contains(out, "(stale info)") {
			return errorf("%s", leaseFailed(remote, ref, have))
		}
		return errorf("git push of %s to %s failed: %s", ref, remote, strings.Join(strings.Fields(errOut+" "+out), " "))
	}
	return notePushed(store, local, want)
}

// notePushed records in local the sha this clone's remote ref now holds ("" none).
func notePushed(store, local, sha string) error {
	var err error
	if sha == "" {
		_, err = Git(store, "update-ref", "-d", local)
	} else {
		_, err = Git(store, "update-ref", local, sha)
	}
	return err
}
