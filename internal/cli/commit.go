package cli

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdCommit(e *env, p *Parsed) (string, error) {
	message := strip(p.Get("message"))
	if message == "" {
		return "", refuse("-m is empty; say what the hand edit changed")
	}
	if w, err := store.WorkspaceOf(e.here); err != nil {
		return "", err
	} else if w != nil {
		return e.commitWorkspace(*w, message, p)
	}
	dirty, err := gitPorcelain(e.records)
	if err != nil {
		return "", err
	}
	if dirty == "" {
		return "", refuse("nothing to commit in %s", e.records)
	}
	lines := strings.Split(dirty, "\n")
	names := p.Values["paths"]
	if len(names) == 0 {
		listing := make([]string, len(lines))
		for i, l := range lines {
			listing[i] = "  " + l[:3] + "records/" + l[3:]
		}
		return "", refuse("name the records you edited: pm commit -m \"…\" <path>…; other sessions' edits may be in the "+
			"store too. Uncommitted now:\n%s", strings.Join(listing, "\n"))
	}
	paths, err := e.commitPaths(names, lines)
	if err != nil {
		return "", err
	}
	root := fsutil.Resolved(e.records)
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		if isDesignPage(filepath.ToSlash(rel)) {
			return "", refuse("records/%s is a design page: design pages are edited and committed in a code worktree's "+
				"records workspace, not in the shared store. Make the edit in <worktree>/records/%s of a worktree of "+
				"your own (pm init there makes the workspace; pm task claim says how to make the worktree), then pm "+
				"commit it there", rel, rel)
		}
	}
	codeRoot, err := store.CodeRoot(e.here, e.records)
	if err != nil {
		return "", err
	}
	changes := map[string]*string{}
	for _, path := range paths {
		if b, err := os.ReadFile(path); err == nil {
			text := string(b)
			changes[path] = &text
		} else if errors.Is(err, os.ErrNotExist) {
			if err := records.LinkToNothing(root, path); err != nil {
				return "", err
			}
			changes[path] = nil
		} else {
			return "", err
		}
		if strings.HasSuffix(filepath.Base(path), ".summary.json") && fsutil.Exists(path) {
			if _, err := records.ReadSummary(path); err != nil {
				return "", err
			}
		}
	}
	recs, err := store.Committed(e.records, changes)
	if err != nil {
		return "", err
	}
	ws, err := e.work()
	if err != nil {
		return "", err
	}
	items, err := ws.Items()
	if err != nil {
		return "", err
	}
	if _, err := site.Check(recs, records.NewItems(items), filepath.Base(codeRoot), nil); err != nil {
		return "", err
	}
	head, err := store.Commit(e.records, message, paths)
	if err != nil {
		return "", err
	}
	rels := make([]string, len(paths))
	for i, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		rels[i] = "records/" + filepath.ToSlash(rel)
	}
	return fmt.Sprintf("committed %s as %s: %s", strings.Join(rels, ", "), head, message), nil
}

// gitPorcelain is git status --porcelain --untracked-files=all in dir, each line's status columns kept.
func gitPorcelain(dir string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = dir
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git status failed in %s: %s", dir, strip(errOut.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func (e *env) commitPaths(names, lines []string) ([]string, error) {
	changed := map[string]bool{}
	for _, l := range lines {
		parts := strings.Split(l[3:], " -> ")
		changed[strings.Trim(parts[len(parts)-1], `"`)] = true
	}
	root := fsutil.Resolved(e.records)
	var paths []string
	for _, name := range names {
		g := name
		top, under, _ := strings.Cut(name, "/")
		if !filepath.IsAbs(g) && top == "records" {
			g = filepath.Join(e.records, under) // records/... from any worktree, its link may not exist yet
		}
		if !filepath.IsAbs(g) {
			g = filepath.Join(e.here, g)
		}
		path := fsutil.Resolved(g)
		inside, err := filepath.Rel(root, path)
		outside := err != nil || inside == ".." || strings.HasPrefix(inside, "../")
		// A path relative to the store, such as sprints/x.md, from anywhere outside it.
		rel := filepath.ToSlash(filepath.Clean(name))
		if outside && !filepath.IsAbs(name) && rel != ".." && !strings.HasPrefix(rel, "../") &&
			(changed[rel] || fsutil.Exists(filepath.Join(e.records, rel))) {
			path, inside, outside = fsutil.Resolved(filepath.Join(e.records, rel)), rel, false
		}
		if outside {
			return nil, refuse("%s is not in the records store %s", name, e.records)
		}
		inside = filepath.ToSlash(inside)
		if inside == "." {
			return nil, refuse("%s is the records store itself; name the records you edited: pm commit -m \"…\" "+
				"records/<…>.md…", name)
		}
		if !changed[inside] {
			return nil, refuse("%s has no uncommitted change", name)
		}
		paths = append(paths, path)
	}
	return paths, nil
}
