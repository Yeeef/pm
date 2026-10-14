"""The AGENTS.md evaluation's own logic, offline: the rubric and the calls each mode makes, the answers it takes and
refuses, the lines a quote is on, the vote on lines, the credentials a call runs with, a call's timeout, the
labelled cases' edits, and the paths that run the cases job. A fake `claude` stands in for the model;
tests/test_agents_md_live.py runs the real one."""

from __future__ import annotations

import http.server
import io
import json
import os
import re
import signal
import subprocess
import sys
import threading
from pathlib import Path

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


def test_full_mode_makes_five_calls_per_criterion_and_section():
    text = "# T\n\nintro\n\n## A\n\na `x`\n\n## B\n\nb\n"
    assert evaluate.sections(text) == [("A", "# T\n\nintro\n\n## A\n\na `x`\n\n"), ("B", "## B\n\nb\n")]
    calls = evaluate.full_calls(text, CHECKOUT, evaluate.CRITERIA)
    assert [(c.criterion, c.part) for c in calls] == [*[("accurate", "A")] * 5, *[("accurate", "B")] * 5, *(
        (c, "the whole file") for c in evaluate.CRITERIA[1:] for _ in range(5))]
    by = {c.criterion: c.prompt for c in calls}
    assert "<prime_md>" in by["no_copies"] and "<changelog_preamble>" in by["no_copies"]
    assert "<prime_md>" in by["consistent"] and "<prime_md>" not in by["map"]
    assert all("<agents_md>\n" + text.rstrip() + "\n</agents_md>" in c.prompt for c in calls)


def test_diff_mode_checks_accurate_and_complete_five_times_on_each_chunk_cut_at_file_boundaries():
    files = [f"diff --git a/f{i} b/f{i}\n" + "+x\n" * 40 for i in range(5)]
    cut = evaluate.chunks("".join(files), size=len(files[0]) * 2)
    assert cut == ["".join(files[0:2]), "".join(files[2:4]), files[4]]
    assert evaluate.chunks(files[0], size=10) == [files[0]]  # a file over the size, no hunk to cut at: whole
    calls = evaluate.diff_calls(AGENTS_MD, "".join(files))
    assert [(c.criterion, c.part) for c in calls] == [("accurate", "diff 1/1")] * 5 + [("complete", "diff 1/1")] * 5
    assert "<diff>\n" in calls[0].prompt and "make a claim in AGENTS.md false" in calls[0].prompt


def test_a_file_over_the_chunk_size_is_cut_at_its_hunks_each_under_its_header():
    header = "diff --git a/big b/big\nindex 1..2 100644\n--- a/big\n+++ b/big\n"
    hunk = [f"@@ -{i},2 +{i},2 @@\n" + "+y\n" * 30 for i in range(4)]
    small = "diff --git a/s b/s\n+z\n"
    size = len(header) + 2 * len(hunk[0])
    cut = evaluate.chunks(header + "".join(hunk) + small, size=size)
    assert cut == [header + hunk[0] + hunk[1], header + hunk[2] + hunk[3], small]
    assert evaluate.chunks(header + hunk[0], size=10) == [header + hunk[0]]  # one hunk over the size: whole


def test_auto_mode_is_full_when_the_diff_changes_agents_md_or_the_grader_and_diff_otherwise(tmp_path):
    case = cases.CASES["diff_neutral"]
    base = cases.materialize(case, CHECKOUT, tmp_path)
    mode, _, calls = evaluate.plan("auto", tmp_path, tmp_path / "AGENTS.md", base, evaluate.CRITERIA)
    assert mode == "diff" and len(calls) == 10 and "go_cache_trim.py" in calls[0].prompt
    (tmp_path / "AGENTS.md").write_text(AGENTS_MD + "\nmore\n")
    cases.git(tmp_path, "commit", "-qam", "agents")
    mode, _, calls = evaluate.plan("auto", tmp_path, tmp_path / "AGENTS.md", base, evaluate.CRITERIA)
    assert mode == "full" and {c.criterion for c in calls} == set(evaluate.CRITERIA)
    mode, _, calls = evaluate.plan("diff", tmp_path, tmp_path / "AGENTS.md", "HEAD", evaluate.CRITERIA)
    assert (mode, calls) == ("diff", [])  # nothing changed: no call
    for path in ("tests/agents_md/rubric.md", "tests/test_agents_md_live.py", ".github/workflows/agents-md.yml"):
        root = tmp_path / path.replace("/", "_")
        base = cases.materialize(cases.CASES["clean"], CHECKOUT, root)
        (root / path).write_text((root / path).read_text() + "\n# changed\n")
        cases.git(root, "commit", "-qam", "grader")
        mode, _, calls = evaluate.plan("auto", root, root / "AGENTS.md", base, evaluate.CRITERIA)
        assert mode == "full" and {c.criterion for c in calls} == set(evaluate.CRITERIA), path


