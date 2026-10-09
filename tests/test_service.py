"""The pm service (pm/service.py): its units in-process with the supervisor's tools replaced, and `pm service` end to
end against the fake launchctl and systemctl (fake_sched.py) and a real `pm service run`, so no test installs a real
service."""

from __future__ import annotations

import http.server
import json
import os
import plistlib
import re
import socket
import subprocess
import sys
import threading
import urllib.request
from pathlib import Path

import pytest

from conftest import IMPL, PM, TEST_SOURCE, stop_services
from pm import __version__, push, service, tool

IN_PROCESS = pytest.mark.impl("python", reason="the service's units in process; Go pm has them as Go unit tests")


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
    for t in ("bd", "git", "uv"):
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
    # the pm uv tool is this interpreter, this pm its git build, and the site comes up at once:
    # test_service_*_end_to_end runs both for real
    monkeypatch.setattr(tool, "own", lambda: json.dumps(TEST_SOURCE))
    monkeypatch.setattr(tool, "python", lambda: Path(sys.executable))
    monkeypatch.setattr(tool, "current", lambda: Path(sys.executable))
    monkeypatch.setattr(service, "answering", lambda port: (str((main / ".pm/store/records").resolve()), tool.running()))
    monkeypatch.setattr(service, "wait_up", lambda main, port, done, hint="": world.setdefault("waited", []).append(port))
    return main, calls, world


@IN_PROCESS
def test_service_help_holds_the_service_context(capsys):
    """pm prime only points at `pm service --help`, so the help carries what an agent needs."""
    from pm.cli import main
    with pytest.raises(SystemExit):
        main(["service", "--help"])
    text = " ".join(capsys.readouterr().out.split())
    for fact in ("every 600 s, the first push 600 s after it starts", "KeepAlive", "Restart=always",
                 "PORT=<n> pm service install", "the installed unit's port", "<main checkout>/.pm/run/",
                 "service.log", "push.json", "X-PM-Store", "X-PM-Version", "Stale build",
                 "run pm service restart; if that fails, raise an action", "pm uninstall"):
        assert fact in text, fact


@IN_PROCESS
def test_systemd_unit_runs_this_pm_restarts_it_and_quotes_paths(machine, monkeypatch):
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    said = service.install(main, 8123)
    assert said.startswith("installed the pm service: systemd user service local.pm.main."), said
    assert world["waited"] == [8123], "install waits for the site to answer"
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


@IN_PROCESS
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


UNITS = Path(__file__).resolve().parent.parent / "internal/service/testdata/units"


@IN_PROCESS
@pytest.mark.parametrize("kind,ext", [("launchd", "plist"), ("systemd", "service")])
def test_unit_files_equal_the_files_go_pm_is_held_to(monkeypatch, kind, ext):
    """Go pm's unit test holds its unit files to these files, written by this code; this holds Python's to the same, so
    both write the same unit for the same inputs (the clone, PATH, port and command)."""
    for c in json.loads((UNITS / "cases.json").read_text()):
        monkeypatch.setattr(service, "command", lambda py, argv=c["argv"]: argv)
        want = (UNITS / f"{c['name']}.{ext}").read_bytes()
        assert service.unit_bytes(Path(c["main"]), kind, c["path"], c["port"], Path("unused")) == want, c["name"]


def service_run(repo, port: str) -> tuple[subprocess.Popen, int]:
    """`pm service run` on `port` (0: a free one), as the supervisor starts it, its stdout and stderr in one pipe."""
    srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root, env=dict(repo.env, PORT=port),
                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    while not (found := re.search(r"^Serving http://localhost:(\d+)", srv.stdout.readline())):
        assert srv.poll() is None, "pm service run exited before serving"
    return srv, int(found.group(1))


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.mark.integration
def test_service_install_status_restart_and_logs_end_to_end(repo, tmp_path):
    """Under the fake supervisor, which starts the unit's command as launchd or systemd would."""
    status = repo.pm("service", "status")
    assert status.returncode == 1 and "not installed; run pm service install" in status.stdout, status
    repo.dolt()
    port = free_port()
    env = repo.env
    repo.env = dict(env, PORT=str(port))
    res = repo.pm("service", "install")
    assert res.returncode == 0 and res.stdout.startswith("installed the pm service: "), res
    assert f"serving http://localhost:{port}" in res.stdout
    repo.env = env  # the unit carries the port; status and restart read it from there, not from $PORT
    up = repo.pm("service", "status")
    assert up.returncode == 0 and up.stdout.splitlines()[0].endswith(f"running; the site answers on :{port}"), up
    log = repo.root / ".pm/run/service.log"
    assert f"log       {log} (pm service logs)" in up.stdout
    restarted = repo.pm("service", "restart")
    assert restarted.returncode == 0, restarted
    assert restarted.stdout.splitlines()[-1].endswith(f"running; the site answers on :{port}")
    assert re.search(rf"^Serving http://localhost:{port}; .*pushing every 10 min$",
                     repo.pm("service", "logs", "-n", "50").stdout, re.M)
    link = repo.pm("record", "link", "repo-demo.1")  # no $PORT: links use the port the unit serves on
    assert link.stdout == f"http://localhost:{port}/sprints/demo-1.html\n", link
    again = repo.pm("service", "install")
    assert again.returncode == 0 and again.stdout.startswith("already installed and current"), again
    stop_services(tmp_path)  # the process dies; nothing restarts it here
    down = repo.pm("service", "status")
    assert down.returncode == 1 and f"down: nothing answers on :{port}; run pm service restart" in down.stdout
    assert repo.pm("where").stdout.count(f"down: nothing answers on :{port}") == 1
    assert ".pm/" not in repo.git("status", "--porcelain"), "the runtime dir is never committed"


