"""The static refusal check (the pm-go page, Tests): the constant part of each refusal Python pm raises in a command
Go pm runs appears in Go pm's source, so a refusal text the tests or prime.md cite cannot drift between the two. It
reads both sources and runs neither pm."""

from __future__ import annotations

import ast
import re
from pathlib import Path

PM = Path(__file__).resolve().parents[1]
CLI = PM / "src/pm/cli.py"
GO_SOURCES = [*sorted((PM / "internal/cli").glob("*.go")), *sorted((PM / "internal/store").glob("*.go")),
              *sorted((PM / "internal/install").glob("*.go"))]

# The Python functions behind the commands Go pm runs, and the helpers whose refusals they raise.
PORTED = {
    "Repo.sprint", "Repo.project", "check_planned", "parse_sections", "decision_body", "read_text_file",
    "cmd_finding_add", "cmd_feedback_add", "cmd_doc_new", "cmd_design_new", "cmd_postmortem_new", "cmd_project_open",
    "cmd_project_close", "cmd_sprint_open", "cmd_sprint_close", "require_committed", "open_sprint", "open_task",
    "cmd_task_add", "cmd_task_close", "cmd_task_claim", "cmd_task_move", "cmd_show", "record_section", "link_target",
    "cmd_record_link", "cmd_commit", "summarize_one", "ask_model",
    "decision_target", "decision_line", "decision_block", "refuse_unread", "cmd_decision_add", "need_part",
    "need_markdown", "raise_need", "cmd_action_need", "raise_review", "human_issue",
    "cmd_decision_close", "cmd_action_done", "cmd_reply_read",
    # pm init's, upgrade's and uninstall's refusals that Go pm keeps; the Beads, uv and legacy ones it drops by design
    "cmd_upgrade", "cmd_uninstall", "code_top", "check_site_url", "check_hooks_path", "codex_config", "add_codex_roots",
    "remove_codex_roots", "claude_dirs", "setup_claude",
}

GO_STRING = r'"(?:[^"\\\n]|\\.)*"|`[^`]*`'
GO_JOINED = re.compile(rf"(?:{GO_STRING})(?:\s*\+\s*(?:{GO_STRING}))*")


def constants(tree: ast.Module) -> dict[str, str]:
    """The module's string constants by name (TEXT_FORMS, FRAME_SHAPE, …), which refusals interpolate."""
    out = {}
    for node in tree.body:
        if (isinstance(node, ast.Assign) and len(node.targets) == 1 and isinstance(node.targets[0], ast.Name)
                and isinstance(node.value, ast.Constant) and isinstance(node.value.value, str)):
            out[node.targets[0].id] = node.value.value
    return out


def functions(tree: ast.Module):
    """Each function by its name, a method as Class.name."""
    for node in tree.body:
        if isinstance(node, ast.FunctionDef):
            yield node.name, node
        elif isinstance(node, ast.ClassDef):
            for sub in node.body:
                if isinstance(sub, ast.FunctionDef):
                    yield f"{node.name}.{sub.name}", sub


def refusal_parts() -> list[tuple[str, str]]:
    """(function, constant part) of each Refuse(...) the ported functions raise: a message's text between its
    interpolations, and each named module constant it interpolates (Go names them too, and interpolates them)."""
    tree = ast.parse(CLI.read_text())
    named = constants(tree)
    out = []
    for name, fn in functions(tree):
        if name not in PORTED:
            continue
        for node in ast.walk(fn):
            if not (isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == "Refuse"
                    and node.args):
                continue
            msg = node.args[0]
            pieces = []
            for v in (msg.values if isinstance(msg, ast.JoinedStr) else [msg]):
                if isinstance(v, ast.Constant) and isinstance(v.value, str):
                    pieces.append(v.value)
                elif isinstance(v, ast.FormattedValue) and isinstance(v.value, ast.Name) and v.value.id in named:
                    pieces.append(named[v.value.id])
            out += [(name, p.strip()) for p in pieces if len(p.strip()) >= 3]
    return out


def go_strings() -> list[str]:
    """Every string in Go pm's command and store sources, each `"a" + "b"` concatenation joined."""
    out = []
    for path in GO_SOURCES:
        if path.name.endswith("_test.go"):
            continue
        for m in GO_JOINED.finditer(path.read_text()):
            out.append("".join(ast.literal_eval(s) if s.startswith('"') else s[1:-1]
                               for s in re.findall(GO_STRING, m.group(0))))
    return out


# Refusals Go pm words for the work store where Python pm names a bd command: Python's part, Go's.
GO_WORDING = {"is not in a sprint with a record; set its sprint with bd update --parent":
              "is not in a sprint with a record, so no sprint can record the scope change"}


def test_every_ported_refusal_text_is_in_go_source():
    parts = refusal_parts()
    assert {name for name, _ in parts} == PORTED, "a ported function raises no refusal: did cli.py change?"
    assert set(GO_WORDING) <= {part for _, part in parts}, "a GO_WORDING entry names no Python refusal"
    go = go_strings()
    missing = [(name, part) for name, part in parts if not any(GO_WORDING.get(part, part) in s for s in go)]
    assert missing == [], "\n".join(f"{name}: {part!r}" for name, part in missing)
