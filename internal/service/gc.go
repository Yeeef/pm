package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// The garbage collection: the work store keeps every chunk a write replaced until a collection drops the dead ones
// (bd's store here was 123 MB for 596 KB of data, 47 MB after one), so the service collects it once GCInterval has
// passed since the last collection, online, with GCTimeout, and logs the store's size before and after. It
// runs the store's GC (CALL DOLT_GC()), which deletes no item and squashes no commit. The last outcome is kept in
// .pm/run/gc.json for pm service status.
var (
	GCInterval = 24 * time.Hour
	GCCheck    = 10 * time.Minute // between looks at whether a collection is due; the first is GCCheck after start
	GCTimeout  = 10 * time.Minute
)

// GCState is the last collection's outcome.
type GCState struct {
	At      string `json:"at"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Before  int64  `json:"before"`
	After   int64  `json:"after"`
}

func gcFile(main string) string { return filepath.Join(RunDir(main), "gc.json") }

// ReadGC is the last collection's outcome; nil when none is recorded.
func ReadGC(main string) (*GCState, error) {
	data, err := os.ReadFile(gcFile(main))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s GCState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s is not JSON: %v", gcFile(main), err)
	}
	return &s, nil
}

func writeGC(main string, s GCState) error {
	data, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(RunDir(main), 0o755); err != nil {
		return err
	}
	tmp := gcFile(main) + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, gcFile(main))
}

// GCLine is pm service status's line on the garbage collection.
func GCLine(main string) string {
	s, err := ReadGC(main)
	switch {
	case err != nil:
		return "gc        " + err.Error()
	case s == nil:
		return fmt.Sprintf("gc        no collection recorded yet (the pm service collects every %d h)",
			int(GCInterval.Hours()))
	case s.OK:
		return fmt.Sprintf("gc        ok at %s: %s", s.At, s.Message)
	}
	return fmt.Sprintf("gc        error at %s: %s", s.At, s.Message)
}

// dirSize is the bytes of the regular files under dir.
func dirSize(dir string) (int64, error) {
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			n += info.Size()
		}
		return nil
	})
	return n, err
}

func mb(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/1e6) }

// gcDue is whether the last collection is older than GCInterval, or none is recorded. A failed one counts: it is
// retried after GCInterval, not at every look.
func gcDue(main string, now time.Time) (bool, error) {
	s, err := ReadGC(main)
	if err != nil || s == nil {
		return s == nil && err == nil, err
	}
	at, err := time.Parse(time.RFC3339, s.At)
	if err != nil {
		return true, nil
	}
	return now.Sub(at) >= GCInterval, nil
}

// collect runs one collection and records it.
func (s *server) collect() GCState {
	start := time.Now()
	st := GCState{At: start.UTC().Format(time.RFC3339)}
	before, err := dirSize(s.d.WorkDir)
	if err == nil {
		st.Before = before
		ctx, cancel := context.WithTimeout(context.Background(), GCTimeout)
		err = s.d.GC(ctx)
		cancel()
	}
	if err == nil {
		st.After, err = dirSize(s.d.WorkDir)
	}
	took := time.Since(start).Milliseconds()
	if err != nil {
		st.Message = fmt.Sprintf("%v (after %d ms)", err, took)
	} else {
		st.OK = true
		st.Message = fmt.Sprintf("%s -> %s in %d ms", mb(st.Before), mb(st.After), took)
	}
	if werr := writeGC(s.d.Main, st); werr != nil {
		s.logf("gc: could not record the outcome: %v", werr)
	}
	word := "error"
	if st.OK {
		word = "ok"
	}
	s.logf("gc %s: %s", word, st.Message)
	return st
}

// collector looks every GCCheck whether a collection is due, and runs it.
func (s *server) collector() {
	for {
		if !s.sleep(GCCheck) {
			return
		}
		due, err := gcDue(s.d.Main, time.Now())
		if err != nil {
			s.logf("gc: %v", err)
			continue
		}
		if due {
			s.collect()
		}
	}
}
