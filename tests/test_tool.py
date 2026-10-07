"""The pm uv tool (pm/tool.py) in-process, with a fake `uv` on PATH that lays the tool out under tmp as `uv tool
install` does and logs each call, so no test installs or reads the user's tools."""

from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path

import pytest

from conftest import PM
from pm import __version__, tool
from pm.records import RecordError

FAKE_UV = '''#!{py}
import json, os, shutil, sys
from pathlib import Path
args = [a for a in sys.argv[1:] if a not in ("--color", "never")]
with open(os.environ["FAKE_UV_LOG"], "a") as f:
    f.write(json.dumps(args) + "\\n")
tools, bindir = Path(os.environ["UV_TOOL_DIR"]), Path(os.environ["UV_TOOL_BIN_DIR"])
if args == ["tool", "dir"]:
    print(tools)
elif args == ["tool", "dir", "--bin"]:
    print(bindir)
elif args[:3] == ["tool", "install", "--reinstall"]:  # as conftest.install_tool lays it out, plus the spec's PEP 610
    # record: a dist-info first on the tool interpreter's path, so its pm names the commit the spec installs
    url, rest = args[3].removeprefix("git+").rsplit("@", 1)
    commit, _, sub = rest.partition("#subdirectory=")
    shutil.rmtree(tools / "pm", ignore_errors=True)
    dist = tools / "pm/site" / "pm-{version}.dist-info"
    dist.mkdir(parents=True)
    (dist / "METADATA").write_text("Metadata-Version: 2.1\\nName: pm\\nVersion: {version}\\n")
    (dist / "direct_url.json").write_text(json.dumps({{"url": url, "subdirectory": sub,
                                                       "vcs_info": {{"vcs": "git", "commit_id": commit}}}}))
    (tools / "pm/bin").mkdir()
    for name, target in (("python", {py!r}), ("pm", {pm!r})):
        (tools / "pm/bin" / name).write_text(f'#!/bin/sh\\nPYTHONPATH="{{tools / 'pm/site'}}" exec "{{target}}" "$@"\\n')
        (tools / "pm/bin" / name).chmod(0o755)
    bindir.mkdir(exist_ok=True)
    if not (bindir / "pm").is_symlink():
        (bindir / "pm").symlink_to(tools / "pm/bin/pm")
else:
    sys.exit(f"fake uv: unsupported {{args}}")
'''

SOURCE = {"url": "https://github.com/Yeeef/yeeef-agents",
          "vcs_info": {"vcs": "git", "commit_id": "a2ae084e5afaead30af0ae972eec16b984a4bc27",
                       "requested_revision": "pm-init"}, "subdirectory": "pm"}


@pytest.fixture
def uv(tmp_path, monkeypatch):
    """The fake uv first on PATH, the tool not installed, and the running pm said to come from SOURCE (as under
    uvx); returns the log of uv calls."""
    fakes = tmp_path / "fakes"
    fakes.mkdir()
    (fakes / "uv").write_text(FAKE_UV.format(py=sys.executable, pm=PM[0], version=__version__))
    (fakes / "uv").chmod(0o755)
    monkeypatch.setenv("UV_TOOL_DIR", str(tmp_path / "tools"))
    monkeypatch.setenv("UV_TOOL_BIN_DIR", str(tmp_path / "bin"))
    monkeypatch.setenv("FAKE_UV_LOG", str(tmp_path / "uv.log"))
    monkeypatch.setenv("PATH", f"{tmp_path / 'bin'}{os.pathsep}{fakes}{os.pathsep}/usr/bin:/bin")
    (tmp_path / "uv.log").write_text("")
    runs_from(monkeypatch, SOURCE)
    return lambda: [json.loads(l) for l in (tmp_path / "uv.log").read_text().splitlines()]


def runs_from(monkeypatch, source: dict) -> None:
    """The running pm said to come from `source`, as its direct_url.json."""
    direct = json.dumps(source)
    monkeypatch.setattr(tool.importlib.metadata, "distribution",
                        lambda name: type("D", (), {"read_text": lambda self, f: direct})())


def spec(source: dict) -> str:
    return f"git+{source['url']}@{source['vcs_info']['commit_id']}#subdirectory={source['subdirectory']}"


def test_init_installs_the_tool_from_the_git_commit_it_runs_from(uv, tmp_path):
    said = tool.ensure()
    assert ["tool", "install", "--reinstall", spec(SOURCE)] in uv()
    assert said == f"installed the pm uv tool {__version__} from {spec(SOURCE)}"
    assert tool.current() == tmp_path / "tools/pm/bin/python", "the unit runs the tool's interpreter"
    n = len(uv())
    assert tool.ensure() == "" and not [c for c in uv()[n:] if c[:2] == ["tool", "install"]], "current: no reinstall"


def test_path_drops_the_running_environments_bin_dir(uv, monkeypatch):
    """uvx puts its ephemeral environment's bin first on PATH; neither the unit nor the `pm` check may use it."""
    own, rest = Path(sys.prefix) / "bin", os.environ["PATH"]
    monkeypatch.setenv("PATH", f"{own}{os.pathsep}{rest}")
    assert tool.path() == rest
    assert Path(PM[0]).parent.resolve() == own.resolve()


def test_init_replaces_a_tool_built_from_another_commit_of_this_version(uv, monkeypatch):
    """The live check's bug: a tool of the same version from another commit was kept, and the service then ran it."""
    tool.ensure()
    other = {**SOURCE, "vcs_info": {**SOURCE["vcs_info"], "commit_id": "5e8bd2b" + "0" * 33}}
    runs_from(monkeypatch, other)
    with pytest.raises(RecordError, match=re.escape(f"runs pm {__version__} from {spec(SOURCE)}, not pm {__version__} "
                                                    f"from {spec(other)}; run pm init, or install it with uv tool "
                                                    f'install --reinstall "{spec(other)}"')):
        tool.current()
    assert tool.ensure() == f"installed the pm uv tool {__version__} from {spec(other)}"
    assert uv()[-3] == ["tool", "install", "--reinstall", spec(other)]
    assert tool.running() == f"{__version__} at {other['vcs_info']['commit_id']} in pm" and tool.current()


def test_a_launched_pm_keeps_the_launcher_tool_and_takes_any_launcher_as_current(uv, monkeypatch, tmp_path):
    """In a repo pinned to another version the tool launched this pm: installing this build would replace the
    launcher, so pm init leaves the tool and says so, and the service's unit still runs the tool's interpreter."""
    tool.ensure()  # the machine's launcher, built from SOURCE
    runs_from(monkeypatch, {**SOURCE, "vcs_info": {**SOURCE["vcs_info"], "commit_id": "5e8bd2b" + "0" * 33}})
    monkeypatch.setenv("PM_LAUNCHED", __version__)
    n = len(uv())
    py = tmp_path / "tools/pm/bin/python"
    assert tool.ensure() == (f"left the pm uv tool ({py}) as it is: it launched this pm {__version__} for the repo's "
                             "pin, and installing this version would replace the launcher")
    assert not [c for c in uv()[n:] if c[:2] == ["tool", "install"]] and tool.current() == py
    dist = next((tmp_path / "tools/pm/site").glob("*.dist-info"))
    (dist / "METADATA").write_text("Metadata-Version: 2.1\nName: pm\nVersion: 0.1.0\n")  # a tool from before the launcher
    with pytest.raises(RecordError, match=re.escape(f"runs pm 0.1.0 from {spec(SOURCE)}, which cannot run this repo's "
                                                    f"pin {__version__}; install a pm uv tool at 0.1.2 or later")):
        tool.current()
