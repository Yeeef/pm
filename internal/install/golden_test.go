package install

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
)

// The managed pieces on one table of inputs: each piece's Present, Apply, Remove and pm doctor line on each text its
// file may hold; Plan, Drift, Rewrite and Removals on a tree of the user's files; the Codex config edits. The results
// are frozen in testdata/pieces.json; after an intended change, rewrite it with
//
//	go test -tags gms_pure_go ./internal/install -run Golden -update
//
// and review its diff in the PR.

var update = flag.Bool("update", false, "rewrite testdata/pieces.json from the results now")

var goldenSettings = []Settings{{"origin", "main", 8123, ""}, {"upstream", "trunk", 8000, "https://pm.example.com"}}

// Inputs, mostly from the setup tests' fixtures: Beads' hook file with the repo's own lines, settings files of the
// user's, an unterminated .gitignore, an older pm's parts, and the broken forms pm refuses.
const beadsHook = "#!/usr/bin/env sh\n# --- BEGIN BEADS INTEGRATION v1.3.1 ---\n# beads' part\n# --- END BEADS INTEGRATION v1.3.1 ---\n"

const userSettings = `{"permissions": {"allow": ["Bash(ls:*)"]}, "model": "café", "hooks": {"SessionStart": [{"hooks": ` +
	`[{"command": "bd prime --hook-json", "type": "command"}], "matcher": ""}], "PreToolUse": [{"hooks": [{"command": ` +
	`"./lint.sh", "type": "command"}], "matcher": "Bash"}], "Stop": [{"hooks": [{"command": "pm hook stop", "type": ` +
	`"command"}, {"command": "./mine.sh", "type": "command"}]}]}}` + "\n"

const userSettingsIndented = `{
  "permissions": {
    "allow": [
      "Bash(ls:*)"
    ]
  },
  "model": "café",
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "command": "bd prime --hook-json",
            "type": "command"
          }
        ],
        "matcher": ""
      }
    ],
    "PreToolUse": [
      {
        "hooks": [
          {
            "command": "./lint.sh",
            "type": "command"
          }
        ],
        "matcher": "Bash"
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "command": "pm hook stop",
            "type": "command"
          },
          {
            "command": "./mine.sh",
            "type": "command"
          }
        ]
      }
    ]
  }
}
`

func sp(s string) *string { return &s }

var goldenTexts = map[string][]*string{
	"settings": {nil, sp(""), sp("{}\n"), sp(userSettingsIndented), sp(userSettings), sp("{not json"), sp("[]\n"),
		sp("{\n  \"hooks\": []\n}\n"),
		sp("{\n  \"hooks\": {\n    \"Stop\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n" +
			"            \"command\": \"pm hook stop || exit 1\"\n          }\n        ]\n      }\n    ]\n  },\n  \"n\": 1.0,\n" +
			"  \"big\": 100000000000000000000\n}\n")},
	"hook": {nil, sp(""), sp("#!/usr/bin/env sh\n"), sp(beadsHook), sp(beadsHook + "\n# mine\necho done\n"),
		sp(strings.TrimRight(beadsHook, "\n")), sp("#!/bin/sh\necho mine"),
		sp(beadsHook + "# --- BEGIN PM v0.0.1 ---\npm hook git-pre-commit \"$@\" || exit $?\n# --- END PM ---\nmine\n"),
		sp(beadsHook + "# --- BEGIN PM v0.0.1 ---\nno end\n")},
	"gitignore": {nil, sp(""), sp("*.log\nbuild/"), sp("*.log\n"), sp("# --- BEGIN PM ---\n/records/\n# --- END PM ---\nkeep\n"),
		sp("# --- BEGIN PM ---\nno end\n")},
	"whole": {nil, sp(""), sp("old\n")},
}

var goldenCodexTexts = []string{"", "# mine\nmodel = \"o3\"\n",
	"# mine\nmodel = \"o3\"\n\n[sandbox_workspace_write]\nnetwork_access = true\n",
	"[sandbox_workspace_write]\nwritable_roots = [\"/x\"]\n",
	"[sandbox_workspace_write]  # sandbox\nwritable_roots = [ ]\n\n[other]\na = 1\n",
	"sandbox_workspace_write.network_access = true\n", "[sandbox_workspace_write]\nwritable_roots = \"x\"\n",
	"[sandbox_workspace_write]\nwritable_roots = [\n  \"/x\",\n  \"/a\",\n]\n", "not toml ["}

func kindOf(rel string) string {
	switch rel {
	case ".claude/settings.json", ".codex/hooks.json":
		return "settings"
	case HooksRel + "/post-checkout", HooksRel + "/pre-commit":
		return "hook"
	case ".gitignore":
		return "gitignore"
	}
	return "whole"
}

type pieceCase struct {
	Settings int      `json:"settings"`
	Rel      string   `json:"rel"`
	Text     *string  `json:"text"`
	Present  string   `json:"present"`
	Apply    string   `json:"apply"`
	Remove   *string  `json:"remove"`
	Drift    []string `json:"drift"`
}

type treeCase struct {
	Settings int               `json:"settings"`
	Files    map[string]string `json:"files"`
	Plan     [][2]string       `json:"plan"`
	Drift    []string          `json:"drift"`
	Rewrite  [][2]string       `json:"rewrite"`
	Removals [][2]*string      `json:"removals"`
}

