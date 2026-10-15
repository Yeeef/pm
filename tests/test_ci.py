"""The CI workflows' own logic: which changes run the release build test (pm-release-build.yml's push and
pull_request path lists equal, naming what can alter the release build), the Go test job's package list (make
go-test-but-work failing when go list fails) and go_shards.py's shards, go_changes.py's skip of the Go jobs' work on a
PR no Go job reads (each job still running on every PR) and its selection of the test packages a PR's changes can
affect, which the go test targets take as SELECTED, pm-go.yml's macOS jobs running once a day and not on a PR, every
restore of pm-go.yml's Go caches, the parity eval's scenario count, and the release on a push to main (its jobs, their
tokens, main_ci.py, and the scripts of its plan and tag steps, run against a local bare origin and a fake gh). The
expected results come from what each job is for, on fake tools built here."""

from __future__ import annotations

import ast
import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

import pytest
from conftest import REAL

PM_DIR = Path(__file__).resolve().parents[1]


def paths_lists(workflow: Path) -> dict[str, list[str]]:
    """Each trigger's `paths:` list in a workflow: the `- ` items under it, by the trigger they sit under."""
    lists: dict[str, list[str]] = {}
    trigger, inside = None, False
    for line in workflow.read_text().splitlines():
        if m := re.match(r"^  (\w+):", line):
            trigger, inside = m[1], False
        elif re.match(r"^    paths:", line):
            inside = True
            lists[trigger] = []
        elif inside and (m := re.match(r"^      - (\S+)", line)):
            lists[trigger].append(m[1])
        elif inside:
            inside = False
    return lists


def test_the_release_build_runs_on_every_change_to_what_pm_version_runs():
    """`pm version` of the built binary goes through main (cmd/pm) and the launcher (internal/launch) before the version
    stamp (internal/buildinfo): a change to any of them can break the release build's check, so each runs it, on a pull
    request and on a push alike."""
    lists = paths_lists(PM_DIR / ".github/workflows/pm-release-build.yml")
    assert set(lists) == {"pull_request", "push"}
    assert lists["pull_request"] == lists["push"]
    for path in ("release/**", "install.sh", "go.mod", "go.sum", "internal/buildinfo/**", "cmd/pm/**",
                 "internal/launch/**", "tests/test_release.py", ".github/workflows/pm-release-build.yml"):
        assert path in lists["push"], path


def test_the_parity_eval_runs_every_scenario_at_once_and_its_docs_count_them():
    """make test-live-parity gives each scenario a worker, so a run takes as long as its slowest scenario, and the eval's
    docstring names how many there are: both follow the scenarios pytest collects, which a new scenario changes."""
    res = subprocess.run([sys.executable, "-m", "pytest", "--collect-only", "-q", "-p", "no:xdist",
                          "tests/test_agent_parity_live.py"], cwd=PM_DIR, capture_output=True, text=True, check=False)
    scenarios = [line for line in res.stdout.splitlines() if "::" in line]
    assert res.returncode == 0 and scenarios, res.stdout + res.stderr
    target = re.search(r"^test-live-parity:.*\n\t(.*)$", (PM_DIR / "Makefile").read_text(), re.M)
    assert target and re.search(r" -n (\d+) ", target[1])[1] == str(len(scenarios)), target
    doc = ast.get_docstring(ast.parse((PM_DIR / "tests/test_agent_parity_live.py").read_text()))
    counts = re.findall(r"\b[Tt]he (\d+) scenarios", doc)
    assert counts == [str(len(scenarios))], counts


def fake_go(tmp_path: Path, list_fails: bool) -> dict[str, str]:
    """A `go` on PATH whose `list` prints three packages (and fails after them when list_fails) and whose `test` logs
    its arguments."""
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    (bin_dir / "go").write_text(f"""#!/bin/sh
case "$1" in
  list) printf 'm/a\\nm/internal/work\\nm/b\\n'; {"echo 'go: broken' >&2; exit 1" if list_fails else "exit 0"} ;;
  test) echo "$@" >> {tmp_path}/test.log ;;
esac
""")
    (bin_dir / "go").chmod(0o755)
    return {**os.environ, "PATH": f"{bin_dir}:{os.environ['PATH']}"}


def test_go_test_but_work_fails_when_go_list_fails_and_tests_nothing(tmp_path):
    broken = tmp_path / "broken"
    broken.mkdir()
    res = subprocess.run(["make", "-s", "go-test-but-work"], cwd=PM_DIR, env=fake_go(broken, True),
                         capture_output=True, text=True, check=False)
    assert res.returncode != 0
    assert "go: broken" in res.stderr
    assert not (broken / "test.log").exists()

    # folded: test_go_test_but_work_tests_every_package_but_the_work_store
    ok = tmp_path / "ok"
    ok.mkdir()
    res = subprocess.run(["make", "-s", "go-test-but-work"], cwd=PM_DIR, env=fake_go(ok, False),
                         capture_output=True, text=True, check=False)
    assert res.returncode == 0, res.stderr
    assert (ok / "test.log").read_text() == "test -trimpath -tags gms_pure_go -ldflags=-w m/a m/b\n"


