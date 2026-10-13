"""The CI workflows' own logic: which changes run the release build test, the Go test job's package list, and the
release on a push to main (its jobs, their tokens, and the scripts of its plan and tag steps, run against a local bare
origin). The expected results come from what each job is for, on fake tools built here."""

from __future__ import annotations

import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

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
