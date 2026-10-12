"""The CI workflows' own logic: which changes run the release build test, and the Go test job's package list. The
expected results come from what each job is for, on fake tools built here."""

from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path

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
    """make test-live-parity gives each scenario a worker, so a run takes as long as its slowest scenario, and AGENTS.md
    names how many there are: both follow the scenarios pytest collects, which a new scenario changes."""
    res = subprocess.run([sys.executable, "-m", "pytest", "--collect-only", "-q", "-p", "no:xdist",
                          "tests/test_agent_parity_live.py"], cwd=PM_DIR, capture_output=True, text=True, check=False)
    scenarios = [line for line in res.stdout.splitlines() if "::" in line]
    assert res.returncode == 0 and scenarios, res.stdout + res.stderr
    target = re.search(r"^test-live-parity:.*\n\t(.*)$", (PM_DIR / "Makefile").read_text(), re.M)
    assert target and re.search(r" -n (\d+) ", target[1])[1] == str(len(scenarios)), target
    agents = (PM_DIR / "AGENTS.md").read_text()
    counts = re.findall(r"\b[Ii]ts (\d+) scenarios|\bThe (\d+) scenarios", agents)
    assert len(counts) == 2 and {a or b for a, b in counts} == {str(len(scenarios))}, counts


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