def test_the_cases_job_runs_on_a_change_to_the_grader_alone():
    """agents-md.yml's cases job decides with a grep of its own: it must be evaluate.GRADER's paths, each directory
    with what is under it and each file exactly, as evaluate.grader matches them."""
    yml = (CHECKOUT / ".github/workflows/agents-md.yml").read_text()
    grep = re.search(r"grep -qE '([^']+)'", yml)
    assert grep, "the cases job's grep is gone"
    want = "|".join(p.replace(".", r"\.") + ("" if p.endswith("/") else "$") for p in evaluate.GRADER)
    assert grep[1] == f"^({want})"
    graded = [p + "x.py" if p.endswith("/") else p for p in evaluate.GRADER]
    for path in [*graded, "tests/conftest.py", "tests/run.py", "Makefile", "AGENTS.md", "tests/test_agents_md_eval.py",
                 "tests/agents_md_x.py", ".github/workflows/agents-md.yml.bak", "tests/test_agents_md_live.pyc"]:
        assert bool(re.search(grep[1], path)) == evaluate.grader(path) == (path in graded), path


def test_an_answer_is_taken_only_when_well_formed():
    text = "Run `make test` while working.\n"
    ok = {"quote": "Run make test while working", "problem": "p", "evidence": "Makefile:13"}
    assert evaluate.parse(call(), answer([]), text) == ([], "", 0.01)
    assert evaluate.parse(call(), answer([ok]), text) == ([{**ok, "lines": [1]}], "", 0.01)
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
    for dots in ("…", "....", ". . .", "`…` ..."):  # no text: "...." would quote every line with a full stop
        assert "without its quote" in evaluate.parse(call(), answer([{**ok, "quote": dots}]), text)[1], dots
    # a missing rule has no text to quote
    assert evaluate.parse(call("complete"), answer([{**ok, "quote": ""}]), text)[1] == ""


def test_a_quote_maps_to_the_lines_it_is_on():
    text = "# T\n\nRun `make test`\nwhile working.\n\n| a | **b** |\n\nRun make test again.\n"
    assert evaluate.normal_lines(text)[0] == evaluate.normal(text)
    assert evaluate.normal_lines(AGENTS_MD)[0] == evaluate.normal(AGENTS_MD)
    assert evaluate.quote_lines("make test while working", text) == [3, 4]  # across a line break
    assert evaluate.quote_lines("a b", text) == [6]  # without the table's and bold's marks
    assert evaluate.quote_lines("Run make test", text) == [3, 8]  # every place it occurs
    assert evaluate.quote_lines("# T … again", text) == [1, 8]  # each part between ellipses
    assert evaluate.quote_lines("make lint", text) is None


def res(criterion, part, *lines, error=""):
    """A call's result with one finding on each of `lines` (a list of line numbers each)."""
    return evaluate.Result(evaluate.Call(criterion, part, ""), 1.0,
                           [{"quote": f"q{ls}", "problem": "p", "evidence": "e", "lines": ls} for ls in lines], error)


def test_a_part_fails_only_on_a_line_a_majority_of_its_calls_agree_on_and_errors_on_any_bad_call():
    def five(criterion, *found, error=""):
        """A part's five calls: one with each of `found`'s lines, the rest with no finding; the last errs if asked."""
        rs = [res(criterion, "f", *ls) for ls in found] + [res(criterion, "f") for _ in range(5 - len(found))]
        rs[-1].error = error
        return rs

    assert evaluate.VOTES // 2 + 1 == 3  # every part has VOTES calls, so a majority is 3
    r = [*[res("accurate", "A") for _ in range(evaluate.VOTES)], *five("accurate", [[3]], [[3]], [[3]]),  # one part
         *five("map", [[4, 5]], [[5]]),  # 2 of 5 on line 5: passes
         *five("actionable", [[4, 5]], [[5, 6]], [[5]]),  # 3 of 5 on line 5
         *five("non_inferable", [[4, 5]], [[5, 6]], [[3, 4]]),  # a chain: each line has 2 of 5
         *five("no_copies", [[4, 5]], [[9]], [[12]]),  # 3 of 5, disjoint
         *five("no_history", error="timed out after 240s"),
         *five("complete")]
    assert evaluate.verdicts(r) == {"accurate": "fail", "map": "pass", "actionable": "fail", "non_inferable": "pass",
                                    "no_copies": "pass", "no_history": "error", "complete": "pass"}
    # one call's two findings on one passage are one vote
    assert evaluate.verdicts(five("map", [[4], [4]], [[4]])) == {"map": "pass"}
    # complete's findings quote nothing, so have no lines: any agree, and a passage-quoting one agrees with neither
    assert evaluate.verdicts(five("complete", [[]], [[]], [[]])) == {"complete": "fail"}
    assert evaluate.verdicts(five("complete", [[]], [[]])) == {"complete": "pass"}
    assert evaluate.verdicts(five("map", [[]], [[]], [[4]])) == {"map": "pass"}


