package records

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Feedback is one of the repo's two feedback docs, which pm feedback add appends to: a doc at a fixed path, so a repo
// cannot hold two of one, with no date in its name and neither bead nor project in its header, since each entry names
// its own project. A doc with its title anywhere else fails CheckFeedbackDoc.
type Feedback struct {
	Rel, Title string
	Cmd        string // the command that appends to it
	Intro      string // the line under its header
}

var (
	PMFeedback = Feedback{"docs/pm-feedback", "pm feedback", "pm feedback add",
		"Where pm got in the way, one entry per `pm feedback add`, newest last; each names the project it is about, " +
			"when it is about one."}
	RepoFeedback = Feedback{"docs/repo-feedback", "repo feedback", "pm feedback add --repo",
		"What slowed agents down in this repo's code, docs and tools, one entry per `pm feedback add --repo`, newest " +
			"last; each answers `pm feedback questions` and names the project it is about, when it is about one."}
	Feedbacks = []Feedback{PMFeedback, RepoFeedback}
)

// FeedbackAt is the feedback doc at rel, a path under records/ without .md, or nil.
func FeedbackAt(rel string) *Feedback {
	for i := range Feedbacks {
		if Feedbacks[i].Rel == rel {
			return &Feedbacks[i]
		}
	}
	return nil
}

// FeedbackOf is the feedback doc the record is, or nil.
func FeedbackOf(rec *Record) *Feedback {
	if rec.Type() != "doc" {
		return nil
	}
	return FeedbackAt(rec.Rel)
}

// IsFeedbackDoc says whether the record is one of the repo's feedback docs.
func IsFeedbackDoc(rec *Record) bool { return FeedbackOf(rec) != nil }

// CheckFeedbackDoc fails on a feedback doc outside its path, such as one of the per-project docs
// (docs/<date>-<project>-feedback.md) that pm feedback add wrote before the repo's one pm feedback doc.
func CheckFeedbackDoc(rec *Record) error {
	for _, f := range Feedbacks {
		if rec.Type() == "doc" && rec.Title() == f.Title && rec.Rel != f.Rel {
			return errorf("%s: a repo keeps one %s doc, records/%s.md; move this doc's entries into it in time "+
				"order, each tagged with its project (About project `<name>`.), delete this doc and commit both with pm commit",
				rec.Rel, f.Title, f.Rel)
		}
	}
	return nil
}

// feedbackEntry matches the heading of each entry in a feedback doc.
var feedbackEntry = regexp.MustCompile(`(?m)^### \d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC, session ` + "`")

// FeedbackEntries counts the entries in a feedback doc's body.
func FeedbackEntries(body string) int { return len(feedbackEntry.FindAllStringIndex(body, -1)) }

// Doc is the feedback doc f among recs, or nil before its first entry.
func (f Feedback) Doc(recs []*Record) *Record {
	for _, r := range recs {
		if r.Type() == "doc" && r.Rel == f.Rel {
			return r
		}
	}
	return nil
}

// Append is old, the doc's text, with an entry appended: written at now by session sid, tagged with what about names.
// An old of "" is a new doc dated today.
func (f Feedback) Append(old, today string, now time.Time, sid string, about []string, text string) string {
	if old == "" {
		old = fmt.Sprintf("---\ntype: doc\ntitle: %s\ndate: %s\n---\n\n%s\n", f.Title, today, f.Intro)
	}
	entry := fmt.Sprintf("### %s UTC, session `%s`\n\n", now.UTC().Format("2006-01-02 15:04"), sid)
	if about != nil {
		entry += "About " + strings.Join(about, ", ") + ".\n\n"
	}
	return strings.TrimRight(old, "\n") + "\n\n" + entry + text + "\n"
}
