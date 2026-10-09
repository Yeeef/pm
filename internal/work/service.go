package work

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// What the pm service needs of the store beside the Store interface: a cheap fingerprint to tell whether to read it
// again, the remote it syncs through, and its garbage collection (the pm-go page, Store sharing).

// Fingerprint is a fingerprint of the store in dir that changes on every write and costs a few stat calls, taken
// without the gate: Dolt appends each write to its chunk journal, so a file grows, and a garbage collection rewrites
// the manifest. Sizes only, because a read touches the files' mtimes; and not the journal's index (journal.idx), a
// cache of the journal that a read may write. The chunk directory's inode too, so a store made anew in its place (a
// new import) is a change even when its sizes match the old one's. It fails when dir holds no store.
func Fingerprint(dir string) (string, error) {
	noms := filepath.Join(dir, dbName, ".dolt", "noms")
	manifest, err := os.ReadFile(filepath.Join(noms, "manifest"))
	if err != nil {
		return "", fmt.Errorf("work store: fingerprint: %w", err)
	}
	st, err := os.Stat(noms)
	if err != nil {
		return "", fmt.Errorf("work store: fingerprint: %w", err)
	}
	var b strings.Builder
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		fmt.Fprintf(&b, "%d\x00", sys.Ino)
	}
	b.Write(manifest)
	var sizes []string
	for _, d := range []string{noms, filepath.Join(noms, "oldgen")} {
		entries, err := os.ReadDir(d)
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("work store: fingerprint: %w", err)
		}
		for _, e := range entries {
			if !e.Type().IsRegular() || e.Name() == journalIndex {
				continue
			}
			info, err := e.Info()
			if os.IsNotExist(err) { // replaced between the listing and the stat: a write, seen on the next look
				continue
			}
			if err != nil {
				return "", fmt.Errorf("work store: fingerprint: %w", err)
			}
			sizes = append(sizes, fmt.Sprintf("%s=%d", filepath.Join(filepath.Base(d), e.Name()), info.Size()))
		}
	}
	sort.Strings(sizes)
	b.WriteString("\x00" + strings.Join(sizes, "\x00"))
	return b.String(), nil
}

// journalIndex is Dolt's index of the chunk journal: derived from the journal, and written by reads too.
const journalIndex = "journal.idx"

// Remote is the git remote URL the store syncs through, and whether it has one.
func (d *Dolt) Remote() (string, bool, error) { return d.remoteURL() }

// GC collects the store's garbage (CALL DOLT_GC()): chunks no commit reaches any more. It deletes no item and
// squashes no commit.
func (d *Dolt) GC(c context.Context) error {
	if d.conn == nil {
		return fmt.Errorf("work store: closed")
	}
	if _, err := d.conn.ExecContext(c, "CALL DOLT_GC()"); err != nil {
		return fmt.Errorf("work store: gc: %w", err)
	}
	return nil
}
