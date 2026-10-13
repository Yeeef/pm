// Package fsutil holds the filesystem predicates shared by commands and setup.
package fsutil

import (
	"os"
	"path/filepath"
)

// Exists is Python's Path.exists(): symlinks followed.
func Exists(p string) bool { _, err := os.Stat(p); return err == nil }

// IsLink is Python's Path.is_symlink().
func IsLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

// IsDir is Python's Path.is_dir(): symlinks followed.
func IsDir(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

// Resolved is Python's Path.resolve() for a path that may not exist yet: existing symlinks followed, a nonexistent
// path's ancestors included.
func Resolved(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return filepath.Join(Resolved(filepath.Dir(abs)), filepath.Base(abs))
}
