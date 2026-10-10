// Package store is the records store: the `records` branch, checked out once per clone at
// <main checkout>/.pm/store/records. Every worktree finds it through the clone's common git dir, so a write is visible
// from every branch and worktree at once. It finds and checks the store, locks it, writes records atomically and
// commits exactly the files written.
package store

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
)

const (
	Branch = "records"
	Setup  = "pm init"
)

func errorf(format string, a ...any) error { return &records.Error{Msg: fmt.Sprintf(format, a...)} }

// run is git in dir; out is stdout, failed says the exit status was not 0.
func run(dir string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return o.String(), e.String(), err
}

// Git is git in dir, its stdout stripped; a failure is a records error carrying git's message.
func Git(dir string, args ...string) (string, error) {
	out, errOut, err := run(dir, args...)
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return "", err
		}
		msg := errOut
		if msg == "" {
			msg = out
		}
		return "", errorf("git %s failed in %s: %s", strings.Join(args, " "), dir, records.Strip(msg))
	}
	return records.Strip(out), nil
}

// PathOf is where the store of the clone containing cwd lives, whether or not it exists yet.
func PathOf(cwd string) (string, error) {
	common, err := Git(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.Base(common) != ".git" {
		return "", errorf("the clone's git dir %s is not a .git directory inside a main checkout", common)
	}
	return filepath.Join(filepath.Dir(common), config.Store), nil
}

// MainOf is the main checkout a store belongs to.
func MainOf(store string) string {
	for range strings.Split(config.Store, "/") {
		store = filepath.Dir(store)
	}
	return store
}

// Find is the store, checked: a worktree of this clone on the records branch. Never a fallback.
func Find(cwd string) (string, error) {
	store, err := PathOf(cwd)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(store); err != nil || !st.IsDir() {
		return "", errorf("no records store at %s; set it up with %s", store, Setup)
	}
	top, err := Git(store, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	branch, err := Git(store, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if resolved(top) != resolved(store) || branch != Branch {
		return "", errorf("%s is not a worktree on branch %s; move it away and run %s", store, Branch, Setup)
	}
	return store, nil
}

// resolved is the path with its symlinks resolved, as far as it exists.
func resolved(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return filepath.Join(resolved(filepath.Dir(abs)), filepath.Base(abs))
}

// CodeRoot is the worktree a command acts on for code commits and the site: cwd's, or the main checkout from inside
// the store.
func CodeRoot(cwd, store string) (string, error) {
	top, err := Git(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if resolved(top) == resolved(store) {
		return MainOf(store), nil
	}
	return top, nil
}

// names is each path relative to the store, as git names it.
func names(store string, paths []string) ([]string, error) {
	root := resolved(store)
	out := make([]string, len(paths))
	for i, p := range paths {
		rel, err := filepath.Rel(root, resolved(p))
		if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return nil, fmt.Errorf("%s is not in the store %s", p, store)
		}
		out[i] = filepath.ToSlash(rel)
	}
	return out, nil
}

// Commit commits exactly paths on the records branch and returns the short hash; any other change in the store, such
// as another session's, stays out. Records are validated before they get here, and code-branch hooks do not apply to
// the store, so hooks are skipped.
func Commit(store, message string, paths []string) (string, error) {
	if len(paths) == 0 {
		return "", errorf("no records named to commit")
	}
	ns, err := names(store, paths)
	if err != nil {
		return "", err
	}
	if _, err := Git(store, append([]string{"add", "-A", "--"}, ns...)...); err != nil {
		return "", err
	}
	if _, err := Git(store, append([]string{"commit", "--quiet", "--no-verify", "-m", message, "--"}, ns...)...); err != nil {
		return "", err
	}
	return Git(store, "rev-parse", "--short", "HEAD")
}

// Committed is the record set the records branch holds once changes (a path's new text, or nil to remove it) are
// committed on top of HEAD. Writes and pm commit check this, not the working store, so another session's uncommitted
// file neither blocks them nor lets them depend on it.
func Committed(store string, changes map[string]*string) ([]*records.Record, error) {
	root := resolved(store)
	cmd := exec.Command("git", "archive", "--format=tar", "HEAD")
	cmd.Dir = store
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Run(); err != nil {
		return nil, errorf("git archive HEAD failed in %s: %s", store, records.Strip(e.String()))
	}
	texts := map[string]string{}
	tr := tar.NewReader(&o)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, ".md") {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		texts[filepath.Join(root, filepath.FromSlash(h.Name))] = string(b)
	}
	for p, text := range changes {
		if filepath.Ext(p) != ".md" {
			continue
		}
		if text == nil {
			delete(texts, resolved(p))
		} else {
			texts[resolved(p)] = *text
		}
	}
	return records.ParseAll(root, texts)
}

// Dates is a design page's created and last-updated dates.
type Dates struct{ Created, Updated string }

// DesignDates is the dates of each design record in recs, by its rel: those of its first and last commits on the
// records branch, following renames within its directory as git log --follow does. A page with uncommitted changes
// counts as updated today, and one never committed as created today too. One git log over the design pages'
// directories, plus one git status.
func DesignDates(store string, recs []*records.Record, today string) (map[string]Dates, error) {
	var rels []string
	dirSet := map[string]bool{}
	for _, r := range recs {
		if r.Type() == "design" {
			rels = append(rels, r.Rel)
			dirSet[path.Dir(r.Rel)] = true
		}
	}
	out := map[string]Dates{}
	if rels == nil {
		return out, nil
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	log, err := Git(store, append([]string{"log", "-M", "--name-status", "--format=%x00%cs", "--"}, dirs...)...)
	if err != nil {
		return nil, err
	}
	created, updated, renamed := map[string]string{}, map[string]string{}, map[string]string{}
	chunks := strings.Split(log, "\x00")
	for _, chunk := range chunks[1:] { // newest commit first
		lines := strings.Split(strings.Trim(chunk, "\n"), "\n")
		day := lines[0]
		for _, line := range lines[1:] {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			status, paths := fields[0], fields[1:]
			last := paths[len(paths)-1]
			name, ok := renamed[last]
			if !ok {
				name = last
			}
			if _, ok := updated[name]; !ok {
				updated[name] = day
			}
			created[name] = day
			if strings.HasPrefix(status, "R") {
				renamed[paths[0]] = name
			}
		}
	}
	files := make([]string, len(rels))
	for i, rel := range rels {
		files[i] = filepath.Join(store, rel+".md")
	}
	dirtyList, err := Uncommitted(store, files)
	if err != nil {
		return nil, err
	}
	dirty := map[string]bool{}
	for _, d := range dirtyList {
		dirty[d] = true
	}
	for _, rel := range rels {
		p := rel + ".md"
		switch {
		case dirty[p]:
			c, ok := created[p]
			if !ok {
				c = today
			}
			out[rel] = Dates{c, today}
		case updated[p] != "":
			out[rel] = Dates{created[p], updated[p]}
		default:
			return nil, errorf("%s: committed and unchanged, yet git log over %s never names it", rel,
				strings.Join(dirs, ", "))
		}
	}
	return out, nil
}

// Uncommitted is those of paths that differ from HEAD in the store (modified, staged, removed or untracked), as store
// paths. It takes no optional lock: the pm service runs it every time the records move (design-page dates), and git
// status's opportunistic index.lock would fail a pm commit running at that moment.
func Uncommitted(store string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil // with no pathspec, git status would list every change in the store
	}
	ns, err := names(store, paths)
	if err != nil {
		return nil, err
	}
	out, errOut, err := run(store, append([]string{"--no-optional-locks", "status", "--porcelain", "--untracked-files=all",
		"-z", "--"}, ns...)...)
	if err != nil {
		return nil, fmt.Errorf("git status failed in %s: %s", store, records.Strip(errOut))
	}
	var dirty []string
	for _, entry := range strings.Split(out, "\x00") {
		if len(entry) > 3 && entry[2] == ' ' {
			dirty = append(dirty, entry[3:])
		}
	}
	return dirty, nil
}

// Lock holds an exclusive lock on the store, shared by every worktree, across a write and its commit; Unlock releases
// it.
func Lock(store string) (unlock func(), err error) {
	fd, err := syscall.Open(store, syscall.O_RDONLY|syscall.O_CLOEXEC, 0) // git children must not hold the lock
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: store, Err: err}
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		syscall.Close(fd)
		return nil, &fs.PathError{Op: "flock", Path: store, Err: err}
	}
	return func() { syscall.Close(fd) }, nil
}