def test_the_go_test_targets_run_only_their_selected_packages_and_fail_on_a_missing_selection(tmp_path):
    """SELECTED, as pm-go.yml sets it on a pull request: a target with none of its packages selected runs no go and
    passes; go-test-but-work tests the selected packages but internal/work; a missing file fails before any go runs."""
    selected = tmp_path / "selected"
    selected.write_text("./internal/site\n.\n")
    env = fake_go(tmp_path, False)

    def make(target, file=selected):
        log = tmp_path / "test.log"
        log.unlink(missing_ok=True)
        res = subprocess.run(["make", "-s", target], cwd=PM_DIR, capture_output=True, text=True, check=False,
                             env={**env, "SELECTED": str(file)})
        return res.returncode, res.stdout + res.stderr, log.read_text() if log.exists() else ""

    for target in ("go-test-work", "go-test-race", "go-test-work-os", "go-test-race-os"):
        code, out, log = make(target)
        assert (code, log) == (0, ""), out
        assert "internal/work is not selected" in out, out
    code, out, log = make("go-test-but-work")
    assert (code, log) == (0, "test -trimpath -tags gms_pure_go -ldflags=-w . ./internal/site\n"), out
    code, out, log = make("go-test-but-work-os")  # site's os tests; the root package has none
    assert code == 0 and log.startswith("test -trimpath -tags gms_pure_go -ldflags=-w -run ^(") and \
        log.endswith(")$ ./internal/site\n"), (out, log)
    selected.write_text(".\n")
    assert make("go-test-but-work-os")[::2] == (0, "")
    selected.write_text("./internal/service\n")
    code, out, log = make("go-test-race-service")
    assert (code, log) == (0, "test -race -tags gms_pure_go -ldflags=-w -run . ./internal/service\n"), out
    for target in ("go-test-work", "go-test-but-work"):
        code, out, log = make(target, tmp_path / "missing")
        assert code != 0 and log == "", (target, out)


SHARDS = PM_DIR / ".github/scripts/go_shards.py"


def fake_test_binary(tmp_path: Path, tests: list[str], skip: str = "", fail: str = "") -> Path:
    """A `go test -c` binary's stand-in: -test.list prints the tests its pattern matches; -test.run runs the ones its
    `^(a|b)$` names, logging each, and reports PASS for each but `skip` (no report) and `fail` (FAIL, exit 1), after
    output of its own left unterminated, and a subtest's PASS."""
    binary = tmp_path / "pkg.test"
    binary.write_text(f"""#!/bin/sh
case "$1" in
  -test.list) printf '%s\\n' {" ".join(tests)} | grep -E "$2" || true ;;
  *) while [ $# -gt 0 ]; do [ "$1" = -test.run ] && re="$2"; shift; done
     rc=0
     for t in $(printf '%s' "$re" | sed 's/^^(//; s/)[$]$//' | tr '|' ' '); do
       echo "$t" >> {tmp_path}/ran.log
       if [ "$t" = "{fail}" ]; then echo "--- FAIL: $t (0.10s)"; rc=1
       elif [ "$t" != "{skip}" ]; then printf 'progress, unterminated'; echo "--- PASS: $t (0.10s)"
         echo "    --- PASS: $t/sub (0.00s)"; fi
     done
     exit $rc ;;
esac
""")
    binary.chmod(0o755)
    return binary


def run_shards(tmp_path: Path, binary: Path, seconds: str, *args: str) -> subprocess.CompletedProcess:
    (tmp_path / "seconds.txt").write_text(seconds)
    return subprocess.run(["python3", str(SHARDS), "--shards", "2", *args, str(binary), str(tmp_path),
                           str(tmp_path / "seconds.txt")], capture_output=True, text=True, check=False)


def test_go_shards_runs_every_test_once_across_the_shards(tmp_path):
    tests = ["TestA", "TestB", "TestC", "TestRacingD"]
    binary = fake_test_binary(tmp_path, tests)
    res = run_shards(tmp_path, binary, "TestA 5\nTestB 1\nTestC 1\nTestRacingD 3\n")
    assert res.returncode == 0, res.stdout + res.stderr
    assert sorted((tmp_path / "ran.log").read_text().split()) == tests
    assert "4 tests in 2 shards" in res.stdout
    (tmp_path / "ran.log").unlink()
    res = run_shards(tmp_path, binary, "TestA 5\nTestB 1\nTestC 1\nTestRacingD 3\n", "--run", "Racing|TestB")
    assert res.returncode == 0, res.stdout + res.stderr
    assert sorted((tmp_path / "ran.log").read_text().split()) == ["TestB", "TestRacingD"]


def test_go_shards_refuses_a_test_with_no_measured_seconds_or_a_stale_one_and_runs_nothing(tmp_path):
    binary = fake_test_binary(tmp_path, ["TestA", "TestNew"])
    res = run_shards(tmp_path, binary, "TestA 1\nTestGone 2\n")
    assert res.returncode == 1
    assert "TestNew has no measured seconds" in res.stderr
    assert "TestGone in" in res.stderr and "is no test of" in res.stderr
    assert "make go-test-work UPDATE=1" in res.stderr
    assert not (tmp_path / "ran.log").exists()


def test_go_shards_refuses_a_run_name_that_matches_no_top_level_test_and_runs_nothing(tmp_path):
    binary = fake_test_binary(tmp_path, ["TestARacingB", "TestC"])
    res = run_shards(tmp_path, binary, "TestARacingB 1\nTestC 1\n", "--run", "Racing|Eight|TestC|Gone")
    assert res.returncode == 1
    assert "--run name Eight matches no top-level test" in res.stderr
    assert "--run name Gone matches no top-level test" in res.stderr
    assert "Racing matches" not in res.stderr and "TestC matches" not in res.stderr
    res = run_shards(tmp_path, binary, "TestARacingB 1\nTestC 1\n", "--run", "TestC/sub")
    assert res.returncode == 1
    assert "must be test names joined by |" in res.stderr
    assert not (tmp_path / "ran.log").exists()