@pytest.mark.integration
def test_install_fails_when_the_service_does_not_come_up(repo):
    """Another server holds the port (another clone's service, say): the new one cannot bind, and install says so
    instead of reporting success."""
    repo.dolt()
    other = http.server.ThreadingHTTPServer(("127.0.0.1", 0), http.server.SimpleHTTPRequestHandler)
    threading.Thread(target=other.serve_forever, daemon=True).start()
    port = other.server_address[1]
    try:
        res = subprocess.run([*PM, "service", "install"], cwd=repo.root, env=dict(repo.env, PORT=str(port)),
                             capture_output=True, text=True)
    finally:
        other.shutdown()
    assert res.returncode == 1, res
    assert f"the site does not answer for this store on :{port}" in res.stderr, res.stderr
    assert "held by a server that is not pm" in res.stderr
    assert "give this clone its own port with PORT=<n> pm service install" in res.stderr


@pytest.mark.integration
def test_service_stops_once_the_pin_moves(repo):
    """A pull after pm upgrade moves the pin under a running service: it stops with an error instead of serving the
    old version, and the supervisor's restart then runs the pm uv tool's."""
    repo.dolt()
    srv, port = service_run(repo, "0")
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}/style.css") as r:
            build = __version__ if IMPL == "go" else tool.build(__version__, json.dumps(TEST_SOURCE))
            assert r.headers["X-PM-Version"] == build  # Go pm's version, stamped at build; Python pm's git build
        cfg = repo.root / ".pm/config.toml"
        moved = cfg.with_suffix(".new")  # replaced whole, as git pull does: the service never reads it half written
        moved.write_text(cfg.read_text().replace(f'version = "{__version__}"', 'version = "9.9.9"'))
        moved.replace(cfg)
        out, _ = srv.communicate(timeout=10)
    finally:
        srv.kill()
    assert srv.returncode == 1
    assert f"now pins pm 9.9.9, but this service runs pm {__version__}; stopping" in out, out


# a service on another version, and on this version built from another commit (the live check's stale tool)
OTHER_BUILDS = ["0.0.1", f"{__version__} at {'a2ae084' + '0' * 33} in pm"]


@IN_PROCESS
@pytest.mark.parametrize("other", OTHER_BUILDS)
def test_install_restarts_a_current_unit_that_answers_on_another_build(machine, monkeypatch, other):
    """The unit's bytes do not change with the tool's build: a held unit answering on another one is restarted."""
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    service.install(main, 8123)
    monkeypatch.setattr(service, "answering", lambda port: (str((main / ".pm/store/records").resolve()), other))
    unit = service.unit_file(main, "systemd")
    assert service.install(main, 8123).startswith("updated the pm service")
    assert calls[-1] == ["systemctl", "--user", "restart", unit.name] and world["waited"] == [8123, 8123]


@IN_PROCESS
def test_free_port_skips_held_ports_and_other_clones_units(monkeypatch):
    """A new repo's default site port: the first from FIRST_PORT up that nothing holds and no pm unit names."""
    with socket.socket() as held:
        held.bind(("127.0.0.1", 0))
        held.listen()
        start = held.getsockname()[1]
        monkeypatch.setattr(service, "FIRST_PORT", start)
        unit = service.unit_file(Path("/elsewhere/other"), service.platform_kind())
        unit.parent.mkdir(parents=True, exist_ok=True)
        unit.write_bytes(service.unit_bytes(Path("/elsewhere/other"), service.platform_kind(), "/bin", start + 1,
                                            Path("/py")))
        try:
            assert start + 1 in service.unit_ports()
            got = service.free_port()
            assert got >= start + 2 and service.port_free(got)
            assert not any(service.port_free(p) for p in range(start + 2, got)), "the first free one"
            assert not service.port_free(start)
        finally:
            unit.unlink()


