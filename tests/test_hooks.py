"""`pm prime` (the session and subagent context) and `pm hook stop` (blocks on this session's uncommitted records),
run as the runtimes run them: JSON on stdin, in a temp clone. The rules' chunks are checked against conftest.chunks(),
cut from prime.md: test_rules_chunks_fit_the_cap_and_add_up_to_the_rules fails when a chunk outgrows Claude Code's
cap; then move a heading in internal/hooks' Starts and in conftest's RULE_STARTS, or add one plus its hook entries,
which test_init_bootstraps_a_brand_new_repo checks."""

from __future__ import annotations

import datetime
import json
import re
import shutil
import socket
import subprocess
import time
from pathlib import Path

import pytest

from conftest import CAP, MACHINERY, PM, RULE_STARTS, SUBAGENT_RULE, chunks, head, help_tree, hook_payload, nouns, \
    rules, stop_services, wait_for, write_config, VERSION

STATE = [*PM, "prime", "--state", "--hook-json"]
SUBAGENT = [*PM, "prime", "--subagent", "--hook-json"]
STOP = [*PM, "hook", "stop"]


def rules_cmd(n):
    return [*PM, "prime", "--rules", str(n), "--hook-json"]


def run(cmd, event, env, cwd):
    return subprocess.run(cmd, input=json.dumps(event), env=env, cwd=cwd, capture_output=True, text=True, timeout=60,
                          check=False)


def init_ready(repo):
    """Session start runs pm init, which installs a missing pm service (under the fake supervisor) on the config's
    port, since it drops $PORT: install the service once, by hand, on a free port, never the user's."""
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    repo.env = dict(repo.env, PORT=str(port))
    first = repo.pm("init")  # a clone set up once, as pm init leaves it
    assert first.returncode == 0, first.stderr


# ---------------------------------------------------------------- session context


def context_of(res, event="SessionStart"):
    assert res.returncode == 0, res.stderr
    out = json.loads(res.stdout)["hookSpecificOutput"]
    assert out["hookEventName"] == event
    return out["additionalContext"]


@pytest.mark.integration
def test_session_start_injects_rules_then_init_where_and_pm_show(repo):
    """One hook per rules chunk, then the state, each under Claude Code's per-hook cap, with pm show whole."""
    init_ready(repo)
    event = {"hook_event_name": "SessionStart", "cwd": str(repo.root)}
    got = [context_of(run(rules_cmd(n), event, repo.env, repo.root)) for n in range(1, len(RULE_STARTS) + 1)]
    assert got == chunks() and got[0].startswith("# pm rules (1 of 3): the introduction; ")
    text = context_of(run(STATE, event, repo.env, repo.root))
    shown = repo.pm("show").stdout.strip()
    ran, _, rest = text.partition("\n\n")
    # quiet when set up: nothing changed, the service is current
    assert ran.startswith(f"`pm init` at session start:\nalready set up: {repo.records} -> {repo.store}\n"), ran
    assert "installed" not in ran and "updated" not in ran and "wrote" not in ran, ran
    located, _, rest = rest.partition("\n\n")
    assert located == "Locations from `pm where` at session start:\n" + repo.pm("where").stdout.strip()
    assert f"checkout  {repo.root}  branch main, records link set up" in located
    header, _, body = rest.partition("\n\n")
    assert re.fullmatch(r"Project state from `pm show` at session start, \d{4}-\d\d-\d\d \d\d:\d\d UTC: .*"
                        r"run `pm show` again before stating project state to the owner\.", header)

    def minute(s: str) -> str:  # a stamp may cross a minute between two calls
        return re.sub(r"\d\d:\d\d UTC", "hh:mm UTC", s)
    assert minute(body) == minute(shown) and "decision .1.2  Ask the owner  (sprint 1)" in body
    plain = repo.pm("prime")  # by hand: the rules whole and in order, then the state, no envelope
    assert plain.returncode == 0 and minute(plain.stdout.strip()) == minute(head() + "\n\n" + text)


@pytest.mark.integration
def test_session_start_sets_up_a_worktree_post_checkout_skipped(repo):
    """Claude Code's worktrees: added with --no-checkout, then reset, so git never runs post-checkout. A branch that
    tracks a copy of records/ (cut from a main branch that still carried an earlier pm's copy) gets that copy where the
    link goes: session start leaves it, and pm init and pm where say to merge the main branch."""
    init_ready(repo)
    for tracked, name in ((False, "bridge"), (True, "copy")):
        if tracked:
            # folded: test_session_start_sets_up_a_worktree_post_checkout_skipped
            repo.records.unlink()
            (repo.records / "sprints").mkdir(parents=True)
            (repo.records / "sprints/demo-1.md").write_text("copy\n")
            repo.git("add", "-f", "records")
            repo.git("commit", "-qm", "copy records")
            shutil.rmtree(repo.records)
            repo.records.symlink_to(repo.store)
        wt = repo.root.parent / name
        repo.git("worktree", "add", "-q", "--no-checkout", "-b", name, str(wt))
        repo.git("reset", "-q", "--hard", cwd=wt)
        assert (wt / "records").is_dir() == tracked and not (wt / "records").is_symlink()
        text = context_of(run(STATE, {"hook_event_name": "SessionStart", "cwd": str(wt)}, repo.env, wt))
        assert repo.git("status", "--porcelain", cwd=wt) == ""
        assert repo.git("config", "--get", "--default=", "core.sparseCheckout", cwd=wt).strip() == ""
        ran, located, rest = text.split("\n\n", 2)
        if tracked:
            assert (wt / "records/sprints/demo-1.md").read_text() == "copy\n", "the tracked copy is left as it is"
            assert (f"{wt / 'records'} is this branch's tracked copy of records/, not the link to {repo.store}; merge "
                    "the main branch, which tracks none, then run pm init again") in ran, ran
            assert (f"checkout  {wt}  branch {name}, records/ is this branch's tracked copy, not the link; merge the "
                    "main branch, which tracks none, then run pm init") in located, located
            continue
        assert (wt / "records").is_symlink() and (wt / "records").resolve() == repo.store.resolve(), text
        assert ran.startswith("`pm init` at session start:\n") and f"linked {wt / 'records'} -> {repo.store}" in ran
        assert f"checkout  {wt}  branch {name}, records link set up" in located
        assert rest.startswith("Project state from `pm show` at session start, ")
        assert "decision .1.2  Ask the owner  (sprint 1)" in rest


