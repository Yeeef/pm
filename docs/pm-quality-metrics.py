#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""pm code and test metrics: measure one commit of pm, then draw the figures and print the tables.

    uv run pm-quality-metrics.py measure REPO DATA [STEP ...]
    uv run pm-quality-metrics.py draw DATA FIGDIR > tables.md

REPO is a detached worktree of the commit to measure (git worktree add --detach <dir> origin/main); DATA holds one
JSON file per step. Steps, in the order `measure` runs them when none is named:
size, complexity, lint, tests, runtime, coverage, ci, trend. `runtime` runs make test and make test-go, so run it
on a quiet machine; `coverage` patches cmd/pm/main.go for its build (a SIGTERM handler that calls os.Exit, so a
killed `pm service run` writes its counters) and restores it after.
Needs on PATH: go, uv, git, gh (logged in, for ci), golangci-lint 2.14.0 (complexity, lint, trend).
"""
import collections
import concurrent.futures
import io
import json
import math
import os
import re
import resource
import shutil
import statistics
import subprocess
import sys
import tempfile
import threading
import time
import tokenize
from datetime import datetime, timezone
from pathlib import Path

MODULE = "github.com/Yeeef/pm"
GH_REPO = "Yeeef/pm"
LINT_VERSION = "2.14.0"
RATCHET = "errcheck,gocyclo,gocognit,funlen,revive,reassign"
GO_ENV = {"CGO_ENABLED": "1"}
CORES = os.cpu_count()


def run(cmd, cwd=None, env=None, check=True, **kw):
    e = dict(os.environ, **GO_ENV, **(env or {}))
    p = subprocess.run(cmd, cwd=cwd, env=e, capture_output=True, text=True, **kw)
    if check and p.returncode != 0:
        sys.exit(f"{' '.join(map(str, cmd))} failed ({p.returncode}):\n{p.stdout[-3000:]}\n{p.stderr[-3000:]}")
    return p


# ---------------------------------------------------------------- Go source: code lines, functions, tests

ANALYZER = r'''package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Func struct {
	Name  string `json:"name"`
	Line  int    `json:"line"`
	Lines int    `json:"lines"`
	Code  int    `json:"code"`
}

type File struct {
	Path    string   `json:"path"`
	Lines   int      `json:"lines"`
	Code    int      `json:"code"`
	Test    bool     `json:"test"`
	Funcs   []Func   `json:"funcs"`
	Tests   []string `json:"tests"`
	Benches int      `json:"benches"`
	Fuzz    int      `json:"fuzz"`
	Runs    int      `json:"runs"`
}

func main() {
	root := os.Args[1]
	files := []File{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files = append(files, measure(rel, src))
		return nil
	})
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(files); err != nil {
		panic(err)
	}
}

// measure counts a file's code lines (a line holding a token that is not a comment) and its functions' code lines.
func measure(rel string, src []byte) File {
	fset := token.NewFileSet()
	tf := fset.AddFile(rel, -1, len(src))
	var s scanner.Scanner
	s.Init(tf, src, nil, scanner.ScanComments)
	code := map[int]bool{}
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT || (tok == token.SEMICOLON && lit == "\n") {
			continue
		}
		a, b := tf.Line(pos), tf.Line(pos)
		if n := len(lit); n > 1 {
			b = tf.Line(pos + token.Pos(n-1))
		}
		for l := a; l <= b; l++ {
			code[l] = true
		}
	}
	f := File{Path: rel, Lines: tf.LineCount(), Code: len(code), Test: strings.HasSuffix(rel, "_test.go"),
		Funcs: []Func{}, Tests: []string{}}
	fs2 := token.NewFileSet()
	af, err := parser.ParseFile(fs2, rel, src, 0)
	if err != nil {
		return f
	}
	for _, d := range af.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		a, b := fs2.Position(fd.Pos()).Line, fs2.Position(fd.End()).Line
		n := 0
		for l := a; l <= b; l++ {
			if code[l] {
				n++
			}
		}
		name := fd.Name.Name
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			name = "(" + recv(fd.Recv.List[0].Type) + ")." + name
		}
		f.Funcs = append(f.Funcs, Func{name, a, b - a + 1, n})
		if f.Test && fd.Recv == nil {
			switch {
			case is(fd, "Test", "T"):
				f.Tests = append(f.Tests, fd.Name.Name)
			case is(fd, "Benchmark", "B"):
				f.Benches++
			case is(fd, "Fuzz", "F"):
				f.Fuzz++
			}
		}
	}
	ast.Inspect(af, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if se, ok := c.Fun.(*ast.SelectorExpr); ok && se.Sel.Name == "Run" && len(c.Args) == 2 {
				if _, ok := c.Args[1].(*ast.FuncLit); ok {
					f.Runs++
				}
			}
		}
		return true
	})
	return f
}

func recv(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return "*" + recv(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return recv(t.X)
	case *ast.IndexListExpr:
		return recv(t.X)
	}
	return "?"
}

// is reports whether fd is a go test function of the kind prefix takes (TestX(t *testing.T) and the like).
func is(fd *ast.FuncDecl, prefix, typ string) bool {
	name := fd.Name.Name
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if rest := name[len(prefix):]; rest != "" {
		if r, _ := utf8.DecodeRuneInString(rest); unicode.IsLower(r) {
			return false
		}
	}
	ps := fd.Type.Params.List
	if len(ps) != 1 || len(ps[0].Names) > 1 {
		return false
	}
	st, ok := ps[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	se, ok := st.X.(*ast.SelectorExpr)
	return ok && se.Sel.Name == typ
}
'''

_ANALYZER_DIR = None
_ANALYZER_LOCK = threading.Lock()


def go_files(repo):
    """Every .go file of repo (outside dot dirs, testdata, vendor): its lines, code lines, functions and tests."""
    global _ANALYZER_DIR
    with _ANALYZER_LOCK:
        if _ANALYZER_DIR is None:
            d = tempfile.mkdtemp(prefix="pm-metrics-go-")
            Path(d, "go.mod").write_text("module pmmetrics\n\ngo 1.22\n")
            Path(d, "main.go").write_text(ANALYZER)
            run(["go", "build", "-o", "analyzer", "."], cwd=d)
            _ANALYZER_DIR = d
    return json.loads(run([f"{_ANALYZER_DIR}/analyzer", str(Path(repo).resolve())]).stdout)


def py_code_lines(path):
    """Lines of a Python file that hold a token other than a comment or layout (docstrings count as code)."""
    lines, skip = set(), {tokenize.COMMENT, tokenize.NL, tokenize.NEWLINE, tokenize.INDENT, tokenize.DEDENT,
                          tokenize.ENCODING, tokenize.ENDMARKER}
    with open(path, "rb") as f:
        for t in tokenize.tokenize(f.readline):
            if t.type not in skip:
                lines.update(range(t.start[0], t.end[0] + 1))
    return len(lines)


def py_files(repo, rel_dirs):
    out = []
    for d in rel_dirs:
        for p in sorted(Path(repo, d).rglob("*.py")):
            if any(part.startswith(".") or part == "__pycache__" for part in p.relative_to(repo).parts):
                continue
            src = p.read_bytes()
            tests = len(re.findall(rb"(?m)^(?:async )?def test_|^    def test_", src))
            out.append(dict(path=str(p.relative_to(repo)), lines=src.count(b"\n"), code=py_code_lines(p),
                            tests=tests))
    return out


def package_of(path):
    d = os.path.dirname(path)
    return d or "(root) pm"


def size_of(repo):
    """Non-test and test code lines per Go package, the Python harness and tools, the largest files and functions."""
    gf = go_files(repo)
    pkgs = collections.defaultdict(lambda: dict(code=0, test=0, files=0, test_files=0, lines=0, test_lines=0))
    for f in gf:
        p = pkgs[package_of(f["path"])]
        if f["test"]:
            p["test"] += f["code"]; p["test_files"] += 1; p["test_lines"] += f["lines"]
        else:
            p["code"] += f["code"]; p["files"] += 1; p["lines"] += f["lines"]
    harness = py_files(repo, ["tests"])
    tools = py_files(repo, ["release", ".github/scripts"])
    funcs = [dict(f, path=x["path"], test=x["test"]) for x in gf for f in x["funcs"]]
    return dict(
        packages=dict(pkgs),
        harness=dict(files=len(harness), code=sum(h["code"] for h in harness), lines=sum(h["lines"] for h in harness),
                     test_files=sum(1 for h in harness if h["tests"]), by_file=harness),
        tools=dict(files=len(tools), code=sum(h["code"] for h in tools), lines=sum(h["lines"] for h in tools),
                   by_file=tools),
        largest_files=sorted(({k: f[k] for k in ("path", "lines", "code", "test")} for f in gf),
                             key=lambda f: -f["code"])[:15],
        largest_funcs=sorted(funcs, key=lambda f: -f["code"])[:15],
        func_count=sum(1 for f in funcs if not f["test"]),
        test_func_count=sum(1 for f in funcs if f["test"]),
        func_code=[f["code"] for f in funcs if not f["test"]],
        go_tests=dict(top_level=sum(len(f["tests"]) for f in gf), benchmarks=sum(f["benches"] for f in gf),
                      fuzz=sum(f["fuzz"] for f in gf), t_run_calls=sum(f["runs"] for f in gf),
                      by_package=dict(sum((collections.Counter({package_of(f["path"]): len(f["tests"])}) for f in gf),
                                          collections.Counter()))),
    )


# ---------------------------------------------------------------- golangci-lint

def lint_json(repo, config_text=None, args=(), env=None):
    """golangci-lint's issues on repo, with the repo's config or the one given."""
    with tempfile.TemporaryDirectory() as tmp:
        out = Path(tmp, "out.json")
        cmd = ["golangci-lint", "run", f"--output.json.path={out}", "--output.text.path=/dev/null",
               "--show-stats=false", "--allow-parallel-runners", *args]
        if config_text:
            Path(tmp, "golangci.yml").write_text(config_text)
            cmd[2:2] = ["-c", f"{tmp}/golangci.yml"]
        p = run(cmd + ["./..."], cwd=repo, env=dict(GOFLAGS="-trimpath", **(env or {})), check=False)
        if p.returncode not in (0, 1) or not out.exists():
            sys.exit(f"golangci-lint failed in {repo} ({p.returncode}):\n{p.stderr[-3000:]}")
        return json.loads(out.read_text()).get("Issues") or []


def check_lint_version():
    v = run(["golangci-lint", "version", "--short"]).stdout.strip()
    if v != LINT_VERSION:
        sys.exit(f"needs golangci-lint {LINT_VERSION} on PATH, found {v}")


COMPLEXITY_CONFIG = """version: "2"
run: {build-tags: [gms_pure_go], tests: true, relative-path-mode: gomod}
linters:
  default: none
  enable: [gocyclo, gocognit]
  settings:
    gocyclo: {min-complexity: 0}
    gocognit: {min-complexity: 0}
