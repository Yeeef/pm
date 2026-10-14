#!/usr/bin/env python3
"""Grade AGENTS.md against the rubric (rubric.md beside this file) with an agent that reads the checkout.

    evaluate.py --mode full                        every criterion on the whole file
    evaluate.py --mode diff --base origin/main     accurate and complete, against the diff since the merge base
    evaluate.py --mode auto --base origin/main     full when that diff changes AGENTS.md or the grader, else diff

Each check is one `claude -p` call from the checkout root, with the read-only tools alone (Read, Grep, Glob), no
settings, hooks, CLAUDE.md or MCP servers, and a JSON schema for its answer. Each part is graded by VOTES (3) calls:
full mode's parts are each criterion on the whole file, accurate's on each `## ` section of AGENTS.md; diff mode's are
accurate and complete on each chunk of the diff. The calls run at once (--jobs), each killed with its process group at
--timeout seconds, which fails the run.

Each finding quotes AGENTS.md, and a quote that is not in it makes the answer malformed; a quote's lines are the
lines of AGENTS.md its text (each part between ellipses, at each place it occurs) is on. The calls on one part (a
criterion on a section, a diff chunk or the whole file) vote on passages: a finding is agreed when a majority of the
part's calls (2 of 3) each report a finding whose lines intersect its lines, its own call counted, and the part fails
on an agreed finding. A complete finding names a missing rule and quotes nothing, so it has no lines: two such
findings agree, and complete fails a part when a majority of its calls each report a missing rule. A criterion fails
when one of its parts fails. A malformed answer, a failed or a timed-out call is no vote: it fails the run. The run
exits 1 on a failed criterion or any such call. It prints a table with every call's findings, a finding no majority
agrees with marked outvoted, then a line on what to do: fix AGENTS.md as agreed findings say, since a rerun gives them
again; rerun after an error. It writes every call's answer (each finding with its lines), seconds and cost to --out.

In GitHub Actions (ACTIONS_ID_TOKEN_REQUEST_URL set) each call first writes a fresh GitHub OIDC token for the Anthropic
API to a file of its own beside $ANTHROPIC_IDENTITY_TOKEN_FILE, for Claude Code's workload identity federation, and the
call runs with that file and without any other Anthropic credential in its environment; with ANTHROPIC_FEDERATION_RULE_ID set and no such token (a fork's
PR) the run fails before any call. Elsewhere it runs with the user's own Claude Code login.
"""

from __future__ import annotations

import argparse
import contextlib
import functools
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from pathlib import Path

HERE = Path(__file__).resolve().parent
RUBRIC = HERE / "rubric.md"
CRITERIA = ("accurate", "non_inferable", "map", "no_copies", "actionable", "no_history", "consistent", "complete")
VOTES = 3  # calls on each part; a finding stands when a majority of them agree on its passage
DIFF_CRITERIA = ("accurate", "complete")
MODEL = "haiku"
TIMEOUT = 240  # seconds per call; a call past it fails the run
REAP = 10  # seconds to drain a killed call's pipes
JOBS = 16
CHUNK = 60_000  # characters of diff per diff-mode call
# Diff paths no AGENTS.md claim rests on: generated expectations and lock files.
DIFF_EXCLUDE = (":(exclude)**/testdata/**", ":(exclude)uv.lock", ":(exclude)go.sum",
                ":(exclude).github/scripts/work-test-seconds.txt")
# The grader: a diff that changes it grades in full mode, since a changed grader must still pass the real AGENTS.md;
# agents-md.yml's cases job runs on the same paths.
GRADER = ("tests/agents_md/", "tests/test_agents_md_live.py", ".github/workflows/agents-md.yml")
TOOLS = "Read,Grep,Glob"
CREDENTIALS = ("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN")
AUDIENCE = "https://api.anthropic.com"

SCHEMA = {
    "type": "object",
    "properties": {"findings": {"type": "array", "items": {
        "type": "object",
        "properties": {"quote": {"type": "string"}, "problem": {"type": "string"}, "evidence": {"type": "string"}},
        "required": ["quote", "problem", "evidence"], "additionalProperties": False}}},
    "required": ["findings"], "additionalProperties": False}

ANSWER = """Answer with the JSON the schema asks for: `findings`, the defects you found on this criterion, each with
`quote` (the AGENTS.md text, verbatim), `problem` and `evidence`. An empty list means the criterion passes. Read only;
change nothing."""