// WriteAtomic writes text to p through a temporary file in its directory, keeping an existing file's mode.
func WriteAtomic(p, text string) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	mode := os.FileMode(0o666) &^ umask()
	if st, err := os.Stat(p); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		return err
	}
	ok = true
	return nil
}

func umask() os.FileMode {
	m := syscall.Umask(0)
	syscall.Umask(m)
	return os.FileMode(m)
}

// Write is one planned record file and its new text.
type Write struct {
	Path, Text string
}

// Rel is a store path as pm names it to the user: records/<path>.
func Rel(store, p string) string {
	ns, err := names(store, []string{p})
	if err != nil {
		return p
	}
	return "records/" + ns[0]
}

// oserror is a failed file operation as Python prints e.strerror: the errno text, capitalised.
func oserror(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		s := errno.Error()
		r := []rune(s)
		r[0] = unicode.ToUpper(r[0])
		return string(r)
	}
	return err.Error()
}

// Restore puts each written file back as it was (removing one that did not exist, nil text) and unstages it, so a
// failed write leaves nothing in the store for another session's pm commit to sweep up; it says what it did.
func Restore(store string, before []Write, existed map[string]bool) string {
	var done, failed []string
	paths := make([]string, len(before))
	for i, w := range before {
		paths[i] = w.Path
		var err error
		if existed[w.Path] {
			err = WriteAtomic(w.Path, w.Text)
		} else if err = os.Remove(w.Path); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%s)", Rel(store, w.Path), oserror(err)))
		} else {
			done = append(done, Rel(store, w.Path))
		}
	}
	ns, _ := names(store, paths)
	staged, _, _ := run(store, append([]string{"diff", "--cached", "--name-only", "--"}, ns...)...)
	if strings.TrimSpace(staged) != "" {
		if _, _, err := run(store, append([]string{"reset", "-q", "--"}, ns...)...); err != nil {
			failed = append(failed, fmt.Sprintf("the index still stages %s; unstage with git -C %s reset -q -- %s",
				strings.Join(strings.Fields(staged), ", "), store, strings.Join(ns, " ")))
		}
	}
	out := "restored nothing"
	if done != nil {
		out = "restored " + strings.Join(done, ", ") + " to the state before this write"
	}
	if failed != nil {
		out += "; could not restore: " + strings.Join(failed, "; ")
	}
	return out
}