def test_go_shards_fails_on_a_failing_test_and_on_a_test_that_reports_nothing(tmp_path):
    seconds = "TestA 1\nTestB 1\nTestC 1\n"
    res = run_shards(tmp_path, fake_test_binary(tmp_path, ["TestA", "TestB", "TestC"], fail="TestB"), seconds)
    assert res.returncode == 1
    assert "--- FAIL: TestB" in res.stdout
    res = run_shards(tmp_path, fake_test_binary(tmp_path, ["TestA", "TestB", "TestC"], skip="TestC"), seconds)
    assert res.returncode == 1
    assert "these tests reported no PASS or SKIP: TestC" in res.stdout


def test_go_shards_update_rewrites_the_seconds_from_a_run_of_every_test(tmp_path):
    binary = fake_test_binary(tmp_path, ["TestB", "TestA"])
    res = run_shards(tmp_path, binary, "", "--update")
    assert res.returncode == 0, res.stdout + res.stderr
    assert (tmp_path / "seconds.txt").read_text() == "TestA 0.10\nTestB 0.10\n"


GO_CHANGES = PM_DIR / ".github/scripts/go_changes.py"
spec = importlib.util.spec_from_file_location("go_changes", GO_CHANGES)
go_changes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(go_changes)


OS_TESTS = PM_DIR / ".github/scripts/go_os_tests.py"


def os_tests(root: Path, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run([sys.executable, str(OS_TESTS), "--root", str(root), *args], capture_output=True, text=True,
                          check=False)


def test_every_go_test_is_classified_by_os_once():
    res = os_tests(PM_DIR, "check")
    assert res.returncode == 0, res.stderr


def test_the_os_lists_name_each_test_of_their_set_and_refuse_a_test_in_no_row_or_two(tmp_path):
    def module(listing: str) -> Path:
        for pkg, tests in {"a": ["TestOne", "TestOneMore", "TestTwo"], "b": ["TestTwo", "TestThree"]}.items():
            (tmp_path / pkg).mkdir(exist_ok=True)
            (tmp_path / pkg / "x_test.go").write_text("".join(f"func {t}(t *testing.T) {{}}\n" for t in tests)
                                                      + "func TestMain(m *testing.M) {}\n")
        (tmp_path / ".github/scripts").mkdir(parents=True, exist_ok=True)
        (tmp_path / ".github/scripts/go-os-tests.txt").write_text("# c\n" + listing)
        return tmp_path

    root = module("a TestOne os x\na TestOneMore any x\na TestTwo any x\nb * os x\n")
    assert os_tests(root, "check").returncode == 0
    assert os_tests(root, "packages", "--os").stdout.split() == ["./a", "./b"]
    assert os_tests(root, "names", "--os", "--package", "b").stdout.strip() == "TestThree|TestTwo"
    assert os_tests(root, "names", "--package", "b", "--skip", "Two").stdout.strip() == "TestThree"
    assert os_tests(root, "names", "--os", "--package", "b", "--only", "Two").stdout.strip() == "TestTwo"
    # as substrings TestOne also picks TestOneMore, an any test; anchored, a's any TestTwo shares b's os name
    for args in (["--package", "a"], ["--anchored"]):
        res = os_tests(root, "names", "--os", *args)
        assert res.returncode == 1 and "outside the set" in res.stderr, res
    assert os_tests(root, "names", "--os", "--anchored", "--but", "a").stdout.strip() == "^(TestThree|TestTwo)$"
    # --in keeps a selection's packages (go_changes.py select's file): an empty package list prints nothing and passes,
    # empty names still fail, and a package with no tests fails
    selected = tmp_path / "selected"
    selected.write_text("./b\n")
    assert os_tests(root, "packages", "--os", "--in", str(selected)).stdout.split() == ["./b"]
    assert os_tests(root, "names", "--os", "--anchored", "--in", str(selected)).stdout.strip() == "^(TestThree|TestTwo)$"
    selected.write_text("./a\n")
    res = os_tests(root, "packages", "--os", "--but", "a", "--in", str(selected))
    assert (res.returncode, res.stdout) == (0, "")
    assert os_tests(root, "names", "--os", "--but", "a", "--in", str(selected)).returncode == 1
    selected.write_text("./a\n./c\n")
    res = os_tests(root, "packages", "--in", str(selected))
    assert res.returncode == 1 and "names c, no package with tests" in res.stderr

    root = module("a TestOne os x\na TestTwo any x\na TestTwo os x\nb TestNone os x\nc * any x\n")
    res = os_tests(root, "check")
    assert res.returncode == 1
    for named in ("a TestOneMore: in 0 rows", "a TestTwo: in 2 rows", "b TestNone: no such top-level test",
                  "b TestThree: in 0 rows", "b TestTwo: in 0 rows", "c *: no package with tests"):
        assert named in res.stderr, named
    assert not os_tests(root, "names").stdout  # nothing printed while a test is unclassified

    # every form go test finds (an unnamed or wrapped parameter), none it does not (Testlower, TestMain), and a file go
    # builds only on some systems refused
    (tmp_path / "c").mkdir()
    (tmp_path / "c" / "y_test.go").write_text("func TestFour(*testing.T) {}\nfunc TestFive(\n\tt *testing.T,\n) {}\n"
                                              "func Testlower(t *testing.T) {}\n")
    root = module("a * any x\nb * any x\nc TestFive os x\nc TestFour any x\n")
    assert os_tests(root, "names", "--os").stdout.strip() == "TestFive"
    (tmp_path / "c" / "z_linux_test.go").write_text("func TestSix(t *testing.T) {}\n")
    res = os_tests(root, "check")
    assert res.returncode == 1 and "c/z_linux_test.go builds only on some systems" in res.stderr


def test_go_changes_skips_the_go_jobs_only_for_docs_and_harness_python_that_no_go_file_reads():
    go_files = {"internal/a/a_test.go": 'x := filepath.Abs("../../tests/fake_x.py")\n'
                                        'y := filepath.Join("..", "..", "AGENTS.md")\n',
                "assets_test.go": 'os.ReadFile("website/data.json")\n'}
    affected = lambda *changed: go_changes.go_affected(list(changed), go_files)[0]  # noqa: E731
    assert not affected("README.md", "CLAUDE.md", "tests/test_pm.py", "tests/conftest.py", "release/merge_ready.py",
                        "website/index.html")
    for read in ("tests/fake_x.py", "AGENTS.md", "website/data.json"):  # up from a package, joined, or from the root
        assert affected("tests/test_pm.py", read), read
    for go in ("internal/a/a.go", "go.mod", "Makefile", "prime.md", "CHANGELOG.md", "changelog.d/x.md",
               "tests/render-pages/main.go", "tests/cases.json", ".github/workflows/pm-go.yml"):
        assert affected("tests/test_pm.py", go), go
    assert affected()  # no change found: nothing is skipped
    real = {f: (PM_DIR / f).read_text() for f in subprocess.run(
        ["git", "ls-files", "*.go"], cwd=PM_DIR, capture_output=True, text=True, check=True).stdout.split()}
    assert go_changes.go_affected(["tests/fake_sched.py"], real)[0]  # internal/service's lifecycle test runs it
    # README.md is in NO_GO, but the root package's assets_test.go names "README.md" (changelog.d's), which reads as the
    # root's: a README-only PR runs the Go jobs, conservatively
    assert go_changes.go_affected(["README.md"], real) == (True, "README.md is read by a Go file")
    assert not go_changes.go_affected(["tests/test_ci.py", "AGENTS.md"], real)[0]


def test_go_changes_reads_a_pull_requests_merge_commit_and_runs_every_job_on_a_push(tmp_path):
    repo = tmp_path / "repo"
    repo.mkdir()

    def git(*args):
        subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@t", *args], cwd=repo, check=True,
                       capture_output=True)

    def run(event):
        out = tmp_path / "out"
        out.write_text("")
        r = subprocess.run([sys.executable, str(GO_CHANGES)], cwd=repo, capture_output=True, text=True, check=False,
                           env={**os.environ, "GITHUB_EVENT_NAME": event, "GITHUB_OUTPUT": str(out)})
        return r.returncode, out.read_text(), r.stdout + r.stderr

    git("init", "-q", "-b", "main")
    (repo / "a.go").write_text("package a\n")
    git("add", "."), git("commit", "-qm", "base")
    code, out, log = run("pull_request")
    assert code != 0 and out == "", log  # no first parent: a checkout too shallow fails, nothing guessed
    git("checkout", "-qb", "pr")
    (repo / "tests").mkdir()
    (repo / "tests/test_x.py").write_text("x = 1\n")
    git("add", "."), git("commit", "-qm", "pr")
    code, out, log = run("pull_request")  # the PR's head, one parent: its last commit is not the PR's changes
    assert code != 0 and out == "" and "merge commit" in log, log
    git("checkout", "-q", "main"), git("merge", "-q", "--no-ff", "-m", "merge", "pr")  # as refs/pull/N/merge is
    assert run("pull_request")[:2] == (0, "go=false\n")
    assert run("push")[:2] == (0, "go=true\n")
    git("checkout", "-q", "pr")
    (repo / "a.go").write_text("package a // changed\n")
    git("commit", "-qam", "go"), git("checkout", "-q", "main"), git("merge", "-q", "--no-ff", "-m", "m2", "pr")
    assert run("pull_request")[:2] == (0, "go=true\n")


