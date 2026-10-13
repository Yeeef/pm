package cli

import (
	"fmt"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"path/filepath"
	"strings"
	"time"
)

// feedbackDoc is the repo's pm feedback doc, or nil before its first entry.
func feedbackDoc(recs []*records.Record) *records.Record {
	for _, r := range recs {
		if records.IsFeedbackDoc(r) {
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
	path := filepath.Join(r.records, records.FeedbackDoc+".md")
	var old string
	if doc := feedbackDoc(r.recs); doc != nil {
		old = doc.Text
	} else if exists(path) {
		return "", refuse("%s exists but is no pm feedback doc (type: doc); move it, then run this again", r.rel(path))
	} else {
		old = fmt.Sprintf("---\ntype: doc\ntitle: %s\ndate: %s\n---\n\n"+
			"Where pm got in the way, one entry per `pm feedback add`, newest last; each names the project it is about, "+
			"when it is about one.\n", records.FeedbackTitle, store.Today())
	}
	w := []store.Write{{Path: path, Text: strings.TrimRight(old, "\n") + "\n\n" + entry}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "added feedback to "+r.rel(path), "pm: ")
}
