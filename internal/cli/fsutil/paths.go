// Package fsutil holds the filesystem predicates shared by commands and setup.
package fsutil

import (
	"os"
	"path/filepath"
)

func Exists(p string) bool { _, err := os.Stat(p); return err == nil }
func IsLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}
func IsDir(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

// Resolved follows existing symlinks, including a nonexistent path's ancestors.
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