def test_go_changes_select_runs_the_test_packages_a_change_can_affect_and_everything_for_one_it_cannot_place():
    """A graph built here: store (a library) linked by the tests of work and cli; site linked by cli's alone; root
    embeds prime.md and is linked by hooks; svc's test names ../../tests/fake.py; root's tests read every .go file
    (go_changes.SOURCE_READERS)."""
    P = go_changes.Package  # noqa: N806
    pkgs = {
        ".": P(".", {"assets.go": "//go:embed prime.md"}, {"size_test.go": 'strings.HasSuffix(p, ".go")'},
               ["prime.md", "prompts/a.txt"], links={"."}),
        "internal/store": P("internal/store", {"s.go": ""}),
        "internal/work": P("internal/work", {"w.go": ""}, {"w_test.go": ""}, links={"internal/work", "internal/store"}),
        "internal/site": P("internal/site", {"site.go": ""}, {"site_test.go": ""}, test_embeds=["golden/*.html"],
                           links={"internal/site"}),
        "internal/cli": P("internal/cli", {"cli.go": ""}, {"cli_test.go": ""},
                          links={"internal/cli", "internal/site", "internal/store", "internal/work"}),
        "internal/hooks": P("internal/hooks", {"h.go": ""}, {"h_test.go": ""}, links={"internal/hooks", "."}),
        "internal/svc": P("internal/svc", {"svc.go": ""}, {"svc_test.go": 'filepath.Abs("../../tests/fake.py")'},
                          links={"internal/svc"}),
    }

    def chosen(*changed):
        return go_changes.select(list(changed), pkgs)[0]

    # a store change runs every test binary linking store, and the root's, which reads every .go file
    assert chosen("internal/store/s.go") == {".", "internal/work", "internal/cli"}
    # a site-only change does not run internal/work's tests
    assert chosen("internal/site/site.go") == {".", "internal/site", "internal/cli"}
    assert chosen("internal/site/site_test.go") == {".", "internal/site"}  # a test file: its own package's binary
    # testdata and a test-only embed run the reader's tests; an embedded file runs every binary linking its package
    assert chosen("internal/site/testdata/x.json") == {"internal/site"}
    assert chosen("internal/site/golden/a.html") == {"internal/site"}
    assert chosen("prime.md") == chosen("prompts/a.txt") == {".", "internal/hooks"}
    assert chosen("tests/fake.py") == {"internal/svc"}  # named by a path from the package's directory
    assert chosen("AGENTS.md", "tests/test_pm.py") == set()  # NO_GO files no Go file names
    # what the selector cannot place, and go.mod or go.sum, run every package
    for path in ("Makefile", ".github/workflows/pm-go.yml", "prompts/new.txt", "internal/gone/x.go", "go.mod", "go.sum"):
        assert chosen("internal/site/site.go", path) is go_changes.ALL, path
    assert chosen() is go_changes.ALL  # no change found

    # a package whose tests read Go source must be in SOURCE_READERS, and one listed must still read it
    pkgs["internal/svc"].test_files["parse_test.go"] = 'import "go/parser"'
    errors = go_changes.source_reader_errors(pkgs)
    assert any(e.startswith("internal/svc: its tests read Go source") for e in errors), errors
    assert any(e.startswith("internal/item: in SOURCE_READERS") for e in errors), errors