def test_a_broad_quote_is_one_vote_on_each_of_its_lines_and_carries_no_other_finding():
    """A call that quotes a phrase found on many lines ("make test") votes once on each: two other calls, each on a
    line of its own among them, make 2 votes on each line, not a majority; three calls on one line do."""
    broad = [10, 22, 41, 80]
    rs = [res("map", "f", broad), res("map", "f", [10]), res("map", "f", [80]), res("map", "f"), res("map", "f")]
    assert evaluate.agreed(rs) == [] and evaluate.verdicts(rs) == {"map": "pass"}
    rs[3] = res("map", "f", [10])
    assert evaluate.agreed(rs) == [rs[0].findings[0], rs[1].findings[0], rs[3].findings[0]]  # line 10: 3 of 5
    assert evaluate.verdicts(rs) == {"map": "fail"}
    assert len(evaluate.quote_lines("make test", AGENTS_MD)) > 1  # such quotes are real


def test_advice_says_fix_on_any_agreed_finding_and_rerun_on_any_error_both_when_both():
    fix, rerun = "A rerun will not change agreed findings.", "A rerun may help."
    agreed = [res("map", "A", [4]) for _ in range(3)] + [res("map", "A"), res("map", "A")]
    errored = [res("map", "B") for _ in range(4)] + [res("map", "B", error="timed out after 420s")]
    assert evaluate.verdicts(agreed + errored) == {"map": "error"}  # the verdict keeps error first
    one = agreed[:4] + [res("map", "A", error="not JSON")]  # in one part: the four answers still agree 3 of 5
    for rs in (agreed + errored, one):
        both = evaluate.advice(rs)
        assert len(both) == 2 and fix in both[0] and rerun in both[1]
    assert [rerun in a for a in evaluate.advice(errored)] == [True]
    assert [fix in a for a in evaluate.advice(agreed)] == [True]


def test_five_calls_where_only_three_agree_fail_on_those_three_findings_and_print_the_others_outvoted():
    rs = [res("map", "f", [10, 11]), res("map", "f", [11, 12]), res("map", "f", [30]), res("map", "f", [11]),
          res("map", "f")]
    assert evaluate.agreed(rs) == [rs[0].findings[0], rs[1].findings[0], rs[3].findings[0]]
    assert evaluate.verdicts(rs) == {"map": "fail"}
    table = evaluate.report(rs)
    assert "map            fail    5 call(s), 4 with findings, 3 agreed" in table
    assert "[f] 'q[10, 11]' (lines 10-11)" in table and "[f] outvoted: 'q[30]' (line 30)" in table


def test_a_call_drops_other_anthropic_credentials_only_under_federation():
    env = {"ANTHROPIC_API_KEY": "k", "CLAUDE_CODE_OAUTH_TOKEN": "o", "PATH": "/bin"}
    assert evaluate.child_env(env) == env
    ci = {**env, "ACTIONS_ID_TOKEN_REQUEST_URL": "u", "ANTHROPIC_FEDERATION_RULE_ID": "fdrl"}
    assert evaluate.child_env(ci) == {"PATH": "/bin", "ACTIONS_ID_TOKEN_REQUEST_URL": "u",
                                      "ANTHROPIC_FEDERATION_RULE_ID": "fdrl"}


def test_each_call_writes_a_fresh_oidc_token_for_the_anthropic_api_to_a_file_of_its_own(tmp_path):
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
        own = [evaluate.refresh_oidc(env), evaluate.refresh_oidc(env)]
    finally:
        server.shutdown()
    assert asked == [("/token?x=1&audience=https://api.anthropic.com", "bearer req")] * 2
    # each call's token in a file of its own, which no other call reads or replaces
    assert [Path(p).read_text() for p in own] == ["jwt-1", "jwt-2"]
    assert all(Path(p).parent == tmp_path and Path(p).name.startswith("oidc.") for p in own)
    assert not (tmp_path / "oidc").exists()
    evaluate.refresh_oidc({})  # outside Actions: nothing to fetch
    with pytest.raises(RuntimeError, match="ANTHROPIC_IDENTITY_TOKEN_FILE"):
        evaluate.refresh_oidc({"ACTIONS_ID_TOKEN_REQUEST_URL": "u", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "t"})


