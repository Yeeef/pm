package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"path/filepath"
	"strings"
	"time"
)

// feedbackDoc is the repo's feedback doc f, or nil before its first entry.
func feedbackDoc(recs []*records.Record, f records.Feedback) *records.Record {
	for _, r := range recs {
		if r.Type() == "doc" && r.Rel == f.Rel {
			return r
		}
	}
	return nil
}

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
	entry := fmt.Sprintf("### %s UTC, session `%s`\n\n", time.Now().UTC().Format("2006-01-02 15:04"), sid)
	if about != nil {
		entry += "About " + strings.Join(about, ", ") + ".\n\n"
	}
	entry += text + "\n"
	f := records.PMFeedback
	if p.Get("repo") == "true" {
		f = records.RepoFeedback
	}
	path := filepath.Join(r.records, f.Rel+".md")
	var old string
	if doc := feedbackDoc(r.recs, f); doc != nil {
		old = doc.Text
	} else if fsutil.Exists(path) {
		return "", refuse("%s exists but is no %s doc (type: doc); move it, then run this again", r.rel(path), f.Title)
	} else {
		old = fmt.Sprintf("---\ntype: doc\ntitle: %s\ndate: %s\n---\n\n%s\n", f.Title, store.Today(), f.Intro)
	}
	w := []store.Write{{Path: path, Text: strings.TrimRight(old, "\n") + "\n\n" + entry}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "added feedback to "+r.rel(path), "pm: ")
}