issues: {max-issues-per-linter: 0, max-same-issues: 0, uniq-by-line: false}
"""

SCORE = re.compile(r"complexity (\d+) of func `([^`]+)`")


def complexity_of(repo):
    """gocyclo and gocognit of every function (thresholds 0), and the repo config's funlen findings."""
    check_lint_version()
    rows = collections.defaultdict(dict)
    for i in lint_json(repo, COMPLEXITY_CONFIG):
        m = SCORE.search(i["Text"])
        key = (i["Pos"]["Filename"], i["Pos"]["Line"])
        rows[key].update(file=key[0], line=key[1], func=m.group(2), test=key[0].endswith("_test.go"))
        rows[key][i["FromLinter"]] = int(m.group(1))
    return dict(funcs=sorted(rows.values(), key=lambda r: (r["file"], r["line"])))


def lint_of(repo):
    """make lint's result; the ratcheted linters' backlog on the whole tree; //nolint directives by linter."""
    check_lint_version()
    t0 = time.time()
    p = run(["make", "lint"], cwd=repo, check=False)
    make_lint = dict(exit=p.returncode, seconds=round(time.time() - t0, 1), tail=(p.stdout + p.stderr)[-1500:])
    issues = lint_json(repo, args=["--enable-only", RATCHET])
    backlog = collections.Counter()
    files = collections.defaultdict(collections.Counter)
    for i in issues:
        name = i["FromLinter"]
        if name == "funlen":
            name = "funlen (statements)" if "statements" in i["Text"] else "funlen (lines)"
        if name == "revive":
            name = "revive file-length-limit" if "file-length" in i["Text"] or "lines" in i["Text"] else "revive"
        backlog[name] += 1
        files[name][i["Pos"]["Filename"]] += 1
    nolint = collections.Counter()
    directives = 0
    for f in go_files(repo):
        src = Path(repo, f["path"]).read_text()
        for m in re.finditer(r"//nolint:([\w,-]+)", src):
            directives += 1
            for name in m.group(1).split(","):
                nolint[name] += 1
    return dict(make_lint=make_lint, backlog=dict(backlog), backlog_files={k: dict(v) for k, v in files.items()},
                backlog_issues=[dict(linter=i["FromLinter"], file=i["Pos"]["Filename"], line=i["Pos"]["Line"],
                                     text=i["Text"]) for i in issues],
                nolint=dict(nolint), nolint_directives=directives)


# ---------------------------------------------------------------- tests: counts, runtimes, coverage