type codexCase struct {
	Text   string   `json:"text"`
	Roots  []string `json:"roots"`
	Add    string   `json:"add"`
	Remove string   `json:"remove"`
}

type golden struct {
	Pieces []pieceCase `json:"pieces"`
	Trees  []treeCase  `json:"trees"`
	Codex  []codexCase `json:"codex"`
}

func result(s string, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	return s
}

// tempConfig is a refusal's temp config path, which differs on each run.
var tempConfig = regexp.MustCompile(`/\S*/config\.toml`)

func pairs(t *testing.T, how func(string, Settings) ([]Planned, error), top string, s Settings) [][2]string {
	planned, err := how(top, s)
	if err != nil {
		t.Fatal(err)
	}
	out := [][2]string{}
	for _, p := range planned {
		out = append(out, [2]string{p.Piece.Rel, p.New})
	}
	sort.Slice(out, func(a, b int) bool { return out[a][0] < out[b][0] })
	return out
}

func results(t *testing.T) golden {
	var g golden
	for n, s := range goldenSettings {
		for _, p := range Pieces(s) {
			for _, text := range goldenTexts[kindOf(p.Rel)] {
				c := pieceCase{Settings: n, Rel: p.Rel, Text: text, Drift: []string{}}
				present, err := p.Present(text)
				c.Present = result(map[bool]string{true: "true", false: "false"}[present], err)
				c.Apply = result(p.Apply(text))
				if text != nil {
					removed, err := p.Remove(*text)
					if err != nil {
						removed = sp("error: " + err.Error())
					}
					c.Remove = removed
				}
				top := t.TempDir()
				if text != nil {
					writeTree(t, top, map[string]string{p.Rel: *text})
				}
				lines, err := Drift(top, s)
				if err != nil {
					t.Fatal(err)
				}
				for _, l := range lines {
					if strings.HasPrefix(l, p.Rel+":") {
						c.Drift = append(c.Drift, l)
					}
				}
				g.Pieces = append(g.Pieces, c)
			}
		}
		for tree := 0; tree < 3; tree++ { // nothing; the user's files; the user's files with pm installed
			top := t.TempDir()
			files := map[string]string{}
			if tree > 0 {
				files = map[string]string{".claude/settings.json": userSettingsIndented,
					HooksRel + "/pre-commit": beadsHook + "\n# mine\necho done\n", ".gitignore": "*.log\nbuild/",
					".pm/README.md": "old\n"}
			}
			writeTree(t, top, files)
			if tree == 2 {
				rewrite, err := Rewrite(top, s)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range rewrite {
					files[p.Piece.Rel] = p.New
				}
				writeTree(t, top, files)
			}
			c := treeCase{Settings: n, Files: files, Plan: pairs(t, Plan, top, s), Rewrite: pairs(t, Rewrite, top, s),
				Removals: [][2]*string{}}
			drift, err := Drift(top, s)
			if err != nil {
				t.Fatal(err)
			}
			c.Drift = append([]string{}, drift...)
			removals, err := Removals(top, s)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range removals {
				rel, _ := filepath.Rel(top, r.Path)
				c.Removals = append(c.Removals, [2]*string{sp(filepath.ToSlash(rel)), r.New})
			}
			g.Trees = append(g.Trees, c)
		}
	}
	for _, text := range goldenCodexTexts {
		for _, roots := range [][]string{{"/a b/.git", "/é/store"}, {"/x"}} {
			path := filepath.Join(t.TempDir(), "config.toml")
			write := func() {
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write()
			add := func() string { // pm init adds only what is missing, and nothing when nothing is
				t, _, have, err := CodexConfig(path)
				if err != nil {
					return "error: " + err.Error()
				}
				var missing []string
				for _, r := range roots {
					if !contains(have, r) {
						missing = append(missing, r)
					}
				}
				if len(missing) == 0 {
					return t
				}
				return result(AddCodexRoots(path, t, have, missing))
			}()
			write()
			remove := result(RemoveCodexRoots(path, roots))
			g.Codex = append(g.Codex, codexCase{Text: text, Roots: roots,
				Add: tempConfig.ReplaceAllString(add, "<config>"), Remove: tempConfig.ReplaceAllString(remove, "<config>")})
		}
	}
	return g
}

func TestPiecesEqualTheGoldenResults(t *testing.T) {
	old := buildinfo.Version
	buildinfo.Version = "0.9.9" // the version pm's parts name
	t.Cleanup(func() { buildinfo.Version = old })
	b, err := json.MarshalIndent(results(t), "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got := string(b) + "\n"
	file := filepath.Join("testdata", "pieces.json")
	if *update {
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("the pieces' results differ from %s (- golden, + now):\n%s", file, lineDiff(string(want), got))
	}
}

// lineDiff is the lines of a and b from the first that differs to the last, each marked.
func lineDiff(a, b string) string {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	var out []string
	for _, l := range x[pre : len(x)-suf] {
		out = append(out, "- "+l)
	}
	for _, l := range y[pre : len(y)-suf] {
		out = append(out, "+ "+l)
	}
	return strings.Join(out, "\n")
}
