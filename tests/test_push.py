"""In-process tests of the scheduled push (pm/push.py): failure paths and the schedulers' files, with git, the
schedulers and the platform replaced where a subprocess run cannot reach them."""

from __future__ import annotations

import json
import os
import plistlib
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
    git(main, "worktree", "add", "-q", ".pm/store/records", "records")
    git(tmp_path, "clone", "-q", "-b", "records", str(origin), str(other))
    (other / "a.md").write_text("remote\n")
    git(other, "commit", "-qam", "remote")
    git(other, "push", "-q", "origin", "records")
    store = main / ".pm/store/records"
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


def test_push_writes_each_line_to_the_log_once_and_schedulers_do_not(clone, monkeypatch):
    main, store = clone
    monkeypatch.setattr(push, "push_beads", lambda m: (True, "fine"))
    push.push(main, "origin", lambda: store, lambda: (True, "skipped"))
    assert len(push.files(main)[1].read_text().splitlines()) == 3
    assert push.launchd_job(main, "/bin")["StandardOutPath"] == "/dev/null"


# ---------------------------------------------------------------- the schedulers

@pytest.fixture
def linux(monkeypatch, tmp_path):
    """sys.platform linux, every tool on PATH, and the schedulers replaced by a recorder of calls and a crontab."""
    monkeypatch.setattr(sys, "platform", "linux")
    bindir = tmp_path / "tools"
    bindir.mkdir()
    for t in ("uv", "bd", "git"):
        (bindir / t).write_text("#!/bin/sh\n")
        (bindir / t).chmod(0o755)
    monkeypatch.setenv("PATH", f"{bindir}%x{os.pathsep}/usr/bin:/bin")
    (tmp_path / "tools%x").symlink_to(bindir)
    calls, world = [], {"systemd": True, "crontab": None, "crontab_error": "no crontab for t"}

    def quiet(cmd, input=None):
        calls.append(cmd)
        ok = subprocess.CompletedProcess(cmd, 0, "", "")
        if cmd[:2] == ["systemctl", "--user"]:
            return ok if world["systemd"] else subprocess.CompletedProcess(cmd, 1, "", "Failed to connect to bus")
        if cmd == ["crontab", "-l"]:
            return (subprocess.CompletedProcess(cmd, 0, world["crontab"], "") if world["crontab"] is not None
                    else subprocess.CompletedProcess(cmd, 1, "", world["crontab_error"]))
        if cmd == ["crontab", "-"]:
            world["crontab"] = input
            return ok
        raise AssertionError(cmd)
    monkeypatch.setattr(push, "quiet", quiet)
    return calls, world


def test_systemd_units_quote_paths_and_escape_percent(clone, linux):
    main, _ = clone
    calls, world = linux
    assert push.install(main).startswith("installed the push schedule: systemd user timer")
    service, timer = push.systemd_units(main)
    text = service.read_text()
    path = os.environ["PATH"].replace("%", "%%")
    assert f"WorkingDirectory={main}\n" in text
    assert f'Environment="PATH={path}"' in text
    assert f'ExecStart="{main}/bin/pm" push' in text
    assert "StandardOutput=null" in text and f'StandardError=append:{push.files(main)[1]}' in text
    assert "OnUnitActiveSec=600s" in timer.read_text()
    assert ["systemctl", "--user", "enable", "--now", timer.name] in calls
    assert "installed_at" in push.read_state(main)


def test_cron_is_used_without_a_systemd_user_instance(clone, linux):
    main, _ = clone
    calls, world = linux
    world["systemd"] = False
    world["crontab"] = "0 * * * * keep me\n"
    assert push.install(main).startswith("installed the push schedule: crontab entry")
    lines = world["crontab"].splitlines()
    assert lines[0] == "0 * * * * keep me" and lines[1].endswith(f"# {push.label(main)}")
    assert "> /dev/null 2>>" in lines[1] and lines[1].startswith("*/10 * * * * cd ")
    world["systemd"] = True
    assert push.install(main) == "", "the cron job counts as installed once systemd shows up"
    assert not push.systemd_units(main)[1].exists()


def test_cron_refuses_a_crontab_it_cannot_read(clone, linux):
    main, _ = clone
    calls, world = linux
    world["systemd"] = False
    world["crontab_error"] = "crontab: permission denied"
    with pytest.raises(RecordError, match="permission denied"):
        push.install(main)
    assert ["crontab", "-"] not in calls


def test_install_refuses_a_path_without_the_tools(clone, linux, monkeypatch):
    main, _ = clone
    monkeypatch.setenv("PATH", "/nonexistent")
    with pytest.raises(RecordError, match="uv, bd, git"):
        push.install(main)


def test_launchd_plist_contents(clone, monkeypatch):
    main, _ = clone
    job = push.launchd_job(main, "/x/bin")
    assert job == {"Label": push.label(main), "ProgramArguments": [str(main / "bin/pm"), "push"],
                   "WorkingDirectory": str(main), "StartInterval": 600, "RunAtLoad": True,
                   "StandardOutPath": "/dev/null", "StandardErrorPath": str(push.files(main)[1]),
                   "EnvironmentVariables": {"PATH": "/x/bin"}}
    assert plistlib.loads(plistlib.dumps(job)) == job
