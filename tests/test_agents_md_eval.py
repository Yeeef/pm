"""The AGENTS.md evaluation's own logic, offline: the rubric and the calls each mode makes, the answers it takes and
refuses, the verdicts, the credentials a call runs with, a call's timeout, and the labelled cases' edits. A fake
`claude` stands in for the model; tests/test_agents_md_live.py runs the real one."""

from __future__ import annotations

import http.server
import json
import os
import signal
import subprocess
import sys
import threading

import pytest

from agents_md import cases, evaluate
from conftest import CHECKOUT

AGENTS_MD = (CHECKOUT / "AGENTS.md").read_text()


def answer(findings: list[dict], **res) -> str:
    return json.dumps({"type": "result", "subtype": "success", "is_error": False, "total_cost_usd": 0.01,
                       "structured_output": {"findings": findings}, **res})


def call(criterion: str = "accurate") -> evaluate.Call:
    return evaluate.Call(criterion, "the whole file", "")


def test_the_rubric_has_every_criterion_in_order():
    pre, crit = evaluate.rubric(evaluate.RUBRIC.read_text())
    assert tuple(crit) == evaluate.CRITERIA and "No evidence, no fail" in pre
    with pytest.raises(SystemExit, match="criteria are"):
        evaluate.rubric("# r\n\n## accurate\n\nx\n")


def test_full_mode_makes_three_calls_per_criterion_and_section_and_one_complete_call():
    text = "# T\n\nintro\n\n## A\n\na `x`\n\n## B\n\nb\n"
    assert evaluate.sections(text) == [("A", "# T\n\nintro\n\n## A\n\na `x`\n\n"), ("B", "## B\n\nb\n")]
    calls = evaluate.full_calls(text, CHECKOUT, evaluate.CRITERIA)
    assert [(c.criterion, c.part) for c in calls] == [*[("accurate", "A")] * 3, *[("accurate", "B")] * 3, *(
        (c, "the whole file") for c in evaluate.CRITERIA[1:-1] for _ in range(3)), ("complete", "the whole file")]
    by = {c.criterion: c.prompt for c in calls}
    assert "<prime_md>" in by["no_copies"] and "<changelog_preamble>" in by["no_copies"]
    assert "<prime_md>" in by["consistent"] and "<prime_md>" not in by["map"]
    assert all("<agents_md>\n" + text.rstrip() + "\n</agents_md>" in c.prompt for c in calls)


def test_diff_mode_checks_accurate_and_complete_on_each_chunk_cut_at_file_boundaries():
    files = [f"diff --git a/f{i} b/f{i}\n" + "+x\n" * 40 for i in range(5)]
    cut = evaluate.chunks("".join(files), size=len(files[0]) * 2)
    assert cut == ["".join(files[0:2]), "".join(files[2:4]), files[4]]
    assert evaluate.chunks(files[0], size=10) == [files[0]]  # a file over the size, no hunk to cut at: whole
    calls = evaluate.diff_calls(AGENTS_MD, "".join(files))
    assert [(c.criterion, c.part) for c in calls] == [("accurate", "diff 1/1"), ("complete", "diff 1/1")]
    assert "<diff>\n" in calls[0].prompt and "make a claim in AGENTS.md false" in calls[0].prompt


def test_a_file_over_the_chunk_size_is_cut_at_its_hunks_each_under_its_header():
    header = "diff --git a/big b/big\nindex 1..2 100644\n--- a/big\n+++ b/big\n"
    hunk = [f"@@ -{i},2 +{i},2 @@\n" + "+y\n" * 30 for i in range(4)]
    small = "diff --git a/s b/s\n+z\n"
    size = len(header) + 2 * len(hunk[0])
    cut = evaluate.chunks(header + "".join(hunk) + small, size=size)
    assert cut == [header + hunk[0] + hunk[1], header + hunk[2] + hunk[3], small]
    assert evaluate.chunks(header + hunk[0], size=10) == [header + hunk[0]]  # one hunk over the size: whole


