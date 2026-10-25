package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"path/filepath"
	"time"
)

func noSession() error {
	return refuse("no agent session: neither %s nor %s is set; name one with --session", sessionEnv, codexSessionEnv)
}

func cmdFeedbackAdd(e *env, p *Parsed) (string, error) {
	text := p.Get("text")
	if text == "" {
		return "", refuse("the feedback text is empty; pass it with %s", textForms)
	}
	sid := strip(p.Get("session"))
	if sid == "" {
		sid = currentSession()
	}
	if sid == "" {
		return "", noSession()
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	var about []string
	if name, ok := given(p, "project"); ok {
		prec, err := r.project(name)
		if err != nil {
			return "", err
		}
		about = append(about, fmt.Sprintf("project `%s`", prec.Name()))
	}
	for _, f := range []struct{ flag, dest, word string }{{"--sprint", "sprint", "sprint"}, {"--task", "task", "task"}} {
		if id, ok := given(p, f.dest); ok {
			if r.item(id) == nil {
				return "", refuse("%s %s is not in the work store", f.flag, id)
			}
			about = append(about, fmt.Sprintf("%s `%s`", f.word, id))
		}
	}
	for _, rec := range r.recs { // a per-project doc from before the repo's one doc: merged by hand first
		if err := records.CheckFeedbackDoc(rec); err != nil {
			return "", refuse("%s", err)
		}
	}
	f := records.PMFeedback
	if p.Get("repo") == "true" {
		f = records.RepoFeedback
	}
	path := filepath.Join(r.records, f.Rel+".md")
	old := ""
	if doc := f.Doc(r.recs); doc != nil {
		old = doc.Text
	} else if fsutil.Exists(path) {
		return "", refuse("%s exists but is no %s doc (type: doc); move it, then run this again", r.rel(path), f.Title)
	}
	w := []store.Write{{Path: path, Text: f.Append(old, store.Today(), time.Now(), sid, about, text)}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "added feedback to "+r.rel(path), "pm: ")
}
