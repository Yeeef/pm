package cli

import (
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
	"github.com/Yeeef/pm/internal/textwrap"
	"github.com/Yeeef/pm/internal/work"
	"strings"
)

func cmdFindingAdd(e *env, p *Parsed) (string, error) {
	text := p.Get("text") // --text, or --text-file's body (runAgent)
	if words, body := p.Values["words"], p.Values["text"]; len(words) > 0 {
		if body != nil {
			return "", refuse("give the finding as one argument or with %s, not both", textForms)
		}
		text = strip(strings.Join(words, " "))
		if strings.HasPrefix(text, "--") {
			return "", refuse("the finding text starts with --, an option pm finding add does not have: %s; give "+
				"the text as one quoted argument, with --text=\"…\", or with --text-file - <<'EOF'", pyRepr(text))
		}
	}
	if text == "" {
		return "", refuse("the finding text is empty")
	}
	r, err := e.load(false)
	if err != nil {
		return "", err
	}
	sprintID := p.Get("sprint")
	rec, err := r.sprint(sprintID)
	if err != nil {
		return "", err
	}
	if it := r.item(sprintID); it != nil && it.Status == work.Closed {
		return "", refuse("sprint %s is closed; add the finding to an open sprint", sprintID)
	}
	entry := textwrap.Fill(text, 78, "- ", "  ")
	updated, err := records.InsertEntry(rec.Text, "Findings", entry)
	if err != nil {
		return "", err
	}
	w := []store.Write{{Path: rec.Path, Text: updated}}
	if err := r.checkPlanned(w, nil); err != nil {
		return "", err
	}
	return r.apply(w, "added a finding to "+r.rel(rec.Path), "pm: ")
}

// feedbackDoc is the repo's pm feedback doc, or nil before its first entry.