@pytest.mark.integration
def test_session_start_starts_a_down_service_once(repo, tmp_path):
    """Every pm store command needs the service, so session start starts an installed one that does not answer,
    under the clone's install lock: once, however many sessions start at once. A running one is left as it is."""
    init_ready(repo)
    port = int(repo.env["PORT"])
    repo.env = {k: v for k, v in repo.env.items() if k != "PORT"}
    stop_services(tmp_path)  # the process dies; the fake supervisor still holds the unit
    calls = (tmp_path / "sched.log").read_text()
    starts = [subprocess.Popen([*PM, "init", "--session-start"], cwd=repo.root, env=repo.env, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE) for _ in range(3)]
    outs = [p.communicate(timeout=60) for p in starts]
    assert all(p.returncode == 0 for p in starts), outs
    assert sum("restarted the pm service" in out for out, _ in outs) == 1, outs
    kicks = (tmp_path / "sched.log").read_text()[len(calls):]
    assert kicks.count("kickstart") + kicks.count('"restart"') == 1, kicks
    assert repo.pm("show", "repo-demo.1").returncode == 0, "the store answers again"
    text = context_of(run(STATE, {"hook_event_name": "SessionStart", "cwd": str(repo.root)}, repo.env, repo.root))
    assert "restarted the pm service" not in text.partition("\n\n")[0], "a running service is left as it is"
    assert f":{port}" in text


@pytest.mark.integration
def test_session_start_fails_open_with_one_line(repo):
    init_ready(repo)
    shutil.rmtree(repo.root / ".pm/store/work")  # pm show reads the work store, which is gone
    text = context_of(run(STATE, {"cwd": str(repo.root)}, repo.env, repo.root))
    _, located, shown = text.split("\n\n", 2)
    assert located.startswith("Locations from `pm where` at session start:\n")  # pm where reads no work store
    assert shown.startswith("pm show failed at session start (") and "\n" not in shown


def test_prime_lists_every_agent_command_pm_help_lists(repo, tmp_path):
    """pm prime's last line names every noun pm --help lists but the runtimes' and the scheduler's (MACHINERY), in its
    order; prime.md points at --help for the rest."""
    last = head().split("\n")[-1]
    shown = repo.pm("prime", "--rules", str(len(RULE_STARTS)))
    assert shown.stdout.rstrip("\n").split("\n")[-1] == last, shown.stderr
    listed = re.findall(r"`(\w+)`", last[len("`pm` nouns: "):])
    assert listed == [n for n in nouns() if n not in MACHINERY] and "show" in listed and "prime" not in listed
    assert "for more, run `pm <noun> [cmd] --help`." in rules()  # the pointer the compact list relies on

    # folded: test_subagent_start_envelope
    # the rules chunks and the git line, each a SubagentStart envelope; no pm show
    bare = tmp_path / "bare"
    subprocess.run(["git", "init", "-q", str(bare)], check=True)
    write_config(bare)
    (bare / "bin").mkdir()
    (bare / "bin/git").symlink_to(shutil.which("git"))
    event = {"hook_event_name": "SubagentStart", "cwd": str(bare)}
    env = {"PATH": str(bare / "bin")}  # git for pm's config check
    text = context_of(run(SUBAGENT, event, env, bare), "SubagentStart")
    assert text == SUBAGENT_RULE
    got = [context_of(run(rules_cmd(n), event, env, bare), "SubagentStart") for n in range(1, len(RULE_STARTS) + 1)]
    assert got == chunks() and all(len(c) <= CAP for c in got)


def options_of(help_text: str) -> set[str]:
    """The long flags a --help's options section lists: what the command's parser defines (help.go and pflag
    generate that section from it), never its usage line or prose, which a hand may write."""
    _, _, options = help_text.partition("\noptions:\n")
    return {m.group(1) for m in re.finditer(r"^ {2,6}(?:-\w, )?(--[a-z][a-z-]*)", options, re.M)}


def flags_named(text: str):
    """Each `pm <words> --flag` the text names, as (the words, the flags), the mention running to the end of its
    code span, clause or row (a line and the lines indented under it, as a help text wraps one); a usage line names the
    command it is the help of, so it is skipped."""
    for row in re.split(r"\n(?! {7})", text):
        for m in re.finditer(r"(?<![\w/.$:-])pm ", row):
            if row[:m.start()].endswith("usage: "):
                continue
            words = []
            for tok in row[m.end():].split():
                end = "`" in tok or tok.startswith("(") or tok == "pm" or tok[-1] in ".,;:)"
                tok = tok.split("`")[0].rstrip(".,;:)") if not tok.startswith("(") and tok != "pm" else ""
                words += [w for w in re.split(r"\\?\|", tok) if w]
                if end:
                    break
            flags = [f.group(1) for w in words if (f := re.match(r"\[?(--[a-z][a-z-]*)", w))]
            yield words, [f for f in flags if f != "--help"]


