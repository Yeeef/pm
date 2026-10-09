package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/Yeeef/pm/internal/work"
)

// The reply spool: the service appends each checked reply here, fsynced, before it answers the POST, and a "done" line
// once the reply is in the work store, so a reply taken survives a crash or kill and a restart delivers it. It lives
// in the clone's git dir, next to the store and never committed; it is emptied whenever no reply in it is pending.
// The format is Python pm's, so a spool one leaves pending the other delivers.
const SpoolName = "pm-replies.jsonl"

// Entry is one spooled reply: its reply id, the need it answers, its text and when it was taken (epoch seconds).
type Entry struct {
	RID  string  `json:"rid"`
	ID   string  `json:"id"`
	Text string  `json:"text"`
	At   float64 `json:"at"`
}

type line struct {
	Entry
	Done string `json:"done,omitempty"`
}

// flocked holds an exclusive flock on path, created if missing, while fn runs: the spool for its own edits, its .lock
// for a delivery.
func flocked(path string, fn func() error) error {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT, 0o600)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return err
	}
	return fn()
}

// spoolRead is the spool's replies in order, and the reply ids already in the work store.
func spoolRead(spool string) ([]Entry, map[string]bool, error) {
	data, err := os.ReadFile(spool)
	if os.IsNotExist(err) {
		return nil, map[string]bool{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var entries []Entry
	seen, done := map[string]int{}, map[string]bool{}
	for n, l := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if l == "" && len(data) == 0 {
			break
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(l), &raw); err != nil {
			return nil, nil, refuse("%s:%d is not a JSON line: %q; fix or remove that line", spool, n+1, l)
		}
		var e line
		_ = json.Unmarshal([]byte(l), &e)
		if _, ok := raw["done"]; ok {
			done[e.Done] = true
			continue
		}
		if i, ok := seen[e.RID]; ok {
			entries[i] = e.Entry
			continue
		}
		seen[e.RID] = len(entries)
		entries = append(entries, e.Entry)
	}
	return entries, done, nil
}

// spoolAppend appends one line and fsyncs it.
func spoolAppend(spool string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(spool, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// SpoolAdd appends a reply unless the spool already holds its reply id (a resubmit); true when it was added.
func SpoolAdd(spool string, e Entry) (bool, error) {
	added := false
	err := flocked(spool, func() error {
		entries, _, err := spoolRead(spool)
		if err != nil {
			return err
		}
		for _, have := range entries {
			if have.RID == e.RID {
				return nil
			}
		}
		added = true
		return spoolAppend(spool, e)
	})
	return added, err
}

// spoolDone marks a reply stored, and empties the spool once none of its replies is pending.
func spoolDone(spool, rid string) error {
	return flocked(spool, func() error {
		entries, done, err := spoolRead(spool)
		if err != nil {
			return err
		}
		if err := spoolAppend(spool, map[string]string{"done": rid}); err != nil {
			return err
		}
		done[rid] = true
		for _, e := range entries {
			if !done[e.RID] {
				return nil
			}
		}
		return os.Truncate(spool, 0)
	})
}

// Pending is the spool's replies not yet in the work store: those a crash or kill left.
func Pending(spool string) ([]Entry, error) {
	entries, done, err := spoolRead(spool)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		if !done[e.RID] {
			out = append(out, e)
		}
	}
	return out, nil
}

// replyMark is the last line of a site reply's comment: its reply id, so a retry never stores it twice.
func replyMark(rid string) string { return fmt.Sprintf("<!-- pm-reply %s -->", rid) }

// deliverReply stores a spooled reply in the work store effectively once, then marks it done: under the spool's
// delivery lock (so two servers never race on it), it skips a reply marked done, and adds the comment, ending in its
// mark, only if no comment on the need carries that mark yet (a crash between the write and the mark).
func (s *server) deliverReply(e Entry) error {
	lock := strings.TrimSuffix(s.d.Spool, ".jsonl") + ".lock"
	return flocked(lock, func() error {
		_, done, err := spoolRead(s.d.Spool)
		if err != nil {
			return err
		}
		if done[e.RID] {
			return nil
		}
		mark := replyMark(e.RID)
		err = s.withStore("reply "+e.ID, func(st work.Store) error {
			got, err := st.Get(e.ID)
			if err != nil {
				return err
			}
			for _, c := range got[0].Comments {
				if strings.Contains(c.Text, mark) {
					return nil
				}
			}
			_, err = st.Comment(e.ID, work.Reply, ReplyAuthor, e.Text+"\n\n"+mark)
			return err
		})
		if err != nil {
			return err
		}
		return spoolDone(s.d.Spool, e.RID)
	})
}