def test_auto_mode_is_full_when_the_diff_changes_agents_md_and_diff_otherwise(tmp_path):
    case = cases.CASES["diff_neutral"]
    base = cases.materialize(case, CHECKOUT, tmp_path)
    mode, _, calls = evaluate.plan("auto", tmp_path, tmp_path / "AGENTS.md", base, evaluate.CRITERIA)
    assert mode == "diff" and len(calls) == 2 and "go_cache_trim.py" in calls[0].prompt
    (tmp_path / "AGENTS.md").write_text(AGENTS_MD + "\nmore\n")
    cases.git(tmp_path, "commit", "-qam", "agents")
    mode, _, calls = evaluate.plan("auto", tmp_path, tmp_path / "AGENTS.md", base, evaluate.CRITERIA)
    assert mode == "full" and {c.criterion for c in calls} == set(evaluate.CRITERIA)
    mode, _, calls = evaluate.plan("diff", tmp_path, tmp_path / "AGENTS.md", "HEAD", evaluate.CRITERIA)
    assert (mode, calls) == ("diff", [])  # nothing changed: no call


def test_an_answer_is_taken_only_when_well_formed():
    text = "Run `make test` while working.\n"
    ok = {"quote": "Run make test while working", "problem": "p", "evidence": "Makefile:13"}
    assert evaluate.parse(call(), answer([]), text) == ([], "", 0.01)
    assert evaluate.parse(call(), answer([ok]), text) == ([ok], "", 0.01)
    assert evaluate.parse(call(), answer([{**ok, "quote": "Run make … working"}]), text)[1] == ""
    refused = {
        "not JSON": "oops",
        "claude failed": answer([], subtype="error_max_turns", is_error=True),
        "no structured answer": json.dumps({"subtype": "success", "result": "text"}),
        "malformed finding": answer([{"quote": "q"}]),
        "without its quote or evidence": answer([{**ok, "evidence": " "}]),
        "not in AGENTS.md": answer([{**ok, "quote": "make lint runs go-build"}]),
    }
    for why, out in refused.items():
        assert why in evaluate.parse(call(), out, text)[1], why
    # a missing rule has no text to quote
    assert evaluate.parse(call("complete"), answer([{**ok, "quote": ""}]), text)[1] == ""


def test_a_criterion_fails_when_most_calls_on_one_part_find_and_errors_on_any_bad_call():
    def res(criterion, part, found=False, error=""):
        return evaluate.Result(evaluate.Call(criterion, part, ""), 1.0, [{"quote": "q"}] if found else [], error)

    r = [res("accurate", "A"), res("accurate", "B", found=True),  # one call on a part: its finding fails it
         res("map", "f", found=True), res("map", "f"), res("map", "f"),  # 1 of 3: passes
         res("actionable", "f", found=True), res("actionable", "f"), res("actionable", "f", found=True),  # 2 of 3
         res("no_history", "f"), res("no_history", "f"), res("no_history", "f", error="timed out after 240s"),
         res("complete", "f")]
    assert evaluate.verdicts(r) == {"accurate": "fail", "map": "pass", "actionable": "fail", "no_history": "error",
                                    "complete": "pass"}


def test_a_call_drops_other_anthropic_credentials_only_under_federation():
    env = {"ANTHROPIC_API_KEY": "k", "CLAUDE_CODE_OAUTH_TOKEN": "o", "PATH": "/bin"}
    assert evaluate.child_env(env) == env
    ci = {**env, "ACTIONS_ID_TOKEN_REQUEST_URL": "u", "ANTHROPIC_FEDERATION_RULE_ID": "fdrl"}
    assert evaluate.child_env(ci) == {"PATH": "/bin", "ACTIONS_ID_TOKEN_REQUEST_URL": "u",
                                      "ANTHROPIC_FEDERATION_RULE_ID": "fdrl"}