def test_calls_at_once_under_federation_each_read_their_own_token_and_leave_no_file(fake_claude, tmp_path,
                                                                                    monkeypatch):
    served = iter(range(1, 100))
    monkeypatch.setattr(evaluate.urllib.request, "urlopen", lambda req, timeout: io.StringIO(
        json.dumps({"value": f"jwt-{next(served)}"})))
    tokens = tmp_path / "tokens"
    tokens.mkdir()
    env = {**os.environ, "ACTIONS_ID_TOKEN_REQUEST_URL": "http://oidc", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "req",
           "ANTHROPIC_IDENTITY_TOKEN_FILE": str(tokens / "oidc"), "ANTHROPIC_FEDERATION_RULE_ID": "fdrl"}
    rs = evaluate.run_calls([call()] * 4, "haiku", tmp_path, AGENTS_MD, jobs=4, env=env)
    assert [r.error for r in rs] == [""] * 4
    read = [json.loads(line)["token"] for line in fake_claude.read_text().splitlines()]
    assert sorted(read) == ["jwt-1", "jwt-2", "jwt-3", "jwt-4"]
    assert list(tokens.iterdir()) == []


def test_federation_without_an_oidc_token_fails_before_any_call(fake_claude, tmp_path):
    with pytest.raises(SystemExit, match="no GitHub OIDC token.*a fork's PR gets none.*push the branch to Yeeef/pm"):
        evaluate.run_calls([call()], "haiku", tmp_path, AGENTS_MD,
                           env={**os.environ, "ANTHROPIC_FEDERATION_RULE_ID": "fdrl"})
    assert not fake_claude.exists()