@dataclass
class Call:
    """One `claude -p` check: a criterion on one part of AGENTS.md (a section, a diff chunk, or the whole file)."""
    criterion: str
    part: str
    prompt: str


@dataclass
class Result:
    call: Call
    seconds: float
    findings: list[dict] = field(default_factory=list)
    error: str = ""  # the call did not give a well-formed answer: the run fails
    cost_usd: float = 0.0

    def as_json(self) -> dict:
        return {"criterion": self.call.criterion, "part": self.call.part, "seconds": round(self.seconds, 2),
                "cost_usd": self.cost_usd, "findings": self.findings, "error": self.error}


def rubric(text: str) -> tuple[str, dict[str, str]]:
    """The rubric's preamble and its criteria's sections, by criterion id, from rubric.md's `## <id>` headings."""
    parts = re.split(r"^## (\w+)\n", text, flags=re.M)
    preamble, sections = parts[0], dict(zip(parts[1::2], parts[2::2], strict=True))
    if tuple(sections) != CRITERIA:
        raise SystemExit(f"evaluate.py: rubric.md's criteria are {tuple(sections)}, want {CRITERIA}")
    return preamble.strip(), {k: v.strip() for k, v in sections.items()}


def sections(agents_md: str) -> list[tuple[str, str]]:
    """AGENTS.md's `## ` sections as (heading, text); the text before the first heading joins the first section."""
    parts = re.split(r"^(## .+)$", agents_md, flags=re.M)
    if len(parts) < 3:
        return [("the whole file", agents_md)]
    out = [(parts[1][3:].strip(), parts[0] + parts[1] + parts[2])]
    out += [(h[3:].strip(), h + body) for h, body in zip(parts[3::2], parts[4::2], strict=True)]
    return out


def changelog_preamble(root: Path) -> str:
    text = (root / "CHANGELOG.md").read_text()
    return text.split("\n## [", 1)[0]


def fenced(name: str, text: str) -> str:
    return f'<{name}>\n{text.rstrip()}\n</{name}>'


def full_calls(agents_md: str, root: Path, criteria: tuple[str, ...]) -> list[Call]:
    """Full mode's calls: each criterion on the whole file, accurate on each section; VOTES calls on each."""
    pre, crit = rubric(RUBRIC.read_text())
    head = (f"{pre}\n\nYou grade the AGENTS.md below on one criterion. The checkout is the current directory; the "
            "AGENTS.md under review is the text given here, not the file on disk, which may differ.\n\n"
            + fenced("agents_md", agents_md))
    calls = []
    for c in criteria:
        task = f"## Criterion: {c}\n\n{crit[c]}"
        if c == "accurate":
            for heading, _ in sections(agents_md):
                calls += [Call(c, heading, f"{head}\n\n{task}\n\nCheck only the claims in the section "
                                           f"`{heading}`; the rest of the file is context.\n\n{ANSWER}")] * VOTES
            continue
        extra = ""
        if c in ("no_copies", "consistent"):
            extra = "\n\n" + fenced("prime_md", (root / "prime.md").read_text())
            if c == "no_copies":
                extra += "\n\n" + fenced("changelog_preamble", changelog_preamble(root))
        calls += [Call(c, "the whole file", f"{head}{extra}\n\n{task}\n\n{ANSWER}")] * VOTES
    return calls


def chunks(diff: str, size: int = CHUNK) -> list[str]:
    """The diff cut at its file boundaries into chunks of about `size` characters. A larger file is cut at its `@@`
    hunks, each piece under the file's own header (`diff --git` to `+++`); a single hunk over `size` goes whole."""
    files = re.split(r"(?=^diff --git )", diff, flags=re.M)
    out: list[str] = []
    for f in (f for f in files if f.strip()):
        for piece in hunks(f, size):
            if out and len(out[-1]) + len(piece) <= size:
                out[-1] += piece
            else:
                out.append(piece)
    return out


def hunks(file_diff: str, size: int) -> list[str]:
    """One file's diff as pieces of about `size` characters, cut at its hunks, each under the file's header."""
    if len(file_diff) <= size:
        return [file_diff]
    parts = re.split(r"(?=^@@ )", file_diff, flags=re.M)
    header, body = parts[0], parts[1:]
    out: list[str] = []
    for h in body:
        if out and len(out[-1]) + len(h) <= size:
            out[-1] += h
        else:
            out.append(header + h)
    return out or [file_diff]