def test_each_call_writes_a_fresh_oidc_token_for_the_anthropic_api(tmp_path):
    asked = []

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            asked.append((self.path, self.headers["Authorization"]))
            body = json.dumps({"value": f"jwt-{len(asked)}"}).encode()
            self.send_response(200)
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *_):
            pass

    server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        env = {"ACTIONS_ID_TOKEN_REQUEST_URL": f"http://127.0.0.1:{server.server_port}/token?x=1",
               "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "req", "ANTHROPIC_IDENTITY_TOKEN_FILE": str(tmp_path / "oidc")}
        evaluate.refresh_oidc(env)
        evaluate.refresh_oidc(env)
    finally:
        server.shutdown()
    assert asked == [("/token?x=1&audience=https://api.anthropic.com", "bearer req")] * 2
    assert (tmp_path / "oidc").read_text() == "jwt-2"
    evaluate.refresh_oidc({})  # outside Actions: nothing to fetch
    with pytest.raises(RuntimeError, match="ANTHROPIC_IDENTITY_TOKEN_FILE"):
        evaluate.refresh_oidc({"ACTIONS_ID_TOKEN_REQUEST_URL": "u", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "t"})


def test_federation_without_an_oidc_token_fails_before_any_call(fake_claude, tmp_path):
    with pytest.raises(SystemExit, match="no GitHub OIDC token.*a fork's PR gets none.*push the branch to Yeeef/pm"):
        evaluate.run_calls([call()], "haiku", tmp_path, AGENTS_MD,
                           env={**os.environ, "ANTHROPIC_FEDERATION_RULE_ID": "fdrl"})
    assert not fake_claude.exists()


FAKE = """#!{python}
import json, os, subprocess, sys, time
prompt = sys.stdin.read()
open(os.environ["FAKE_LOG"], "a").write(json.dumps({{"argv": sys.argv[1:], "cwd": os.getcwd()}}) + "\\n")
if os.environ.get("FAKE_DETACH"):  # a descendant out of the call's process group, holding its stdout
    child = subprocess.Popen(["sleep", "60"], start_new_session=True)
    open(os.environ["FAKE_DETACH"], "w").write(str(child.pid))
    time.sleep(60)
if os.environ.get("FAKE_HANG"):
    child = subprocess.Popen(["sleep", "60"])
    open(os.environ["FAKE_HANG"], "w").write(str(child.pid))
    time.sleep(60)
answers = os.environ.get("FAKE_ANSWERS")  # a JSON list: the n-th call (by the log's lines) gets the n-th answer
print(json.loads(answers)[len(open(os.environ["FAKE_LOG"]).readlines()) - 1] if answers else os.environ["FAKE_ANSWER"])
"""


@pytest.fixture
def fake_claude(tmp_path, monkeypatch):
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    (bin_dir / "claude").write_text(FAKE.format(python=sys.executable))
    (bin_dir / "claude").chmod(0o755)
    monkeypatch.setenv("PATH", f"{bin_dir}{os.pathsep}{os.environ['PATH']}")
    monkeypatch.setenv("FAKE_LOG", str(tmp_path / "log"))
    monkeypatch.setenv("FAKE_ANSWER", answer([]))
    return tmp_path / "log"


def test_a_call_runs_claude_read_only_without_settings_from_the_checkout(fake_claude, tmp_path):
    r = evaluate.run_call(call(), "haiku", tmp_path, AGENTS_MD)
    assert (r.findings, r.error, r.cost_usd) == ([], "", 0.01)
    logged = json.loads(fake_claude.read_text())
    argv = logged["argv"]
    assert logged["cwd"] == str(tmp_path)
    for flag, value in [("--model", "haiku"), ("--setting-sources", ""), ("--tools", "Read,Grep,Glob"),
                        ("--allowedTools", "Read,Grep,Glob"), ("--disallowedTools", "Bash,Edit,Write"),
                        ("--output-format", "json")]:
        assert argv[argv.index(flag) + 1] == value, flag
    assert "--bare" not in argv and "--strict-mcp-config" in argv
    assert json.loads(argv[argv.index("--json-schema") + 1]) == evaluate.SCHEMA


def test_a_call_past_its_timeout_fails_and_its_process_group_is_killed(fake_claude, tmp_path, monkeypatch):
    monkeypatch.setenv("FAKE_HANG", str(tmp_path / "child"))
    r = evaluate.run_call(call(), "haiku", tmp_path, AGENTS_MD, timeout=2)
    assert r.error == "timed out after 2s" and r.seconds < 10
    pid = (tmp_path / "child").read_text()
    for _ in range(200):  # the fake's own child, killed with its group: gone, or a zombie not yet reaped
        ps = subprocess.run(["ps", "-o", "stat=", "-p", pid], capture_output=True, text=True, check=False)
        if (state := ps.stdout.strip()) == "" or state.startswith("Z"):
            break
        threading.Event().wait(0.05)
    else:
        pytest.fail(f"the fake's child {pid} outlived the timeout: {state}")


def test_a_timed_out_call_does_not_wait_on_a_descendant_that_holds_its_pipes(fake_claude, tmp_path, monkeypatch):
    monkeypatch.setenv("FAKE_DETACH", str(tmp_path / "child"))
    monkeypatch.setattr(evaluate, "REAP", 1)
    try:
        r = evaluate.run_call(call(), "haiku", tmp_path, AGENTS_MD, timeout=2)
        assert r.error == "timed out after 2s" and r.seconds < 10
    finally:
        os.kill(int((tmp_path / "child").read_text()), signal.SIGKILL)


def test_main_exits_1_on_a_finding_and_writes_every_call_to_out(fake_claude, tmp_path, monkeypatch, capsys):
    out = tmp_path / "o" / "eval.json"
    argv = ["--root", str(CHECKOUT), "--criteria", "map,actionable", "--out", str(out)]
    assert evaluate.main(argv) == 0
    assert json.loads(out.read_text())["verdicts"] == {"map": "pass", "actionable": "pass"}
    line = next(x for x in AGENTS_MD.splitlines() if x.strip())
    monkeypatch.setenv("FAKE_ANSWER", answer([{"quote": line, "problem": "vague", "evidence": "Makefile:1"}]))
    assert evaluate.main(argv) == 1
    got = json.loads(out.read_text())
    assert got["verdicts"] == {"map": "fail", "actionable": "fail"} and len(got["calls"]) == 6
    assert "map            fail" in capsys.readouterr().out
    with pytest.raises(SystemExit):
        evaluate.main(["--criteria", "bogus"])
    with pytest.raises(SystemExit, match="needs --base"):
        evaluate.main(["--mode", "diff"])


def test_a_judgement_criterion_fails_on_two_of_its_three_calls_and_reports_their_findings(fake_claude, monkeypatch,
                                                                                          capsys):
    line = next(x for x in AGENTS_MD.splitlines() if x.strip())
    found = answer([{"quote": line, "problem": "too deep", "evidence": "x.go:1"}])
    argv = ["--root", str(CHECKOUT), "--criteria", "map", "--jobs", "1"]  # one at a time: the n-th call, the n-th answer
    monkeypatch.setenv("FAKE_ANSWERS", json.dumps([found, answer([]), answer([])]))
    assert evaluate.main(argv) == 0
    assert "map            pass    3 call(s), 1 with findings" in capsys.readouterr().out
    fake_claude.unlink()
    monkeypatch.setenv("FAKE_ANSWERS", json.dumps([found, answer([]), found]))
    assert evaluate.main(argv) == 1
    out = capsys.readouterr().out
    assert "map            fail    3 call(s), 2 with findings" in out and out.count("too deep") == 2
    fake_claude.unlink()
    monkeypatch.setenv("FAKE_ANSWERS", json.dumps([answer([]), answer([]), "oops"]))  # a malformed answer is no vote
    assert evaluate.main(argv) == 1 and "error: not JSON" in capsys.readouterr().out


@pytest.mark.parametrize("name", cases.CASES)
def test_each_case_edit_applies_to_the_checkout_and_changes_it(name):
    case = cases.CASES[name]
    edited = cases.edited(case, CHECKOUT)
    assert set(edited) == set(case.edits)
    if case.criterion and case.mode == "full" and case.criterion != "complete":
        assert evaluate.normal(case.marker) in evaluate.normal(edited["AGENTS.md"])
        assert evaluate.normal(case.marker) not in evaluate.normal(AGENTS_MD)
    if case.criterion == "complete":
        assert evaluate.normal(case.marker) not in evaluate.normal(edited["AGENTS.md"])


def test_a_case_is_a_repo_whose_diff_is_its_edits(tmp_path):
    base = cases.materialize(cases.CASES["diff_renamed_target"], CHECKOUT, tmp_path)
    diff, names = evaluate.pr_diff(tmp_path, base)
    assert names == ["Makefile"] and "+merge-check:" in diff
    assert cases.planted(tmp_path, base) == ""
    base = cases.materialize(cases.CASES["actionable_dont"], CHECKOUT, tmp_path / "planted")
    assert cases.planted(tmp_path / "planted", base) == "Don't break the hooks.\n"
    assert (tmp_path / "CLAUDE.md").is_symlink() and (tmp_path / "AGENTS.md").read_text() == AGENTS_MD


def test_a_planted_case_counts_only_a_fail_that_names_its_defect():
    case = cases.CASES["no_history_dated"]

    def res(*fs):
        return [evaluate.Result(evaluate.Call("no_history", "f", ""), 1.0, list(fs))]

    assert cases.outcome(case, res()) == "pass"
    assert cases.outcome(case, res({"quote": "Claude 189/189, median", "problem": "", "evidence": ""})) == "fail"
    assert cases.outcome(case, res({"quote": "other", "problem": "x", "evidence": ""})).startswith("fail elsewhere")
    assert cases.outcome(case, res({"quote": "The judge's live eval", "problem": "", "evidence": ""}),
                         added="The judge's live eval on 2026-10-12") == "fail"
    clean = cases.CASES["clean"]
    assert cases.outcome(clean, res()) == "pass"
    assert cases.outcome(clean, res({"quote": "q", "problem": "", "evidence": ""})) == "fail: no_history"