def pytest_collect(repo, *args):
    """Tests collected per file (run.py passes -q; one more -q prints `file: N` lines)."""
    p = run(["uv", "run", "--quiet", "python", "tests/run.py", "--collect-only", "-q", *args], cwd=repo, check=False)
    per = {m.group(1): int(m.group(2)) for m in re.finditer(r"(?m)^(tests/\S+\.py): (\d+)$", p.stdout)}
    if p.returncode not in (0, 5) or not per:
        sys.exit(f"pytest collection failed ({p.returncode}):\n{p.stdout[-2000:]}\n{p.stderr[-2000:]}")
    return dict(total=sum(per.values()), by_file=per)


def tests_of(repo):
    run(["make", "go-build"], cwd=repo)
    return dict(
        pytest_all=pytest_collect(repo),
        pytest_light=pytest_collect(repo, "-m", "not integration"),
        pytest_integration=pytest_collect(repo, "-m", "integration"),
        pytest_live=pytest_collect(repo, "-m", "not integration", "-k", "owner_request_prompt_live"),
    )


def timed(cmd, cwd, env=None):
    before = resource.getrusage(resource.RUSAGE_CHILDREN)
    load = os.getloadavg()[0]
    t0 = time.monotonic()
    p = run(cmd, cwd=cwd, env=env, check=False)
    wall = time.monotonic() - t0
    after = resource.getrusage(resource.RUSAGE_CHILDREN)
    cpu = (after.ru_utime - before.ru_utime) + (after.ru_stime - before.ru_stime)
    workers = re.search(r"(\d+) workers", p.stdout + p.stderr)
    summary = re.findall(r"(\d+ (?:passed|failed).* in [\d.]+s)", p.stdout)
    return dict(cmd=" ".join(cmd), env=env or {}, exit=p.returncode, wall=round(wall, 1), cpu=round(cpu, 1),
                cores_busy=round(cpu / wall, 1), load_before=round(load, 1), cores=CORES,
                xdist_workers=int(workers.group(1)) if workers else None, summary=summary[-1] if summary else None,
                tail=(p.stdout + p.stderr)[-800:])


def runtime_of(repo):
    """Wall and CPU time of make test and make test-go (tests uncached: GOFLAGS=-count=1), go build cache warm."""
    run(["make", "go-build"], cwd=repo)
    count1 = dict(GOFLAGS="-count=1")
    parts = re.search(r"(?m)^test-go: (.+)$", Path(repo, "Makefile").read_text()).group(1).split()
    return dict(make_test=timed(["make", "test"], repo),
                make_test_go=timed(["make", "test-go"], repo, env=count1),
                test_go_parts=[timed(["make", t], repo, env=count1) for t in parts])


COVPLUG = '''"""pytest plugin: give each test its own GOCOVERDIR, under $COV_BASE/<safe nodeid>."""
import os
import re

import pytest

BASE = os.environ["COV_BASE"]
os.makedirs(os.path.join(BASE, "_orphan"), exist_ok=True)
os.environ["GOCOVERDIR"] = os.path.join(BASE, "_orphan")


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_protocol(item, nextitem):
    d = os.path.join(BASE, re.sub(r"[^A-Za-z0-9_.-]", "_", item.nodeid))
    os.makedirs(d, exist_ok=True)
    os.environ["GOCOVERDIR"] = d
    yield
    os.environ["GOCOVERDIR"] = os.path.join(BASE, "_orphan")
'''


def makefile_var(repo, name):
    return re.search(rf"(?m)^{name} := (\S+)", Path(repo, "Makefile").read_text()).group(1)


def build_cover(repo):
    """The harness's three binaries, built with -cover over the whole module, pm exiting on SIGTERM."""
    main = Path(repo, "cmd/pm/main.go")
    orig = main.read_text()
    patched = orig.replace('import (\n\t"fmt"\n\t"os"\n', 'import (\n\t"fmt"\n\t"os"\n\t"os/signal"\n\t"syscall"\n', 1)
    patched = patched.replace("func main() {\n", "func main() {\n\tgo func() { c := make(chan os.Signal, 1); "
                              "signal.Notify(c, syscall.SIGTERM, syscall.SIGINT); <-c; os.Exit(143) }()\n", 1)
    if patched.count("signal.Notify") != 1:
        sys.exit("cmd/pm/main.go changed shape: the coverage build's SIGTERM patch does not apply")
    try:
        main.write_text(patched)
        for out, pkg, var in (("pm", "./cmd/pm", "VERSION"), ("render-pages", "./tests/render-pages", "VERSION"),
                              ("pm-before", "./cmd/pm", "VERSION_BEFORE")):
            run(["go", "build", "-trimpath", "-tags", "gms_pure_go", "-cover", f"-coverpkg={MODULE}/...",
                 "-ldflags", f"-X {MODULE}/internal/buildinfo.Version={makefile_var(repo, var)}",
                 "-o", f".go/{out}", pkg], cwd=repo)
    finally:
        main.write_text(orig)


def profile_blocks(path):
    blocks = {}
    for line in open(path):
        if line.startswith("mode:"):
            continue
        k, n, c = line.rsplit(" ", 2)
        n, c = int(n), int(c)
        prev = blocks.get(k, (n, 0))
        blocks[k] = (n, max(prev[1], c))
    return blocks


def coverage_of(repo, data):
    """Statement coverage per package: the Go tests, the light harness on -cover builds, and the two combined."""
    work = Path(data, "coverage-work")
    shutil.rmtree(work, ignore_errors=True)
    (work / "hcov").mkdir(parents=True)
    (work / "plug").mkdir()
    (work / "plug/covplug.py").write_text(COVPLUG)
    go_cmd = ["go", "test", "-trimpath", "-tags", "gms_pure_go", "-count=1", "-json", "-cover",
              f"-coverpkg={MODULE}/...", f"-coverprofile={work}/go.txt", "./..."]
    with open(work / "go.json", "w") as gout:
        gproc = subprocess.Popen(go_cmd, cwd=repo, env=dict(os.environ, **GO_ENV), stdout=gout,
                                 stderr=subprocess.STDOUT)
        build_cover(repo)
        h = run(["uv", "run", "--quiet", "python", "tests/run.py", "-p", "covplug", "-n", str(CORES), "-m",
                 "not integration"], cwd=repo, check=False,
                env=dict(COV_BASE=str(work / "hcov"), PYTHONPATH=str(work / "plug")))
        gexit = gproc.wait()
    run(["make", "go-build"], cwd=repo)
    dirs = sorted({str(p.parent) for p in (work / "hcov").rglob("covmeta.*")})
    run(["go", "tool", "covdata", "textfmt", "-i=" + ",".join(dirs), f"-o={work}/harness.txt"])
    go_b, h_b = profile_blocks(work / "go.txt"), profile_blocks(work / "harness.txt")
    per = collections.defaultdict(lambda: dict(stmts=0, go=0, harness=0, combined=0))
    for k in set(go_b) | set(h_b):
        n = (go_b.get(k) or h_b.get(k))[0]
        file = k.split(":")[0].removeprefix(MODULE + "/")
        p = per[package_of(file)]
        g, hh = go_b.get(k, (n, 0))[1] > 0, h_b.get(k, (n, 0))[1] > 0
        p["stmts"] += n; p["go"] += n * g; p["harness"] += n * hh; p["combined"] += n * (g or hh)
    # Go test events: top-level tests and subtests run, by outcome
    outcomes = collections.Counter()
    for line in open(work / "go.json"):
        if not line.startswith("{"):
            continue
        e = json.loads(line)
        if e.get("Test") and e.get("Action") in ("pass", "fail", "skip"):
            outcomes[("sub" if "/" in e["Test"] else "top", e["Action"])] += 1
    summary = re.findall(r"(\d+) (passed|failed|skipped|errors?|deselected)", h.stdout)
    return dict(packages=dict(per), go_exit=gexit, harness_exit=h.returncode,
                harness_summary={k: int(v) for v, k in summary},
                go_events={f"{a}_{b}": n for (a, b), n in outcomes.items()})


