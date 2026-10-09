package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/pyjson"
)

// The managed pieces against Python pm's on one table of inputs: $PM_PARITY/install.json, which
// tests/go_parity_corpus.py writes (make test-go). Each text, plan, drift line, rewrite, removal and Codex edit must be
// byte-equal; a refusal must be one too, but for the JSON or TOML parser's own words inside it.

type reference struct {
	Version  string  `json:"version"`
	Settings [][]any `json:"settings"`
	Pieces   []struct {
		Settings int     `json:"settings"`
		Rel      string  `json:"rel"`
		Text     *string `json:"text"`
		Present  any     `json:"present"`
		Apply    string  `json:"apply"`
		Remove   any     `json:"remove"`
		Drift    []string
	} `json:"pieces"`
	Trees []struct {
		Settings int               `json:"settings"`
		Files    map[string]string `json:"files"`
		Plan     [][2]string       `json:"plan"`
		Drift    []string          `json:"drift"`
		Rewrite  [][2]string       `json:"rewrite"`
		Removals [][2]*string      `json:"removals"`
	} `json:"trees"`
	CodexAdd    []codexCase `json:"codex_add"`
	CodexRemove []codexCase `json:"codex_remove"`
}

type codexCase struct {
	Text   string   `json:"text"`
	Roots  []string `json:"roots"`
	Result string   `json:"result"`
}

