"""The pm service (pm/service.py): its units in-process with the supervisor's tools replaced, and `pm service` end to
end against the fake launchctl and systemctl (fake_sched.py) and a real `pm service run`, so no test installs a real
service."""

from __future__ import annotations

import os
import plistlib
import re
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

from conftest import PM
from pm import push, service
from pm.records import RecordError


@pytest.fixture
def machine(tmp_path, monkeypatch):
    """A main checkout dir, HOME in tmp, bd and git on a PATH holding a '%', and the supervisor's tools replaced by
    a recorder: `world["systemd"]` says whether a systemd user instance answers, `world["up"]` what it holds."""
    main = tmp_path / "main"
    main.mkdir()
    monkeypatch.setenv("HOME", str(tmp_path / "home"))
    monkeypatch.setenv("XDG_CONFIG_HOME", str(tmp_path / "home/.config"))
    bindir = tmp_path / "tools%x"
    bindir.mkdir()
    for t in ("bd", "git"):
        (bindir / t).write_text("#!/bin/sh\n")
        (bindir / t).chmod(0o755)
    monkeypatch.setenv("PATH", f"{bindir}{os.pathsep}/usr/bin:/bin")
    calls, world = [], {"systemd": True, "up": set()}

    def quiet(cmd):
        calls.append(cmd)
        res = lambda code: subprocess.CompletedProcess(cmd, code, "", "" if code == 0 else "failed")
        if cmd[:2] == ["systemctl", "--user"]:
            if not world["systemd"]:
                return res(1)
            if cmd[2] == "is-active":
                return res(0 if cmd[3] in world["up"] else 3)
            if cmd[2:4] == ["enable", "--now"]:
                world["up"].add(cmd[4])
            return res(0)
        if cmd[0] == "launchctl":
            name = cmd[-1].rsplit("/", 1)[-1].removesuffix(".plist")
            if cmd[1] == "print":
                return res(0 if name in world["up"] else 113)
            if cmd[1] == "bootstrap":
                world["up"].add(name)
            if cmd[1] == "bootout":
                world["up"].discard(name)
            return res(0)
        raise AssertionError(cmd)
    monkeypatch.setattr(service, "quiet", quiet)
    return main, calls, world


def test_systemd_unit_runs_this_pm_restarts_it_and_quotes_paths(machine, monkeypatch):
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    said = service.install(main, 8123)
    assert said.startswith("installed the pm service: systemd user service local.pm.main."), said
    unit = service.unit_file(main, "systemd")
    text = unit.read_text()
    assert f"WorkingDirectory={main}\n" in text
    assert f'Environment="PATH={os.environ["PATH"].replace("%", "%%")}" "PORT=8123"\n' in text
    assert f'ExecStart="{sys.executable}" "-m" "pm.cli" "service" "run"\n' in text
    assert "Type=simple\n" in text and "Restart=always\n" in text and "WantedBy=default.target\n" in text
    assert f"StandardOutput=append:{main}/.pm/run/service.log\n" in text
    assert ["systemctl", "--user", "enable", "--now", unit.name] in calls
    assert "installed_at" in push.read_state(main), "a push that never succeeds is flagged overdue from the install"
    assert service.unit_port(main, "systemd") == 8123
    n = len(calls)
    assert service.install(main, 8123) == "", "installed, current and active: nothing to do"
    assert all(c[2] in ("show-environment", "is-active") for c in calls[n:])
    assert service.install(main, 8124).startswith("updated the pm service")
    assert calls[-2:] == [["systemctl", "--user", "daemon-reload"], ["systemctl", "--user", "restart", unit.name]]


def test_launchd_agent_keeps_the_service_alive(machine, monkeypatch):
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "darwin")
    assert service.install(main, 8123).startswith("installed the pm service: launchd agent")
    plist = service.unit_file(main, "launchd")
    log = str(main / ".pm/run/service.log")
    assert plistlib.loads(plist.read_bytes()) == {
        "Label": service.label(main), "ProgramArguments": [sys.executable, "-m", "pm.cli", "service", "run"],
        "WorkingDirectory": str(main), "RunAtLoad": True, "KeepAlive": True, "StandardOutPath": log,
        "StandardErrorPath": log, "EnvironmentVariables": {"PATH": os.environ["PATH"], "PORT": "8123"}}
    assert (main / ".pm/run").is_dir(), "launchd opens the log but does not make its directory"
    assert service.install(main, 8123) == ""
    assert service.install(main, 9000).startswith("updated")
    uid = os.getuid()
    assert [c for c in calls if c[1] != "print"][-2:] == [
        ["launchctl", "bootout", f"gui/{uid}/{service.label(main)}"],
        ["launchctl", "bootstrap", f"gui/{uid}", str(plist)]], "a changed plist is loaded again, once launchd let go"


def test_a_machine_without_launchd_or_systemd_is_refused(machine, monkeypatch):
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    world["systemd"] = False
    with pytest.raises(RecordError, match="needs launchd .* or a systemd user instance"):
        service.install(main, 8000)
    assert not service.unit_file(main, "systemd").exists()
    ok, line = service.health(main, main / ".records")
    assert not ok and line.startswith("service   no service manager here")