def test_every_flag_prime_md_and_the_help_texts_name_parses():
    """A flag named after `pm <command>` in prime.md or any --help is one that command parses: its own --help lists
    it. The two forms `Execute` routes before the tree (`pm show ID`, `pm task add --parent`) have their own help."""
    tree = help_tree()
    forms = {("show", "ID"): ("show", "ID"), ("task", "add", "--parent"): ("task", "add", "--parent", "TASK")}
    helps = tree | {form: subprocess.run([*PM, *argv, "--help"], cwd="/", check=True, capture_output=True,
                                         text=True).stdout for form, argv in forms.items()}
    docs = {"prime.md": rules()} | {" ".join(("pm", *path, "--help")): text for path, text in helps.items()}
    checked, wrong = 0, []
    for source, text in docs.items():
        for words, flags in flags_named(text):
            path = ()
            while len(path) < len(words) and (*path, words[len(path)]) in tree:
                path = (*path, words[len(path)])
            rest = words[len(path):]
            if path == ("show",) and rest and not rest[0].startswith(("-", "[")):
                path = ("show", "ID")
            elif path == ("task", "add") and "--parent" in flags:
                path = ("task", "add", "--parent")
            for flag in flags:
                checked += 1
                if flag not in options_of(helps[path]):
                    wrong.append(f"{source}: `pm {' '.join(words)}` names {flag}, which pm {' '.join(path)} does not parse")
    assert checked > 50 and not wrong, "\n".join(wrong)


def test_pm_init_is_the_one_install_command():
    """No `pm setup`: pm --help lists init only."""
    assert "setup" not in nouns() and "init" in nouns()
    res = subprocess.run([*PM, "setup"], cwd="/", capture_output=True, text=True, check=False)
    assert res.returncode == 2 and "invalid choice: 'setup'" in res.stderr, res.stderr


def test_rules_chunks_fit_the_cap_and_add_up_to_the_rules():
    """Each chunk reaches the session inline (a hook over CAP arrives as a 2 KB preview), and the chunks without
    their titles are the rules and the command list: nothing lost, nothing twice. A failure here means prime.md
    outgrew its chunks: move a heading in RULE_STARTS and pm's, or add one and its hook entries."""
    cs = chunks()
    assert len(cs) == len(RULE_STARTS) == 3
    assert all(len(c) <= CAP for c in cs), [len(c) for c in cs]
    titles, bodies = zip(*(c.split("\n\n", 1) for c in cs), strict=True)
    assert "\n\n".join(bodies) == head()
    assert [b.split("\n", 1)[0] for b in bodies] == list(RULE_STARTS)
    for n, t in enumerate(titles, 1):  # hooks arrive in any order, so each title names its place and its sections
        assert t.startswith(f"# pm rules ({n} of {len(cs)}): ") and "\n" not in t
    assert "What — 1. The layers" in titles[0] and "What — 3. Records" in titles[1] and "How; Commands" in titles[2]


# ---------------------------------------------------------------- uncommitted records


def claude_transcript(path, *tool_inputs):
    """A Claude Code transcript whose assistant turns call tools with `tool_inputs`."""
    lines = [{"type": "user", "message": {"role": "user", "content": "go"}}]
    lines += [{"type": "assistant", "message": {"role": "assistant", "content": [
        {"type": "tool_use", "id": f"t{n}", "name": "Edit", "input": i}]}} for n, i in enumerate(tool_inputs)]
    path.write_text("\n".join(json.dumps(line) for line in lines) + "\n")
    return str(path)


def edit_sprint(repo):
    (repo.store / "sprints/demo-1.md").write_text(
        (repo.store / "sprints/demo-1.md").read_text().replace("Ship it.", "Ship it soon."))


def subagent_lines(path, prompt, done):
    """A Claude Code transcript whose assistant starts a background subagent with `prompt`, as Claude Code writes it,
    then, with `done`, the task notification of its return."""
    lines = [{"type": "user", "message": {"role": "user", "content": "go"}},
             {"type": "assistant", "message": {"role": "assistant", "content": [
                 {"type": "tool_use", "id": "toolu_a1", "name": "Agent", "input": {"prompt": prompt}}]}},
             {"type": "user", "message": {"role": "user", "content": [
                 {"type": "tool_result", "tool_use_id": "toolu_a1", "content": [{"type": "text", "text": "launched"}]}]},
              "toolUseResult": {"isAsync": True, "status": "async_launched", "prompt": prompt}}]
    if done:
        lines.append({"type": "user", "message": {"role": "user", "content": (
            "<task-notification>\n<task-id>a1</task-id>\n<tool-use-id>toolu_a1</tool-use-id>\n"
            "<status>completed</status>\n</task-notification>")}})
    return [json.dumps(line) for line in lines]