def test_go_changes_select_on_this_repo_follows_its_import_graph_and_its_readers(monkeypatch):
    # go list loads every dependency's source: from the user's module cache, which conftest's temp HOME hides
    real = {k: v for k, v in {**os.environ, "HOME": REAL["HOME"], "XDG_CONFIG_HOME": REAL["XDG_CONFIG_HOME"]}.items()
            if v}
    for var in ("GOCACHE", "GOMODCACHE"):
        monkeypatch.setenv(var, subprocess.run(["go", "env", var], env=real, capture_output=True, text=True,
                                               check=True).stdout.strip())
    pkgs = go_changes.go_list(PM_DIR)
    assert go_changes.source_reader_errors(pkgs) == []

    def chosen(*changed):
        return go_changes.select(list(changed), pkgs)[0]

    store = chosen("internal/work/sync.go")  # internal/cli, config, hooks, install and service import internal/work
    assert {"internal/work", "internal/cli", "internal/config", "internal/hooks", "internal/install",
            "internal/service"} <= store
    assert not store & {"internal/site", "internal/records", "internal/design"}, store
    for path in ("internal/site/site.go", "internal/records/records.go", "internal/hooks/prime.go",
                 "internal/install/pieces.go", "internal/site/testdata/serve/filled.html"):
        assert "internal/work" not in chosen(path), path  # TestTheItemReadersLinkNoDolt holds site's and records'
    assert "internal/site" in chosen("internal/site/testdata/serve/filled.html")
    assert "internal/hooks" in chosen("prime.md")  # embedded by the root package, which hooks imports
    assert "internal/launch" in chosen("changelog.d/x.md")  # an embedded directory's file
    assert "internal/service" in chosen("tests/fake_sched.py")  # internal/service's lifecycle test runs it
    assert "internal/work" in chosen("internal/work/testdata/bd-export.jsonl")
    for path in ("go.mod", "Makefile", ".github/scripts/go-os-tests.txt", ".pm/hooks/post-checkout"):
        assert chosen(path) is go_changes.ALL, path
    assert chosen("AGENTS.md", "tests/test_pm.py") == set()


def test_go_changes_select_writes_a_pull_requests_packages_and_every_package_on_a_push(tmp_path):
    """A module built here, with no dependency to fetch: lib (no tests), a (imports lib), b (reads testdata), and
    SOURCE_READERS' packages, whose tests read Go source."""
    repo = tmp_path / "repo"

    def git(*args):
        subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@t", *args], cwd=repo, check=True,
                       capture_output=True)

    def write(path, text):
        (repo / path).parent.mkdir(parents=True, exist_ok=True)
        (repo / path).write_text(text)

    def run(event, *args):
        r = subprocess.run([sys.executable, str(GO_CHANGES), "select", ".go/selected", *args], cwd=repo,
                           capture_output=True, text=True, check=False,
                           env={**os.environ, "GITHUB_EVENT_NAME": event, "GOFLAGS": "-mod=mod"})
        out = repo / ".go/selected"
        return r.returncode, out.read_text() if out.exists() else None, r.stdout + r.stderr

    repo.mkdir()
    write("go.mod", "module m\n\ngo 1.21\n")
    write("lib/lib.go", "package lib\n\nfunc X() int { return 1 }\n")
    write("a/a.go", "package a\n")
    write("a/a_test.go", 'package a\n\nimport (\n\t"testing"\n\n\t"m/lib"\n)\n\nfunc TestA(t *testing.T) { _ = lib.X() }\n')
    write("b/b_test.go", "package b\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) {}\n")
    write("b/testdata/x.txt", "x\n")
    for d in (".", "internal/item", "internal/cli"):
        write(f"{d}/src_test.go", f'package {Path(d).name or "m"}\n\nimport "testing"\n\n// keeps ".go" files\n'
                                  'func TestSrc(t *testing.T) {}\n')
    git("init", "-q", "-b", "main"), git("add", "."), git("commit", "-qm", "base")
    assert run("push")[:2] == (0, ".\n./a\n./b\n./internal/cli\n./internal/item\n")
    # lib's change runs a, which links it, and the root's and internal/item's tests, which read every .go file
    for n, (path, want) in enumerate((("lib/lib.go", ".\n./a\n./internal/item\n"), ("b/testdata/x.txt", "./b\n"))):
        git("checkout", "-qb", f"pr{n}", "main")
        write(path, (repo / path).read_text() + "// changed\n")
        git("commit", "-qam", "pr"), git("checkout", "-q", "main")
        git("merge", "-q", "--no-ff", "-m", "merge", f"pr{n}")  # as refs/pull/N/merge is
        code, out, log = run("pull_request")
        assert (code, out) == (0, want), log
        git("reset", "-q", "--hard", "HEAD^1")
    code, out, log = run("pull_request")  # main's own commit, one parent: not a PR's merge commit
    assert code != 0 and "merge commit" in log, log