def test_install_refuses_a_path_without_bd_and_git(machine, monkeypatch):
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    monkeypatch.setenv("PATH", "/nonexistent")
    with pytest.raises(RecordError, match="needs bd, git on PATH; bd, git not found"):
        service.install(main, 8000)


def test_restart_and_logs_refuse_before_install(machine, monkeypatch):
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    with pytest.raises(RecordError, match="not installed .*; run pm service install"):
        service.restart(main, main / ".records")
    with pytest.raises(RecordError, match="no service log at .*; run pm service install"):
        service.logs(main, 10)


def test_install_refuses_a_pm_running_from_another_worktree(machine, monkeypatch, tmp_path):
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    real = Path(shutil.which("git", path="/usr/bin:/usr/local/bin:/opt/homebrew/bin")).parent
    monkeypatch.setenv("PATH", f"{real}{os.pathsep}{os.environ['PATH']}")  # the fixture's git is a stub
    subprocess.run(["git", "init", "-q", "-b", "main", str(main)], check=True)
    subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "--allow-empty", "-m", "x"],
                   cwd=main, check=True)
    tree = tmp_path / "feature"
    subprocess.run(["git", "worktree", "add", "-q", "-b", "feature", str(tree)], cwd=main, check=True)
    monkeypatch.setattr(sys, "prefix", str(tree / "pm/.venv"))
    with pytest.raises(RecordError, match="inside the worktree .*feature; the service would run that worktree's code"):
        service.install(main, 8000)
    assert not service.unit_file(main, "systemd").exists()
    monkeypatch.setattr(sys, "prefix", str(main / "pm/.venv"))
    assert service.install(main, 8000).startswith("installed"), "the main checkout's environment is fine"


def service_run(repo, port: str, log: Path | None = None) -> tuple[subprocess.Popen, int]:
    """`pm service run` on `port` (0: a free one), as the supervisor starts it; its output to `log` when given."""
    out = open(log, "a") if log else subprocess.DEVNULL
    srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root, env=dict(repo.env, PORT=port),
                           stdout=subprocess.PIPE if not log else out, stderr=subprocess.STDOUT if log else out, text=True)
    if log:
        return srv, int(port)
    return srv, int(re.search(r"http://localhost:(\d+)", srv.stdout.readline()).group(1))


def test_service_install_status_restart_and_logs_end_to_end(repo):
    status = repo.pm("service", "status")
    assert status.returncode == 1 and "not installed; run pm service install" in status.stdout, status
    repo.dolt()
    probe, port = service_run(repo, "0")  # a free port, then the service is installed on it
    probe.terminate()
    probe.wait()
    env = repo.env
    repo.env = dict(env, PORT=str(port))
    res = repo.pm("service", "install")
    assert res.returncode == 0 and res.stdout.startswith("installed the pm service: "), res
    assert f"serving http://localhost:{port}" in res.stdout
    repo.env = env  # the unit carries the port; status and restart read it from there, not from $PORT
    down = repo.pm("service", "status")
    assert down.returncode == 1 and f"down: nothing answers on :{port}; run pm service restart" in down.stdout
    assert repo.pm("where").stdout.count(f"down: nothing answers on :{port}") == 1
    log = repo.root / ".pm/run/service.log"
    srv, _ = service_run(repo, str(port), log)
    try:
        restarted = repo.pm("service", "restart")
        assert restarted.returncode == 0, restarted
        assert restarted.stdout.splitlines()[-1].endswith(f"running; the site answers on :{port}")
        up = repo.pm("service", "status")
        assert up.returncode == 0, up
        assert up.stdout.splitlines()[0].endswith(f"running; the site answers on :{port}")
        assert f"log       {log} (pm service logs)" in up.stdout
        assert re.search(rf"^Serving http://localhost:{port}; .*pushing every 10 min$",
                         repo.pm("service", "logs", "-n", "50").stdout, re.M)
        link = repo.pm("record", "link", "demo.1")  # no $PORT: links use the port the unit serves on
        assert link.stdout == f"http://localhost:{port}/sprints/demo-1.html\n", link
        again = repo.pm("service", "install")
        assert again.returncode == 0 and again.stdout.startswith("already installed and current"), again
    finally:
        srv.terminate()
        srv.wait()
    assert ".pm/" not in repo.git("status", "--porcelain"), "the runtime dir is never committed"


def test_status_flags_a_failed_push(repo):
    push.write_state(repo.root, {"installed_at": push.now().isoformat(),
                                 "beads": {"at": push.now().isoformat(), "ok": False, "message": "bd dolt push failed",
                                           "last_ok": None, "first": push.now().isoformat()}})
    res = repo.pm("service", "status")
    assert res.returncode == 1
    assert "push      needs attention: beads push failed at " in res.stdout and "bd dolt push failed" in res.stdout
