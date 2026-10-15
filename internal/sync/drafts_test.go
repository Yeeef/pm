package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/store"
)

// The expected outcomes follow from the record-drafts page's Draft refs and Follow, and the records-store page's Push:
// a store only behind is fast-forwarded and its clean workspaces follow, so a draft another clone's close published
// leaves the draft and its ref; a draft page whose sprint closed elsewhere with other text is stranded, never silently.

const body = "a\n\nb\n\nc\n\nd\n\ne\n"

func sh(t *testing.T, dir string, cmds ...string) string {
	t.Helper()
	var out []byte
	for _, c := range cmds {
		cmd := exec.Command("sh", "-c", c)
		cmd.Dir = dir
		var err error
		if out, err = cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s in %s: %v\n%s", c, dir, err, out)
		}
	}
	return strings.TrimSpace(string(out))
}

// clones is clone A (root a, store sa) with design/p.md published, its bare origin, and clone B of it (store sb), each
// with its id, as pm init leaves them.
func clones(t *testing.T) (a, sa, sb string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(tmp, "gitconfig")
	put(t, cfg, "[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n")
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	a, b := filepath.Join(tmp, "a"), filepath.Join(tmp, "b")
	put(t, filepath.Join(a, ".gitignore"), "/records\n/.pm/store\n")
	sh(t, a, "git init -q", "git add .gitignore", "git commit -qm code", "git worktree add -q --detach .pm/store/records")
	sa = filepath.Join(a, ".pm/store/records")
	put(t, filepath.Join(sa, "design/p.md"), body)
	sh(t, sa, "git checkout -q --orphan records", "git rm -rqf .", "git add design", "git commit -qm records")
	origin := filepath.Join(tmp, "origin.git")
	sh(t, a, "git clone -q --bare . "+origin, "git remote add origin "+origin, "git fetch -q origin")
	sh(t, tmp, "git clone -q origin.git b")
	sh(t, b, "git worktree add -q .pm/store/records records")
	sb = filepath.Join(b, ".pm/store/records")
	for _, dir := range []string{a, b} {
		if _, err := store.MakeCloneID(dir); err != nil {
			t.Fatal(err)
		}
	}
	return a, sa, sb
}

// draftIn makes a's worktree wa with its records workspace, and drafts design/p.md there for pm-1 with old replaced.
func draftIn(t *testing.T, a, sa string, edits ...string) store.Workspace {
	t.Helper()
	code := filepath.Join(filepath.Dir(a), "wa")
	if _, err := os.Stat(code); err != nil {
		sh(t, a, "git worktree add -q -b wa "+code)
		if _, err := store.AddWorkspace(sa, code); err != nil {
			t.Fatal(err)
		}
	}
	w, err := store.WorkspaceAt(code)
	if err != nil || w == nil {
		t.Fatal(w, err)
	}
	p := filepath.Join(w.Dir, "design/p.md")
	text := get(t, p)
	for i := 0; i+1 < len(edits); i += 2 {
		text = strings.Replace(text, edits[i], edits[i+1], 1)
	}
	put(t, p, text)
	if _, err := store.CommitDraft(sa, *w, map[string]string{"design/p.md": "pm-1"}); err != nil {
		t.Fatal(err)
	}
	return *w
}

// syncOnce is one run of the records and drafts steps in the clone of records, with closed the work store's closed
// sprints.
func syncOnce(t *testing.T, records string, closed ...string) (string, string) {
	t.Helper()
	isClosed := func() (store.SprintClosed, error) {
		return func(id string) bool { return strings.Contains(strings.Join(closed, " "), id) }, nil
	}
	ok, rec := PushRecords(records, "origin", isClosed)
	if !ok {
		t.Fatalf("records step: %s", rec)
	}
	ok, drafts := PushDrafts(records, "origin")
	if !ok {
		t.Fatalf("drafts step: %s", drafts)
	}
	return rec, drafts
}