def test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names(repo, tmp_path):
    """A subagent shares its session's id, and the parent's transcript holds only its prompt: a path that prompt alone
    names is the running subagent's to commit. Once it returns, or when the session's own tool call names the path too,
    the path is the session's."""
    edit_sprint(repo)
    path = str(repo.records / "sprints/demo-1.md")
    # folded: test_stop_leaves_out_a_record_only_a_running_subagents_prompt_names (its other three cases)
    for done, own_edit in ((True, True), (False, False), (False, True), (True, False)):
        lines = subagent_lines(tmp_path, f"Write the frame in {path} and commit it.", done)
        if own_edit:
            lines.append(json.dumps({"type": "assistant", "message": {"role": "assistant", "content": [
                {"type": "tool_use", "id": "toolu_e1", "name": "Edit", "input": {"file_path": path}}]}}))
        (tmp_path / "t.jsonl").write_text("\n".join(lines) + "\n")
        res = run(STOP, {"cwd": str(repo.root), "transcript_path": str(tmp_path / "t.jsonl")}, repo.env, repo.root)
        assert res.returncode == 0, res.stderr
        if done or own_edit:
            assert json.loads(res.stdout)["decision"] == "block" and "- records/sprints/demo-1.md" in res.stdout
        else:
            assert res.stdout == ""

    # folded: test_stop_passes_when_stop_hook_active
    t = claude_transcript(tmp_path / "t.jsonl", {"command": "sed -i '' s/a/b/ records/sprints/demo-1.md"})
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": t, "stop_hook_active": True}, repo.env, repo.root)
    assert res.returncode == 0 and res.stdout == ""

    # folded: test_stop_blocks_on_a_record_this_session_edited
    t = claude_transcript(tmp_path / "t.jsonl", {"file_path": str(repo.records / "sprints/demo-1.md")})
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": t, "stop_hook_active": False}, repo.env, repo.root)
    out = json.loads(res.stdout)
    assert out["decision"] == "block"
    assert "- records/sprints/demo-1.md" in out["reason"] and "pm commit -m" in out["reason"]

    # folded: test_stop_passes_another_sessions_edit
    # A dirty record no tool call of this session names is another session's; a path only in a tool result (here
    # the user's text) does not count either.
    t = tmp_path / "t.jsonl"
    t.write_text(json.dumps({"type": "user", "message": {"content": "git status: sprints/demo-1.md"}}) + "\n")
    claude_transcript(tmp_path / "other.jsonl", {"file_path": str(repo.records / "sprints/demo-2.md")})
    t.write_text(t.read_text() + (tmp_path / "other.jsonl").read_text())
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": str(t)}, repo.env, repo.root)
    assert res.returncode == 0 and res.stdout == ""

# ---------------------------------------------------------------- Claude Code and Codex alike

ME = "sess-me"
ROOT, CHILD = "01a12bee-db2f-7050-ba39-116b615a19b8", "01a12bef-027d-7c33-8fb3-8fd52363eaf7"  # a Codex thread and its subagent


def codex_rollout(path, thread, root, *commands, cwd):
    """A Codex 0.154 rollout of `thread`, a subagent of `root` when they differ: its session_meta, then a turn that runs
    each of `commands` through the exec tool in `cwd`, as Codex writes them."""
    meta = {"id": thread, "session_id": root, "cwd": str(cwd), "originator": "codex_exec", "cli_version": "0.154.0"}
    if thread != root:
        meta.update(parent_thread_id=root, forked_from_id=root, thread_source="subagent")
    at = "2026-10-11T17:07:23.155Z"
    lines = [{"timestamp": at, "type": "session_meta", "payload": meta},
             {"timestamp": at, "type": "turn_context", "payload": {"turn_id": "t1", "cwd": str(cwd)}}]
    lines += [{"timestamp": at, "type": "response_item", "payload": {
        "type": "custom_tool_call", "status": "completed", "call_id": f"call_{n}", "name": "exec",
        "input": f"text(await tools.exec_command({{cmd:{json.dumps(c)},workdir:{json.dumps(str(cwd))}}}));\n"}}
        for n, c in enumerate(commands)]
    path.write_text("".join(json.dumps(line) + "\n" for line in lines))
    return str(path)


def test_prime_answers_claude_codes_and_codexs_input_alike(repo, tmp_path):
    """Each rules chunk and the subagent line, from Claude Code's and Codex's real SessionStart and SubagentStart
    input: the same envelope."""
    for event, cmds in (("SessionStart", [rules_cmd(n) for n in range(1, len(RULE_STARTS) + 1)]),
                        ("SubagentStart", [rules_cmd(n) for n in range(1, len(RULE_STARTS) + 1)] + [SUBAGENT])):
        got = {}
        for runtime in ("claude", "codex"):
            payload = hook_payload(runtime, event, session=ME, cwd=str(repo.root), transcript=str(tmp_path / "t.jsonl"))
            got[runtime] = [context_of(run(cmd, payload, repo.env, repo.root), event) for cmd in cmds]
        assert got["claude"] == got["codex"], event
        assert got["codex"][:len(RULE_STARTS)] == chunks()


@pytest.mark.integration
def test_session_start_state_answers_claude_codes_and_codexs_input_alike(repo, tmp_path):
    init_ready(repo)
    got = {}
    for runtime in ("claude", "codex"):
        payload = hook_payload(runtime, "SessionStart", session=ME, cwd=str(repo.root), transcript=str(tmp_path / "t"))
        text = context_of(run(STATE, payload, repo.env, repo.root))
        got[runtime] = re.sub(r"at session start, [^:]*:\d\d(:\d\d)? UTC", "at session start, <time> UTC", text)
    assert got["claude"] == got["codex"]
    assert "Locations from `pm where` at session start" in got["codex"]


@pytest.mark.parametrize("dirty", [True, False], ids=["uncommitted", "committed"])
def test_stop_decides_alike_on_claude_codes_and_codexs_input(repo, tmp_path, dirty):
    """A record this session edited, left uncommitted or not, in each runtime's Stop input and transcript: the same
    verdict."""
    if dirty:
        edit_sprint(repo)
    path = str(repo.records / "sprints/demo-1.md")
    transcripts = {"claude": claude_transcript(tmp_path / "claude.jsonl", {"file_path": path}),
                   "codex": codex_rollout(tmp_path / "codex.jsonl", ROOT, ROOT, f"sed -i s/it./it_soon./ {path}",
                                          cwd=repo.root)}
    got = {}
    for runtime, t in transcripts.items():
        res = run(STOP, hook_payload(runtime, "Stop", session=ME, cwd=str(repo.root), transcript=t, reply="Done."),
                  repo.env, repo.root)
        assert res.returncode == 0, res.stderr
        got[runtime] = res.stdout
    assert got["claude"] == got["codex"]
    if dirty:
        assert json.loads(got["codex"])["decision"] == "block" and "- records/sprints/demo-1.md" in got["codex"]
    else:
        assert got["codex"] == ""