# ---------------------------------------------------------------- CI from the GitHub jobs API

def gh_api(path):
    """Every page of a paginated GitHub API answer (gh prints the pages back to back)."""
    text, pages, dec, i = run(["gh", "api", "--paginate", path]).stdout, [], json.JSONDecoder(), 0
    while i < len(text):
        if text[i].isspace():
            i += 1
            continue
        page, i = dec.raw_decode(text, i)
        pages.append(page)
    return pages


def ts(s):
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def ci_of(repo, prs=30):
    """Per merged PR (the last `prs`), the first CI attempt on its head: wall time to the last check, per-job queue
    and run time; and over every pull_request run in the window, the share re-run and the share whose rerun passed."""
    p = run(["gh", "pr", "list", "-R", GH_REPO, "--state", "merged", "--limit", str(prs), "--json",
             "number,title,headRefOid,mergedAt,createdAt"])
    pulls = sorted(json.loads(p.stdout), key=lambda x: x["number"])
    out = []

    def one(pr):
        pages = gh_api(f"repos/{GH_REPO}/actions/runs?head_sha={pr['headRefOid']}&event=pull_request&per_page=100")
        runs = [r for pg in pages for r in pg["workflow_runs"]]
        # the runs the head's push started: a later run on the same head (a label or edit event) is not CI time
        start = min(ts(r["created_at"]) for r in runs) if runs else None
        later = [r for r in runs if (ts(r["created_at"]) - start).total_seconds() > 120]
        runs = [r for r in runs if (ts(r["created_at"]) - start).total_seconds() <= 120]
        jobs = []
        for r in runs:
            jp = gh_api(f"repos/{GH_REPO}/actions/runs/{r['id']}/attempts/1/jobs?per_page=100")
            for j in (j for pg in jp for j in pg["jobs"]):
                if j["conclusion"] == "skipped" or not j["started_at"] or not j["completed_at"]:
                    continue
                jobs.append(dict(workflow=r["name"], job=j["name"], conclusion=j["conclusion"],
                                 created=j["created_at"], started=j["started_at"], completed=j["completed_at"],
                                 queue=(ts(j["started_at"]) - ts(j["created_at"])).total_seconds(),
                                 run=(ts(j["completed_at"]) - ts(j["started_at"])).total_seconds()))
        wall = None
        if jobs:
            first = min(ts(r["created_at"]) for r in runs)
            wall = (max(ts(j["completed"]) for j in jobs) - first).total_seconds()
        return dict(number=pr["number"], title=pr["title"], merged=pr["mergedAt"], head=pr["headRefOid"],
                    runs=len(runs), later_runs=len(later), reran=sum(1 for r in runs if r["run_attempt"] > 1), wall=wall, jobs=jobs)

    with concurrent.futures.ThreadPoolExecutor(8) as ex:
        out = list(ex.map(one, pulls))
    since = min(x["createdAt"] for x in pulls)[:10]
    pages = gh_api(f"repos/{GH_REPO}/actions/runs?event=pull_request&created=%3E%3D{since}&per_page=100")
    allruns = [r for pg in pages for r in pg["workflow_runs"]]
    reruns = []
    for r in allruns:
        if r["run_attempt"] > 1:
            first = gh_api(f"repos/{GH_REPO}/actions/runs/{r['id']}/attempts/1")[0]
            failed = []
            if first["conclusion"] == "failure":
                jp = gh_api(f"repos/{GH_REPO}/actions/runs/{r['id']}/attempts/1/jobs?per_page=100")
                failed = [j["name"] for pg in jp for j in pg["jobs"] if j["conclusion"] == "failure"]
            reruns.append(dict(id=r["id"], workflow=r["name"], attempts=r["run_attempt"], branch=r["head_branch"],
                               first=first["conclusion"], last=r["conclusion"], created=r["created_at"],
                               failed_jobs=failed))
    return dict(prs=out, since=since, runs_in_window=len(allruns),
                runs_by_workflow=dict(collections.Counter(r["name"] for r in allruns)),
                runs_in_window_completed=sum(1 for r in allruns if r["status"] == "completed"), reruns=reruns)


# ---------------------------------------------------------------- trend over the sprint PRs

TREND_CONFIG = """version: "2"
run: {build-tags: [gms_pure_go], tests: true, relative-path-mode: gomod}
linters:
  default: none
  enable: [staticcheck, gocyclo]
  settings:
    gocyclo: {min-complexity: 30}
    staticcheck:
      checks: ["all", "-ST1000", "-ST1003", "-ST1016", "-ST1020", "-ST1021", "-ST1022", "-QF1001"]
issues: {max-issues-per-linter: 0, max-same-issues: 0, uniq-by-line: false}
"""


def trend_commits(repo):
    log = run(["git", "log", "--format=%H%x09%cI%x09%s", "--grep", "pm-quality sprint", "HEAD"], cwd=repo).stdout
    rows = []
    for line in log.splitlines():
        sha, date, subj = line.split("\t")
        m = re.search(r"pm-quality sprint (\d+)[^)]*\) \(#(\d+)\)$", subj)
        if m:
            rows.append(dict(sha=sha, date=date, subject=subj, sprint=int(m.group(1)), pr=int(m.group(2))))
    head = run(["git", "log", "-1", "--format=%H%x09%cI%x09%s", "HEAD"], cwd=repo).stdout.strip().split("\t")
    if head[0] not in {r["sha"] for r in rows}:
        m = re.search(r"\(#(\d+)\)$", head[2])
        rows.append(dict(sha=head[0], date=head[1], subject=head[2], sprint=None, pr=int(m.group(1)) if m else None))
    return sorted(rows, key=lambda r: datetime.fromisoformat(r["date"]))


