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


@pytest.mark.parametrize("installed", [False, True])
def test_a_pm_with_no_git_source_refuses_to_install_or_check_the_tool(uv, monkeypatch, installed):
    """The review's bug: a pm from a local checkout never matched the git-installed tool and named the install
    command, which changed nothing, since the running pm was still the checkout. It refuses up front, naming how to
    run pm as the tool, whether or not the tool is installed."""
    if installed:
        tool.ensure()
    n = len(uv())
    runs_from(monkeypatch, {"url": "file:///src/pm", "dir_info": {"editable": True}})
    release = f"git+https://github.com/Yeeef/yeeef-agents@pm-v{__version__}#subdirectory=pm"
    said = re.escape(f"pm {__version__} here runs from file:///src/pm, not from git, and a local checkout cannot "
                     f'install or check the pm uv tool; run pm as the tool (uv tool install "{release}", then pm '
                     f'init), or once with uvx --from "{release}" pm init')
    for check in (tool.ensure, tool.current, tool.install_command):
        with pytest.raises(RecordError, match=said):
            check()
    assert uv()[n:] == [], "refused before uv runs"


def test_one_commit_spelled_another_way_is_one_build(uv, monkeypatch):
    """A tool installed from a URL typed another way (.git, a trailing slash, case) but the same commit and
    subdirectory is current: no reinstall, so no service restart; the URL still names the install."""
    tool.ensure()
    for url in ("https://github.com/Yeeef/yeeef-agents.git", "https://github.com/Yeeef/yeeef-agents/",
                "https://GitHub.com/yeeef/Yeeef-Agents"):
        runs_from(monkeypatch, {**SOURCE, "url": url})
        n = len(uv())
        assert tool.ensure() == "" and not [c for c in uv()[n:] if c[:2] == ["tool", "install"]], url
        assert tool.current()
        assert tool.install_command() == f'uv tool install --reinstall "{spec({**SOURCE, "url": url})}"'
    other = {**SOURCE, "subdirectory": "other"}
    runs_from(monkeypatch, other)
    with pytest.raises(RecordError, match="not pm"):
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


def test_init_refuses_a_tool_whose_source_it_cannot_read(uv, tmp_path):
    """A tool with no direct_url.json (an install from an index) may be any build: init names the command to replace
    it instead of guessing."""
    tool.ensure()
    next((tmp_path / "tools/pm/site").glob("*.dist-info/direct_url.json")).unlink()
    n = len(uv())
    with pytest.raises(RecordError, match=re.escape(f"runs pm {__version__} with no source pm can read (no PEP 610 "
                                                    "direct_url.json), so pm cannot tell its build; replace it with "
                                                    f'uv tool install --reinstall "{spec(SOURCE)}", then run pm init '
                                                    "again")):
        tool.ensure()
    assert not [c for c in uv()[n:] if c[:2] == ["tool", "install"]]