def test_a_subagents_uncommitted_record_blocks_alike_in_claude_code_and_codex(repo, tmp_path):
    """A subagent edited a record and left it uncommitted. Claude Code's parent transcript holds the subagent's prompt,
    which names the record, so the parent's Stop blocks once the subagent returned. Codex encrypts the prompt in the
    parent's rollout, so the subagent's own SubagentStop blocks, on its rollout (agent_transcript_path), with the same
    reason."""
    edit_sprint(repo)
    path = str(repo.records / "sprints/demo-1.md")
    claude_t = tmp_path / "claude.jsonl"
    claude_t.write_text("\n".join(subagent_lines(tmp_path, f"Write the frame in {path}.", True)) + "\n")
    parent = codex_rollout(tmp_path / "parent.jsonl", ROOT, ROOT, "git status", cwd=repo.root)
    child = codex_rollout(tmp_path / "child.jsonl", CHILD, ROOT, f"sed -i s/it./it_soon./ {path}", cwd=repo.root)
    claude = run(STOP, hook_payload("claude", "Stop", session=ME, cwd=str(repo.root), transcript=str(claude_t),
                                    reply="Done."), repo.env, repo.root)
    codex = run(STOP, hook_payload("codex", "SubagentStop", session=ROOT, cwd=str(repo.root), transcript=parent,
                                   agent_transcript=child, reply="Done."), repo.env, repo.root)
    assert claude.returncode == codex.returncode == 0, (claude.stderr, codex.stderr)
    assert json.loads(codex.stdout)["decision"] == "block" and codex.stdout == claude.stdout
    # the parent's own Stop sees no call that names the record
    parent_stop = run(STOP, hook_payload("codex", "Stop", session=ROOT, cwd=str(repo.root), transcript=parent,
                                         reply="Done."), repo.env, repo.root)
    assert parent_stop.returncode == 0 and parent_stop.stdout == "", parent_stop.stderr


INBOX = [*PM, "hook", "inbox"]


def need_with_reply(repo, nid: str, session: str) -> None:
    """An open decision need `session` raised, holding a site reply not delivered yet."""
    at = {"created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z"}
    reply = {"id": f"6f1c0e2a-1b2c-4d3e-8f40-5a6b7c8d9e0{nid[-1]}", "issue_id": nid, "author": "owner (site reply)",
             "text": f"Small, until tables.\n\n<!-- pm-reply 0b6c7c1e-2d3f-4a5b-9c6d-7e8f9a0b1c2{nid[-1]} -->",
             "created_at": "2026-10-03T12:00:00Z"}
    repo.add_issue({"id": nid, "title": "Parser?", "status": "open", "issue_type": "task", "parent": "repo-demo.1",
                    "labels": ["human"], "metadata": {"session": session}, "comments": [reply], **at})


def test_inbox_hands_a_reply_over_alike_in_claude_code_and_codex(repo, tmp_path):
    """A reply to a need of this session that no push delivered (Codex has none) goes in as context beside the next
    prompt, the same from either runtime's input, and is marked delivered, so it goes in once."""
    need_with_reply(repo, "repo-demo.1.3", "sess-claude")
    need_with_reply(repo, "repo-demo.1.4", "sess-codex")
    got = {}
    for runtime, session, nid in (("claude", "sess-claude", "repo-demo.1.3"), ("codex", "sess-codex", "repo-demo.1.4")):
        payload = hook_payload(runtime, "UserPromptSubmit", session=session, cwd=str(repo.root),
                               transcript=str(tmp_path / "t"), prompt="go on")
        res = run(INBOX, payload, repo.env, repo.root)
        assert res.returncode == 0 and res.stderr == "", res.stderr
        got[runtime] = res.stdout.replace(nid, "<id>")
        assert repo.items()[nid]["need"]["delivered"] == 1
        again = run(INBOX, payload, repo.env, repo.root)
        assert again.returncode == 0 and again.stdout == "", again.stdout
    assert got["claude"] == got["codex"]
    text = json.loads(got["codex"])["hookSpecificOutput"]["additionalContext"]
    assert text.startswith("The owner replied to requests this session raised:\n\n"
                           "pm: owner reply to decision <id> (Parser?), relayed from the site:")
    assert "Small, until tables." in text


def test_inbox_hands_nothing_over_at_a_stop(repo, tmp_path):
    """A block at a stop would end the turn that acts on the reply with stop_hook_active, which lets the stop checks
    through; at a stop the hook does nothing, and the reply stays undelivered."""
    need_with_reply(repo, "repo-demo.1.3", ME)
    for runtime in ("claude", "codex"):
        payload = hook_payload(runtime, "Stop", session=ME, cwd=str(repo.root), transcript=str(tmp_path / "t"),
                               reply="Done.")
        res = run(INBOX, payload, repo.env, repo.root)
        assert res.returncode == 0 and res.stdout == "", res.stderr
    assert repo.items()["repo-demo.1.3"]["need"]["delivered"] == 0


@pytest.mark.integration
def test_session_start_of_a_codex_session_started_from_claude_code_takes_no_claude_inbox(repo, tmp_path):
    """Codex's hooks get no CODEX_* variables and inherit the Claude Code session's that started Codex: session start
    runs pm show as the Codex session, so its needs trade the Claude Code socket an earlier pm stored on them for the
    session's CODEX_HOME, which the pm service queues the reply under."""
    repo.add_issue({"id": "repo-demo.1.3", "title": "Parser?", "status": "open", "issue_type": "task",
                    "parent": "repo-demo.1", "labels": ["human"], "created_at": "2026-10-01T12:00:00Z",
                    "updated_at": "2026-10-01T12:00:00Z",
                    "metadata": {"session": ROOT, "inbox": "/run/claude.sock", "inbox_host": socket.gethostname()}})
    init_ready(repo)
    env = dict(repo.env, CLAUDE_CODE_SESSION_ID="claude-parent", CLAUDE_CODE_MESSAGING_SOCKET="/run/claude.sock")
    payload = hook_payload("codex", "SessionStart", session=ROOT, cwd=str(repo.root), transcript=str(tmp_path / "t"))
    context_of(run(STATE, payload, env, repo.root))
    assert repo.items()["repo-demo.1.3"]["need"]["raised_by"]["inbox"] == f"codex-queue:{repo.env['CODEX_HOME']}"