def trend_of(repo, data):
    """At each sprint PR's merge commit: code lines, test lines, Go tests, gocyclo > 30 and staticcheck findings,
    one worktree per commit, all at once."""
    check_lint_version()
    commits = trend_commits(repo)
    base = Path(data, "trend-wt").resolve()
    base.mkdir(parents=True, exist_ok=True)
    gomax = str(max(2, CORES // len(commits)))

    def one(c):
        wt = base / c["sha"][:7]
        if wt.exists():
            run(["git", "worktree", "remove", "--force", str(wt)], cwd=repo, check=False)
        run(["git", "-c", "core.hooksPath=/dev/null", "worktree", "add", "--detach", str(wt), c["sha"]], cwd=repo)
        try:
            t0 = time.time()
            s = size_of(wt)
            issues = lint_json(wt, TREND_CONFIG, env=dict(GOMAXPROCS=gomax))
            by = collections.Counter(i["FromLinter"] for i in issues)
            return dict(c, code=sum(p["code"] for p in s["packages"].values()),
                        test=sum(p["test"] for p in s["packages"].values()), harness=s["harness"]["code"],
                        go_tests=s["go_tests"]["top_level"], gocyclo30=by["gocyclo"], staticcheck=by["staticcheck"],
                        seconds=round(time.time() - t0))
        finally:
            run(["git", "worktree", "remove", "--force", str(wt)], cwd=repo, check=False)

    with concurrent.futures.ThreadPoolExecutor(len(commits)) as ex:
        rows = list(ex.map(one, commits))
    shutil.rmtree(base, ignore_errors=True)
    return dict(commits=rows, gomaxprocs=gomax)


# ---------------------------------------------------------------- measure

STEPS = ["size", "complexity", "lint", "tests", "runtime", "coverage", "ci", "trend"]


def measure(repo, data, steps):
    data = Path(data)
    data.mkdir(parents=True, exist_ok=True)
    head = run(["git", "log", "-1", "--format=%H%x09%cI%x09%s", "HEAD"], cwd=repo).stdout.strip().split("\t")
    meta = dict(commit=head[0], committed=head[1], subject=head[2], cores=CORES,
                go=run(["go", "version"], cwd=repo).stdout.strip())
    for step in steps or STEPS:
        t0 = time.time()
        fn = dict(size=lambda: size_of(repo), complexity=lambda: complexity_of(repo), lint=lambda: lint_of(repo),
                  tests=lambda: tests_of(repo), runtime=lambda: runtime_of(repo),
                  coverage=lambda: coverage_of(repo, data), ci=lambda: ci_of(repo),
                  trend=lambda: trend_of(repo, data))[step]
        result = fn()
        result["_meta"] = dict(meta, step=step, measured=datetime.now(timezone.utc).isoformat(timespec="seconds"),
                               seconds=round(time.time() - t0, 1))
        Path(data, f"{step}.json").write_text(json.dumps(result, indent=1, default=dict))
        print(f"{step}: {result['_meta']['seconds']} s", file=sys.stderr)


# ---------------------------------------------------------------- draw: SVG figures and Markdown tables

# Light chart surface (the site shows every image on a white card, in both themes), reference palette.
SURFACE, INK, INK2, MUTED, GRID, AXIS = "#fcfcfb", "#0b0b0b", "#52514e", "#898781", "#e1e0d9", "#c3c2b7"
BLUE, ORANGE, AQUA, YELLOW = "#2a78d6", "#eb6834", "#1baf7a", "#eda100"
FONT = "font-family:system-ui,-apple-system,'Segoe UI',sans-serif"
W = 620  # the site shows an image in a paragraph at most 68ch (about 612 px) wide: draw at that width, unscaled


class Svg:
    def __init__(self, w, h, title):
        self.w, self.h, self.parts = w, h, []
        self.parts.append(f'<rect width="{w}" height="{h}" fill="{SURFACE}"/>')
        self.title = title

    def text(self, x, y, s, size=12, fill=INK2, anchor="start", weight=400, extra=""):
        s = str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        self.parts.append(f'<text x="{x:.1f}" y="{y:.1f}" font-size="{size}" fill="{fill}" text-anchor="{anchor}" '
                          f'font-weight="{weight}" {extra}>{s}</text>')

    def line(self, x1, y1, x2, y2, stroke=GRID, width=1, extra=""):
        self.parts.append(f'<line x1="{x1:.1f}" y1="{y1:.1f}" x2="{x2:.1f}" y2="{y2:.1f}" stroke="{stroke}" '
                          f'stroke-width="{width}" {extra}/>')

    def rect(self, x, y, w, h, fill, rx=0, tip=None):
        t = f"<title>{tip}</title>" if tip else ""
        self.parts.append(f'<rect x="{x:.1f}" y="{y:.1f}" width="{max(w, 0):.1f}" height="{max(h, 0):.1f}" '
                          f'fill="{fill}" rx="{rx}">{t}</rect>')

    def path(self, pts, stroke, width=2):
        d = " ".join(f"{'M' if i == 0 else 'L'}{x:.1f},{y:.1f}" for i, (x, y) in enumerate(pts))
        self.parts.append(f'<path d="{d}" fill="none" stroke="{stroke}" stroke-width="{width}" '
                          f'stroke-linejoin="round" stroke-linecap="round"/>')

    def dot(self, x, y, fill, r=4, tip=None):
        t = f"<title>{tip}</title>" if tip else ""
        self.parts.append(f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{r}" fill="{fill}" stroke="{SURFACE}" '
                          f'stroke-width="2">{t}</circle>')

    def legend(self, x, y, items):
        for label, color in items:
            self.rect(x, y - 9, 10, 10, color, rx=2)
            self.text(x + 15, y, label, 12, INK2)
            x += 22 + 7 * len(label)

    def save(self, path):
        body = "\n".join(self.parts)
        Path(path).write_text(
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {self.w} {self.h}" width="{self.w}" '
            f'height="{self.h}" role="img" aria-label="{self.title}" style="{FONT}">\n'
            f'<title>{self.title}</title>\n{body}\n</svg>\n')


def scale(v, n=4):
    """An axis top at or above v, and its ticks: about n steps of 1, 2 or 5 times a power of ten."""
    raw = max(v, 1e-9) / n
    mag = 10 ** math.floor(math.log10(raw))
    step = next(m * mag for m in (1, 2, 5, 10) if m * mag >= raw)
    k = math.ceil(v / step - 1e-9)
    return k * step, [i * step for i in range(k + 1)]


def compact(v):
    if v >= 1000:
        s = f"{v / 1000:.1f}".rstrip("0").rstrip(".")
        return s + "K"
    return f"{v:g}" if isinstance(v, float) else str(v)


def short_pkg(p):
    return p.replace("internal/", "")


def fig_size(d, path):
    pk = sorted(d["packages"].items(), key=lambda kv: -(kv[1]["code"] + kv[1]["test"]))
    rows = [(short_pkg(k), v["code"], v["test"]) for k, v in pk]
    rows.append(("Python harness", 0, d["harness"]["code"]))
    rows.append(("Python tools", d["tools"]["code"], 0))
    left, top, bar, gap, w = 150, 56, 14, 7, W
    h = top + len(rows) * (bar + gap) + 40
    s = Svg(w, h, "Code lines per package, non-test and test")
    s.text(16, 22, "Code lines per package: non-test and test", 14, INK, weight=600)
    s.legend(16, 44, [("non-test", BLUE), ("test", ORANGE)])
    mx, tk = scale(max(a + b for _, a, b in rows), 5)
    plot = w - left - 56
    sx = lambda v: left + v / mx * plot  # noqa: E731
    for t in tk:
        s.line(sx(t), top - 4, sx(t), h - 34, GRID)
        s.text(sx(t), h - 18, compact(t), 11, MUTED, "middle")
    for i, (name, a, b) in enumerate(rows):
        y = top + i * (bar + gap)
        s.text(left - 8, y + bar - 3, name, 12, INK2, "end")
        if a:
            s.rect(left, y, sx(a) - left - (2 if b else 0), bar, BLUE, 2, f"{name}: {a} non-test")
        if b:
            s.rect(sx(a), y, sx(a + b) - sx(a), bar, ORANGE, 2, f"{name}: {b} test")
        s.text(sx(a + b) + 6, y + bar - 3, f"{a + b:,}", 11, INK2)
    s.line(left, top - 4, left, h - 34, AXIS)
    s.save(path)


BINS = [(1, 1), (2, 2), (3, 5), (6, 10), (11, 15), (16, 20), (21, 30), (31, 40), (41, 60), (61, 10 ** 6)]


def bins_of(key):
    """gocyclo is at least 1; gocognit is 0 for a function without a branch (golangci-lint reports it as no line)."""
    return BINS if key == "gocyclo" else [(0, 0), *BINS]


def bin_label(lo, hi):
    return str(lo) if lo == hi else (f"{lo}+" if hi > 10 ** 5 else f"{lo}–{hi}")


def fig_complexity(c, path):
    funcs = c["funcs"]
    panels = [("gocyclo (cyclomatic)", "gocyclo", 30), ("gocognit (cognitive)", "gocognit", 40)]
    w, pw, ph = W, W - 56, 130
    h = 70 + 2 * (ph + 70)
    s = Svg(w, h, "gocyclo and gocognit distributions over all Go functions")
    s.text(16, 22, f"Complexity per function, {len(funcs):,} Go functions (tests included)", 14, INK, weight=600)
    s.legend(16, 44, [("at or under the lint threshold", BLUE), ("over it (ratcheted)", ORANGE)])
    for pi, (title, key, thr) in enumerate(panels):
        x0, y0 = 40, 86 + pi * (ph + 70)
        bins = bins_of(key)
        counts = [sum(1 for f in funcs if lo <= f.get(key, 0) <= hi) for lo, hi in bins]
        mx, tk = scale(max(counts), 4)
        s.text(x0, y0 - 8, title, 12, INK, weight=600)
        bw = pw / len(bins)
        for t in tk:
            y = y0 + ph - t / mx * ph
            s.line(x0, y, x0 + pw, y, GRID)
            s.text(x0 - 6, y + 4, compact(int(t)), 11, MUTED, "end")
        for i, ((lo, hi), n) in enumerate(zip(bins, counts)):
            bh = n / mx * ph
            x = x0 + i * bw + 2
            s.rect(x, y0 + ph - bh, bw - 4, bh, ORANGE if lo > thr else BLUE, 2, f"{bin_label(lo, hi)}: {n}")
            s.text(x + (bw - 4) / 2, y0 + ph - bh - 4, n, 10, INK2, "middle")
            s.text(x + (bw - 4) / 2, y0 + ph + 16, bin_label(lo, hi), 10, MUTED, "middle")
        s.line(x0, y0 + ph, x0 + pw, y0 + ph, AXIS)
        s.text(x0 + pw / 2, y0 + ph + 34, f"{key} score (threshold {thr})", 11, MUTED, "middle")
    s.save(path)


def fig_coverage(cov, path):
    pk = sorted(((k, v) for k, v in cov["packages"].items() if v["stmts"]), key=lambda kv: -kv[1]["stmts"])
    left, top, bar, gap, w = 130, 56, 13, 8, W
    h = top + len(pk) * (bar + gap) + 40
    s = Svg(w, h, "Statement coverage per package, Go tests and light harness combined")
    s.text(16, 22, "Statement coverage per package: Go tests + light harness", 14, INK, weight=600)
    s.legend(16, 44, [("combined", BLUE)])
    s.line(110, 34, 110, 46, INK, 2)
    s.text(117, 44, "Go tests alone", 12, INK2)
    plot = w - left - 125
    sx = lambda v: left + v / 100 * plot  # noqa: E731
    for t in range(0, 101, 20):
        s.line(sx(t), top - 4, sx(t), h - 34, GRID)
        s.text(sx(t), h - 18, f"{t}%", 11, MUTED, "middle")
    for i, (k, v) in enumerate(pk):
        y = top + i * (bar + gap)
        comb, go = 100 * v["combined"] / v["stmts"], 100 * v["go"] / v["stmts"]
        s.text(left - 8, y + bar - 2, short_pkg(k), 12, INK2, "end")
        s.rect(left, y, sx(comb) - left, bar, BLUE, 2, f"{k}: {comb:.1f}% combined, {go:.1f}% Go alone")
        s.line(sx(go), y - 2, sx(go), y + bar + 2, INK, 2)
        s.text(sx(100) + 8, y + bar - 2, f"{comb:.1f}%  of {v['stmts']:,} stmts", 11, INK2)
    s.save(path)


def fig_ci(ci, path, marks=(48, 43, 44, 53)):
    prs = sorted((p for p in ci["prs"] if p["wall"] is not None), key=lambda p: p["merged"])
    w, h, left, top, right = W, 330, 40, 64, 10
    pw, ph = w - left - right, h - top - 76
    sx = lambda i: left + 12 + i / (len(prs) - 1) * (pw - 24)  # noqa: E731
    slow = [max(j["run"] for j in p["jobs"]) / 60 for p in prs]
    mx, tk = scale(max(p["wall"] / 60 for p in prs), 4)
    sy = lambda v: top + ph - v / mx * ph  # noqa: E731
    s = Svg(w, h, "CI wall time per PR, first attempt on the merged head")
    s.text(16, 22, f"CI per PR, minutes: the last {len(prs)} merged PRs, in merge order", 14, INK, weight=600)
    s.legend(16, 46, [("wall: push to last check", BLUE), ("slowest job's run time", ORANGE)])
    for t in tk:
        s.line(left, sy(t), left + pw, sy(t), GRID)
        s.text(left - 6, sy(t) + 4, f"{t:g}", 11, MUTED, "end")
    for i, p in enumerate(prs):
        if p["number"] in marks:
            s.line(sx(i), top - 6, sx(i), top + ph, MUTED, 1, 'stroke-dasharray="3 3"')
    day = None
    for i, p in enumerate(prs):
        s.text(sx(i), top + ph + 14, p["number"], 9, INK if p["number"] in marks else MUTED, "middle",
               600 if p["number"] in marks else 400)
        if p["merged"][:10] != day:
            day = p["merged"][:10]
            s.line(sx(i) - 6, top + ph, sx(i) - 6, top + ph + 34, AXIS)
            s.text(sx(i) - 2, top + ph + 32, day[5:], 10, MUTED)
    for series, color, vals in (("wall", BLUE, [p["wall"] / 60 for p in prs]), ("slowest job", ORANGE, slow)):
        pts = [(sx(i), sy(v)) for i, v in enumerate(vals)]
        s.path(pts, color, 2)
        for p, (x, y), v in zip(prs, pts, vals):
            s.dot(x, y, color, 3.5, f"#{p['number']} {series}: {v:.1f} min")
    s.line(left, top + ph, left + pw, top + ph, AXIS)
    s.text(left + pw, h - 8, "x: PR number, then merge date (UTC); dashed: the merges of #48, #43, #44, #53", 11,
           MUTED, "end")
    s.save(path)


def fig_trend(tr, path):
    rows = tr["commits"]
    panels = [("Non-test Go code lines", [("code", "Go non-test", BLUE)]),
              ("Test code lines", [("test", "Go tests", BLUE), ("harness", "Python harness", ORANGE)]),
              ("Go top-level tests", [("go_tests", "tests", BLUE)]),
              ("Functions with gocyclo > 30", [("gocyclo30", "functions", BLUE)]),
              ("staticcheck findings", [("staticcheck", "findings", BLUE)])]
    cols, pw, ph = 2, 245, 120
    w = W
    h = 80 + 3 * (ph + 82)
    s = Svg(w, h, "Metrics at each pm-quality sprint merge")
    s.text(16, 22, "At each pm-quality sprint PR's merge commit (x: sprint, then PR)", 14, INK, weight=600)
    s.legend(16, 44, [("Go", BLUE), ("Python harness", ORANGE)])
    labels = [str(r["sprint"]) if r["sprint"] else "main" for r in rows]
    for pi, (title, series) in enumerate(panels):
        x0, y0 = 50 + (pi % cols) * (pw + 50), 92 + (pi // cols) * (ph + 82)
        s.text(x0, y0 - 12, title, 12, INK, weight=600)
        mx, tk = scale(max(r[k] for k, _, _ in series for r in rows) or 1, 3)
        sx = lambda i: x0 + 10 + i / (len(rows) - 1) * (pw - 20)  # noqa: E731
        sy = lambda v: y0 + ph - v / mx * ph  # noqa: E731
        for t in tk:
            s.line(x0, sy(t), x0 + pw, sy(t), GRID)
            s.text(x0 - 6, sy(t) + 4, compact(int(t)), 11, MUTED, "end")
        s.line(x0, y0 + ph, x0 + pw, y0 + ph, AXIS)
        for i, lab in enumerate(labels):
            s.text(sx(i), y0 + ph + 14, lab, 9, MUTED, "middle")
            if rows[i]["pr"]:
                s.text(sx(i), y0 + ph + 26, f"#{rows[i]['pr']}", 8, MUTED, "middle")
        for k, name, color in series:
            pts = [(sx(i), sy(r[k])) for i, r in enumerate(rows)]
            s.path(pts, color, 2)
            for (x, y), r, lab in zip(pts, rows, labels):
                s.dot(x, y, color, 3.5, f"{lab} (#{r['pr']}): {r[k]:,} {name}")
            s.text(pts[-1][0] + 6, pts[-1][1] + 4, f"{rows[-1][k]:,}", 10, INK2)
            s.text(pts[0][0] - 2, pts[0][1] - 8, f"{rows[0][k]:,}", 10, INK2, "start")
    s.save(path)


def pct(a, b):
    return f"{100 * a / b:.1f}%" if b else "-"


def md_table(head, rows):
    out = ["| " + " | ".join(head) + " |", "|" + "|".join("---" for _ in head) + "|"]
    out += ["| " + " | ".join(str(c) for c in r) + " |" for r in rows]
    return "\n".join(out)


def draw(data, figdir):
    data, figdir = Path(data), Path(figdir)
    figdir.mkdir(parents=True, exist_ok=True)
    load = lambda n: json.loads((data / f"{n}.json").read_text()) if (data / f"{n}.json").exists() else None  # noqa
    out = []
    size = load("size")
    if size:
        fig_size(size, figdir / "pm-quality-metrics-size.svg")
        pk = sorted(size["packages"].items(), key=lambda kv: -(kv[1]["code"] + kv[1]["test"]))
        rows = [(f"`{k}`", v["files"], f"{v['code']:,}", v["test_files"], f"{v['test']:,}",
                 f"{v['test'] / v['code']:.2f}" if v["code"] else "-") for k, v in pk]
        tc, tt = sum(v["code"] for _, v in pk), sum(v["test"] for _, v in pk)
        rows.append(("**Go total**", sum(v["files"] for _, v in pk), f"**{tc:,}**",
                     sum(v["test_files"] for _, v in pk), f"**{tt:,}**", f"{tt / tc:.2f}"))
        hh, tl = size["harness"], size["tools"]
        rows.append(("Python harness `tests/`", "-", "-", hh["files"], f"{hh['code']:,}", "-"))
        rows.append(("Python tools `release/`, `.github/scripts/`", tl["files"], f"{tl['code']:,}", "-", "-", "-"))
        out += ["## size", md_table(["package", "files", "non-test lines", "test files", "test lines", "test/non-test"],
                                    rows)]
        out += ["### largest files", md_table(["file", "code lines", "lines"],
                [(f"`{f['path']}`", f["code"], f["lines"]) for f in size["largest_files"][:10]])]
        out += ["### largest functions", md_table(["function", "file:line", "code lines"],
                [(f"`{f['name']}`", f"`{f['path']}:{f['line']}`", f["code"]) for f in size["largest_funcs"][:10]])]
        fc = sorted(size["func_code"])
        out.append(f"non-test funcs {len(fc)}, median {statistics.median(fc)} code lines, p90 "
                   f"{fc[int(.9 * len(fc))]}, max {fc[-1]}; over 100 lines: {sum(1 for x in fc if x > 100)}")
        out.append(f"go tests: {size['go_tests']}")
    c = load("complexity")
    if c:
        fig_complexity(c, figdir / "pm-quality-metrics-complexity.svg")
        fs = c["funcs"]
        for key, thr in (("gocyclo", 30), ("gocognit", 40)):
            v = sorted(f.get(key, 0) for f in fs)
            nt = sorted(f.get(key, 0) for f in fs if not f["test"])
            out.append(f"{key}: n {len(v)}, mean {statistics.mean(v):.2f}, median {statistics.median(v)}, p90 "
                       f"{v[int(.9 * len(v))]}, p99 {v[int(.99 * len(v))]}, max {v[-1]}; >{thr}: "
                       f"{sum(1 for x in v if x > thr)} (non-test {sum(1 for x in nt if x > thr)}); "
                       f">15: {sum(1 for x in v if x > 15)}")
            out.append(md_table(["bin", "all", "non-test"],
                                [(bin_label(lo, hi), sum(1 for x in v if lo <= x <= hi),
                                  sum(1 for x in nt if lo <= x <= hi)) for lo, hi in bins_of(key)]))
        over = sorted((f for f in fs if f.get("gocyclo", 0) > 30 or f.get("gocognit", 0) > 40),
                      key=lambda f: -f.get("gocognit", 0))
        out += ["### over a threshold", md_table(["function", "file:line", "gocyclo", "gocognit"],
                [(f"`{f['func']}`", f"`{f['file']}:{f['line']}`", f.get("gocyclo"), f.get("gocognit")) for f in over])]
    lint = load("lint")
    if lint:
        out += ["## lint", f"make lint exit {lint['make_lint']['exit']} in {lint['make_lint']['seconds']} s",
                md_table(["linter", "findings", "non-test", "test", "files"],
                         [(k, n, *(lambda tt: (n - tt, tt))(sum(1 for f, c in lint["backlog_files"][k].items()
                                                              for _ in range(c) if f.endswith("_test.go"))),
                           len(lint["backlog_files"][k])) for k, n in sorted(lint["backlog"].items())]),
                f"nolint directives {lint['nolint_directives']}: {lint['nolint']}"]
        out.append(md_table(["linter", "file:line", "text"],
                            [(i["linter"], f"`{i['file']}:{i['line']}`", i["text"][:90])
                             for i in lint["backlog_issues"] if i["linter"] in ("funlen", "revive", "reassign")]))
    t = load("tests")
    if t:
        out += ["## tests", json.dumps({k: v for k, v in t.items() if k != "_meta"})]
    rt = load("runtime")
    if rt:
        out.append(md_table(["command", "wall s", "cpu s", "cores busy (cpu/wall)", "load before", "result"],
                            [(f"`{' '.join(f'{k}={v}' for k, v in r['env'].items())} {r['cmd']}`".replace("` ", "`"),
                              r["wall"], r["cpu"], r["cores_busy"], r["load_before"],
                              f"exit {r['exit']}; {r['summary'] or ''}")
                             for r in (rt["make_test"], rt["make_test_go"], *rt.get("test_go_parts", []))]))
    cov = load("coverage")
    if cov:
        fig_coverage(cov, figdir / "pm-quality-metrics-coverage.svg")
        pk = sorted(((k, v) for k, v in cov["packages"].items() if v["stmts"]), key=lambda kv: -kv[1]["stmts"])
        tot = {x: sum(v[x] for _, v in pk) for x in ("stmts", "go", "harness", "combined")}
        out += [f"go exit {cov['go_exit']}, harness exit {cov['harness_exit']}, {cov['harness_summary']}, "
                f"go events {cov['go_events']}",
                md_table(["package", "statements", "Go tests", "light harness", "combined"],
                         [(f"`{k}`", f"{v['stmts']:,}", pct(v["go"], v["stmts"]), pct(v["harness"], v["stmts"]),
                           pct(v["combined"], v["stmts"])) for k, v in pk] +
                         [("**module**", f"{tot['stmts']:,}", pct(tot["go"], tot["stmts"]),
                           pct(tot["harness"], tot["stmts"]), f"**{pct(tot['combined'], tot['stmts'])}**")])]
    ci = load("ci")
    if ci:
        fig_ci(ci, figdir / "pm-quality-metrics-ci.svg")
        prs = [p for p in ci["prs"] if p["wall"] is not None]
        out += ["## ci", md_table(["PR", "merged", "wall min", "runs", "jobs", "slowest job", "its run s"],
                [(f"#{p['number']}", p["merged"][:16].replace("T", " "), f"{p['wall'] / 60:.1f}", p["runs"],
                  len(p["jobs"]), *(lambda j: (f"{j['workflow']}/{j['job']}", round(j["run"])))(
                      max(p["jobs"], key=lambda j: j["queue"] + j["run"]))) for p in prs])]
        jobs = collections.defaultdict(list)
        for p in prs:
            for j in p["jobs"]:
                jobs[(j["workflow"], j["job"])].append(j)
        q = lambda xs, f: sorted(xs)[min(len(xs) - 1, int(f * len(xs)))]  # noqa: E731
        out.append(md_table(["workflow / job", "jobs", "queue median s", "queue p90 s", "run median s", "run p90 s",
                             "run max s"],
                            [(f"{w} / {jn}", len(js), round(statistics.median(j["queue"] for j in js)),
                              round(q([j["queue"] for j in js], .9)), round(statistics.median(j["run"] for j in js)),
                              round(q([j["run"] for j in js], .9)), round(max(j["run"] for j in js)))
                             for (w, jn), js in sorted(jobs.items(), key=lambda kv: (-len(kv[1]), kv[0]))]))
        rr = ci["reruns"]
        flaky = [r for r in rr if r["first"] == "failure" and r["last"] == "success"]
        out.append(f"runs in window since {ci['since']}: {ci['runs_in_window']} (completed "
                   f"{ci['runs_in_window_completed']}); re-run: {len(rr)}; failed then passed on rerun: {len(flaky)}; "
                   f"{[(r['workflow'], r['branch'], r['first'], r['last'], r['attempts'], r['failed_jobs']) for r in rr]}")
    tr = load("trend")
    if tr:
        fig_trend(tr, figdir / "pm-quality-metrics-trend.svg")
        out += ["## trend", md_table(["sprint", "PR", "commit", "merged", "non-test lines", "Go test lines",
                                      "harness lines", "Go tests", "gocyclo > 30", "staticcheck"],
                [(r["sprint"] or "-", f"#{r['pr']}", r["sha"][:7], r["date"][:10], f"{r['code']:,}",
                  f"{r['test']:,}", f"{r['harness']:,}", r["go_tests"], r["gocyclo30"], r["staticcheck"])
                 for r in tr["commits"]])]
    print("\n\n".join(out))


if __name__ == "__main__":
    if len(sys.argv) >= 4 and sys.argv[1] == "measure":
        bad = [s for s in sys.argv[4:] if s not in STEPS]
        if bad:
            sys.exit(f"unknown steps {bad}; steps: {' '.join(STEPS)}")
        measure(sys.argv[2], sys.argv[3], sys.argv[4:])
    elif len(sys.argv) == 4 and sys.argv[1] == "draw":
        draw(sys.argv[2], sys.argv[3])
    else:
        sys.exit(__doc__)
