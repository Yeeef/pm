"""The live evals copy the user's logins (Codex's auth.json, Claude Code's .credentials.json) into temp dirs: each copy
is the user's alone (mode 600) and is gone once its test ends, a failed test too. login_copy is the one way to make one;
this file holds it to that, and the harness to using it."""

from __future__ import annotations

import ast
import re
import stat
from pathlib import Path

import pytest
from agents import login_copy

TESTS = Path(__file__).resolve().parent


def test_a_login_copy_is_the_users_alone_and_gone_after_a_failing_body(tmp_path):
    src = tmp_path / "login"  # not a login's name: pytest keeps tmp_path, and a find for leftover copies must miss it
    src.write_text('{"tokens": "secret"}')
    dst = tmp_path / "home" / "auth.json"
    dst.parent.mkdir()
    seen = {}

    def failing_test():
        with login_copy(src, dst):
            seen.update(text=dst.read_text(), mode=stat.S_IMODE(dst.stat().st_mode))
            raise RuntimeError("the test failed")

    with pytest.raises(RuntimeError):
        failing_test()
    assert seen == {"text": '{"tokens": "secret"}', "mode": 0o600}
    assert list(dst.parent.iterdir()) == []
    assert src.read_text() == '{"tokens": "secret"}'


def test_the_harness_copies_a_login_only_through_login_copy():
    """A plain copy (shutil.copy, copy2, copyfile, copytree) of a login, or of anything from the user's own Codex or
    Claude Code dir (REAL…, CLAUDE_DIR), leaves it readable under umask 002 and behind after the run."""
    calls = [(path, node) for path in sorted(TESTS.glob("*.py")) for node in ast.walk(ast.parse(path.read_text()))
             if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute)
             and node.func.attr in ("copy", "copy2", "copyfile", "copytree")]
    found = [f"{path.name}:{node.lineno}: {ast.unparse(node)}" for path, node in calls
             if re.search(r"auth|cred|login|REAL|CLAUDE_DIR", " ".join(ast.unparse(a) for a in node.args), re.I)]
    assert not found, found
