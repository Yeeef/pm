package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/pyjson"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/site"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/work"
)

// The context the agent commands share: the records store, the work store, the records and the items, and the checks
// a write passes. Python source: Repo, load, check_planned, apply_writes and the session helpers in cli.py.

// refuse is a command's refusal: "error: <message>" on stderr, exit 1, nothing changed.
func refuse(format string, a ...any) error { return &refusal{fmt.Sprintf(format, a...)} }

// env is one command's run: where it runs, its streams, and its connection to the work store through the pm service,
// made on first use and closed when the command ends (the pm-go page, "Store access"): an open connection blocks no
// one, so a command keeps it across slow work.
type env struct {
	here, records string
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	ws            work.Store
	lockRecords   bool   // a write: the records lock is taken as the command connects to the work store
	unlock        func() // releases the records lock while it is held
}

// work is the connection to the work store, made now if it is not yet; for a write, the records lock is taken next.
func (e *env) work() (work.Store, error) {
	if e.ws == nil {
		ws, err := OpenWork(store.MainOf(e.records))
		if err != nil {
			return nil, err
		}
		e.ws = ws
		if e.lockRecords && e.unlock == nil {
			if e.unlock, err = store.Lock(e.records); err != nil {
				return nil, errors.Join(err, e.closeWork())
			}
		}
	}
	return e.ws, nil
}

// closeWork closes the connection to the work store if it is open. A write keeps the records lock until it ends
// (release).
func (e *env) closeWork() error {
	if e.ws == nil {
		return nil
	}
	err := e.ws.Shutdown()
	e.ws = nil
	return err
}

// release ends a command: the records lock released, then the connection to the work store closed.
func (e *env) release() error {
	if e.unlock != nil {
		e.unlock()
		e.unlock = nil
	}
	return e.closeWork()
}

// repo is Python's Repo: the worktree acted on, the store, every item and the records.
type repo struct {
	root, records string
	items         []work.Item
	x             *records.Items
	recs          []*records.Record
}

// load is the records and the items: the records branch's HEAD, which every write builds on, or with working the
// store as it is on disk, uncommitted edits included, for show. It only parses; a write validates what it touches
// (checkPlanned).
func (e *env) load(working bool) (*repo, error) {
	root, err := store.CodeRoot(e.here, e.records)
	if err != nil {
		return nil, err
	}
	ws, err := e.work()
	if err != nil {
		return nil, err
	}
	items, err := ws.Items()
	if err != nil {
		return nil, err
	}
	var recs []*records.Record
	if working {
		recs, err = records.Read(e.records, nil)
	} else {
		recs, err = store.Committed(e.records, nil)
	}
	if err != nil {
		return nil, err
	}
	return &repo{root: root, records: e.records, items: items, x: records.NewItems(items), recs: recs}, nil
}

// item is the item with this id, or nil.
func (r *repo) item(id string) *work.Item { return r.x.Get(id) }

// sprint is the sprint record whose bead is id.
func (r *repo) sprint(id string) (*records.Record, error) {
	var known []string
	for _, rec := range r.recs {
		if rec.Type() == "sprint" {
			if rec.Bead() == id {
				return rec, nil
			}
			known = append(known, rec.Bead())
		}
	}
	return nil, refuse("no sprint record has bead %s; known sprints: %s", id, strings.Join(known, ", "))
}

// project is the project record named name.
func (r *repo) project(name string) (*records.Record, error) {
	var known []string
	for _, rec := range r.recs {
		if rec.Type() == "project" {
			if rec.Name() == name {
				return rec, nil
			}
			known = append(known, rec.Name())
		}
	}
	list := strings.Join(known, ", ")
	if list == "" {
		list = "none"
	}
	return nil, refuse("no project record named %s; known projects: %s", pyRepr(name), list)
}

// rel is a store path as pm names it: records/<path>.
func (r *repo) rel(p string) string { return store.Rel(r.records, p) }

// with is the items with each of changed in place of the item of its id, or added.
func (r *repo) with(changed ...work.Item) []work.Item {
	out := make([]work.Item, 0, len(r.items)+len(changed))
	at := map[string]int{}
	for _, it := range r.items {
		at[it.ID] = len(out)
		out = append(out, it)
	}
	for _, it := range changed {
		if i, ok := at[it.ID]; ok {
			out[i] = it
		} else {
			out = append(out, it)
		}
	}
	return out
}

