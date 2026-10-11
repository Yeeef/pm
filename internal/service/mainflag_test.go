package service

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheMainCheckoutIsFlaggedOffMainOrBehindItsRemote(t *testing.T) {
	tmp := t.TempDir()
	origin, main, other := filepath.Join(tmp, "origin.git"), filepath.Join(tmp, "main"), filepath.Join(tmp, "other")
	git := func(dir string, args ...string) {
		t.Helper()
		args = append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	texts := Texts{main, "origin", "main"}
	flag := func() string {
		t.Helper()
		line, err := texts.MainFlag()
		if err != nil {
			t.Fatal(err)
		}
		return line
	}
	git(tmp, "init", "-q", "-b", "main", main)
	git(main, "commit", "-q", "--allow-empty", "-m", "one")
	if got := flag(); got != "" {
		t.Fatalf("no remote-tracking ref yet: %q", got)
	}
	git(tmp, "init", "-q", "--bare", "-b", "main", origin)
	git(main, "remote", "add", "origin", origin)
	git(main, "push", "-q", "origin", "main")
	git(tmp, "clone", "-q", origin, other)
	git(other, "commit", "-q", "--allow-empty", "-m", "two")
	git(other, "push", "-q", "origin", "main")
	if got := flag(); got != "" {
		t.Fatalf("not fetched yet, so current as far as the clone knows: %q", got)
	}
	git(main, "fetch", "-q", "origin")
	want := "main checkout " + main + " is 1 commit(s) behind origin/main, so hooks and rules run old code; run git -C " +
		main + " pull --ff-only origin main"
	if got := flag(); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(texts.mainBanner(), "<li>"+want+"</li>") {
		t.Fatal(texts.mainBanner())
	}
	git(main, "pull", "-q", "--ff-only", "origin", "main")
	if got := flag(); got != "" || texts.mainBanner() != "" {
		t.Fatalf("up to date: %q", got)
	}
	git(main, "switch", "-q", "-c", "feature")
	want = "main checkout " + main + " is on branch feature, not main, so hooks and rules read another tree; run " +
		"git -C " + main + " switch main"
	if got := flag(); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	git(main, "switch", "-q", "--detach", "main")
	if got := flag(); !strings.Contains(got, " has a detached HEAD, not main, ") {
		t.Fatal(got)
	}
	if _, err := (Texts{tmp, "origin", "main"}).MainFlag(); err == nil {
		t.Fatal("a main checkout git cannot read is an error, not a clean checkout")
	}
}