def steps(workflow: Path) -> list[str]:
    """Each step's text in a workflow, in order: the lines from one `      - ` step to the next."""
    return re.findall(r"^      - .*?(?=^      - |^  \S|\Z)", workflow.read_text(), re.M | re.S)


@pytest.mark.parametrize("workflow", ["pm-go.yml", "pm-lint.yml"])
def test_a_go_job_skips_its_steps_after_the_change_check_and_still_runs_on_every_pr(workflow):
    """The Go jobs run on every pull request, so merge_ready.py requires their checks; a PR whose changes no Go job
    reads skips every step after the check: each one carries its output, or needs a step that does to have run."""
    path = PM_DIR / ".github/workflows" / workflow
    assert re.search(r"^  pull_request:\n  push:\n", path.read_text(), re.M)  # no paths: every PR runs it
    go = "steps.changes.outputs.go == 'true'"
    gated: set[str] = set()
    for body in steps(path):
        if "uses: actions/checkout" in body and gated:  # the next job: pm-lint.yml's Python job, which always runs
            break
        if "id: changes" in body or "uses: actions/checkout" in body:
            continue
        cond = re.search(r"^        if: (.*(?:\n          .*)*)", body, re.M)
        after = {m[1] for m in re.finditer(r"steps\.(\w+)\.(?:outcome|outputs\.[\w-]+ != '')", cond[1])} if cond else set()
        assert cond and (go in cond[1] or after & gated), body  # a skipped step's outcome is skipped, its outputs ''

        if sid := re.search(r"^        id: (\S+)", body, re.M):
            gated.add(sid[1])
    assert "go" in gated and ("suite" in gated) == (workflow == "pm-go.yml")


def test_pm_go_runs_linux_on_every_pr_and_push_and_macos_once_a_day():
    """The owner asked for the macOS jobs once a day, not on every PR and push: job macos runs only on the schedule and
    on demand, every test (no PR subset), and job test, Linux's, only on a PR or a push; both run the same steps."""
    path = PM_DIR / ".github/workflows/pm-go.yml"
    text = path.read_text()
    assert re.search(r'^  schedule:\n    - cron: "\d+ \d+ \* \* \*"\n  workflow_dispatch:\n', text, re.M)
    js = jobs(path)
    assert set(js) == {"test", "macos"}
    assert "    if: github.event_name == 'pull_request' || github.event_name == 'push'\n" in js["test"]
    assert "    if: github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'\n" in js["macos"]
    assert "macos-" not in js["test"] and "ubuntu-" not in js["macos"]
    assert "    steps: &steps\n" in js["test"] and js["macos"].endswith("    steps: *steps\n")
    assert "pr_run" not in text


def test_every_restore_of_pm_go_s_caches_names_the_key_pm_go_saves():
    """pm-lint.yml and pm-release-build.yml restore the caches pm-go.yml saves: a key in an old form restores a stale
    cache, which its own restores keep from expiring, or none."""
    key = re.compile(r"go-[\w-]*?(?:linux-amd64|darwin-arm64|\$\{\{ matrix\.target \}\})-")
    keys = {f.name: set(key.findall(f.read_text())) for f in (PM_DIR / ".github/workflows").glob("*.yml")}
    keys = {name: ks for name, ks in keys.items() if ks}
    assert keys == {"pm-go.yml": {"go-unzipped-${{ matrix.target }}-"}, "pm-lint.yml": {"go-unzipped-linux-amd64-"},
                    "pm-release-build.yml": {"go-unzipped-${{ matrix.target }}-"}}


RELEASE_YML = PM_DIR / ".github/workflows/pm-release.yml"
spec = importlib.util.spec_from_file_location("main_ci", PM_DIR / "release" / "main_ci.py")
main_ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(main_ci)


def jobs(workflow: Path) -> dict[str, str]:
    """Each job's text, by its name: the lines under `jobs:` from one two-space-indented name to the next."""
    text = workflow.read_text().split("\njobs:\n", 1)[1]
    return dict(re.findall(r"^  ([\w-]+):\n(.*?)(?=^  [\w-]+:\n|\Z)", text, re.M | re.S))


