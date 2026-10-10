"""The CI workflows' own logic: which changes run the release build test, and the Go test job's package list. The
expected results come from what each job is for, on fake tools built here."""

from __future__ import annotations

import os
import re
import subprocess
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


def test_go_test_but_work_tests_every_package_but_the_work_store(tmp_path):
    res = subprocess.run(["make", "-s", "go-test-but-work"], cwd=PM_DIR, env=fake_go(tmp_path, False),
                         capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert (tmp_path / "test.log").read_text() == "test -tags gms_pure_go m/a m/b\n"


def test_go_test_but_work_fails_when_go_list_fails_and_tests_nothing(tmp_path):
    res = subprocess.run(["make", "-s", "go-test-but-work"], cwd=PM_DIR, env=fake_go(tmp_path, True),
                         capture_output=True, text=True)
    assert res.returncode != 0
    assert "go: broken" in res.stderr
    assert not (tmp_path / "test.log").exists()
