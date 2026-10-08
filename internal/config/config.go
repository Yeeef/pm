// Package config reads the repo's pm settings: .pm/config.toml in the main checkout, tracked, pinning the pm version
// every session runs. Every pm command reads it first and fails hard when it is missing, malformed, or pins a version
// other than the one running; nothing falls back to a default. Python source: config.py.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Yeeef/yeeef-agents/pm/internal/buildinfo"
	"github.com/Yeeef/yeeef-agents/pm/internal/proc"
)

const (
	Rel   = ".pm/config.toml"
	Store = ".pm/store/records" // the records store, under the main checkout
	Run   = ".pm/run"           // runtime state in the main checkout, never committed
	Repo  = "https://github.com/Yeeef/yeeef-agents"
)

// keys are the config's keys and the Python type each must have, in KEYS order.
var keys = map[string]string{"version": "str", "remote": "str", "main_branch": "str", "port": "int", "site_url": "str"}
var required = []string{"version", "remote", "main_branch", "port"}

// Error is a config that is missing, malformed, or pins another pm version.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

// Config is .pm/config.toml, checked.
type Config struct {
	Path       string
	Version    string
	Remote     string
	MainBranch string
	Port       int64
	SiteURL    string // "" when unset: links use http://localhost:<port>
}

// Root is the checkout whose config applies: cwd's worktree, or the main checkout from inside the store (the records
// branch carries no config).
func Root(cwd string) (string, error) {
	res, err := proc.Run([]string{"git", "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir"},
		proc.Options{Cwd: &cwd})
	if err != nil {
		return "", &Error{fmt.Sprintf("git did not run, so pm cannot find this repo's %s: %s", Rel, err)}
	}
	if res.Code != 0 {
		why := res.Stderr
		if why == "" {
			why = res.Stdout
		}
		return "", &Error{fmt.Sprintf("%s is not in a git worktree: %s", cwd, PyStrip(why))}
	}
	lines := strings.Split(res.Stdout, "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("git rev-parse printed %q, not a top level and a common dir", res.Stdout)
	}
	top, common := resolve(lines[0]), resolve(lines[1])
	if filepath.Base(common) == ".git" && top == filepath.Join(filepath.Dir(common), Store) {
		return filepath.Dir(common), nil
	}
	return top, nil
}

// resolve is Python's Path.resolve(): symlinks followed where the path exists.
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// Load is the config of the checkout containing cwd, checked against the running pm's version.
func Load(cwd string) (Config, error) {
	c, err := Read(cwd)
	if err != nil {
		return c, err
	}
	if v := buildinfo.Version; c.Version != v {
		return c, &Error{fmt.Sprintf("this repo pins pm %s in %s, but pm %s is running, launched for that pin: release "+
			"tag pm-v%s at %s builds pm %s; fix the tag, or move the pin to %s with pm upgrade --to %s",
			c.Version, c.Path, v, c.Version, Repo, v, v, v)}
	}
	return c, nil
}

// Read is the config of the checkout containing cwd, checked for its keys but not its pin: pm upgrade moves it.
func Read(cwd string) (Config, error) {
	root, err := Root(cwd)
	if err != nil {
		return Config{}, err
	}
	path := filepath.Join(root, Rel)
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return Config{}, &Error{fmt.Sprintf("this repo has no %s (looked for %s); create it with pm init", Rel, path)}
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	data := map[string]any{}
	meta, err := toml.Decode(string(text), &data)
	if err != nil {
		var perr toml.ParseError
		if errors.As(err, &perr) {
			return Config{}, &Error{fmt.Sprintf("%s is not valid TOML: %s", path, perr.Message)}
		}
		return Config{}, &Error{fmt.Sprintf("%s is not valid TOML: %s", path, err)}
	}
	var unknown, missing, wrong []string
	for k := range data {
		if _, ok := keys[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown) // Python sorts by code point, as Go compares strings
	for _, k := range required {
		if _, ok := data[k]; !ok {
			missing = append(missing, k)
		}
	}
	for _, key := range meta.Keys() { // in file order, as the Python dict keeps them
		if len(key) != 1 {
			continue
		}
		k := key[0]
		if want, ok := keys[k]; ok && pyType(data[k]) != want {
			wrong = append(wrong, fmt.Sprintf("%s (want %s)", k, want))
		}
	}
	if len(unknown)+len(missing)+len(wrong) > 0 {
		var parts []string
		if len(unknown) > 0 {
			parts = append(parts, "unknown keys "+strings.Join(unknown, ", "))
		}
		if len(missing) > 0 {
			parts = append(parts, "missing keys "+strings.Join(missing, ", "))
		}
		if len(wrong) > 0 {
			parts = append(parts, "wrong types for "+strings.Join(wrong, ", "))
		}
		return Config{}, &Error{fmt.Sprintf("%s: %s", path, strings.Join(parts, "; "))}
	}
	site, _ := data["site_url"].(string)
	return Config{Path: path, Version: data["version"].(string), Remote: data["remote"].(string),
		MainBranch: data["main_branch"].(string), Port: data["port"].(int64),
		SiteURL: strings.TrimRight(PyStrip(site), "/")}, nil
}

// pyType is the name of the Python type tomllib gives a TOML value, for the two types the config checks.
func pyType(v any) string {
	switch v.(type) {
	case string:
		return "str"
	case int64:
		return "int"
	}
	return "other"
}

// PyStrip is Python's str.strip(): it removes the characters str.isspace() names, which include \x1c-\x1f.
func PyStrip(s string) string {
	return strings.TrimFunc(s, IsPySpace)
}

// IsPySpace is Python's str.isspace() for one character.
func IsPySpace(r rune) bool {
	switch {
	case r >= 0x09 && r <= 0x0d, r >= 0x1c && r <= 0x20, r == 0x85, r == 0xa0, r == 0x1680, r >= 0x2000 && r <= 0x200a,
		r == 0x2028, r == 0x2029, r == 0x202f, r == 0x205f, r == 0x3000:
		return true
	}
	return false
}