def step_script(workflow: Path, step_id: str) -> str:
    """The `run: |` script of the step with that id, dedented."""
    lines = workflow.read_text().splitlines()
    i = next(n for n, line in enumerate(lines) if line.strip() == f"id: {step_id}")
    i = next(n for n in range(i, len(lines)) if lines[n].strip() == "run: |") + 1
    indent = len(lines[i]) - len(lines[i].lstrip())
    script = []
    while i < len(lines) and (not lines[i].strip() or len(lines[i]) - len(lines[i].lstrip()) >= indent):
        script.append(lines[i][indent:])
        i += 1
    return "\n".join(script) + "\n"


def test_the_release_runs_on_a_push_to_main_one_run_at_a_time_and_only_its_tag_and_publish_jobs_write():
    """A push to main releases with GITHUB_TOKEN alone: a tag it pushes starts no workflow, so the same run builds,
    tags and publishes; no run is cut short, so a pushed commit never lacks its tag; only the two jobs that push and
    publish may write."""
    text = RELEASE_YML.read_text()
    assert re.search(r"^on:\n  push:\n    branches: \[main\]\n    tags: \[\"pm-v\*\"\]\n", text, re.M)
    assert re.search(r"^permissions:\n  contents: read\n", text, re.M)
    # main's pushes share one group; each hand-pushed tag has its own, so no merge replaces its pending run
    assert re.search(r"^concurrency:\n  group: pm-release-\$\{\{ github.ref \}\}\n  cancel-in-progress: false\n",
                     text, re.M)
    j = jobs(RELEASE_YML)
    assert list(j) == ["version", "build", "tag", "publish"]
    assert "needs: version\n" in j["build"] and "needs: [version, build]\n" in j["tag"]
    assert "needs: [version, build, tag]\n" in j["publish"]
    for name, body in j.items():
        writes = re.search(r"permissions:\n(?:      .*\n)*?      contents: write", body) is not None
        assert writes == (name in ("tag", "publish")), name
    assert "actions: read" in j["version"]  # main_ci.py reads the runs on the commit
    assert "release/main_ci.py \"$GITHUB_SHA\"" in j["version"] and "release/changelog.py plan" in j["version"]
    assert 'git push --atomic origin HEAD:refs/heads/main "refs/tags/pm-v$X"' in j["tag"]
    assert "github.ref_type == 'branch'" in j["tag"]
    assert "secrets." not in text


def test_main_ci_waits_for_every_push_workflow_and_fails_on_any_that_did_not_pass():
    assert main_ci.push_workflows(PM_DIR) == {"pm changelog", "pm go", "pm lint", "pm tests"}  # paths: not every push
    required = {"pm go", "pm tests"}
    ok = [{"databaseId": 1, "workflowName": "pm go", "status": "completed", "conclusion": "success"},
          {"databaseId": 2, "workflowName": "pm tests", "status": "completed", "conclusion": "skipped"},
          {"databaseId": 3, "workflowName": "pm release", "status": "in_progress", "conclusion": ""}]
    assert main_ci.verdict(ok, required) == ([], [])
    assert main_ci.verdict(ok[:1], required) == ([], ["pm tests has no run yet"])
    going = [*ok, {"databaseId": 4, "workflowName": "pm release build", "status": "queued", "conclusion": ""}]
    assert main_ci.verdict(going, required) == ([], ["pm release build is queued"])
    failed, _ = main_ci.verdict([*ok, {"databaseId": 5, "workflowName": "pm lint", "status": "completed",
                                       "conclusion": "cancelled", "url": "u"}], required)
    assert failed == ["pm lint concluded cancelled (u)"]


class Origin:
    """A clone of pm's release files on a local bare origin, at a release tag, with a fake gh: `gh release view` passes
    unless the tag is in $UNPUBLISHED, and `gh run list` prints $FAKE_RUNS."""

    def __init__(self, tmp: Path):
        self.tmp, self.dir = tmp, tmp / "clone"
        bin_dir = tmp / "bin"
        bin_dir.mkdir()
        (bin_dir / "gh").write_text("""#!/bin/sh
case "$1 $2" in
  "release view") case " $UNPUBLISHED " in *" $3 "*) echo "release not found" >&2; exit 1 ;; esac ;;
  "run list") cat "$FAKE_RUNS" ;;
  *) echo "fake gh: $*" >&2; exit 2 ;;
esac
""")
        (bin_dir / "gh").chmod(0o755)
        self.runs = tmp / "runs.json"
        self.env = {**os.environ, "PATH": f"{bin_dir}:{os.environ['PATH']}", "FAKE_RUNS": str(self.runs),
                    "UNPUBLISHED": "", "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t",
                    "GIT_COMMITTER_EMAIL": "t@t"}
        self.dir.mkdir()
        for path in ("CHANGELOG.md", "changelog.d/README.md", "install.sh", "release/changelog.py",
                     "release/main_ci.py"):
            (self.dir / path).parent.mkdir(exist_ok=True)
            shutil.copy(PM_DIR / path, self.dir / path)
        shutil.copytree(PM_DIR / ".github", self.dir / ".github")
        self.git("init", "-q", "-b", "main")
        self.commit("base")
        newest = re.search(r"^## \[(\S+)\]", (self.dir / "CHANGELOG.md").read_text(), re.M)[1]
        self.last = newest
        self.git("tag", f"pm-v{newest}")
        subprocess.run(["git", "init", "-q", "--bare", "-b", "main", str(tmp / "origin.git")], check=True)
        self.git("remote", "add", "origin", str(tmp / "origin.git"))
        self.git("push", "-q", "origin", "main", "--tags")
        self.passed()

    def git(self, *args: str) -> str:
        return subprocess.run(["git", *args], cwd=self.dir, env=self.env, check=True, capture_output=True,
                              text=True).stdout.strip()

    def commit(self, message: str):
        self.git("add", "-A")
        self.git("commit", "-q", "--allow-empty", "-m", message)

    def merge(self, files: dict[str, str]) -> str:
        """A PR merged: files written, committed and pushed to main."""
        for name, body in files.items():
            (self.dir / name).parent.mkdir(exist_ok=True)
            (self.dir / name).write_text(body)
        self.commit("a merge")
        self.git("push", "-q", "origin", "HEAD:main")
        return self.git("rev-parse", "HEAD")

    def passed(self, conclusion: str = "success"):
        self.runs.write_text(json.dumps([{"databaseId": i, "workflowName": w, "status": "completed",
                                          "conclusion": conclusion, "url": "u"}
                                         for i, w in enumerate(sorted(main_ci.push_workflows(PM_DIR)))]))

    def step(self, step_id: str, sha: str, **extra: str) -> tuple[subprocess.CompletedProcess, dict[str, str]]:
        """Run the release workflow's step as GitHub runs it on a push of sha: bash -e, its outputs read back."""
        out = self.tmp / "output"
        out.write_text("")
        env = {k: v for k, v in self.env.items() if not k.startswith(("GIT_AUTHOR_", "GIT_COMMITTER_"))}  # as a runner
        r = subprocess.run(["bash", "-e", "-c", step_script(RELEASE_YML, step_id)], cwd=self.dir, capture_output=True,
                           text=True, check=False, env={**env, "GITHUB_SHA": sha, "GITHUB_OUTPUT": str(out),
                                                        "GITHUB_STEP_SUMMARY": str(self.tmp / "summary"), **extra})
        return r, dict(line.split("=", 1) for line in out.read_text().splitlines())


