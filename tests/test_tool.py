"""The pm uv tool (pm/tool.py) in-process, with a fake `uv` on PATH that lays the tool out under tmp as `uv tool
install` does and logs each call, so no test installs or reads the user's tools."""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

import pytest

from conftest import PM
from pm import __version__, tool
from pm.records import RecordError

FAKE_UV = '''#!{py}
import json, os, sys
from pathlib import Path
args = [a for a in sys.argv[1:] if a not in ("--color", "never")]
with open(os.environ["FAKE_UV_LOG"], "a") as f:
    f.write(json.dumps(args) + "\\n")
tools, bindir = Path(os.environ["UV_TOOL_DIR"]), Path(os.environ["UV_TOOL_BIN_DIR"])
if args == ["tool", "dir"]:
    print(tools)
elif args == ["tool", "dir", "--bin"]:
    print(bindir)
elif args[:2] == ["tool", "install"]:  # as conftest.install_tool lays it out
    (tools / "pm/bin").mkdir(parents=True)
    for name, target in (("python", {py!r}), ("pm", {pm!r})):
        (tools / "pm/bin" / name).write_text(f'#!/bin/sh\\nexec "{{target}}" "$@"\\n')
        (tools / "pm/bin" / name).chmod(0o755)
    bindir.mkdir(exist_ok=True)
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
    (fakes / "uv").write_text(FAKE_UV.format(py=sys.executable, pm=PM[0]))
    (fakes / "uv").chmod(0o755)
    monkeypatch.setenv("UV_TOOL_DIR", str(tmp_path / "tools"))
    monkeypatch.setenv("UV_TOOL_BIN_DIR", str(tmp_path / "bin"))
    monkeypatch.setenv("FAKE_UV_LOG", str(tmp_path / "uv.log"))
    monkeypatch.setenv("PATH", f"{tmp_path / 'bin'}{os.pathsep}{fakes}{os.pathsep}/usr/bin:/bin")
    (tmp_path / "uv.log").write_text("")
    direct = json.dumps(SOURCE)
    monkeypatch.setattr(tool.importlib.metadata, "distribution",
                        lambda name: type("D", (), {"read_text": lambda self, f: direct})())
    return lambda: [json.loads(l) for l in (tmp_path / "uv.log").read_text().splitlines()]


def test_init_installs_the_tool_from_the_git_commit_it_runs_from(uv, tmp_path):
    said = tool.ensure()
    spec = f"git+https://github.com/Yeeef/yeeef-agents@{SOURCE['vcs_info']['commit_id']}#subdirectory=pm"
    assert ["tool", "install", spec] in uv()
    assert said == f"installed the pm uv tool {__version__} from {spec}"
    assert tool.current() == tmp_path / "tools/pm/bin/python", "the unit runs the tool's interpreter"
    n = len(uv())
    assert tool.ensure() == "" and not [c for c in uv()[n:] if c[:2] == ["tool", "install"]], "current: no reinstall"


def test_init_refuses_a_pm_with_no_git_source_when_the_tool_is_missing(uv, monkeypatch):
    monkeypatch.setattr(tool.importlib.metadata, "distribution", lambda name: type("D", (), {
        "read_text": lambda self, f: json.dumps({"url": "file:///src/pm", "dir_info": {"editable": True}})})())
    with pytest.raises(RecordError, match=r"was not installed from git; install the tool with uv tool install "
                                          r"\"git\+https://github.com/Yeeef/yeeef-agents@pm-v"):
        tool.ensure()
    assert not [c for c in uv() if c[:2] == ["tool", "install"]]
    with pytest.raises(RecordError, match="is not installed, not pm"):
        tool.current()


def test_init_refuses_when_pm_on_path_is_not_the_tools(uv, tmp_path, monkeypatch):
    other = tmp_path / "other"
    other.mkdir()
    (other / "pm").write_text("#!/bin/sh\n")
    (other / "pm").chmod(0o755)
    monkeypatch.setenv("PATH", f"{other}{os.pathsep}{os.environ['PATH']}")
    with pytest.raises(RecordError, match=rf"`pm` on PATH is {other}/pm, not the pm uv tool's .*uv tool update-shell"):
        tool.ensure()


def test_path_drops_the_running_environments_bin_dir(uv, monkeypatch):
    """uvx puts its ephemeral environment's bin first on PATH; neither the unit nor the `pm` check may use it."""
    own, rest = Path(sys.prefix) / "bin", os.environ["PATH"]
    monkeypatch.setenv("PATH", f"{own}{os.pathsep}{rest}")
    assert tool.path() == rest
    assert Path(PM[0]).parent.resolve() == own.resolve()