FAKE = """#!{python}
import json, os, subprocess, sys, time
prompt = sys.stdin.read()
token = os.environ.get("ANTHROPIC_IDENTITY_TOKEN_FILE")
token = open(token).read() if token else None
open(os.environ["FAKE_LOG"], "a").write(json.dumps({{"argv": sys.argv[1:], "cwd": os.getcwd(), "token": token}}) + "\\n")
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
    assert got["verdicts"] == {"map": "fail", "actionable": "fail"} and len(got["calls"]) == 10
    printed = capsys.readouterr().out
    assert "map            fail" in printed and "map, actionable failed on findings a majority of calls agreed on" in printed
    assert "A rerun will not change agreed findings." in printed and "A rerun may help" not in printed
    # a malformed finding: the error is printed, not the finding, and the table, advice and --out are all written
    monkeypatch.setenv("FAKE_ANSWER", answer([{"quote": "q"}, "x"]))
    assert evaluate.main(argv) == 1
    got = json.loads(out.read_text())
    assert got["verdicts"] == {"map": "error", "actionable": "error"} and got["calls"][0]["findings"][0] == {"quote": "q"}
    printed = capsys.readouterr().out
    assert "map            error" in printed and "error: malformed finding" in printed and "A rerun may help" in printed
    with pytest.raises(SystemExit):
        evaluate.main(["--criteria", "bogus"])
    with pytest.raises(SystemExit, match="needs --base"):
        evaluate.main(["--mode", "diff"])


def test_a_judgement_criterion_fails_when_three_of_its_five_calls_quote_overlapping_lines(fake_claude, monkeypatch,
                                                                                          capsys):
    lines = [x for x in AGENTS_MD.splitlines() if len(x) > 40]
    head, here, there = lines[0], lines[0][5:35], lines[-1]  # `here` lies within `head`'s line, `there` far from it

    def found(quote):
        return answer([{"quote": quote, "problem": "too deep", "evidence": "x.go:1"}])

    argv = ["--root", str(CHECKOUT), "--criteria", "map", "--jobs", "1"]  # one at a time: the n-th call, the n-th answer

    def run(*answers):
        if fake_claude.exists():
            fake_claude.unlink()
        monkeypatch.setenv("FAKE_ANSWERS", json.dumps(list(answers)))
        return evaluate.main(argv), capsys.readouterr().out

    empty = answer([])
    code, out = run(found(head), empty, empty, empty, empty)
    assert code == 0 and "map            pass    5 call(s), 1 with findings, 0 agreed" in out and "outvoted:" in out
    code, out = run(found(head), empty, found(here), empty, empty)  # 2 of 5 on overlapping lines
    assert code == 0 and "map            pass    5 call(s), 2 with findings, 0 agreed" in out
    assert out.count("outvoted:") == 2
    code, out = run(found(head), empty, found(here), empty, found(head))  # 3 of 5 on overlapping lines
    assert code == 1 and "map            fail    5 call(s), 3 with findings, 3 agreed" in out
    assert out.count("too deep") == 3 and "outvoted:" not in out
    code, out = run(found(head), found(there), found(there), empty, found(here))  # 4 of 5, 2 and 2 overlap
    assert code == 0 and "map            pass    5 call(s), 4 with findings, 0 agreed" in out
    assert out.count("outvoted:") == 4
    code, out = run(found(there), found(head), found(here), found(head), empty)  # 4 of 5, only 3 overlap
    assert code == 1 and "map            fail    5 call(s), 4 with findings, 3 agreed" in out
    assert out.count("outvoted:") == 1 and f"outvoted: {there!r}" in out
    code, out = run(empty, empty, empty, empty, "oops")  # a malformed answer is no vote
    assert code == 1 and "error: not JSON" in out
    assert "agents-md: map: a call gave no answer" in out and "A rerun may help." in out
    assert "A rerun will not change" not in out


def test_diff_mode_fails_only_on_a_passage_a_majority_of_a_chunks_calls_agree_on(fake_claude, tmp_path, monkeypatch,
                                                                                capsys):
    """Run 38567955117 failed on one call's finding in one of five chunks: each diff-mode part now votes."""
    root = tmp_path / "case"
    base = cases.materialize(cases.CASES["diff_neutral"], CHECKOUT, root)
    line = next(x for x in AGENTS_MD.splitlines() if len(x) > 40)
    found = answer([{"quote": line, "problem": "stale", "evidence": "Makefile:1"}])
    missing = answer([{"quote": "", "problem": "a new rule", "evidence": "x.py:1"}])
    argv = ["--root", str(root), "--mode", "diff", "--base", base, "--jobs", "1"]  # the n-th call, the n-th answer

    def run(*answers):  # the chunk's five accurate calls, then its five complete calls
        if fake_claude.exists():
            fake_claude.unlink()
        monkeypatch.setenv("FAKE_ANSWERS", json.dumps(list(answers)))
        return evaluate.main(argv), capsys.readouterr().out

    empty = answer([])
    none = [empty] * 5
    code, out = run(found, empty, found, empty, empty, missing, empty, missing, empty, empty)  # 2 of 5 each
    assert code == 0 and "accurate       pass    5 call(s), 2 with findings, 0 agreed" in out
    assert "complete       pass    5 call(s), 2 with findings, 0 agreed" in out and out.count("outvoted:") == 4
    code, out = run(found, empty, found, empty, found, *none)
    assert code == 1 and "accurate       fail    5 call(s), 3 with findings, 3 agreed" in out
    code, out = run(*none, missing, missing, empty, missing, empty)
    assert code == 1 and "complete       fail    5 call(s), 3 with findings, 3 agreed" in out


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


def test_a_planted_case_counts_only_an_agreed_fail_that_names_its_defect():
    case = cases.CASES["no_history_dated"]

    def run(*fs, outvoted=()):
        """Five calls: three each with the findings `fs` (on line 7), two with the findings `outvoted` (on line 40)."""
        def r(found, line):
            return evaluate.Result(evaluate.Call("no_history", "f", ""), 1.0,
                                   [{"problem": "", "evidence": "", "lines": [line], **f} for f in found])
        return [r(fs, 7), r(fs, 7), r(fs, 7), r(outvoted, 40), r(outvoted, 40)]

    assert cases.outcome(case, run()) == "pass"
    assert cases.outcome(case, run({"quote": "Claude 189/189, median"})) == "fail"
    assert cases.outcome(case, run({"quote": "other", "problem": "x"})) == "fail elsewhere: other; other; other"
    assert cases.outcome(case, run({"quote": "other"}, outvoted=[{"quote": "Claude 189/189"}])) == \
        "fail elsewhere: other; other; other"  # the outvoted finding that names the defect does not count
    assert cases.outcome(case, run({"quote": "The judge's live eval"}),
                         added="The judge's live eval on 2026-10-12") == "fail"
    clean = cases.CASES["clean"]
    assert cases.outcome(clean, run()) == "pass"
    assert cases.outcome(clean, run({"quote": "q"})) == "fail: no_history"
    assert cases.outcome(clean, run(outvoted=[{"quote": "q"}])) == "pass"