// closeFromB is pm sprint close pm-1's publish, run in clone B: fetch, plan, commit, drop.
func closeFromB(t *testing.T, sb string) {
	t.Helper()
	if err := store.FetchOtherDrafts(sb, "origin"); err != nil {
		t.Fatal(err)
	}
	p, err := store.PlanPublish(sb, "pm-1", nil)
	if err != nil || len(p.Pages) != 1 {
		t.Fatalf("B's close plans %v, %v", p, err)
	}
	if err := store.Apply(sb, []store.Write{{Path: filepath.Join(sb, "design/p.md"), Text: *p.Pages["design/p.md"]}},
		"close\n\n"+p.Trailer("pm-1"), "", ""); err != nil {
		t.Fatal(err)
	}
	if err := p.Drop(sb); err != nil {
		t.Fatal(err)
	}
}

func TestAStoreOnlyBehindFastForwardsAndItsPublishedDraftLeavesTheRemote(t *testing.T) {
	a, sa, sb := clones(t)
	w := draftIn(t, a, sa, "c\n", "C in A\n")
	syncOnce(t, sa)
	syncOnce(t, sb)
	closeFromB(t, sb)
	syncOnce(t, sb, "pm-1")

	rec, drafts := syncOnce(t, sa, "pm-1")
	if rec != "fast-forwarded 1 commit(s) from origin/records" || drafts != "pushed 0 and deleted 1 draft refs; "+
		"fetched the other clones'" {
		t.Errorf("A's one sync said %q, %q", rec, drafts)
	}
	if mine, theirs := sh(t, sa, "git rev-parse records"), sh(t, sa, "git rev-parse origin/records"); mine != theirs {
		t.Errorf("A's records %s, origin's %s", mine, theirs)
	}
	if got := sh(t, sa, "git ls-remote origin 'refs/pm/drafts/*'"); got != "" {
		t.Errorf("A's draft ref is still on the remote: %s", got)
	}
	if pages, _, err := store.DraftPages(sa, w); err != nil || len(pages) != 0 {
		t.Errorf("A's draft still holds %v (%v)", pages, err)
	}
	if dirty, err := w.Dirty(); err != nil || dirty != nil {
		t.Errorf("A's workspace has uncommitted %v (%v)", dirty, err)
	}
}

func TestADraftCommittedAfterItsPushIsStrandedWhenItsSprintClosesElsewhere(t *testing.T) {
	a, sa, sb := clones(t)
	draftIn(t, a, sa, "c\n", "C in A\n")
	syncOnce(t, sa)
	syncOnce(t, sb)
	w := draftIn(t, a, sa, "a\n", "A after the push\n") // committed in A after its last push
	closeFromB(t, sb)
	syncOnce(t, sb, "pm-1")

	rec, drafts := syncOnce(t, sa, "pm-1")
	if !strings.Contains(rec, "drafts/wa followed; stranded design/p.md") || !strings.Contains(drafts, "deleted 1") {
		t.Errorf("A's sync said %q, %q", rec, drafts)
	}
	text := get(t, filepath.Join(w.Dir, "design/p.md"))
	if !strings.Contains(text, "A after the push\n") || !strings.Contains(text, "C in A\n") {
		t.Errorf("A's page lost its text: %q", text)
	}
	if pages, _, err := store.DraftPages(sa, w); err != nil || len(pages) != 0 {
		t.Errorf("A's draft still holds %v (%v)", pages, err)
	}
	stranded, err := w.Stranded()
	if err != nil || stranded["design/p.md"] != "pm-1" {
		t.Fatalf("Stranded = %v, %v", stranded, err)
	}
	said, err := w.Describe(sa)
	if err != nil || !strings.Contains(said, store.StrandedLine("design/p.md", "pm-1")) {
		t.Errorf("pm where's workspace line %q (%v) does not say what to do", said, err)
	}
	if d := (store.Clone{Store: sa}).Drafts(); len(d) != 1 || !strings.Contains(d[0].State, "records/design/p.md "+
		"stranded: sprint pm-1 closed without it") {
		t.Errorf("the site's draft of wa: %+v", d)
	}
	if _, err := store.CommitDraft(sa, w, map[string]string{"design/p.md": "pm-2"}); err != nil {
		t.Fatal(err)
	}
	if stranded, err := w.Stranded(); err != nil || len(stranded) != 0 {
		t.Errorf("committed under pm-2, the page is still noted stranded: %v, %v", stranded, err)
	}
}

// put writes text at p, making its directory.
func put(t *testing.T, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