@IN_PROCESS
def test_port_free_counts_a_port_in_time_wait_as_free():
    """The server closes first, so its side of the connection lingers in TIME_WAIT, as after this clone's service
    stopped: the service binds with SO_REUSEADDR and could serve there, so the port is free."""
    with socket.socket() as srv:
        srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        srv.bind(("127.0.0.1", 0))
        srv.listen()
        port = srv.getsockname()[1]
        cli = socket.create_connection(("127.0.0.1", port))
        conn, _ = srv.accept()
        conn.close()  # the active close: TIME_WAIT on the server's port
        cli.close()
    assert service.port_free(port)


@IN_PROCESS
def test_check_port_takes_its_own_units_port_where_nothing_answers(tmp_path):
    """A process holds the port and answers no HTTP: refused, unless this clone's installed unit serves on that port
    (its own service hung, which install restarts)."""
    main = tmp_path / "main"
    main.mkdir()
    with socket.socket() as held:
        held.bind(("127.0.0.1", 0))
        held.listen()
        port = held.getsockname()[1]
        with pytest.raises(service.RecordError, match="held by a process that does not answer HTTP"):
            service.check_port(main, port)
        kind = service.platform_kind()
        unit = service.unit_file(main, kind)
        unit.parent.mkdir(parents=True, exist_ok=True)
        unit.write_bytes(service.unit_bytes(main, kind, "/bin", port + 1, Path("/py")))
        try:
            with pytest.raises(service.RecordError, match="does not answer HTTP"):
                service.check_port(main, port)  # its unit serves on another port
            unit.write_bytes(service.unit_bytes(main, kind, "/bin", port, Path("/py")))
            service.check_port(main, port)
        finally:
            unit.unlink()


@IN_PROCESS
def test_unit_ports_skips_a_drop_in_directory():
    """systemd keeps a unit's overrides in local.pm.<name>.service.d/, which the glob matches too."""
    kind = service.platform_kind()
    unit = service.unit_file(Path("/elsewhere/other"), kind)
    drop_in = unit.parent / f"{unit.name}.d"
    drop_in.mkdir(parents=True, exist_ok=True)
    unit.write_bytes(service.unit_bytes(Path("/elsewhere/other"), kind, "/bin", 8765, Path("/py")))
    try:
        assert 8765 in service.unit_ports()
    finally:
        unit.unlink()
        drop_in.rmdir()


@IN_PROCESS
def test_install_holds_the_clones_install_lock(machine, monkeypatch):
    """Two installs of one clone never overlap: while one runs (here, waiting for the site), the lock is held."""
    import fcntl
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    held = []

    def wait_up(main, port, done, hint=""):
        with open(main / ".pm/run/install.lock") as f:
            try:
                fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                held.append(True)
            else:
                held.append(False)
    monkeypatch.setattr(service, "wait_up", wait_up)
    service.install(main, 8123)
    assert held == [True]
    with open(main / ".pm/run/install.lock") as f:
        fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB)  # released once install returns


@IN_PROCESS
def test_a_unit_an_old_pin_wrote_for_its_own_tool_is_stale_until_pm_service_install(machine, monkeypatch):
    """pm 0.1.0, launched with its own uv tool dirs, wrote a unit running that tool; once the pin moves on, the unit
    keeps restarting 0.1.0, which exits on the new pin. Health (session start, pm where) names the fix: install."""
    main, calls, world = machine
    monkeypatch.setattr(sys, "platform", "linux")
    service.install(main, 8123)
    unit = service.unit_file(main, "systemd")
    old = "/data/pm/pins/0.1.0/tools/pm/bin/python"
    unit.write_text(unit.read_text().replace(f'ExecStart="{sys.executable}"', f'ExecStart="{old}"'))
    ok, line = service.health(main, main / ".pm/store/records")
    assert not ok and line.endswith(f"stale: its unit runs {old} -m pm.cli service run, not the pm uv tool "
                                    f"({sys.executable} -m pm.cli service run); run pm service install"), line
    assert service.install(main, 8123).startswith("updated the pm service")
    assert service.health(main, main / ".pm/store/records")[0]