DIFF_TASK = {
    "accurate": """Does this diff make a claim in AGENTS.md false? A claim that the diff touches: a Make target, flag,
file, function, test, environment variable or behaviour that the diff renames, removes or changes, while AGENTS.md
still states the old one. Check each such claim against the checkout. A claim the diff does not touch is out of
scope. Quote the AGENTS.md text that the diff makes false.""",
    "complete": """Does this diff add a rule that an agent developing pm must know and would get wrong untold, that
it cannot infer from the code, its comments or a `--help` text, and that AGENTS.md lacks? For example a new step every
PR must take, a new required environment variable for a kind of test, a new file that must be edited together with
another. Code that documents itself, a refactor or a new test does not add such a rule. A finding has an empty quote
and names the rule as its problem.""",
}


def diff_calls(agents_md: str, diff: str) -> list[Call]:
    """Diff mode's calls: accurate and complete on each chunk of the diff, VOTES calls on each."""
    pre, crit = rubric(RUBRIC.read_text())
    head = (f"{pre}\n\nYou review a pull request's effect on AGENTS.md. The checkout (the current directory) is at the "
            "PR's head; the diff below is the PR's change since its merge base.\n\n" + fenced("agents_md", agents_md))
    cut = chunks(diff)
    return [Call(c, f"diff {i}/{len(cut)}",
                 f"{head}\n\n{fenced('diff', part)}\n\n## Criterion: {c}\n\n{crit[c]}\n\n## This review\n\n"
                 f"{DIFF_TASK[c]}\n\n{ANSWER}")
            for i, part in enumerate(cut, 1) for c in DIFF_CRITERIA for _ in range(VOTES)]


def git(root: Path, *args: str) -> str:
    return subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True, check=True).stdout


def pr_diff(root: Path, base: str) -> tuple[str, list[str]]:
    """The diff since the merge base of `base` and HEAD (DIFF_EXCLUDE left out), and every path it changes."""
    mb = git(root, "merge-base", base, "HEAD").strip()
    names = git(root, "diff", "--name-only", f"{mb}...HEAD").split()
    return git(root, "diff", f"{mb}...HEAD", "--", ".", *DIFF_EXCLUDE), names


