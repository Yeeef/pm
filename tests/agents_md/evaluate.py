#!/usr/bin/env python3
"""Grade AGENTS.md against the rubric (rubric.md beside this file) with an agent that reads the checkout.

    evaluate.py --mode full                        every criterion on the whole file
    evaluate.py --mode diff --base origin/main     accurate and complete, against the diff since the merge base
    evaluate.py --mode auto --base origin/main     full when that diff changes AGENTS.md, else diff

Each check is one `claude -p` call from the checkout root, with the read-only tools alone (Read, Grep, Glob), no
settings, hooks, CLAUDE.md or MCP servers, and a JSON schema for its answer. Full mode makes one call per criterion,
and one accurate call per `## ` section of AGENTS.md; diff mode one accurate and one complete call per chunk of the
diff. The calls run at once (--jobs), each killed with its process group at --timeout seconds, which fails the run.

A criterion fails when any of its calls reports a finding; each finding quotes AGENTS.md, and a quote that is not in
it makes the answer malformed. The run exits 1 on a failed criterion, a malformed answer, a failed or timed-out call.
It prints a table, and writes every call's answer, seconds and cost to --out.

In GitHub Actions (ACTIONS_ID_TOKEN_REQUEST_URL set) each call first writes a fresh GitHub OIDC token for the Anthropic
API to $ANTHROPIC_IDENTITY_TOKEN_FILE, for Claude Code's workload identity federation, and the call runs without any
other Anthropic credential in its environment. Elsewhere it runs with the user's own Claude Code login.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from pathlib import Path

HERE = Path(__file__).resolve().parent
RUBRIC = HERE / "rubric.md"
CRITERIA = ("accurate", "non_inferable", "map", "no_copies", "actionable", "no_history", "consistent", "complete")
DIFF_CRITERIA = ("accurate", "complete")
MODEL = "haiku"
TIMEOUT = 240  # seconds per call; a call past it fails the run
JOBS = 16
CHUNK = 60_000  # characters of diff per diff-mode call
# Diff paths no AGENTS.md claim rests on: generated expectations and lock files.
DIFF_EXCLUDE = (":(exclude)**/testdata/**", ":(exclude)uv.lock", ":(exclude)go.sum",
                ":(exclude).github/scripts/work-test-seconds.txt")
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
    """Full mode's calls: one per criterion on the whole file, accurate split by AGENTS.md section."""
    pre, crit = rubric(RUBRIC.read_text())
    head = (f"{pre}\n\nYou grade the AGENTS.md below on one criterion. The checkout is the current directory; the "
            "AGENTS.md under review is the text given here, not the file on disk, which may differ.\n\n"
            + fenced("agents_md", agents_md))
    calls = []
    for c in criteria:
        task = f"## Criterion: {c}\n\n{crit[c]}"
        if c == "accurate":
            for heading, _ in sections(agents_md):
                calls.append(Call(c, heading, f"{head}\n\n{task}\n\nCheck only the claims in the section "
                                              f"`{heading}`; the rest of the file is context.\n\n{ANSWER}"))
            continue
        extra = ""
        if c in ("no_copies", "consistent"):
            extra = "\n\n" + fenced("prime_md", (root / "prime.md").read_text())
            if c == "no_copies":
                extra += "\n\n" + fenced("changelog_preamble", changelog_preamble(root))
        calls.append(Call(c, "the whole file", f"{head}{extra}\n\n{task}\n\n{ANSWER}"))
    return calls


def chunks(diff: str, size: int = CHUNK) -> list[str]:
    """The diff cut at its file boundaries into chunks of about `size` characters; a larger file is a chunk alone."""
    files = re.split(r"(?=^diff --git )", diff, flags=re.M)
    out: list[str] = []
    for f in (f for f in files if f.strip()):
        if out and len(out[-1]) + len(f) <= size:
            out[-1] += f
        else:
            out.append(f)
    return out


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
    """Diff mode's calls: accurate and complete on each chunk of the diff."""
    pre, crit = rubric(RUBRIC.read_text())
    head = (f"{pre}\n\nYou review a pull request's effect on AGENTS.md. The checkout (the current directory) is at the "
            "PR's head; the diff below is the PR's change since its merge base.\n\n" + fenced("agents_md", agents_md))
    cut = chunks(diff)
    return [Call(c, f"diff {i}/{len(cut)}",
                 f"{head}\n\n{fenced('diff', part)}\n\n## Criterion: {c}\n\n{crit[c]}\n\n## This review\n\n"
                 f"{DIFF_TASK[c]}\n\n{ANSWER}")
            for i, part in enumerate(cut, 1) for c in DIFF_CRITERIA]


def git(root: Path, *args: str) -> str:
    return subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True, check=True).stdout


def pr_diff(root: Path, base: str) -> tuple[str, list[str]]:
    """The diff since the merge base of `base` and HEAD (DIFF_EXCLUDE left out), and every path it changes."""
    mb = git(root, "merge-base", base, "HEAD").strip()
    names = git(root, "diff", "--name-only", f"{mb}...HEAD").split()
    return git(root, "diff", f"{mb}...HEAD", "--", ".", *DIFF_EXCLUDE), names


_token_lock = threading.Lock()


