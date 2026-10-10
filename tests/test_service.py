"""The pm service end to end: `pm service` against the fake launchctl and systemctl (fake_sched.py) and a real
`pm service run`, so no test installs a real service. Its units and their parts are Go unit tests (internal/service)."""

from __future__ import annotations

import http.server
import re
import socket
import subprocess
import threading
import urllib.request

import pytest

from conftest import PM, VERSION, stop_services


def service_run(repo, port: str) -> tuple[subprocess.Popen, int]:
    """`pm service run` on `port` (0: a free one), as the supervisor starts it, its stdout and stderr in one pipe."""
    repo.stop_service()  # this test's own service holds the work store
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
    old version, and the supervisor's restart then runs the installed pm."""
    srv, port = service_run(repo, "0")
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}/style.css") as r:
            assert r.headers["X-PM-Version"] == VERSION  # the version stamped at build
        cfg = repo.root / ".pm/config.toml"
        moved = cfg.with_suffix(".new")  # replaced whole, as git pull does: the service never reads it half written
        moved.write_text(cfg.read_text().replace(f'version = "{VERSION}"', 'version = "9.9.9"'))
        moved.replace(cfg)
        out, _ = srv.communicate(timeout=10)
    finally:
        srv.kill()
    assert srv.returncode == 1
    assert f"now pins pm 9.9.9, but this service runs pm {VERSION}; stopping" in out, out