def refresh_oidc(env: dict[str, str]) -> str | None:
    """In GitHub Actions: write a fresh OIDC token for the Anthropic API to a new file beside
    $ANTHROPIC_IDENTITY_TOKEN_FILE and return its path, for one call alone: with one file shared by the calls running
    at once, several exchange one token and the file changes under a running call, and 14 of 52 calls failed the
    token exchange with 401 (CI, PR #80)."""
    url = env.get("ACTIONS_ID_TOKEN_REQUEST_URL")
    if not url:
        return None
    path = env.get("ANTHROPIC_IDENTITY_TOKEN_FILE")
    if not path or not env.get("ACTIONS_ID_TOKEN_REQUEST_TOKEN"):
        raise RuntimeError("ACTIONS_ID_TOKEN_REQUEST_URL is set but ANTHROPIC_IDENTITY_TOKEN_FILE or "
                           "ACTIONS_ID_TOKEN_REQUEST_TOKEN is not")
    sep = "&" if "?" in url else "?"
    req = urllib.request.Request(f"{url}{sep}audience={AUDIENCE}",
                                 headers={"Authorization": f"bearer {env['ACTIONS_ID_TOKEN_REQUEST_TOKEN']}"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        token = json.load(resp)["value"]
    fd, own = tempfile.mkstemp(dir=Path(path).parent, prefix=Path(path).name + ".")
    with os.fdopen(fd, "w") as f:
        f.write(token)
    return own


def require_oidc(env: dict[str, str]) -> None:
    """Under federation, fail before any call when the run has no GitHub OIDC token to give it."""
    if env.get("ANTHROPIC_FEDERATION_RULE_ID") and not env.get("ACTIONS_ID_TOKEN_REQUEST_URL"):
        raise SystemExit("evaluate.py: ANTHROPIC_FEDERATION_RULE_ID is set but this run has no GitHub OIDC token "
                         "(ACTIONS_ID_TOKEN_REQUEST_URL is unset; a fork's PR gets none): push the branch to "
                         "Yeeef/pm and re-run")


def child_env(env: dict[str, str]) -> dict[str, str]:
    """The call's environment: in CI, with federation set up, no other Anthropic credential."""
    if env.get("ACTIONS_ID_TOKEN_REQUEST_URL") and env.get("ANTHROPIC_FEDERATION_RULE_ID"):
        return {k: v for k, v in env.items() if k not in CREDENTIALS}
    return dict(env)


def command(model: str) -> list[str]:
    return ["claude", "-p", "--model", model, "--setting-sources", "", "--strict-mcp-config",
            "--tools", TOOLS, "--allowedTools", TOOLS, "--disallowedTools", "Bash,Edit,Write",
            "--no-session-persistence", "--output-format", "json", "--json-schema", json.dumps(SCHEMA)]


MARKS = "`*_|\\"


def normal(s: str) -> str:
    return re.sub(r"\s+", " ", re.sub(r"[`*_|\\]", "", s)).strip().lower()


@functools.lru_cache(maxsize=8)
def normal_lines(agents_md: str) -> tuple[str, tuple[int, ...]]:
    """normal(agents_md), and for each of its characters the 1-based line of AGENTS.md it comes from."""
    chars: list[str] = []
    lines: list[int] = []
    for n, line in enumerate(agents_md.splitlines(keepends=True), 1):
        for c in line:
            if c in MARKS:
                continue
            if c.isspace():
                if chars and chars[-1] != " ":
                    chars.append(" ")
                    lines.append(n)
                continue
            for low in c.lower():
                chars.append(low)
                lines.append(n)
    if chars and chars[-1] == " ":
        chars.pop()
        lines.pop()
    return "".join(chars), tuple(lines)


def quote_lines(quote: str, agents_md: str) -> list[int] | None:
    """The lines of AGENTS.md a finding's quote is on: each part between ellipses, without Markdown marks or spacing,
    at every place it occurs; None when a part is not AGENTS.md text."""
    text, line_of = normal_lines(agents_md)
    out: set[int] = set()
    for part in filter(None, (normal(p) for p in re.split(r"\.\.\.|…", quote))):
        at = text.find(part)
        if at < 0:
            return None
        while at >= 0:
            out.update(line_of[at:at + len(part)])
            at = text.find(part, at + 1)
    return sorted(out)


def parse(call: Call, out: str, agents_md: str) -> tuple[list[dict], str, float]:
    """A call's findings, its error ("" when well-formed) and its cost, from `claude -p --output-format json`."""
    try:
        res = json.loads(out)
    except json.JSONDecodeError:
        return [], f"not JSON: {out[:300]!r}", 0.0
    cost = float(res.get("total_cost_usd") or 0.0)
    if res.get("is_error") or res.get("subtype") != "success":
        return [], f"claude failed: {res.get('subtype')}: {str(res.get('result'))[:300]}", cost
    answer = res.get("structured_output")
    if not isinstance(answer, dict) or not isinstance(answer.get("findings"), list):
        return [], f"no structured answer: {str(res.get('result'))[:300]!r}", cost
    findings = answer["findings"]
    for f in findings:
        if not isinstance(f, dict) or not all(isinstance(f.get(k), str) for k in ("quote", "problem", "evidence")):
            return findings, f"malformed finding: {f!r}", cost
        if call.criterion == "complete":
            continue
        if not f["evidence"].strip() or not normal(f["quote"]):
            return findings, f"a finding without its quote or evidence: {f!r}", cost
        if not (lines := quote_lines(f["quote"], agents_md)):  # None, or [] for a quote of ellipses alone
            return findings, f"a finding quotes text that is not in AGENTS.md: {f['quote']!r}", cost
        f["lines"] = lines
    return findings, "", cost


def run_call(call: Call, model: str, root: Path, agents_md: str, timeout: float = TIMEOUT,
             env: dict[str, str] | None = None) -> Result:
    """Run one check, killed with its process group at `timeout` seconds."""
    env = dict(os.environ if env is None else env)
    start = time.monotonic()
    try:
        token = refresh_oidc(env)
    except (OSError, RuntimeError, KeyError, ValueError) as e:
        return Result(call, time.monotonic() - start, error=f"OIDC token: {e}")
    child = child_env(env)
    if token:
        child["ANTHROPIC_IDENTITY_TOKEN_FILE"] = token
    try:
        return claude(call, model, root, agents_md, timeout, child, start)
    finally:
        if token:
            os.unlink(token)


def claude(call: Call, model: str, root: Path, agents_md: str, timeout: float, env: dict[str, str],
           start: float) -> Result:
    """One claude -p run of the call in env, timed from start."""
    proc = subprocess.Popen(command(model), cwd=root, env=env, stdin=subprocess.PIPE,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
    try:
        out, err = proc.communicate(call.prompt, timeout=timeout)
    except subprocess.TimeoutExpired:
        with contextlib.suppress(ProcessLookupError):  # the group may have exited at the timeout itself
            os.killpg(proc.pid, signal.SIGKILL)
        # A descendant that left the group may still hold the pipes: wait for them only briefly.
        with contextlib.suppress(subprocess.TimeoutExpired):
            proc.communicate(timeout=REAP)
        return Result(call, time.monotonic() - start, error=f"timed out after {timeout:.0f}s")
    took = time.monotonic() - start
    if proc.returncode != 0 and not out.strip():
        return Result(call, took, error=f"claude exited {proc.returncode}: {err.strip()[-300:]}")
    findings, error, cost = parse(call, out, agents_md)
    if proc.returncode != 0 and not error:
        error = f"claude exited {proc.returncode}: {err.strip()[-300:]}"
    return Result(call, took, findings, error, cost)


def run_calls(calls: list[Call], model: str, root: Path, agents_md: str, jobs: int = JOBS,
              timeout: float = TIMEOUT, env: dict[str, str] | None = None) -> list[Result]:
    require_oidc(dict(os.environ if env is None else env))
    with ThreadPoolExecutor(max(1, min(jobs, len(calls)))) as pool:
        return list(pool.map(lambda c: run_call(c, model, root, agents_md, timeout, env), calls))


def parts(results: list[Result]) -> dict[tuple[str, str], list[Result]]:
    """The results by part: (criterion, part)."""
    out: dict[tuple[str, str], list[Result]] = {}
    for r in results:
        out.setdefault((r.call.criterion, r.call.part), []).append(r)
    return out


def overlap(f: dict, g: dict) -> bool:
    """Two findings are on one passage when their lines intersect; two without lines (complete's, which name a missing
    rule and quote nothing) agree."""
    a, b = set(f.get("lines", ())), set(g.get("lines", ()))
    return bool(a & b) or not (a or b)


def agreed(rs: list[Result]) -> list[dict]:
    """The findings of one part's calls that stand: each finding that overlaps a finding in each of a majority of the
    calls, its own call counted."""
    need = len(rs) // 2 + 1
    return [f for r in rs for f in r.findings if sum(any(overlap(f, g) for g in o.findings) for o in rs) >= need]


def verdicts(results: list[Result]) -> dict[str, str]:
    """Each criterion's verdict: error when a call of it did not answer, fail when one of its parts has an agreed
    finding, else pass."""
    out: dict[str, str] = {}
    for (crit, _), rs in parts(results).items():
        v = "error" if any(r.error for r in rs) else "fail" if agreed(rs) else "pass"
        out[crit] = max(out.get(crit, "pass"), v, key=("pass", "fail", "error").index)
    return out


def report(results: list[Result]) -> str:
    """The table: a line per criterion, then each finding (outvoted ones marked) and error under it."""
    lines = [f"{'criterion':14} {'verdict':7} evidence"]
    by = parts(results)
    for crit, v in verdicts(results).items():
        rs = [r for r in results if r.call.criterion == crit]
        stand = [id(f) for (c, _), prs in by.items() if c == crit for f in agreed(prs)]
        lines.append(f"{crit:14} {v:7} {len(rs)} call(s), {sum(bool(r.findings) for r in rs)} with findings, "
                     f"{len(stand)} agreed, {max(r.seconds for r in rs):.0f}s")
        for r in rs:
            if r.error:
                lines.append(f"    [{r.call.part}] error: {r.error}")
            lines += [f"    [{r.call.part}]{'' if id(f) in stand else ' outvoted:'} {f['quote']!r}"
                      f"{span(f)}: {f['problem']} ({f['evidence']})" for f in r.findings]
    return "\n".join(lines)


def advice(verdict: dict[str, str]) -> list[str]:
    """What the author does about a failed run: agreed findings stand on a rerun, a call that gave no answer may not."""
    out = []
    if failed := [c for c, v in verdict.items() if v == "fail"]:
        out.append(f"agents-md: {', '.join(failed)} failed on findings a majority of calls agreed on: fix AGENTS.md as "
                   "they say (cut text that restates what the code, a test or a lint already says; correct a false "
                   "claim; add a missing rule). A rerun will not change agreed findings.")
    if errored := [c for c, v in verdict.items() if v == "error"]:
        out.append(f"agents-md: {', '.join(errored)}: a call gave no answer (an error or a timeout, above), so the run "
                   "has no verdict there. A rerun may help.")
    return out


def span(f: dict) -> str:
    """A finding's lines, as the table prints them."""
    ls = f.get("lines")
    if not ls:
        return ""
    return f" (line {ls[0]})" if len(ls) == 1 else f" (lines {ls[0]}-{ls[-1]})"


def plan(mode: str, root: Path, agents_md_path: Path, base: str | None,
         criteria: tuple[str, ...]) -> tuple[str, str, list[Call]]:
    """The mode the run takes, the AGENTS.md text graded, and its calls."""
    agents_md = agents_md_path.read_text()
    diff = ""
    if mode in ("diff", "auto"):
        if not base:
            raise SystemExit(f"evaluate.py: --mode {mode} needs --base")
        diff, names = pr_diff(root, base)
        if mode == "auto":
            mode = "full" if any(n == "AGENTS.md" or n.startswith(GRADER) for n in names) else "diff"
    if mode == "full":
        return mode, agents_md, full_calls(agents_md, root, criteria)
    return mode, agents_md, diff_calls(agents_md, diff) if diff.strip() else []


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__.split("\n\n", 1)[0])
    p.add_argument("--mode", choices=("full", "diff", "auto"), default="full")
    p.add_argument("--base", help="the ref the PR merges into, for diff and auto (origin/main)")
    p.add_argument("--model", default=MODEL)
    p.add_argument("--agents-md", type=Path, help="grade this file instead of the checkout's AGENTS.md")
    p.add_argument("--root", type=Path, help="the checkout (default: the repo this script is in)")
    p.add_argument("--criteria", help=f"full mode's criteria, comma-separated (default: all of {','.join(CRITERIA)})")
    p.add_argument("--jobs", type=int, default=JOBS)
    p.add_argument("--timeout", type=float, default=TIMEOUT)
    p.add_argument("--out", type=Path, help="write every call's answer here, as JSON")
    a = p.parse_args(argv)
    root = (a.root or Path(git(HERE, "rev-parse", "--show-toplevel").strip())).resolve()
    criteria = tuple(a.criteria.split(",")) if a.criteria else CRITERIA
    if unknown := set(criteria) - set(CRITERIA):
        p.error(f"unknown criteria: {', '.join(sorted(unknown))}")
    mode, agents_md, calls = plan(a.mode, root, a.agents_md or root / "AGENTS.md", a.base, criteria)
    if not calls:
        print(f"agents-md: {mode} mode, the diff since {a.base} changes nothing AGENTS.md is checked against: pass")
        return 0
    print(f"agents-md: {mode} mode, {len(calls)} calls on {a.model}, {min(a.jobs, len(calls))} at a time, "
          f"{a.timeout:.0f}s each at most", flush=True)
    start = time.monotonic()
    results = run_calls(calls, a.model, root, agents_md, a.jobs, a.timeout)
    cost = sum(r.cost_usd for r in results)
    print(report(results))
    for line in advice(verdicts(results)):
        print(line)
    print(f"agents-md: {time.monotonic() - start:.0f}s, ${cost:.3f}")
    if a.out:
        a.out.parent.mkdir(parents=True, exist_ok=True)
        a.out.write_text(json.dumps({"mode": mode, "model": a.model, "base": a.base, "verdicts": verdicts(results),
                                     "cost_usd": cost, "calls": [r.as_json() for r in results]}, indent=2) + "\n")
    return 0 if all(v == "pass" for v in verdicts(results).values()) else 1


if __name__ == "__main__":
    sys.exit(main())