// Refusal is a write that failed and was undone: Python's Refuse from apply_writes.
type Refusal struct{ Msg string }

func (r *Refusal) Error() string { return r.Msg }

// Apply writes the records and commits exactly those files on the records branch, with the message prefix+message.
// If a write or the commit fails, the files are restored, so the failure leaves nothing behind; undo names the command
// that undoes the work-store step, if any.
func Apply(store string, writes []Write, message, undo, prefix string) error {
	hint := ""
	if undo != "" {
		hint = "; undo the work-store step with: " + undo
	}
	var before []Write
	existed := map[string]bool{}
	for _, w := range writes {
		b, err := os.ReadFile(w.Path)
		switch {
		case err == nil:
			existed[w.Path] = true
		case !errors.Is(err, fs.ErrNotExist): // a record that cannot be read is never overwritten
			return &Refusal{fmt.Sprintf("writing %s failed: %s; no record was changed%s", w.Path, oserror(err), hint)}
		}
		before = append(before, Write{w.Path, string(b)})
	}
	var written []Write
	fail := func(what string) error {
		undone := "no record was changed"
		if written != nil {
			undone = Restore(store, written, existed)
		}
		return &Refusal{what + "; " + undone + hint}
	}
	for i, w := range writes {
		if err := WriteAtomic(w.Path, w.Text); err != nil {
			name := "the record"
			var pe *fs.PathError
			if errors.As(err, &pe) {
				name = pe.Path
			}
			return fail(fmt.Sprintf("writing %s failed: %s", name, oserror(err)))
		}
		written = append(written, before[i])
	}
	paths := make([]string, len(writes))
	for i, w := range writes {
		paths[i] = w.Path
	}
	if _, err := Commit(store, prefix+message, paths); err != nil {
		return fail("committing failed: " + err.Error())
	}
	return nil
}

// Today is the local date, as Python's date.today().isoformat() gives it.
func Today() string { return time.Now().Format("2006-01-02") }