// checkPlanned validates what a planned change touches, on the records branch with the planned writes committed: the
// written records, the records of every item the change alters or its ancestors, and the answered-need check for the
// altered items. A write builds on HEAD's text, so it refuses a target with uncommitted changes. items nil is the
// items unchanged.
func (r *repo) checkPlanned(writes []store.Write, items []work.Item) error {
	paths := make([]string, len(writes))
	for i, w := range writes {
		paths[i] = w.Path
	}
	dirty, err := store.Uncommitted(r.records, paths)
	if err != nil {
		return err
	}
	if len(dirty) > 0 {
		name := "records/" + dirty[0]
		return refuse("%s has uncommitted changes, a hand edit or another session's edit in progress; commit them "+
			"first with pm commit -m \"…\" %s, or revert them, then run this again", name, name)
	}
	if items == nil {
		items = r.items
	}
	err = func() error {
		changes := map[string]*string{}
		for _, w := range writes {
			text := w.Text
			changes[w.Path] = &text
		}
		recs, err := store.Committed(r.records, changes)
		if err != nil {
			return err
		}
		x := records.NewItems(items)
		changed := map[string]bool{}
		for i := range items {
			if old := r.item(items[i].ID); old == nil || !reflect.DeepEqual(*old, items[i]) {
				changed[items[i].ID] = true
			}
		}
		touched := map[string]bool{}
		for id := range changed {
			touched[id] = true
			for _, a := range x.Ancestors(id) {
				touched[a] = true
			}
		}
		written := map[string]bool{}
		for _, p := range paths {
			written[resolvedPath(p)] = true
		}
		if err := site.CheckNeedsAnswered(recs, x, changed); err != nil {
			return err
		}
		for _, rec := range recs {
			if written[resolvedPath(rec.Path)] || (rec.Bead() != "" && touched[rec.Bead()]) {
				if _, err := site.RenderRecord(rec, recs, x, nil); err != nil {
					return err
				}
			}
		}
		return nil
	}()
	var re *records.Error
	if errors.As(err, &re) {
		return refuse("the change would not render: %s", re.Msg)
	}
	return err
}

// resolvedPath is Python's Path.resolve() for a path that may not exist yet.
func resolvedPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return filepath.Join(resolvedPath(filepath.Dir(abs)), filepath.Base(abs))
}

// apply writes the records and commits exactly those files, the message prefix + message; it returns message. A
// failed write or commit restores the files.
func (r *repo) apply(writes []store.Write, message, undo, prefix string) (string, error) {
	if err := store.Apply(r.records, writes, message, undo, prefix); err != nil {
		return "", err
	}
	return message, nil
}

// ---------------------------------------------------------------- sessions and holders

const (
	sessionEnv      = "CLAUDE_CODE_SESSION_ID" // Claude Code exports it to every command a session runs
	codexSessionEnv = "CODEX_THREAD_ID"        // Codex exports it to every command a thread runs
	inboxEnv        = "CLAUDE_CODE_MESSAGING_SOCKET"
	liveWindow      = int(LiveWindow / time.Second) // seconds, as refusals name it
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func codexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(homeDir(), ".codex")
}

// holder is who holds an item, as pm show gives it: the session pm task claim recorded, when, and whether it is live.
// The work store has no assignee, which Python pm gives from bd; it is always null here.
type holder struct {
	Session   string
	ClaimedAt time.Time
	Live      bool
}

func holderOf(it *work.Item) *holder {
	if it.Holder == nil {
		return nil
	}
	return &holder{Session: it.Holder.Session, ClaimedAt: it.Holder.ClaimedAt, Live: live(it.Holder.Session)}
}

// age is '12m', '3h' or '2d' since t; '?' for the zero time.
func age(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	s := time.Since(t).Seconds()
	switch {
	case s < 3600:
		return fmt.Sprintf("%dm", int(pyFloorDiv(s, 60)))
	case s < 86400:
		return fmt.Sprintf("%dh", int(pyFloorDiv(s, 3600)))
	}
	return fmt.Sprintf("%dd", int(pyFloorDiv(s, 86400)))
}

func pyFloorDiv(a, b float64) float64 {
	q := a / b
	if q < 0 && q != float64(int64(q)) {
		return float64(int64(q) - 1)
	}
	return float64(int64(q))
}

// holderText is a held task's tail: "held by <session's first 8>, <age>, live|idle".
func holderText(h *holder) string {
	state := "idle"
	if h.Live {
		state = "live"
	}
	return fmt.Sprintf("held by %s, %s, %s", firstRunes(h.Session, 8), age(h.ClaimedAt), state)
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// ---------------------------------------------------------------- bodies

const textForms = "--text or --text-file - <<'EOF'" // the two ways to give a body, as refusals name them

// body is a command's body: --text-file's, read before the store is opened, else --text stripped, else "".
func (e *env) body(p *Parsed) (string, error) {
	path, given := p.values["text_file"]
	if !given || len(path) == 0 {
		return config.PyStrip(p.Get("text")), nil
	}
	return readTextFile(path[len(path)-1], e.stdin)
}

var sentenceEnd = regexp.MustCompile(`^(.+?[.!?])(` + records.PyS + `|$)`) // Python's \s

// firstSentence is the first sentence of text's first paragraph, cut to limit characters with "…".
func firstSentence(text string, limit int) string {
	s := records.FirstPara(text)
	if m := sentenceEnd.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if r := []rune(s); len(r) > limit {
		return strings.TrimRightFunc(string(r[:limit-1]), config.IsPySpace) + "…"
	}
	return s
}

// pyRepr is Python's repr() of a string.
func pyRepr(s string) string { return pyjson.StrRepr(s) }