func loadReference(t *testing.T) reference {
	dir := os.Getenv("PM_PARITY")
	if dir == "" {
		t.Skip("PM_PARITY is unset: make test-go writes Python pm's reference and sets it")
	}
	b, err := os.ReadFile(filepath.Join(dir, "install.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ref reference
	if err := json.Unmarshal(b, &ref); err != nil {
		t.Fatal(err)
	}
	old := buildinfo.Version
	buildinfo.Version = ref.Version
	t.Cleanup(func() { buildinfo.Version = old })
	return ref
}

func settingsAt(ref reference, n int) Settings {
	s := ref.Settings[n]
	return Settings{Remote: s[0].(string), MainBranch: s[1].(string), Port: int(s[2].(float64)), SiteURL: s[3].(string)}
}

// parserWords is a refusal's parser message, which Python's json and tomllib word differently from Go's.
var parserWords = regexp.MustCompile(`(is not valid JSON|would leave it invalid TOML) \(.*?\); |(cannot parse [^:]*): .*?; fix`)

func same(v string) string { return parserWords.ReplaceAllString(v, "$1$2 (…); fix") }

// What differs from Python pm 0.1.x by design (the work-store page, Cut-over), and no more: pm's git hook files live in
// .pm/hooks, not in Beads' .beads/hooks (renamed: Python's path and every text naming it), and Go pm takes Beads' hook
// entries out of the settings files it plans (withoutBeads) and names each one left in pm doctor (beadsLine). Each
// piece's own Present, Apply, Part and Remove are unchanged; setup_test.go holds the Beads removal itself.

func renamed(s string) string { return strings.ReplaceAll(s, ".beads/hooks/", HooksRel+"/") }

var beadsLine = regexp.MustCompile(`^[^:]+: holds .*, which pm \S+ removes$`)

// withoutBeadsLines is Go's drift lines without those naming a Beads piece left, which Python pm keeps.
func withoutBeadsLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !beadsLine.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// withoutBeads is a settings file without the hook entries whose command starts with "bd ", a group or event that
// empties gone with them; any other text as it is.
func withoutBeads(text string) string {
	v, err := pyjson.Loads(text)
	if err != nil {
		return text
	}
	data, ok := v.(*pyjson.Object)
	if !ok {
		return text
	}
	events, ok := data.Get("hooks").(*pyjson.Object)
	if !ok {
		return text
	}
	changed := false
	for _, e := range append([]string(nil), events.Keys...) {
		gs, _ := events.Get(e).([]any)
		kept := []any{}
		for _, g := range gs {
			o := g.(*pyjson.Object)
			hs, _ := o.Get("hooks").([]any)
			others := []any{}
			for _, h := range hs {
				if c, _ := h.(*pyjson.Object).Get("command").(string); strings.HasPrefix(c, "bd ") {
					continue
				}
				others = append(others, h)
			}
			if len(others) == len(hs) {
				kept = append(kept, g)
				continue
			}
			changed = true
			if len(others) > 0 {
				o.Set("hooks", others)
				kept = append(kept, g)
			}
		}
		if len(kept) == len(gs) {
			continue
		}
		if len(kept) == 0 {
			events.Delete(e)
		} else {
			events.Set(e, kept)
		}
	}
	if !changed {
		return text
	}
	if len(events.Keys) == 0 {
		data.Delete("hooks")
	}
	return DumpJSON(data)
}

func result(s string, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	return s
}

func TestPiecesMatchPython(t *testing.T) {
	ref := loadReference(t)
	for i, c := range ref.Pieces {
		var p Piece
		for _, q := range Pieces(settingsAt(ref, c.Settings)) {
			if q.Rel == renamed(c.Rel) {
				p = q
			}
		}
		if p.Rel == "" {
			t.Fatalf("Go pm has no piece %s", renamed(c.Rel))
		}
		c.Rel = p.Rel
		c.Apply = renamed(c.Apply)
		if s, ok := c.Remove.(string); ok {
			c.Remove = renamed(s)
		}
		if s, ok := c.Present.(string); ok {
			c.Present = renamed(s)
		}
		for j := range c.Drift {
			c.Drift[j] = renamed(c.Drift[j])
		}
		present, err := p.Present(c.Text)
		var gotPresent any = present
		if err != nil {
			gotPresent = "error: " + err.Error()
		}
		if same(toString(gotPresent)) != same(toString(c.Present)) {
			t.Errorf("case %d %s %q: present %v, Python %v", i, c.Rel, deref(c.Text), gotPresent, c.Present)
		}
		if got := result(p.Apply(c.Text)); same(got) != same(c.Apply) {
			t.Errorf("case %d %s %q: apply\n%q\nPython\n%q", i, c.Rel, deref(c.Text), got, c.Apply)
		}
		if c.Text != nil {
			n, err := p.Remove(*c.Text)
			var got any
			switch {
			case err != nil:
				got = "error: " + err.Error()
			case n != nil:
				got = *n
			}
			if same(toString(got)) != same(toString(c.Remove)) {
				t.Errorf("case %d %s %q: remove %q, Python %q", i, c.Rel, *c.Text, got, c.Remove)
			}
		}
		top := t.TempDir()
		if c.Text != nil {
			path := filepath.Join(top, c.Rel)
			os.MkdirAll(filepath.Dir(path), 0o755)
			os.WriteFile(path, []byte(*c.Text), 0o644)
		}
		lines, err := Drift(top, settingsAt(ref, c.Settings))
		if err != nil {
			t.Fatal(err)
		}
		var mine []string
		for _, l := range withoutBeadsLines(lines) {
			if len(l) > len(c.Rel) && l[:len(c.Rel)+1] == c.Rel+":" {
				mine = append(mine, same(l))
			}
		}
		var want []string
		for _, l := range c.Drift {
			want = append(want, same(l))
		}
		if !reflect.DeepEqual(mine, want) {
			t.Errorf("case %d %s %q: drift %q, Python %q", i, c.Rel, deref(c.Text), mine, want)
		}
	}
}

func TestTreesMatchPython(t *testing.T) {
	ref := loadReference(t)
	for i, c := range ref.Trees {
		s := settingsAt(ref, c.Settings)
		top := t.TempDir()
		files := map[string]string{}
		for rel, text := range c.Files {
			files[renamed(rel)] = text
			path := filepath.Join(top, renamed(rel))
			os.MkdirAll(filepath.Dir(path), 0o755)
			os.WriteFile(path, []byte(text), 0o644)
		}
		pairs := func(planned []Planned, err error) [][2]string {
			if err != nil {
				t.Fatal(err)
			}
			out := [][2]string{}
			for _, p := range planned {
				out = append(out, [2]string{p.Piece.Rel, p.New})
			}
			return out
		}
		// Python's pairs, renamed, each settings file without Beads' entries, and a settings file Python leaves as it is
		// but for those entries planned too
		python := func(ps [][2]string) [][2]string {
			out := [][2]string{}
			for _, p := range nonNil(ps) {
				out = append(out, [2]string{renamed(p[0]), withoutBeads(p[1])})
			}
			for _, rel := range leftoverRels {
				text, ok := files[rel]
				listed := false
				for _, p := range out {
					listed = listed || p[0] == rel
				}
				if ok && !listed && withoutBeads(text) != text {
					out = append(out, [2]string{rel, withoutBeads(text)})
				}
			}
			sort.Slice(out, func(a, b int) bool { return out[a][0] < out[b][0] })
			return out
		}
		sorted := func(ps [][2]string) [][2]string {
			sort.Slice(ps, func(a, b int) bool { return ps[a][0] < ps[b][0] })
			return ps
		}
		if got, want := sorted(pairs(Plan(top, s))), python(c.Plan); !reflect.DeepEqual(got, want) {
			t.Errorf("tree %d: plan %q, Python %q", i, got, want)
		}
		if got, want := sorted(pairs(Rewrite(top, s))), python(c.Rewrite); !reflect.DeepEqual(got, want) {
			t.Errorf("tree %d: rewrite %q, Python %q", i, got, want)
		}
		drift, err := Drift(top, s)
		if err != nil {
			t.Fatal(err)
		}
		drift = withoutBeadsLines(drift)
		for j := range c.Drift {
			c.Drift[j] = renamed(c.Drift[j])
		}
		if !reflect.DeepEqual(drift, c.Drift) && !(len(drift) == 0 && len(c.Drift) == 0) {
			t.Errorf("tree %d: drift %q, Python %q", i, drift, c.Drift)
		}
		for _, r := range c.Removals {
			*r[0] = renamed(*r[0])
		}
		removals, err := Removals(top, s)
		if err != nil {
			t.Fatal(err)
		}
		got := [][2]*string{}
		for _, r := range removals {
			rel, _ := filepath.Rel(top, r.Path)
			got = append(got, [2]*string{ptr(filepath.ToSlash(rel)), r.New})
		}
		if toJSON(got) != toJSON(nonNilP(c.Removals)) {
			t.Errorf("tree %d: removals %s, Python %s", i, toJSON(got), toJSON(c.Removals))
		}
	}
}

func TestCodexEditsMatchPython(t *testing.T) {
	ref := loadReference(t)
	for i, c := range ref.CodexAdd {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(c.Text), 0o644)
		got := func() string {
			text, _, have, err := CodexConfig(path)
			if err != nil {
				return "error: " + err.Error()
			}
			var add []string
			for _, r := range c.Roots {
				if !contains(have, r) {
					add = append(add, r)
				}
			}
			if len(add) == 0 {
				return text
			}
			return result(AddCodexRoots(path, text, have, add))
		}()
		if same(repath(got, path)) != same(repath(c.Result, "")) {
			t.Errorf("add %d %q %q:\n%q\nPython\n%q", i, c.Text, c.Roots, got, c.Result)
		}
	}
	for i, c := range ref.CodexRemove {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(c.Text), 0o644)
		got := result(RemoveCodexRoots(path, c.Roots))
		if same(repath(got, path)) != same(repath(c.Result, "")) {
			t.Errorf("remove %d %q %q:\n%q\nPython\n%q", i, c.Text, c.Roots, got, c.Result)
		}
	}
}

// repath is a refusal with its temp config path as one word, since each side ran in its own temp dir.
var tempConfig = regexp.MustCompile(`/\S*/config\.toml`)

func repath(s, _ string) string { return tempConfig.ReplaceAllString(s, "<config>") }

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func toString(v any) string { return toJSON(v) }

func toJSON(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func nonNil(v [][2]string) [][2]string {
	if v == nil {
		return [][2]string{}
	}
	return v
}

func nonNilP(v [][2]*string) [][2]*string {
	if v == nil {
		return [][2]*string{}
	}
	return v
}