ADDED = {"changelog.d/a.md": "Adds A.\n\n### Added\n\n- A.\n"}


@pytest.mark.integration  # a bare origin
def test_a_push_to_main_releases_only_what_the_plan_says_once_main_s_ci_passed(tmp_path):
    o = Origin(tmp_path)
    major, minor, patch = (int(p) for p in o.last.split("."))
    sha = o.merge({"README": "docs only"})
    r, out = o.step("plan", sha)
    assert r.returncode == 0 and out == {"release": "false"}, r.stderr
    sha = o.merge({**ADDED, "release/hold": "Feature F, completed by #9\n"})
    r, out = o.step("plan", sha)
    assert r.returncode == 0 and out == {"release": "false"}, r.stderr
    assert "release/hold holds releases: Feature F" in (tmp_path / "summary").read_text()
    o.git("rm", "-q", "release/hold")
    o.commit("the feature is complete")
    o.git("push", "-q", "origin", "main")
    sha = o.git("rev-parse", "HEAD")
    r, out = o.step("plan", sha)
    x = f"{major}.{minor}.{patch + 1}"  # an Added entry with no upgrade step: a patch
    assert r.returncode == 0 and out == {"release": "true", "version": x, "prerelease": "false", "tag": f"pm-v{x}"}, (
        r.stderr)

    o.passed("failure")  # main's CI on the commit failed: no release, and the run fails
    r, out = o.step("plan", sha)
    assert r.returncode == 1 and "did not pass, so it does not release" in r.stderr and "release" not in out
    o.passed()
    r, _ = o.step("plan", sha, UNPUBLISHED=f"pm-v{o.last}")  # a tag whose publish failed comes first
    assert r.returncode == 1 and f"tag pm-v{o.last} has no GitHub release" in r.stdout
    r, out = o.step("plan", o.git("rev-parse", "HEAD^"))  # main moved past the pushed commit
    assert r.returncode == 0 and out == {"release": "false"}


@pytest.mark.integration  # a bare origin
def test_the_tag_step_pushes_the_release_commit_and_tag_together_or_neither_when_main_moved(tmp_path):
    o = Origin(tmp_path)
    sha = o.merge(ADDED)
    x = "9.9.9"  # the version the plan gave; any version newer than the changelog's
    r, out = o.step("push", sha, X=x)
    assert r.returncode == 0 and out == {"pushed": "true"}, r.stderr
    o.git("fetch", "-q", "origin", "--tags")
    released = o.git("rev-parse", "origin/main")
    assert o.git("rev-parse", f"pm-v{x}^{{commit}}") == released and o.git("rev-parse", "origin/main^") == sha
    assert o.git("diff", "--name-status", sha, released).splitlines() == ["M\tCHANGELOG.md", "D\tchangelog.d/a.md"]
    assert o.git("ls-tree", "--name-only", released, "changelog.d/") == "changelog.d/README.md"  # pm embeds the dir
    assert o.git("log", "-1", "--format=%an <%ae>%n%s", released) == (
        "github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>\nRelease pm 9.9.9")
    assert f"## [{x}] - " in o.git("show", f"{released}:CHANGELOG.md")

    o.git("reset", "-q", "--hard", released)
    sha = o.merge({"changelog.d/b.md": "Fixes B.\n\n### Fixed\n\n- B.\n"})
    o.git("checkout", "-q", "-b", "other")
    o.merge({"README": "a later merge"})  # main moves on while the run builds
    o.git("checkout", "-q", "--detach", sha)
    r, out = o.step("push", sha, X="9.9.10")
    assert r.returncode == 0 and out == {"pushed": "false"}, r.stderr
    assert not o.git("ls-remote", "origin", "refs/tags/pm-v9.9.10")
    assert "main moved past" in (tmp_path / "summary").read_text()