def refresh_oidc(env: dict[str, str]) -> None:
    """In GitHub Actions: write a fresh OIDC token for the Anthropic API to $ANTHROPIC_IDENTITY_TOKEN_FILE."""
    url = env.get("ACTIONS_ID_TOKEN_REQUEST_URL")
    if not url:
        return
    path = env.get("ANTHROPIC_IDENTITY_TOKEN_FILE")
    if not path or not env.get("ACTIONS_ID_TOKEN_REQUEST_TOKEN"):
        raise RuntimeError("ACTIONS_ID_TOKEN_REQUEST_URL is set but ANTHROPIC_IDENTITY_TOKEN_FILE or "
                           "ACTIONS_ID_TOKEN_REQUEST_TOKEN is not")
    sep = "&" if "?" in url else "?"
    req = urllib.request.Request(f"{url}{sep}audience={AUDIENCE}",
                                 headers={"Authorization": f"bearer {env['ACTIONS_ID_TOKEN_REQUEST_TOKEN']}"})
    with _token_lock:
        with urllib.request.urlopen(req, timeout=30) as resp:
            token = json.load(resp)["value"]
        fd, tmp = tempfile.mkstemp(dir=Path(path).parent)
        with os.fdopen(fd, "w") as f:
            f.write(token)
        os.replace(tmp, path)  # whole, so a call starting meanwhile reads the old token or the new one


def child_env(env: dict[str, str]) -> dict[str, str]:
    """The call's environment: in CI, with federation set up, no other Anthropic credential."""
    if env.get("ACTIONS_ID_TOKEN_REQUEST_URL") and env.get("ANTHROPIC_FEDERATION_RULE_ID"):
        return {k: v for k, v in env.items() if k not in CREDENTIALS}
    return dict(env)


def command(model: str) -> list[str]:
    return ["claude", "-p", "--model", model, "--setting-sources", "", "--strict-mcp-config",
            "--tools", TOOLS, "--allowedTools", TOOLS, "--disallowedTools", "Bash,Edit,Write",
            "--no-session-persistence", "--output-format", "json", "--json-schema", json.dumps(SCHEMA)]


def normal(s: str) -> str:
    return re.sub(r"\s+", " ", re.sub(r"[`*_|\\]", "", s)).strip().lower()


def quoted(quote: str, agents_md: str) -> bool:
    """Whether a finding's quote is AGENTS.md text: each part between ellipses, without Markdown marks or spacing."""
    text = normal(agents_md)
    return all(normal(p) in text for p in re.split(r"\.\.\.|…", quote) if normal(p))


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
        if not quoted(f["quote"], agents_md):
            return findings, f"a finding quotes text that is not in AGENTS.md: {f['quote']!r}", cost
    return findings, "", cost


def run_call(call: Call, model: str, root: Path, agents_md: str, timeout: float = TIMEOUT,
             env: dict[str, str] | None = None) -> Result:
    """Run one check, killed with its process group at `timeout` seconds."""
    env = dict(os.environ if env is None else env)
    start = time.monotonic()
    try:
        refresh_oidc(env)
    except (OSError, RuntimeError, KeyError, ValueError) as e:
        return Result(call, time.monotonic() - start, error=f"OIDC token: {e}")
    proc = subprocess.Popen(command(model), cwd=root, env=child_env(env), stdin=subprocess.PIPE,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
    try:
        out, err = proc.communicate(call.prompt, timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        proc.communicate()
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
    with ThreadPoolExecutor(max(1, min(jobs, len(calls)))) as pool:
        return list(pool.map(lambda c: run_call(c, model, root, agents_md, timeout, env), calls))


def verdicts(results: list[Result]) -> dict[str, str]:
    """Each criterion's verdict: error when a call of it did not answer, fail on any finding, else pass."""
    out: dict[str, str] = {}
    for r in results:
        v = "error" if r.error else "fail" if r.findings else "pass"
        prev = out.get(r.call.criterion, "pass")
        out[r.call.criterion] = max(prev, v, key=("pass", "fail", "error").index)
    return out


def report(results: list[Result]) -> str:
    """The table: a line per criterion, then each finding and error under it."""
    lines = [f"{'criterion':14} {'verdict':7} evidence"]
    for crit, v in verdicts(results).items():
        rs = [r for r in results if r.call.criterion == crit]
        lines.append(f"{crit:14} {v:7} {len(rs)} call(s), {max(r.seconds for r in rs):.0f}s")
        for r in rs:
            if r.error:
                lines.append(f"    [{r.call.part}] error: {r.error}")
            lines += [f"    [{r.call.part}] {f['quote']!r}: {f['problem']} ({f['evidence']})" for f in r.findings]
    return "\n".join(lines)


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
            mode = "full" if "AGENTS.md" in names else "diff"
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
    print(f"agents-md: {time.monotonic() - start:.0f}s, ${cost:.3f}")
    if a.out:
        a.out.parent.mkdir(parents=True, exist_ok=True)
        a.out.write_text(json.dumps({"mode": mode, "model": a.model, "base": a.base, "verdicts": verdicts(results),
                                     "cost_usd": cost, "calls": [r.as_json() for r in results]}, indent=2) + "\n")
    return 0 if all(v == "pass" for v in verdicts(results).values()) else 1


if __name__ == "__main__":
    sys.exit(main())