def test_a_codex_session_whose_hooks_did_not_run_is_told_once(repo):
    """Codex skips a hook the user has not trusted, silently: a pm command in a Codex session whose rollout holds no pm
    session-start context before its first prompt warns once on stderr, naming the fix; a session whose rollout holds
    it, pm's hooks themselves, and a session whose first prompt is not written yet get no warning. The command itself
    runs as ever."""
    def rollout(thread: str, *messages: tuple[str, str]) -> None:
        """A Codex 0.154 rollout's opening: session_meta, Codex's own context, turn_context, then `messages`."""
        lines = [{"type": "session_meta", "payload": {"id": thread, "cwd": str(repo.root), "originator": "codex_exec"}},
                 {"type": "response_item", "payload": {"type": "message", "role": "user", "content": [
                     {"type": "input_text", "text": "# AGENTS.md instructions\n\n<INSTRUCTIONS>…"}]}},
                 {"type": "turn_context", "payload": {"cwd": str(repo.root)}}]
        lines += [{"type": "response_item", "payload": {"type": "message", "role": role,
                                                        "content": [{"type": "input_text", "text": text}]}}
                  for role, text in messages]
        p = Path(repo.env["CODEX_HOME"]) / "sessions/2026/10/11" / f"rollout-2026-10-11T19-07-16-{thread}.jsonl"
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text("".join(json.dumps(line) + "\n" for line in lines))

    skipped, hooked, unwritten = ("01a12bee-db2f-7050-ba39-116b615a19b8", "01a12bef-027d-7c33-8fb3-8fd52363eaf7",
                                  "01a12bef-1111-7c33-8fb3-8fd52363eaf7")
    rollout(skipped, ("user", "Run pm show."))
    rollout(hooked, ("developer", chunks()[0]), ("user", "Run pm show."))
    rollout(unwritten)

    def show(thread: str, *args: str):
        res = subprocess.run([*PM, *(args or ("show",))], cwd=repo.root, input="{}", capture_output=True, text=True,
                             env=dict(repo.env, CODEX_SESSION_ID=thread, CODEX_THREAD_ID=thread), check=False)
        return res

    first = show(skipped)
    assert first.returncode == 0 and "projects:" in first.stdout, first.stderr
    assert first.stderr.count("warning: Codex ran none of pm's hooks in this session") == 1, first.stderr
    assert f"open Codex in {repo.root}, trust pm's hooks in /hooks" in first.stderr
    again = show(skipped)
    assert again.returncode == 0 and "warning" not in again.stderr, "once per session"
    for thread in (hooked, unwritten):
        res = show(thread)
        assert res.returncode == 0 and "warning" not in res.stderr, (thread, res.stderr)
    rollout(unwritten, ("user", "Run pm show."))  # its first prompt is written now: told then
    assert "warning: Codex ran none" in show(unwritten).stderr
    for args in (("prime", "--subagent"), ("hook", "stop")):  # the hooks themselves never warn
        (repo.root / f".pm/run/codex-hooks/{skipped}").unlink(missing_ok=True)
        assert "warning" not in show(skipped, *args).stderr, args


@pytest.mark.integration
def test_a_codex_sessions_start_names_the_pm_hooks_codex_skips(repo, tmp_path):
    """A pm upgrade that rewrites one Codex hook entry leaves Codex's trust in it stale: Codex skips that hook and runs
    the unchanged ones. The state hook of a Codex session (pm runs under a codex process) asks Codex (here the fake
    app-server) which of pm's hooks it skips and opens the state with one line naming them and the fix; it asks again
    only when .codex/hooks.json or the Codex config changed. A Claude Code session asks nothing."""
    init_ready(repo)
    upgraded = repo.pm("upgrade", "--to", VERSION)  # pm's pieces as this pm writes them, .codex/hooks.json included
    assert (repo.root / ".codex/hooks.json").exists(), upgraded.stdout + upgraded.stderr
    codex_home = Path(repo.env["CODEX_HOME"])
    codex_home.mkdir(exist_ok=True)
    (codex_home / "config.toml").write_text("# trust as the user left it\n")
    trust = tmp_path / "trust.json"
    trust.write_text(json.dumps({"stop:0:0": "modified"}))  # the judge's entry changed since the user trusted it
    env = dict(repo.env, FAKE_CODEX_TRUST=str(trust))
    app_log = Path(repo.env["FAKE_CODEX_APP_LOG"])
    payload = hook_payload("codex", "SessionStart", session=ME, cwd=str(repo.root), transcript=str(tmp_path / "t"))

    def start(under: str) -> str:
        """The state hook, run as Codex runs it: under a process named `under`."""
        res = subprocess.run([under, "-c", '"$0" "$@"; :', *STATE], executable="/bin/sh", input=json.dumps(payload),
                             env=env, cwd=repo.root, capture_output=True, text=True, timeout=60, check=False)
        return context_of(res)

    def calls() -> int:
        return len(app_log.read_text().splitlines()) if app_log.exists() else 0

    before = calls()
    first = start("codex")
    line = first.split("\n\n", 1)[0]
    assert line.startswith("warning: this Codex session runs only the pm hooks Codex trusts."), first[:400]
    assert "which changed since you trusted them: stop `pm hook owner-request --runtime codex || exit 1`" in line
    assert f"open Codex in {repo.root} and trust pm's hooks in /hooks" in line
    assert calls() == before + 1
    assert start("codex").split("\n\n", 1)[0] == line and calls() == before + 1, "unchanged files: no second ask"
    # the user trusts it in /hooks, which writes Codex's config: asked again, and nothing to say
    trust.write_text("{}")
    (codex_home / "config.toml").write_text("# trust as the user left it\n[hooks.state]\n")
    assert not start("codex").startswith("warning: this Codex session") and calls() == before + 2
    assert not start("claude").startswith("warning: this Codex session") and calls() == before + 2


