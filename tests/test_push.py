"""In-process tests of the push (pm/push.py) the service runs: failure paths, with git replaced where a subprocess run
cannot reach them."""

from __future__ import annotations

import os
import subprocess
import sys
import time
from datetime import timedelta
from pathlib import Path

import pytest

from conftest import HARNESS

sys.path.insert(0, str(HARNESS))
from pm import push  # noqa: E402
from pm.records import RecordError  # noqa: E402


def git(cwd: Path, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True).stdout.strip()


@pytest.fixture
def clone(tmp_path, monkeypatch):
    """A main checkout with its store on branch records, an origin holding records, and a clone of origin that
    has moved it by a commit conflicting with one in the store."""
    main, origin, other = tmp_path / "main", tmp_path / "origin.git", tmp_path / "other"
    for k, v in {"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@e", "GIT_COMMITTER_NAME": "t",
                 "GIT_COMMITTER_EMAIL": "t@e"}.items():
        monkeypatch.setenv(k, v)
    git(tmp_path, "init", "-q", "--bare", str(origin))
    git(tmp_path, "init", "-q", "-b", "records", str(main))
    (main / "a.md").write_text("base\n")
    git(main, "add", "-A")
    git(main, "commit", "-qm", "base")
    git(main, "remote", "add", "origin", str(origin))
    git(main, "push", "-q", "origin", "records")
    git(main, "checkout", "-q", "-b", "main")
    git(main, "worktree", "add", "-q", ".records", "records")
    git(tmp_path, "clone", "-q", "-b", "records", str(origin), str(other))
    (other / "a.md").write_text("remote\n")
    git(other, "commit", "-qam", "remote")
    git(other, "push", "-q", "origin", "records")
    store = main / ".records"
    (store / "a.md").write_text("local\n")
    git(store, "commit", "-qam", "local")
    monkeypatch.setenv("HOME", str(tmp_path / "home"))
    monkeypatch.setenv("XDG_CONFIG_HOME", str(tmp_path / "home/.config"))
    return main, store


def test_a_failed_rebase_abort_is_reported_as_needing_repair(clone, monkeypatch):
    main, store = clone
    real = push.run
    monkeypatch.setattr(push, "run", lambda cmd, cwd, timeout=None: (False, "abort failed")
                        if cmd[1:3] == ["rebase", "--abort"] else real(cmd, cwd))
    ok, said = push.push_records(store, "origin")
    assert not ok and "needs manual repair" in said and "git -C" in said, said


def test_a_rebase_abort_that_leaves_head_off_the_branch_is_reported(clone, monkeypatch):
    main, store = clone
    real = push.run

    def fake(cmd, cwd, timeout=None):
        if cmd[1:3] == ["rebase", "--abort"]:
            subprocess.run(["git", "rebase", "--quit"], cwd=cwd, capture_output=True)
            subprocess.run(["git", "checkout", "-q", "--detach"], cwd=cwd, capture_output=True)
            return True, ""
        return real(cmd, cwd)
    monkeypatch.setattr(push, "run", fake)
    ok, said = push.push_records(store, "origin")
    assert not ok and "needs manual repair" in said, said


def test_a_timeout_kills_the_whole_process_group(tmp_path, monkeypatch):
    monkeypatch.setattr(push, "TIMEOUT", 1)
    pid = tmp_path / "pid"
    ok, said = push.run(["sh", "-c", f"sleep 30 & echo $! > {pid}; wait"], tmp_path)
    assert not ok and "timed out" in said
    child = int(pid.read_text())
    time.sleep(0.2)
    with pytest.raises(ProcessLookupError):
        os.kill(child, 0)


def test_a_step_that_raises_is_recorded_as_a_failure(clone, monkeypatch):
    main, store = clone
    monkeypatch.setattr(push, "push_beads", lambda m: (True, "fine"))

    def find():
        raise RecordError("no records store")
    code, said = push.push(main, "origin", find, lambda: (True, "skipped"))
    assert code == 1
    state = push.read_state(main)
    assert state["beads"]["ok"] and not state["records"]["ok"] and "no records store" in state["records"]["message"]


def test_a_failed_summary_is_flagged_and_the_records_still_pushed(clone, monkeypatch):
    main, store = clone
    monkeypatch.setattr(push, "push_beads", lambda m: (True, "fine"))
    pushed = []
    monkeypatch.setattr(push, "push_records", lambda s, remote: pushed.append(s) or (True, "pushed 1 commit(s)"))

    code, said = push.push(main, "origin", lambda: store, lambda: (False, "claude -p failed (exit 1): boom"))
    assert code == 1 and pushed == [store]
    assert "summary error: claude -p failed (exit 1): boom" in push.files(main)[1].read_text()
    flagged = push.flags(main, store, "origin")
    assert len(flagged) == 1 and flagged[0].startswith("summary step failed at ") and "boom" in flagged[0], flagged


def test_an_installed_schedule_that_never_ran_is_flagged_overdue(clone):
    main, store = clone
    push.write_state(main, {"installed_at": (push.now() - timedelta(hours=1)).isoformat()})
    lines = push.flags(main, store, "origin")
    assert [l.split(":")[0] for l in lines] == ["beads push overdue", "summary step overdue", "records push overdue"], lines
    push.write_state(main, {"installed_at": push.now().isoformat()})
    assert push.flags(main, store, "origin") == []


def test_push_writes_each_line_to_the_log_once_in_the_runtime_dir(clone, monkeypatch):
    main, store = clone
    monkeypatch.setattr(push, "push_beads", lambda m: (True, "fine"))
    push.push(main, "origin", lambda: store, lambda: (True, "skipped"))
    assert push.files(main) == tuple(main / ".pm/run" / f for f in ("push.json", "push.log", "push.lock"))
    assert len(push.files(main)[1].read_text().splitlines()) == 3