# ---------------------------------------------------------------- ask-tool questions

ASK = [*PM, "hook", "ask", "--runtime"]
COLOR = {"questions": [{"question": "Which color?", "header": "Color", "multiSelect": False,
                        "options": [{"label": "Red", "description": "warm"}, {"label": "Blue", "description": "cool"}]}]}


def held_task(repo, session: str) -> None:
    """repo-demo.1.3, a task `session` holds."""
    repo.add_issue({"id": "repo-demo.1.3", "title": "Paint it", "status": "in_progress", "issue_type": "task",
                    "parent": "repo-demo.1", "created_at": "2026-10-01T12:00:00Z", "updated_at": "2026-10-01T12:00:00Z",
                    "started_at": "2026-10-01T12:00:00Z",
                    "metadata": {"claimed_by": session, "claimed_at": "2026-10-01T12:00:00Z"}})


def questions(repo) -> dict[str, dict]:
    return {i: it for i, it in repo.items().items() if (it.get("need") or {}).get("kind") == "question"}


PROMPT_ID = "d45f14df-4aa7-4792-9e88-0fa538f56795"


def ask_dialog(repo, transcript: Path, **extra) -> subprocess.Popen:
    """pm hook ask, as Claude Code starts it beside an AskUserQuestion dialog: the PermissionRequest input on stdin,
    left running."""
    payload = dict(hook_payload("claude", "PermissionRequest", session=ME, cwd=str(repo.root),
                                transcript=str(transcript), prompt_id=PROMPT_ID, tool_input=COLOR), **extra)
    (transcript.parent / "input.json").write_text(json.dumps(payload))
    with (transcript.parent / "input.json").open() as stdin:
        return subprocess.Popen([*ASK, "claude"], stdin=stdin, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                env=repo.env, cwd=repo.root, text=True)


def dialog_answered(repo, transcript: Path, answer: str) -> subprocess.CompletedProcess:
    """pm hook ask at AskUserQuestion's PostToolUse, as Claude Code runs it once the owner picked answer in the
    dialog: tool_input and tool_response both hold the questions with the answers."""
    response = dict(COLOR, answers={"Which color?": answer}, annotations={})
    return run([*ASK, "claude"], hook_payload("claude", "PostToolUse", session=ME, cwd=str(repo.root),
                                              transcript=str(transcript), prompt_id=PROMPT_ID,
                                              tool_response=response), repo.env, repo.root)


def test_a_claude_question_answered_on_the_site_answers_the_dialog(repo, tmp_path):
    """The hook raises the AskUserQuestion call as a question under the task the session holds and waits; the owner's
    reply (pm reply add writes it as the site does) closes it as answered on the site, delivered, and the hook answers
    the tool with it, the option's label for the owner's text. The tool's PostToolUse then leaves it as it is."""
    held_task(repo, ME)
    transcript = tmp_path / "t.jsonl"
    proc = ask_dialog(repo, transcript)
    try:
        (nid, q), = wait_for(lambda: questions(repo).items(), "the question raised")
        assert q["parent"] == "repo-demo.1.3" and q["status"] == "open" and q["title"] == f"Session {ME} asks: Which color?"
        assert repo.pm("reply", "add", nid, text="Which color?: blue").returncode == 0
        out, err = proc.communicate(timeout=30)
    finally:
        proc.kill()
    assert proc.returncode == 0 and err == "", err
    decision = json.loads(out)["hookSpecificOutput"]
    assert decision == {"hookEventName": "PermissionRequest", "decision": {
        "behavior": "allow", "updatedInput": dict(COLOR, answers={"Which color?": "Blue"})}}
    res = dialog_answered(repo, transcript, "Blue")
    assert res.returncode == 0 and res.stdout == res.stderr == "", res.stderr
    q = repo.items()[nid]
    assert (q["status"], q["resolution"], q["close_reason"], q["need"]["delivered"]) == \
           ("closed", "answered", "answered on the site", 1)


def test_a_claude_question_answered_in_the_dialog_closes_at_post_tool_use(repo, tmp_path):
    """The dialog's answer reaches pm at the tool's PostToolUse, which closes the question with it; the waiting hook
    then exits printing nothing, so the dialog's answer stands. No transcript is read: there is none."""
    transcript = tmp_path / "t.jsonl"
    proc = ask_dialog(repo, transcript)
    try:
        (nid, q), = wait_for(lambda: questions(repo).items(), "the question raised")
        assert "parent" not in q or not q["parent"], "a session that holds no task: no parent"
        res = dialog_answered(repo, transcript, "Red")
        assert res.returncode == 0 and res.stdout == res.stderr == "", res.stderr
        out, err = proc.communicate(timeout=30)
    finally:
        proc.kill()
    assert proc.returncode == 0 and out == "" and err == "", err
    q = repo.items()[nid]
    assert (q["status"], q["resolution"], q["comments"][-1]["text"]) == ("closed", "answered", "Which color?: Red")
    assert not transcript.exists()


def test_a_claude_question_the_session_stops_waiting_on_is_dismissed(repo, tmp_path):
    """Esc fires no hook, but Claude Code sends the waiting hook SIGTERM, as it does when the session stops: the
    question closes as dismissed well within the 1.2 s before Claude Code kills the hook."""
    proc = ask_dialog(repo, tmp_path / "t.jsonl")
    try:
        (nid, _), = wait_for(lambda: questions(repo).items(), "the question raised")
        start = time.monotonic()
        proc.terminate()
        out, err = proc.communicate(timeout=30)
        took = time.monotonic() - start
    finally:
        proc.kill()
    assert proc.returncode == 0 and out == "" and err == "", err
    assert took < 1.0, f"the hook took {took:.2f}s to close the question and exit"
    q = repo.items()[nid]
    assert (q["status"], q["resolution"], q["close_reason"]) == \
           ("closed", "dismissed", "the session stopped waiting for the answer: Esc in the dialog, or the session ended")


def test_ask_steps_aside_for_a_subagent_or_another_tool(repo, tmp_path):
    """No dialog the owner sees: nothing raised, nothing printed, at once."""
    repo.mark()
    for extra in ({"agent_id": "a1", "agent_type": "general-purpose"}, {"tool_name": "Bash"}):
        payload = dict(hook_payload("claude", "PermissionRequest", session=ME, cwd=str(repo.root),
                                    transcript=str(tmp_path / "t"), prompt_id=PROMPT_ID, tool_input=COLOR), **extra)
        res = run([*ASK, "claude"], payload, repo.env, repo.root)
        assert res.returncode == 0 and res.stdout == res.stderr == "", res.stderr
    assert repo.unchanged()


def codex_ask_rollout(repo) -> Path:
    """ROOT's rollout under the test's CODEX_HOME, as Codex 0.154 writes it: a turn that asks with
    request_user_input_async (returning at once). pm reads only its path, which names the CODEX_HOME."""
    ms = int(ROOT[:8] + ROOT[9:13], 16)  # a UUIDv7's start, which names the rollout's date directory
    day = datetime.datetime.fromtimestamp(ms / 1000, datetime.UTC).strftime("%Y/%m/%d")
    path = Path(repo.env["CODEX_HOME"]) / "sessions" / day / f"rollout-2026-10-11T19-07-16-{ROOT}.jsonl"
    path.parent.mkdir(parents=True, exist_ok=True)
    args = json.dumps({"questions": [{"title": "Red or blue?", "options": ["Red", "Blue"]}]})
    lines = [{"type": "session_meta", "payload": {"id": ROOT, "session_id": ROOT, "cwd": str(repo.root)}},
             {"type": "turn_context", "payload": {"turn_id": "t1", "cwd": str(repo.root)}},
             {"type": "response_item", "payload": {"type": "function_call", "name": "request_user_input_async",
                                                   "arguments": args, "call_id": "call_4f11"}},
             {"type": "response_item", "payload": {"type": "function_call_output", "call_id": "call_4f11",
                                                   "output": '{"accepted":true}'}}]
    path.write_text("".join(json.dumps(line) + "\n" for line in lines))
    return path


def codex_asks(repo) -> str:
    """pm hook ask at Codex's PostToolUse of ROOT's request_user_input_async call: the id of the one question it
    raised."""
    path = codex_ask_rollout(repo)
    tool_input = {"questions": [{"title": "Red or blue?", "options": ["Red", "Blue"]}]}
    for _ in range(2):  # the same call again raises nothing
        res = run([*ASK, "codex"], hook_payload("codex", "PostToolUse", session=ROOT, cwd=str(repo.root),
                                                transcript=str(path), tool="request_user_input_async",
                                                tool_input=tool_input, call="call_4f11"), repo.env, repo.root)
        assert res.returncode == 0 and res.stdout == res.stderr == "", res.stderr
    (nid, q), = questions(repo).items()
    assert q["need"]["raised_by"]["inbox"] == f"codex-queue:{repo.env['CODEX_HOME']}"
    assert q["need"]["ask"] == {"call": "call_4f11", "questions": [
        {"text": "Red or blue?", "header": "", "multi": False,
         "options": [{"label": "Red", "detail": ""}, {"label": "Blue", "detail": ""}]}]}
    return nid


def codex_types(repo, prompt: str) -> None:
    """pm hook ask at Codex's UserPromptSubmit, as ROOT's owner sends prompt."""
    res = run([*ASK, "codex"], hook_payload("codex", "UserPromptSubmit", session=ROOT, cwd=str(repo.root),
                                            transcript=str(codex_ask_rollout(repo)), prompt=prompt), repo.env,
              repo.root)
    assert res.returncode == 0 and res.stdout == res.stderr == "", res.stderr


def test_codex_post_tool_use_raises_the_async_question_once(repo):
    """Codex's ask tool returns at once: its PostToolUse raises the question, once by its call id (tool_use_id), under
    the rollout's CODEX_HOME, which the service queues a site answer under; another tool's PostToolUse raises none."""
    path = codex_ask_rollout(repo)
    for tool in ["Bash", "request_user_input"]:
        res = run([*ASK, "codex"], hook_payload("codex", "PostToolUse", session=ROOT, cwd=str(repo.root),
                                                transcript=str(path), tool=tool, tool_input={"command": "ls"},
                                                call="call_9"), repo.env, repo.root)
        assert res.returncode == 0 and res.stdout == res.stderr == "", res.stderr
    assert questions(repo) == {}
    nid = codex_asks(repo)
    assert repo.items()[nid]["status"] == "open"


def test_codex_user_prompt_submit_closes_the_open_question_with_the_prompt(repo):
    """The owner's next prompt is the chat answer to the session's open question: its UserPromptSubmit closes it with
    that text, by the session, and the next prompt finds nothing open."""
    nid = codex_asks(repo)
    codex_types(repo, "Red, thanks")
    q = repo.items()[nid]
    assert (q["status"], q["resolution"], q["comments"][-1]["text"]) == ("closed", "answered", "Red, thanks")
    codex_types(repo, "and another thing")
    assert repo.items()[nid]["comments"] == q["comments"]
