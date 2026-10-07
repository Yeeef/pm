"""pm: the write path for project records, and project actions that touch both records and Beads.

Every write names its target, validates the records branch as its commit will
leave it (HEAD plus the change, never another session's uncommitted file), and
writes nothing if the result would not render or if a record it changes has
uncommitted edits. Records live in
the store, the `records` branch checked out at `<main checkout>/.pm/store/records`; each
write commits there under a lock every worktree shares. Commands that
also call `bd` run the `bd` step first. Claim tasks with `pm task claim`; finding and linking tasks stay plain `bd`.
`pm feedback add` appends where pm got in the way to the project's pm feedback doc.
"""

from __future__ import annotations

import argparse
import contextlib
import copy
import fcntl
import functools
import hashlib
import hmac
import json
import os
import re
import secrets
import shlex
import shutil
import socket
import subprocess
import sys
import tempfile
import textwrap
import time
import tomllib
import urllib.parse
import dataclasses
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

import yaml

from pm import __version__, config, hooks, install, launch, legacy
from pm.beads import (ACTION, HUMAN, MERGE_REPORTED, MERGED, NO_DECISION, PICKED, REPLY_AUTHOR, REPLY_ID,
                           REPLY_MARK, ancestors, bd, blockers, children, dolt_state, dolt_store, kind, load_beads,
                           merge_waiting, owner_tasks, picked_up, reply_body, reply_in_beads, reply_waiting,
                           session_of, show_beads, site_replies, state)
from pm.records import (NONE_YET, NOT_CLOSED, Record, RecordError, decisions, first_para, headings,
                             insert_entry, outcome, parse_records, project_of, read_records, read_summaries,
                             summary_line,
                             read_summary, record_texts, report_part, section_range, section_text,
                             summary_path)
from pm.site import (SERVE_BEHIND, STATUS_SLOT, STYLE, check_needs_answered, cites, dismissed, local_day,
                          fill_replies, fill_status, pr_label, render_page, render_pages, render_record,
                          request_place, sprint_reviews)
from pm.store import (BRANCH, SETUP, code_root, commit, committed_records, design_dates, find_store, head_files,
                           main_of, read_files, store_path, uncommitted)
from pm.store import git as store_git
from pm import push as pushjob
from pm import service, tool

SPRINT_PROMPTS = {
    "Goal": "> What should be true when this sprint ends, and why now?",
    "Scope": "> What's in, and what's explicitly out? Keep this high level; implementation\n"
             "> details go in a design page.",
    "Done when": "> What evidence will show the goal is met?",
    "Design pages": "> Where is the detail?",
    "Progress": "> Where is the sprint now? Generated from Beads when the page is rendered.\n"
                "> Do not write here.",
    "Decisions": "> What did we choose inside this sprint, and why?",
    "Findings": "> What did we learn that changes the design, the plan, or how we work? Add\n"
                "> results with their numbers.",
    "Delivery report": "> Written at close. Each part holds \"Not closed yet.\" until then.",
    "Outcome": "> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.",
    'Against "Done when"': "> Each item, met or not, with its evidence (a page, a command, a number).",
}
PROJECT_PROMPTS = {
    "Goal": "> Why do we do it? What is it? What outcome do we expect?",
    "Progress": "> Where are we now, and what's next? Generated from Beads and the sprint\n"
                "> records when the page is rendered. Do not write here.",
    "Decisions": "> What constrains every future sprint? Sprint-only choices live in the sprint\n> record.",
    "Design pages": "> Where is the detail?",
    "Outcome": "> Written when the project closes: what was achieved against the goal, what\n"
               "> was learned, what was retired, and links to the sprint delivery reports.",
}
DESIGN_PROMPTS = {
    "Problem": "> What are we solving, and why now?",
    "Goals and non-goals": "> What must the design achieve, and what does it deliberately leave out?",
    "Constraints and key facts": "> Which facts, findings and constraints shaped the design? Only those that\n"
                                 "> still hold; sprint records keep the findings as they happened.",
    "Design": "> What is it, in its final state? Free `###` subsections. Decisions and plans\n"
              "> live in the project and sprint records.",
    "Alternatives considered": "> What else was considered and not adopted, and why not?",
    "Prior art": "> Optional. What existing work did we learn from, and what did we take?",
    "Open questions": "> What is still unresolved?",
}
POSTMORTEM_PROMPTS = {
    "Summary": "> What broke, for whom, and how was it noticed?",
    "Timeline": "> What happened when? Times with their zone, from the first cause to the fix.",
    "Cost": "> What did it cost: time lost, sessions or people blocked, work redone?",
    "Root cause": "> Why did it happen? The cause under the trigger.",
    "What changed": "> What was fixed, and what changed so it does not recur? Name the commits and PRs.",
    "What would have caught it earlier": "> Which test, check or rule would have caught it before it cost anything?",
}
FRAME = ["Goal", "Scope", "Done when"]
LEVEL_RULE = ("project if a later sprint must follow it; sprint if it is about this sprint's own work; "
              "skip choices cheap to reverse")
DECISION_SHAPE = ("a decision body is the decision on its first line, then its reason on the next line, e.g.:\n"
                  "  Records live on their own branch, in one store every worktree shares.\n"
                  "  A record kept on a code branch is invisible to the other branches until a merge.")
NEED_SHAPE = ("a decision need's stdin is one part per line: one Question:, one or more Fact:, two or more "
              "Option <label>: each with a Cost: line under it, and one Default: naming the label taken if the "
              "owner does not answer, then its reason, e.g.:\n"
              "  Question: Where does pm keep the public site URL?\n"
              "  Fact: Today, each clone keeps the URL in its git config.\n"
              "  Fact: You asked why the URL is not in `.pm/config.toml`.\n"
              "  Option a: In `.pm/config.toml`. A pm command still writes it.\n"
              "  Cost: the repo has one URL.\n"
              "  Option b: In each clone, as today.\n"
              "  Cost: you must give the URL to each new clone.\n"
              "  Default: a. All clones then give the same link.")
NEED_KEY = re.compile(r"(Question|Fact|Cost|Default|Option ([A-Za-z0-9]+)):(.*)")
SENTENCE_LIMIT = 25  # ASD-STE100: the most words in a descriptive sentence
ACTION_SHAPE = ("an action's description says what the owner should do and why, e.g.:\n"
                "  Restart the site on port 8767: the new proxy expects it there.")
# A request names a PR by its GitHub link or "PR #<n>"; it asks for a review or merge when it also says review, merge
# or approve. Such a request is raised only with pm action need --pr, so its card links the PR and its wait wakes on
# the merge; a PR named in passing ("should we split PR #31?") is allowed.
PR_NAMED = re.compile(r"https://github\.com/[\w.-]+/[\w.-]+/pull/\d+|\bPR\s*#\d+", re.I)
REVIEW_ASKED = re.compile(r"\b(review|merg|approv)\w*", re.I)
REVIEW_FORM = 'pm action need --pr URL --sprint ID --focus "…" [--design SLUG]'
FRAME_SHAPE = ("stdin is the sprint's frame, e.g.:\n"
               "  ## Goal\n  Ship the parser, because the site needs tables.\n"
               "  ## Scope\n  **In:** the parser and its tests.\n  **Out:** the site's styling.\n"
               "  ## Done when\n  - make test passes with a table record.")


class Refuse(Exception):
    """A precondition failed; nothing was changed."""


# ---------------------------------------------------------------- context and writes

@dataclass
class Repo:
    root: Path      # the worktree acted on: its HEAD names task closes, its site/ gets rendered
    records: Path   # the store
    beads: dict[str, dict]
    recs: list[Record]

    def sprint(self, bead_id: str) -> Record:
        rec = next((r for r in self.recs if r.type == "sprint" and r.meta["bead"] == bead_id), None)
        if rec is None:
            known = ", ".join(r.meta["bead"] for r in self.recs if r.type == "sprint")
            raise Refuse(f"no sprint record has bead {bead_id}; known sprints: {known}")
        return rec

    def project(self, name: str) -> Record:
        rec = next((r for r in self.recs if r.type == "project" and r.name == name), None)
        if rec is None:
            known = ", ".join(r.name for r in self.recs if r.type == "project")
            raise Refuse(f"no project record named {name!r}; known projects: {known or 'none'}")
        return rec


def git(root: Path, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=root, check=True, capture_output=True, text=True).stdout.strip()


def load(records: Path, working: bool = False) -> Repo:
    """Records plus Beads: the records branch's HEAD, which every write builds on, or with `working` the store as it
    is on disk, uncommitted edits included, for show and render. It only parses; a write validates what it touches
    (check_planned), and the whole store is validated by render, commit and serve."""
    root = code_root(Path.cwd(), records)
    beads = load_beads(root)
    recs = read_records(records) if working else committed_records(records)
    return Repo(root, records, beads, recs)


def check_planned(repo: Repo, writes: dict[Path, str], beads: dict[str, dict] | None = None) -> None:
    """Validate what the planned change touches, on the records branch with the planned writes committed: the
    written records, the records of every issue the change alters or its ancestors, and the answered-need check for
    the altered issues. A write builds on HEAD's text, so it refuses a target with uncommitted changes, which its
    commit would otherwise carry along."""
    dirty = uncommitted(repo.records, list(writes))
    if dirty:
        name = f"records/{dirty[0]}"
        raise Refuse(f"{name} has uncommitted changes, a hand edit or another session's edit in progress; commit "
                     f"them first with pm commit -m \"…\" {name}, or revert them, then run this again")
    try:
        beads = beads or repo.beads
        recs = committed_records(repo.records, writes)
        changed = {k for k, v in beads.items() if repo.beads.get(k) != v}
        touched = changed | {a for c in changed for a in ancestors(beads, c)}
        written = {p.resolve() for p in writes}
        check_needs_answered(recs, beads, ids=changed)
        for r in recs:
            if r.path.resolve() in written or r.meta.get("bead") in touched:
                render_record(r, recs, beads)
    except RecordError as e:
        raise Refuse(f"the change would not render: {e}")


def write_atomic(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.", suffix=".tmp")
    try:
        with os.fdopen(fd, "w", newline="") as f:
            f.write(text)
        mask = os.umask(0)
        os.umask(mask)
        os.chmod(tmp, (path.stat().st_mode & 0o777) if path.exists() else 0o666 & ~mask)
        os.replace(tmp, path)
    except BaseException:
        Path(tmp).unlink(missing_ok=True)
        raise


def restore(repo: Repo, before: dict[Path, str | None]) -> str:
    """Put each written file back as it was (removing one that did not exist) and unstage it, so a failed write
    leaves nothing in the store for another session's pm commit to sweep up; says what it did."""
    done, failed = [], []
    for path, text in before.items():
        try:
            if text is None:
                path.unlink(missing_ok=True)
            else:
                write_atomic(path, text)
            done.append(rel(repo, path))
        except OSError as e:
            failed.append(f"{rel(repo, path)} ({e.strerror or e})")
    names = [p.resolve().relative_to(repo.records.resolve()).as_posix() for p in before]
    staged = subprocess.run(["git", "diff", "--cached", "--name-only", "--", *names], cwd=repo.records,
                            capture_output=True, text=True)
    if staged.stdout.strip() and subprocess.run(["git", "reset", "-q", "--", *names], cwd=repo.records,
                                                capture_output=True).returncode != 0:
        failed.append(f"the index still stages {', '.join(staged.stdout.split())}; unstage with "
                      f"git -C {repo.records} reset -q -- {' '.join(names)}")
    out = f"restored {', '.join(done)} to the state before this write" if done else "restored nothing"
    return out + (f"; could not restore: {'; '.join(failed)}" if failed else "")


def apply_writes(repo: Repo, writes: dict[Path, str], message: str, undo: str | None = None, prefix: str = "pm: ") -> str:
    """Write the records and commit exactly those files on the records branch, the message `prefix` + `message`;
    returns `message`. If a write or the commit fails, the files are restored, so the failure leaves nothing behind."""
    hint = f"; undo the Beads step with: {undo}" if undo else ""
    before = {path: (path.open(newline="").read() if path.exists() else None) for path in writes}
    written = []
    try:
        for path, text in writes.items():
            write_atomic(path, text)
            written.append(path)
        commit(repo.records, f"{prefix}{message}", list(writes))
    except (OSError, RecordError) as e:
        what = (f"writing {e.filename or 'the record'} failed: {e.strerror or e}" if isinstance(e, OSError)
                else f"committing failed: {e}")
        undone = restore(repo, {p: before[p] for p in written}) if written else "no record was changed"
        raise Refuse(f"{what}; {undone}{hint}")
    return message


def locked(records: Path):
    """Hold an exclusive lock on the store, shared by every worktree, across a write and its commit."""
    fd = os.open(records, os.O_RDONLY)
    fcntl.flock(fd, fcntl.LOCK_EX)
    return fd


def stdin_text() -> str:
    """stdin, read whole; main reads it before taking the store lock, so a caller that leaves stdin open blocks only
    its own command, never another session's write. Commands get it as args.stdin."""
    return "" if sys.stdin.isatty() else sys.stdin.read().strip()


def rel(repo: Repo, path: Path) -> str:
    return "records/" + path.resolve().relative_to(repo.records.resolve()).as_posix()


def short(issue_id: str, under: str) -> str:
    return issue_id[len(under):] if issue_id.startswith(under + ".") else issue_id


def first_sentence(text: str, limit: int = 110) -> str:
    s = first_para(text)
    m = re.match(r"(.+?[.!?])(\s|$)", s)
    s = m.group(1) if m else s
    return s if len(s) <= limit else s[:limit - 1].rstrip() + "…"


def parse_sections(text: str, allowed: list[str]) -> dict[str, str]:
    """Split stdin like '## Goal\\n...\\n## Scope\\n...' into section bodies."""
    parts = re.split(r"(?m)^## (.+?)\s*$", text)
    if parts[0].strip():
        raise Refuse(f"text before the first section heading; start stdin with '## {allowed[0]}'; {FRAME_SHAPE}")
    out = {}
    for name, body in zip(parts[1::2], parts[2::2]):
        if name not in allowed:
            raise Refuse(f"unknown section '## {name}' on stdin; allowed: {', '.join(allowed)}; {FRAME_SHAPE}")
        out[name] = body.strip()
    return out


def created(out: str) -> tuple[str, str]:
    """The id `bd create --json` printed, and the bd command that undoes the create."""
    try:
        data = json.loads(out)
        new_id = (data[0] if isinstance(data, list) else data)["id"]
    except (ValueError, KeyError, IndexError, TypeError):
        raise Refuse(f"bd create succeeded but its output has no id: {out.strip()!r}; "
                     "find the new issue with bd list and delete it with bd delete <id> --force")
    return new_id, f"bd delete {new_id} --force"


# ---------------------------------------------------------------- record templates

def sprint_text(title: str, bead: str, frame: dict[str, str]) -> str:
    p = SPRINT_PROMPTS
    return (f"---\ntype: sprint\ntitle: {title}\nbead: {bead}\n---\n\n"
            f"## Goal\n\n{p['Goal']}\n\n{frame['Goal']}\n\n"
            f"## Scope\n\n{p['Scope']}\n\n{frame['Scope']}\n\n"
            f"## Done when\n\n{p['Done when']}\n\n{frame['Done when']}\n\n"
            f"## Design pages\n\n{p['Design pages']}\n\n{NONE_YET}\n\n"
            f"## Progress\n\n{p['Progress']}\n\n"
            f"## Decisions\n\n{p['Decisions']}\n\n{NONE_YET}\n\n"
            f"## Findings\n\n{p['Findings']}\n\n{NONE_YET}\n\n"
            f"## Delivery report\n\n{p['Delivery report']}\n\n"
            f"### Outcome\n\n{p['Outcome']}\n\n{NOT_CLOSED}\n\n"
            f"### Against \"Done when\"\n\n{p['Against \"Done when\"']}\n\n{NOT_CLOSED}\n")


def project_text(title: str, bead: str, goal: str) -> str:
    p = PROJECT_PROMPTS
    return (f"---\ntype: project\ntitle: {title}\nbead: {bead}\n---\n\n"
            f"## Goal\n\n{p['Goal']}\n\n{goal}\n\n"
            f"## Progress\n\n{p['Progress']}\n\n"
            f"## Decisions\n\n{p['Decisions']}\n\n{NONE_YET}\n\n"
            f"## Design pages\n\n{p['Design pages']}\n\n{NONE_YET}\n\n"
            f"## Outcome\n\n{p['Outcome']}\n\n{NOT_CLOSED}\n")


def design_text(title: str, project: str) -> str:
    sections = "".join(f"## {name}\n\n{prompt}\n\n{NONE_YET}\n\n" for name, prompt in DESIGN_PROMPTS.items())
    return f"---\ntype: design\ntitle: {title}\nproject: {project}\n---\n\n" + sections.rstrip("\n") + "\n"


def postmortem_text(title: str, day: str, target: str) -> str:
    sections = "".join(f"## {name}\n\n{prompt}\n\n{NONE_YET}\n\n" for name, prompt in POSTMORTEM_PROMPTS.items())
    return f"---\ntype: postmortem\ntitle: {title}\ndate: {day}\n{target}\n---\n\n" + sections.rstrip("\n") + "\n"


def yaml_str(s: str) -> str:
    """A header value: plain when YAML reads it back unchanged, otherwise double-quoted."""
    try:
        if yaml.safe_load(f"k: {s}") == {"k": s}:
            return s
    except yaml.YAMLError:
        pass
    return json.dumps(s, ensure_ascii=False)


# ---------------------------------------------------------------- commands: writes

SUMMARY_MODEL = "haiku"
SUMMARY_TIMEOUT = 180  # seconds the model call may take
SUMMARY_PROMPT = """You write the "Today" summary at the top of a project-management site's page for {day}.
Below is everything recorded on {day}: per project, the sprints finished (with their outcome) and opened, the tasks
finished (with how), started and opened, the requests to the owner raised and closed that day, and the commits to the
project records. Write 2 to 4 Markdown bullets for the owner, each line starting with "- ". The owner knows no ids or
sprint numbers and wants to know what shipped. Each bullet is one short sentence. Put first what shipped or finished
and what it changes for the owner. Then name work in progress only if it is notable. Put last what the owner must do,
from the requests raised that day and still open, named by the action. Name work by what it does. Never write ids,
sprint numbers, task numbers, PR numbers or other numbers. Write in ASD-STE100 Simplified Technical English: short
sentences of at most 20 words, active voice, one meaning for each word, no idioms. Do not invent anything that is not
below. Output only the bullets, no heading, no preamble.

{activity}"""


def day_activity_text(repo: Repo, day: str) -> str:
    """Plain text of what happened on the local day `day`: per project, the sprints finished (with their Outcome)
    or opened, the tasks finished (with their close reason), started or opened, and the owner requests raised or
    closed that day; the docs dated that day; and the records committed that day, not counting the generated
    summaries. The summary's input and, hashed, its digest. It holds only events dated that day, never current
    state (a sprint's status, the requests open now), so a day's digest changes only when that day's events do and a
    past day's summary is generated once. Empty when nothing happened that day."""
    out = []
    for p in (r for r in repo.recs if r.type == "project"):
        lines = []
        sprints = sorted((i for i in repo.beads.values()
                          if i.get("parent") == p.meta["bead"] and i.get("issue_type") == "epic"), key=lambda i: i["id"])
        for sp in sprints:
            rows = []
            if local_day(sp.get("closed_at")) == day:
                rec = next((r for r in repo.recs if r.type == "sprint" and r.meta["bead"] == sp["id"]), None)
                rows.append("    sprint finished" + (f": {outcome(rec)}" if rec and outcome(rec) else ""))
            if local_day(sp.get("created_at")) == day:
                rows.append("    sprint opened")
            for t in sorted((i for i in repo.beads.values() if i.get("parent") == sp["id"] and not dismissed(i)),
                            key=lambda i: i["id"]):
                request = HUMAN in (t.get("labels") or [])
                noun = f"request to the owner ({kind(t)})" if request else "task"
                if local_day(t.get("closed_at")) == day:
                    reason = t.get("close_reason") or ""
                    rows.append(f"    {noun} closed: {t['title']}" + (f" ({reason})" if reason else ""))
                elif local_day(t.get("started_at")) == day:
                    rows.append(f"    {noun} started: {t['title']}")
                if local_day(t.get("created_at")) == day:
                    rows.append(f"    {noun} raised: {t['title']}" if request else f"    task opened: {t['title']}")
            if rows:
                lines += [f"  sprint {sp['title']}"] + rows
        if lines:
            out += [f"project {p.title}"] + lines
    out += [f"doc {r.title} ({r.rel})" for r in repo.recs if r.type == "doc" and str(r.meta["date"]) == day]
    log = store_git(repo.records, "log", f"--since={day}T00:00", f"--until={day}T23:59:59", "--format=%x00%s",
                    "--name-only")
    for chunk in log.split("\0")[1:]:
        subject, *names = [l for l in chunk.split("\n") if l]
        names = [n for n in names if not n.endswith(".summary.json")]
        if names:
            out.append(f"records commit: {subject} [{', '.join(names)}]")
    return "\n".join(out)


def ask_model(prompt: str) -> str:
    """The model's answer to `prompt` from the claude CLI, non-interactive, without tools, hooks or a saved session.
    Fails hard when claude is missing, fails or answers nothing; there is no fallback text."""
    cmd = ["claude", "-p", "--model", SUMMARY_MODEL, "--tools", "", "--setting-sources", "",
           "--no-session-persistence"]
    with tempfile.TemporaryDirectory() as cwd:  # no project settings, hooks or CLAUDE.md apply
        try:
            res = subprocess.run(cmd, input=prompt, capture_output=True, text=True, cwd=cwd, timeout=SUMMARY_TIMEOUT)
        except FileNotFoundError:
            raise Refuse("claude is not installed or not on PATH; pm day summarize needs the claude CLI")
        except subprocess.TimeoutExpired:
            raise Refuse(f"claude -p timed out after {SUMMARY_TIMEOUT}s; no summary written")
    text = res.stdout.strip()
    if res.returncode != 0 or not text:
        said = " ".join((res.stderr or res.stdout).split())[:500] or "no output"
        raise Refuse(f"claude -p failed (exit {res.returncode}): {said}; no summary written")
    return text


def summarize_day(records: Path, dry_run: bool = False) -> str:
    """Yesterday's final summary, then today's. Yesterday's is regenerated when its activity differs from its stored
    summary's, so the activity of its last minutes before midnight is summarized too. It is reported only when it
    was (re)generated or failed."""
    yesterday = (date.today() - timedelta(days=1)).isoformat()
    first = summarize_one(records, yesterday, dry_run)
    today = summarize_one(records, date.today().isoformat(), dry_run)
    return today if first.endswith("skipped") or first.endswith("nothing to summarize") else f"{first}\n{today}"


def summarize_one(records: Path, day: str, dry_run: bool) -> str:
    """`day`'s summary: skipped when nothing happened that day, when the activity is unchanged since the stored
    summary (its digest); otherwise asked of the model and
    committed to days/<day>.summary.json on the records branch (with `dry_run`, printed and not written). The model
    call runs without the store lock, so it never blocks another session's write."""
    repo = load(records)
    activity = day_activity_text(repo, day)
    if not activity:
        return f"no activity on {day}; nothing to summarize"
    digest = hashlib.sha256(activity.encode()).hexdigest()[:16]
    path = summary_path(records, day)
    old = read_summary(path) if path.exists() else None
    if old and old["digest"] == digest:
        return f"activity unchanged since the summary of {old['generated_at']}; skipped"
    text = ask_model(SUMMARY_PROMPT.format(day=day, activity=activity))
    data = {"date": day, "generated_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
            "digest": digest, "model": SUMMARY_MODEL, "text": text}
    body = json.dumps(data, indent=1, ensure_ascii=False) + "\n"
    if dry_run:
        return f"dry run, nothing written; would write {rel(repo, path)}:\n{body}"
    fd = locked(records)
    try:
        if uncommitted(records, [path]):
            raise Refuse(f"{rel(repo, path)} has uncommitted changes; revert them and run this again")
        return apply_writes(repo, {path: body}, f"summarized {day} in {rel(repo, path)}")
    finally:
        os.close(fd)


def cmd_day_summarize(args, records: Path) -> str:
    return summarize_day(records, args.dry_run)


def cmd_finding_add(args, records: Path) -> str:
    text = " ".join(args.text).strip()
    if not text:
        raise Refuse("the finding text is empty")
    repo = load(records)
    rec = repo.sprint(args.sprint)
    if repo.beads[args.sprint]["status"] == "closed":
        raise Refuse(f"sprint {args.sprint} is closed; add the finding to an open sprint")
    entry = textwrap.fill(text, width=78, initial_indent="- ", subsequent_indent="  ",
                          break_long_words=False, break_on_hyphens=False)
    new = insert_entry(rec.text, "Findings", entry)
    check_planned(repo, {rec.path: new})
    return apply_writes(repo, {rec.path: new}, f"added a finding to {rel(repo, rec.path)}")


def decision_target(repo: Repo, args) -> Record:
    """The open project or sprint record a decision of `args.level` goes into, named by its flag."""
    if args.level is None:
        raise Refuse(f"--level is required (project|sprint): {LEVEL_RULE}")
    flag, other = ("--project", "--sprint") if args.level == "project" else ("--sprint", "--project")
    if getattr(args, other[2:]) is not None:
        raise Refuse(f"{other} does not match --level {args.level}; name the target with {flag}")
    target = getattr(args, flag[2:])
    if target is None:
        raise Refuse(f"--level {args.level} needs {flag}; nothing is inferred")
    rec = repo.project(target) if args.level == "project" else repo.sprint(target)
    bead = rec.meta["bead"]
    if repo.beads[bead]["status"] == "closed":
        raise Refuse(f"{args.level} {target} ({bead}) is closed; record the decision in an open {args.level}")
    return rec


def decision_body(body: str) -> str:
    if not body:
        raise Refuse(f"the decision body is empty; pipe it on stdin: {DECISION_SHAPE}")
    if len([l for l in body.splitlines() if l.strip()]) < 2:
        raise Refuse(f"the decision body is a single line; {DECISION_SHAPE}")
    if re.search(r"(?m)^:::", body):
        raise Refuse(f"the decision body has a line starting with ':::', and blocks cannot nest; {DECISION_SHAPE}")
    return body


def decision_block(source: str, body: str, until: str | None) -> str:
    extra = ""
    if until is not None:
        if not until.strip() or '"' in until or "\n" in until:
            raise Refuse("--until must be one non-empty line without double quotes")
        extra = f' until="{until.strip()}"'
    return f"::: decision {{source={source} date={date.today().isoformat()}{extra}}}\n{body}\n:::"


def refuse_unread(need: dict) -> None:
    """Closing a request that holds a site reply not delivered yet would bury the reply: pm show flags only open
    requests, so it would reach no session."""
    if reply_waiting(need):
        raise Refuse(f"{need['id']} holds a site reply not delivered yet; read it with pm reply read {need['id']}, "
                     "then run this again if it still stands")


def cmd_decision_add(args, records: Path) -> str:
    """Record a decision; with --need it is the owner's answer to that decision need, which it also closes (bd human
    respond with the same text) unless the owner already closed it."""
    body = args.stdin
    repo = load(records)
    rec = decision_target(repo, args)
    decision_body(body)
    source, need = "agent", None
    if args.need:
        need = human_issue(repo, args.need, "decision")
        if need["status"] == "closed" and dismissed(need):
            raise Refuse(f"need {args.need} was dismissed, so it has no answer to record")
        source = "owner"
        if not cites(body, args.need):
            body += f"\nAnswers `{args.need}`."
    elif args.confirmed:
        source = "owner"
    new = insert_entry(rec.text, "Decisions", decision_block(source, body, args.until))
    done = f"added a source={source} {args.level} decision to {rel(repo, rec.path)}"
    if need is None or need["status"] == "closed":
        check_planned(repo, {rec.path: new})
        return apply_writes(repo, {rec.path: new}, done)
    refuse_unread(need)
    beads = {**repo.beads, args.need: {**need, "status": "closed", "close_reason": "Responded"}}
    check_planned(repo, {rec.path: new}, beads)
    bd(repo.root, "human", "respond", args.need, f"--response={body}")
    return apply_writes(repo, {rec.path: new}, f"closed need {args.need} and {done}", undo=f"bd reopen {args.need}")


def sentences(text: str) -> list[str]:
    """Split at . ! or ? followed by white space; a code span is one unit, so a period in it ends no sentence."""
    out, start = [], 0
    for m in re.finditer(r"(`+).+?\1|[.!?](?=\s)", text):
        if m.group(1) is None:
            out.append(text[start:m.end()].strip())
            start = m.end()
    return out + ([text[start:].strip()] if text[start:].strip() else [])


def words(sentence: str) -> list[str]:
    """Runs between white space that hold a letter or digit; a code span is one word."""
    runs = [m.group(0) for m in re.finditer(r"(?:(`+).+?\1|[^\s`]|`)+", sentence)]
    return [w for w in runs if "`" in w or re.search(r"[^\W_]", w)]


def plain(text: str) -> str:
    """Escape a leading block marker (heading, quote, list item), so a fact stays one plain list item."""
    text = re.sub(r"^([#>]|[-+*](?=\s|$))", r"\\\1", text)
    return re.sub(r"^(\d{1,9})(?=[.)](?:\s|$))", r"\1\\", text)


def need_markdown(stdin: str) -> str:
    """A decision need's description in its one layout, from the parts on stdin (NEED_SHAPE)."""
    question, facts, options, default = [], [], {}, []
    last = None  # label of the option above that has no cost yet
    for n, line in enumerate(stdin.splitlines(), 1):
        line = line.strip()
        if not line:
            continue
        m = NEED_KEY.fullmatch(line)
        if not m:
            raise Refuse(f"line {n} starts with no known key; each line starts with Question:, Fact:, Option <label>:, "
                         f"Cost: or Default:; {NEED_SHAPE}")
        key, label, text = m.group(1), m.group(2), m.group(3).strip()
        if not text:
            raise Refuse(f"line {n}: {key}: has no text; {NEED_SHAPE}")
        if label:
            if label in options:
                raise Refuse(f"two options have the label {label}; give each option its own label")
            options[label] = [text, None]
            last = label
        elif key == "Cost":
            if last is None:
                raise Refuse(f"line {n}: Cost: follows no option without a cost; put one Cost: line under each "
                             "Option line")
            options[last][1], last = text, None
        else:
            {"Question": question, "Fact": facts, "Default": default}[key].append(text)
    if len(question) != 1:
        raise Refuse(f"give exactly one Question: line; {NEED_SHAPE}")
    if not facts:
        raise Refuse(f"give at least one Fact: line, so the owner can decide without other context; {NEED_SHAPE}")
    if len(options) < 2:
        raise Refuse(f"give at least two Option lines; a decision needs a choice; {NEED_SHAPE}")
    for label, (_, cost) in options.items():
        if cost is None:
            raise Refuse(f"option {label} has no Cost: line; put its cost on the line under it")
    if len(default) != 1:
        raise Refuse(f"give exactly one Default: line, the label of the option taken if the owner does not answer; "
                     f"{NEED_SHAPE}")
    choice = re.match(r"[A-Za-z0-9]+", default[0])
    label = choice.group(0) if choice else default[0].split()[0]
    if label not in options:
        raise Refuse(f"Default: {label} names no option; the labels are {', '.join(options)}")
    rest = default[0][len(label):]
    parts = [("the question", question[0]), *((f"fact {i}", f) for i, f in enumerate(facts, 1))]
    for opt, (text, cost) in options.items():
        parts += [(f"option {opt}", text), (f"the cost of option {opt}", cost)]
    parts.append(("the default", rest.lstrip(" .,;:")))
    for part, text in parts:
        for s in sentences(text):
            if len(found := words(s)) > SENTENCE_LIMIT:
                raise Refuse(f'the sentence "{" ".join(found[:6])} …" in {part} has {len(found)} words; the limit is '
                             f"{SENTENCE_LIMIT} (ASD-STE100); split it")
    bullets = []
    for opt, (text, cost) in options.items():
        first = sentences(text)[0]
        more = text[len(first):].strip()
        bullets.append(f"- **({opt}) {first}** " + (f"{more} " if more else "") + f"*Cost:* {cost}")
    return (f"**Question:** {question[0]}\n\n**Facts:**\n\n" + "".join(f"- {plain(f)}\n" for f in facts)
            + "\n**Options:**\n\n" + "".join(b + "\n" for b in bullets) + f"\n**Default:** ({label}){rest}")


def raise_need(args, records: Path, want: str) -> str:
    """Raise a decision need or an action: a Beads task labelled human (and action, for an action)."""
    title = args.title.strip()
    if not title:
        raise Refuse("--title is empty")
    desc = args.stdin
    text = f"{title}\n{desc}"
    if PR_NAMED.search(text) and REVIEW_ASKED.search(text):
        raise Refuse(f"this asks the owner to review or merge a PR; raise it with the review form, so its card links "
                     f"the PR, the sprints and design pages and its wait wakes on the merge: {REVIEW_FORM}. If it "
                     "only mentions the PR, say what you ask without review, merge or approve")
    if want == "action":
        if not desc:
            raise Refuse(f"the description is empty; pipe it on stdin: {ACTION_SHAPE}")
    else:
        if not desc:
            raise Refuse(f"the description is empty; pipe it on stdin: {NEED_SHAPE}")
        desc = need_markdown(desc)
    repo = load(records)
    parent = repo.beads.get(args.parent)
    if parent is None:
        raise Refuse(f"--parent {args.parent} is not a Beads issue")
    if parent["status"] == "closed":
        raise Refuse(f"--parent {args.parent} is closed; raise it under an open sprint or task")
    epics = {r.meta["bead"] for r in repo.recs if r.type == "project"}
    if not epics & set(ancestors(repo.beads, args.parent)):
        raise Refuse(f"--parent {args.parent} is not inside a project with a record, so no page would show it")
    labels = HUMAN if want == "decision" else f"{HUMAN},{ACTION}"
    meta = raised_by()
    out = bd(repo.root, "create", "--type=task", f"--parent={args.parent}", f"--labels={labels}",
             f"--title={title}", f"--description={desc}", *([f"--metadata={json.dumps(meta)}"] if meta else []),
             "--json")
    need_id, _ = created(out)
    if want == "action":
        return (f"raised action {need_id} under {args.parent}; {delivery_hint(need_id)}; once you see the "
                f"owner has done it: pm action done {need_id} --reason \"<what you saw>\"")
    return (f"raised decision need {need_id} under {args.parent}; {delivery_hint(need_id)}; once the owner "
            f"answers: pm decision add --need {need_id} --level … with the answer as the body if it sets a rule, else "
            f"pm decision close {need_id} --reason \"<why it sets no rule>\" with the answer on stdin")


SESSION_ENV = "CLAUDE_CODE_SESSION_ID"  # Claude Code exports it to every command a session runs
CODEX_SESSION_ENV = "CODEX_THREAD_ID"  # Codex exports it to every command a thread runs
LIVE_WINDOW = 30 * 60  # seconds: a session is live while its transcript was written this recently


def current_session() -> str | None:
    """This command's agent session: Claude Code's session id, else Codex's thread id, else None."""
    for name in (SESSION_ENV, CODEX_SESSION_ENV):
        sid = os.environ.get(name, "").strip()
        if sid:
            return sid
    return None


def transcripts(sid: str) -> list[Path]:
    """The session's transcript files: Claude Code's <config>/projects/<project dir>/<id>.jsonl (any project dir,
    since each worktree has its own) and Codex's $CODEX_HOME/sessions/<date>/rollout-<time>-<id>.jsonl."""
    if not re.fullmatch(r"[\w-]+", sid):
        return []
    claude = Path(os.environ.get("CLAUDE_CONFIG_DIR") or Path.home() / ".claude")
    return [*(claude / "projects").glob(f"*/{sid}.jsonl"), *(codex_home() / "sessions").glob(f"**/rollout-*-{sid}.jsonl")]


def live(sid: str) -> bool:
    """A session is live while one of its transcripts was modified within LIVE_WINDOW."""
    return any(time.time() - p.stat().st_mtime < LIVE_WINDOW for p in transcripts(sid))


def holder(issue: dict) -> dict | None:
    """Who holds an in-progress issue: the session `pm task claim` recorded and when, or only the bd assignee for a
    claim made without pm."""
    if issue.get("status") != "in_progress":
        return None
    meta = issue.get("metadata") if isinstance(issue.get("metadata"), dict) else {}
    sid = meta.get("claimed_by")
    if not sid:
        return {"session": None, "assignee": issue.get("assignee"), "claimed_at": issue.get("started_at"), "live": None}
    return {"session": sid, "assignee": issue.get("assignee"), "claimed_at": meta.get("claimed_at"), "live": live(sid)}


def age(stamp: str | None) -> str:
    """'12m', '3h' or '2d' since an ISO time; '?' when unknown."""
    if not stamp:
        return "?"
    s = (datetime.now(timezone.utc) - datetime.fromisoformat(stamp.replace("Z", "+00:00"))).total_seconds()
    return f"{int(s // 60)}m" if s < 3600 else f"{int(s // 3600)}h" if s < 86400 else f"{int(s // 86400)}d"


def holder_text(h: dict) -> str:
    if h["session"] is None:
        return f"held by {h['assignee'] or '?'} without a session, {age(h['claimed_at'])}"
    return f"held by {h['session'][:8]}, {age(h['claimed_at'])}, {'live' if h['live'] else 'idle'}"
INBOX_ENV = "CLAUDE_CODE_MESSAGING_SOCKET"  # Claude Code exports its session's inbox socket to every command


def delivery_hint(need_id: str) -> str:
    """How the raising agent hears the answer: the pm service pushes it into the session's inbox when there is one."""
    if os.environ.get(INBOX_ENV, "").strip():
        return (f"the pm service pushes the owner's reply into this session; if the session has ended by then, pm show "
                f"flags it for the next one, which reads it with pm reply read {need_id}")
    return (f"this session has no inbox (${INBOX_ENV} unset), so no reply is pushed to it; pm show flags a reply, "
            f"and pm reply read {need_id} prints it")


def raised_by() -> dict:
    """The issue metadata naming the session that raises a request (Claude Code's session id or Codex's thread id,
    which the owner-request Stop hook matches against its session_id) and its inbox socket, or nothing outside a known
    session. Never the inbox's token: Beads syncs to the remote."""
    sid = current_session()
    inbox = os.environ.get(INBOX_ENV, "").strip()
    return ({"session": sid} if sid else {}) | ({"inbox": inbox, "inbox_host": socket.gethostname()} if inbox else {})


def refresh_inbox(repo: Repo) -> None:
    """Point this session's open requests at its inbox as of now: a resumed session keeps its id but binds a new
    socket, so pushes to the stored one would fail for the rest of its life. Reads nothing beyond `repo`'s Beads."""
    meta = raised_by()
    if "inbox" not in meta or "session" not in meta:
        return
    for i in repo.beads.values():
        m = i.get("metadata") if isinstance(i.get("metadata"), dict) else {}
        if (i["status"] != "closed" and HUMAN in (i.get("labels") or []) and m.get("session") == meta["session"]
                and (m.get("inbox"), m.get("inbox_host")) != (meta["inbox"], meta["inbox_host"])):
            for key in ("inbox", "inbox_host"):
                bd(repo.root, "update", i["id"], f"--set-metadata={key}={meta[key]}")


def cmd_decision_need(args, records: Path) -> str:
    return raise_need(args, records, "decision")


def cmd_action_need(args, records: Path) -> str:
    if args.pr is None:
        for flag, val in (("--sprint", args.sprint), ("--focus", args.focus), ("--design", args.design)):
            if val is not None:
                raise Refuse(f"{flag} is only for a PR review; give --pr URL with it, or drop it")
        if args.parent is None:
            raise Refuse("--parent is required (or --pr for a PR review)")
        if args.title is None:
            raise Refuse("--title is required (or --pr for a PR review)")
        return raise_need(args, records, "action")
    if args.parent is not None:
        raise Refuse("--parent is not allowed with --pr; a PR review goes under the first sprint it delivers")
    for flag, val in (("--sprint", args.sprint), ("--focus", args.focus)):
        if val is None:
            raise Refuse(f"{flag} is required with --pr")
    return raise_review(args, records)


def raise_review(args, records: Path) -> str:
    """Raise a PR review as an action carrying what the owner reads first: the PR, its sprints, the design pages
    behind it and the focus; kept as issue metadata so the site links each to its page."""
    pr = args.pr.strip()
    if not re.fullmatch(r"https?://\S+", pr):
        raise Refuse(f"--pr {pr!r} is not a URL; give the pull request's link")
    focus = args.focus.strip()
    if not focus:
        raise Refuse("--focus is empty; say what to look at first: the risky changes and the open choices")
    extra = args.stdin
    repo = load(records)
    sprints = list(dict.fromkeys(args.sprint))
    # A sprint closes only once its PR merges, so every sprint a PR under review delivers is open, and its delivery
    # report is written, for the owner to read next to the diff. The review goes under the first sprint and blocks
    # its close.
    for sid in sprints:
        rec = repo.sprint(sid)
        if repo.beads[sid]["status"] == "closed":
            raise Refuse(f"sprint {sid} is closed, but a sprint closes only after its PR merges; a PR under review "
                         "delivers only open sprints")
        unwritten = [f"'{p}'" for p, text in (("Outcome", outcome(rec)),
                                              ('Against "Done when"', report_part(rec, 'Against "Done when"')))
                     if text in ("", NOT_CLOSED)]
        if unwritten:
            raise Refuse(f"{rel(repo, rec.path)}: the committed Delivery report {' and '.join(unwritten)} "
                         f"{'are' if len(unwritten) > 1 else 'is'} still "
                         f"'{NOT_CLOSED}'; write the Outcome (done, partial or voided, plus one sentence, then optional "
                         "bullets) and each 'Done when' item with its evidence, and commit them with pm commit, so "
                         "the owner reads the report while reviewing the PR")
    parent = sprints[0]
    designs = list(dict.fromkeys(args.design or []))
    known = {r.name for r in repo.recs if r.type == "design"}
    for slug in designs:
        if slug not in known:
            raise Refuse(f"--design {slug} has no design page; known: {', '.join(sorted(known)) or 'none'}")
    n = re.search(r"/pull/(\d+)", pr)
    title = (args.title or "").strip() or (f"Review PR #{n.group(1)}" if n else f"Review {pr}")
    meta = {"review": {"pr": pr, "sprints": sprints, "focus": focus, "designs": designs}, **raised_by()}
    desc = (f"Review {pr}, which delivers {', '.join(sprints)}.\n\nFocus: {focus}"
            + (f"\n\nDesign pages: {', '.join(designs)}" if designs else "") + (f"\n\n{extra}" if extra else ""))
    out = bd(repo.root, "create", "--type=task", f"--parent={parent}", f"--labels={HUMAN},{ACTION}",
             f"--title={title}", f"--description={desc}", f"--external-ref={pr}",
             f"--metadata={json.dumps(meta)}", "--json")
    need_id, _ = created(out)
    return (f"raised review {need_id} under {parent}; {delivery_hint(need_id)}; the pm service pushes the PR's merge to "
            f"main the same way; once the PR is on main: pm action done {need_id} --reason \"merged as <sha>\"")


def human_issue(repo: Repo, issue_id: str, want: str) -> dict:
    """The `human` issue `issue_id`, refused unless it waits for `want` (decision or action)."""
    need = repo.beads.get(issue_id)
    if need is None:
        raise Refuse(f"{issue_id} is not a Beads issue")
    if HUMAN not in (need.get("labels") or []):
        raise Refuse(f"{issue_id} is not labelled human, so it is not a need; record your own decision with pm decision add "
                     "without --need")
    if kind(need) != want:
        hint = (f"close it with pm action done {issue_id}" if kind(need) == "action" else
                f"answer it with pm decision add --need {issue_id}, or pm decision close {issue_id} if the answer "
                "sets no rule")
        raise Refuse(f"{issue_id} is {'an action' if kind(need) == 'action' else 'a decision need'}; {hint}")
    return need


def cmd_decision_close(args, records: Path) -> str:
    """Close a decision need whose answer sets no rule, with the reason in Beads, then label it no-decision; a need
    the owner already closed with bd human respond only gets the reason, as a comment, and the label. The label comes
    last, so a failure leaves a closed need the render check still flags, and running the command again finishes it
    without repeating the reason."""
    answer = args.stdin
    why = (args.reason or "").strip()
    if not why:
        raise Refuse("--reason is empty; say why the answer sets no rule")
    repo = load(records)
    need = human_issue(repo, args.need_id, "decision")
    labels = list(need.get("labels") or [])
    if NO_DECISION in labels:
        raise Refuse(f"need {args.need_id} is already marked {NO_DECISION}")
    if need["status"] == "closed" and dismissed(need):
        raise Refuse(f"need {args.need_id} was dismissed, so it has no answer to mark")
    if need["status"] != "closed" and not answer:
        raise Refuse("the answer is empty; pipe the owner's answer on stdin, as they gave it")
    refuse_unread(need)
    note = f"No decision record: {why}"
    beads = {**repo.beads, args.need_id: {**need, "status": "closed", "labels": labels + [NO_DECISION],
                                          "close_reason": need.get("close_reason") or "Responded"}}
    check_planned(repo, {}, beads)
    retry = f'run the same command again to label it: pm decision close {args.need_id} --reason "{why}"'
    if need["status"] == "closed":
        comments = json.loads(bd(repo.root, "comments", args.need_id, "--json") or "[]") or []
        if not any(note in c.get("text", "") for c in comments):
            bd(repo.root, "comments", "add", args.need_id, note)
        done = f"marked need {args.need_id}, already answered, as setting no rule: {why}"
        failed = f"the reason is on {args.need_id} as a comment; {retry} (it does not add the note twice)"
    else:
        bd(repo.root, "human", "respond", args.need_id, f"--response={answer}\n\n{note}")
        done = f"closed need {args.need_id} with the owner's answer and no decision record: {why}"
        failed = (f"need {args.need_id} is closed with the answer but not labelled {NO_DECISION}, so the render "
                  f"check flags it; {retry}")
    try:
        bd(repo.root, "update", args.need_id, f"--add-label={NO_DECISION}")
    except RecordError as e:
        raise Refuse(f"{e}; {failed}")
    return f"{done}; undo with: bd update {args.need_id} --remove-label={NO_DECISION}"


def cmd_action_done(args, records: Path) -> str:
    reason = (args.reason or "").strip()
    if not reason:
        raise Refuse("--reason is empty; say what showed you the owner did it")
    repo = load(records)
    need = human_issue(repo, args.need_id, "action")
    if need["status"] == "closed":
        raise Refuse(f"action {args.need_id} is already closed")
    refuse_unread(need)
    beads = {**repo.beads, args.need_id: {**need, "status": "closed", "close_reason": reason}}
    check_planned(repo, {}, beads)
    bd(repo.root, "close", args.need_id, f"--reason={reason}")
    return f"closed action {args.need_id}: {reason}"


def open_sprint(repo: Repo, sprint_id: str) -> Record:
    """The record of an open sprint epic; loading the records already checked it sits inside a project record."""
    rec = repo.sprint(sprint_id)
    epic = repo.beads.get(sprint_id)
    if epic is None or epic["issue_type"] != "epic":
        raise Refuse(f"sprint {sprint_id} is not a Beads epic")
    if epic["status"] == "closed":
        raise Refuse(f"sprint {sprint_id} is closed; use an open sprint")
    return rec


def cmd_task_add(args, records: Path) -> str:
    title = args.title.strip()
    if not title:
        raise Refuse("--title is empty")
    desc = args.stdin
    repo = load(records)
    open_sprint(repo, args.sprint)
    planned_id = "pm-planned-task"
    beads = {**repo.beads, planned_id: {"id": planned_id, "title": title, "status": "open", "issue_type": "task",
                                        "parent": args.sprint,
                                        "created_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}}
    check_planned(repo, {}, beads)
    out = bd(repo.root, "create", "--type=task", f"--parent={args.sprint}", f"--title={title}",
             *([f"--description={desc}"] if desc else []), "--json")
    task_id, _ = created(out)
    return f"created task {task_id} in sprint {args.sprint}; claim it with pm task claim {task_id}"


def open_task(repo: Repo, task_id: str) -> dict:
    """An open, non-epic task."""
    task = repo.beads.get(task_id)
    if task is None:
        raise Refuse(f"{task_id} is not a Beads issue")
    if task["issue_type"] == "epic":
        raise Refuse(f"{task_id} is an epic; close a sprint with pm sprint close, a project with pm project close")
    if task["status"] == "closed":
        raise Refuse(f"task {task_id} is already closed")
    return task


def cmd_task_close(args, records: Path) -> str:
    repo = load(records)
    task = open_task(repo, args.task_id)
    if HUMAN in (task.get("labels") or []):
        raise Refuse(f"{args.task_id} is labelled human, so it is a need; answer a decision with pm decision add "
                     f"--need {args.task_id} (or pm decision close if the answer sets no rule), close an action with "
                     f"pm action done {args.task_id}")
    reason = (args.reason or "").strip() or "Done"
    if args.commit:
        res = subprocess.run(["git", "rev-parse", "--verify", "--quiet", "--short", f"{args.commit}^{{commit}}"],
                             cwd=repo.root, capture_output=True, text=True)
        if res.returncode != 0:
            raise Refuse(f"--commit {args.commit} is not a commit")
        commit = res.stdout.strip()
    else:
        # HEAD names the work only if it was committed after the task started.
        started = datetime.fromisoformat((task.get("started_at") or task["created_at"]).replace("Z", "+00:00"))
        res = subprocess.run(["git", "log", "-1", "--format=%h %cI"], cwd=repo.root, capture_output=True, text=True)
        head, _, when = res.stdout.strip().partition(" ")
        commit = head if res.returncode == 0 and head and datetime.fromisoformat(when) >= started else None
    if commit is None:
        print(f"warning: no commit since {args.task_id} started, so the reason names none; "
              "name one with --commit if the work is committed", file=sys.stderr)
    else:
        reason += f" (commit {commit})"
    if git(repo.root, "status", "--porcelain"):
        print(f"warning: the working tree has uncommitted changes; the commit may not contain "
              f"the work of {args.task_id}", file=sys.stderr)
    beads = {**repo.beads, args.task_id: {**task, "status": "closed", "close_reason": reason}}
    check_planned(repo, {}, beads)
    bd(repo.root, "close", args.task_id, f"--reason={reason}")
    return f"closed {args.task_id}: {reason}"


def cmd_task_claim(args, records: Path) -> str:
    sid = (args.session or "").strip() or current_session()
    if not sid:
        raise Refuse(f"no agent session: neither {SESSION_ENV} nor {CODEX_SESSION_ENV} is set; name one with --session")
    repo = load(records)
    task = open_task(repo, args.task_id)
    h = holder(task)
    if h and h["session"] and h["session"] != sid and h["live"]:
        raise Refuse(f"{args.task_id} is held by live session {h['session']} (claimed {age(h['claimed_at'])} ago; its "
                     f"transcript was written in the last {LIVE_WINDOW // 60} minutes); leave it, or ask that session")
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    bd(repo.root, "update", args.task_id, "--claim", f"--set-metadata=claimed_by={sid}", f"--set-metadata=claimed_at={now}")
    was = f"; took it over from idle session {h['session']}" if h and h["session"] and h["session"] != sid else ""
    return f"claimed {args.task_id} for session {sid} at {now}{was}"


def cmd_task_move(args, records: Path) -> str:
    reason = args.stdin
    repo = load(records)
    task = open_task(repo, args.task_id)
    source = task.get("parent")
    if source is None or not any(r.type == "sprint" and r.meta["bead"] == source for r in repo.recs):
        raise Refuse(f"{args.task_id} is not in a sprint with a record; set its sprint with bd update --parent")
    rec = repo.sprint(source)
    if args.to == source:
        raise Refuse(f"{args.task_id} is already in sprint {source}")
    open_sprint(repo, args.to)
    decision_body(reason)
    body = f"Moved {args.task_id} to {args.to}: {reason}"
    new = insert_entry(rec.text, "Decisions", decision_block("agent", body, None))
    beads = {**repo.beads, args.task_id: {**task, "parent": args.to}}
    check_planned(repo, {rec.path: new}, beads)
    bd(repo.root, "update", args.task_id, f"--parent={args.to}")
    return apply_writes(repo, {rec.path: new}, f"moved {args.task_id} from {source} to {args.to} and added a "
                        f"sprint decision to {rel(repo, rec.path)}", undo=f"bd update {args.task_id} --parent={source}")


def cmd_doc_new(args, records: Path) -> str:
    if not re.fullmatch(r"[a-z0-9]+(-[a-z0-9]+)*", args.slug):
        raise Refuse(f"doc slug {args.slug!r} must be lowercase words joined by '-'")
    title = args.title.strip()
    if not title:
        raise Refuse("--title is empty")
    body = args.stdin
    if not body:
        raise Refuse("the doc body is empty; pipe it on stdin")
    repo = load(records)
    if args.bead is not None:
        if args.bead not in repo.beads:
            raise Refuse(f"--bead {args.bead} is not a Beads issue")
        epics = {r.meta["bead"] for r in repo.recs if r.type == "project"}
        if not epics & {args.bead, *ancestors(repo.beads, args.bead)}:
            raise Refuse(f"--bead {args.bead} is not inside a project with a record")
        target = f"bead: {args.bead}"
    else:
        target = f"project: {repo.project(args.project).name}"
    day = date.today().isoformat()
    path = records / "docs" / f"{day}-{args.slug}.md"
    if path.exists():
        raise Refuse(f"{rel(repo, path)} already exists; edit it by hand or choose another slug")
    text = f"---\ntype: doc\ntitle: {yaml_str(title)}\ndate: {day}\n{target}\n---\n\n{body}\n"
    check_planned(repo, {path: text})
    return apply_writes(repo, {path: text}, f"created {rel(repo, path)}")


FEEDBACK_ENTRY = re.compile(r"(?m)^### \d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC, session `")


def feedback_docs(recs: list[Record], project: str) -> list[Record]:
    """The project's pm feedback docs, docs/<date>-<project>-feedback.md naming the project; more than one is an error
    for pm feedback add to refuse."""
    name = re.compile(rf"\d{{4}}-\d{{2}}-\d{{2}}-{re.escape(project)}-feedback\.md")
    return [r for r in recs if r.type == "doc" and r.meta.get("project") == project and name.fullmatch(r.path.name)]


def cmd_feedback_add(args, records: Path) -> str:
    """Append one entry to the project's pm feedback doc, docs/<date of first use>-<project>-feedback.md, creating it
    on first use."""
    text = (args.text if args.text is not None else args.stdin).strip()
    if not text:
        raise Refuse("the feedback text is empty; pass --text or pipe it on stdin")
    sid = (args.session or "").strip() or current_session()
    if not sid:
        raise Refuse(f"no agent session: neither {SESSION_ENV} nor {CODEX_SESSION_ENV} is set; name one with --session")
    repo = load(records)
    project = repo.project(args.project).name
    for flag, issue in (("--sprint", args.sprint), ("--task", args.task)):
        if issue is not None and issue not in repo.beads:
            raise Refuse(f"{flag} {issue} is not a Beads issue")
    docs = feedback_docs(repo.recs, project)
    if len(docs) > 1:
        raise Refuse(f"project {project} has {len(docs)} feedback docs ({', '.join(rel(repo, r.path) for r in docs)}); "
                     "merge them into one by hand and commit with pm commit")
    now = datetime.now(timezone.utc)
    about = ", ".join(f"{k} `{v}`" for k, v in (("sprint", args.sprint), ("task", args.task)) if v is not None)
    entry = (f"### {now:%Y-%m-%d %H:%M} UTC, session `{sid}`\n\n" + (f"About {about}.\n\n" if about else "")
             + f"{text}\n")
    if docs:
        path, old = docs[0].path, docs[0].text
    else:
        day = date.today().isoformat()
        path = records / "docs" / f"{day}-{project}-feedback.md"
        if path.exists():
            raise Refuse(f"{rel(repo, path)} already exists but does not name project {project}; fix its header by hand")
        old = (f"---\ntype: doc\ntitle: pm feedback\ndate: {day}\nproject: {project}\n---\n\n"
               "Where pm got in the way, one entry per `pm feedback add`, newest last.\n")
    new = old.rstrip("\n") + "\n\n" + entry
    check_planned(repo, {path: new})
    return apply_writes(repo, {path: new}, f"added feedback to {rel(repo, path)}")


def cmd_design_new(args, records: Path) -> str:
    if not re.fullmatch(r"[a-z0-9]+(-[a-z0-9]+)*", args.slug):
        raise Refuse(f"design slug {args.slug!r} must be lowercase words joined by '-'")
    title = args.title.strip()
    if not title:
        raise Refuse("--title is empty")
    repo = load(records)
    project = repo.project(args.project).name
    path = records / "design" / f"{args.slug}.md"
    if path.exists():
        raise Refuse(f"{rel(repo, path)} already exists; edit it by hand or choose another slug")
    text = design_text(yaml_str(title), project)
    check_planned(repo, {path: text})
    return apply_writes(repo, {path: text}, f"created {rel(repo, path)}; fill in its sections by hand in the "
                        f"store, then pm commit -m \"…\" {rel(repo, path)}")


def cmd_postmortem_new(args, records: Path) -> str:
    if not re.fullmatch(r"[a-z0-9]+(-[a-z0-9]+)*", args.slug):
        raise Refuse(f"postmortem slug {args.slug!r} must be lowercase words joined by '-'")
    title = args.title.strip()
    if not title:
        raise Refuse("--title is empty")
    repo = load(records)
    target = (f"sprint: {repo.sprint(args.sprint).meta['bead']}" if args.sprint is not None
              else f"project: {repo.project(args.project).name}")
    day = date.today().isoformat()
    path = records / "postmortems" / f"{day}-{args.slug}.md"
    if path.exists():
        raise Refuse(f"{rel(repo, path)} already exists; edit it by hand or choose another slug")
    text = postmortem_text(yaml_str(title), day, target)
    check_planned(repo, {path: text})
    return apply_writes(repo, {path: text}, f"created {rel(repo, path)}; fill in its sections by hand in the "
                        f"store, then pm commit -m \"…\" {rel(repo, path)}")


def cmd_project_open(args, records: Path) -> str:
    if not re.fullmatch(r"[a-z0-9]+(-[a-z0-9]+)*", args.name):
        raise Refuse(f"project name {args.name!r} must be lowercase words joined by '-'")
    title = args.title.strip()
    if not title:
        raise Refuse("--title is empty")
    goal = args.stdin
    if not goal:
        raise Refuse("Goal is empty; pipe the project's goal on stdin")
    repo = load(records)
    path = records / "projects" / f"{args.name}.md"
    if path.exists() or any(r.type == "project" and r.name == args.name for r in repo.recs):
        raise Refuse(f"project {args.name!r} already exists ({rel(repo, path)})")
    planned_id = "pm-planned-project"
    beads = dict(repo.beads)
    beads[planned_id] = {"id": planned_id, "title": title, "status": "open", "issue_type": "epic",
                         "created_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}
    check_planned(repo, {path: project_text(yaml_str(title), planned_id, goal)}, beads)

    out = bd(repo.root, "create", "--type=epic", f"--title={title}", "--json")
    epic, undo = created(out)
    text = project_text(yaml_str(title), epic, goal)
    try:
        check_planned(repo, {path: text}, {**repo.beads, epic: show_beads(repo.root, [epic])[epic]})
    except (Refuse, RecordError) as e:
        raise Refuse(f"{e}; undo the Beads step with: {undo}")
    return apply_writes(repo, {path: text}, f"opened project {args.name}: epic {epic}, record {rel(repo, path)}", undo)


def cmd_sprint_open(args, records: Path) -> str:
    title = args.title.strip()
    if not title:
        raise Refuse("--title is empty")
    frame = parse_sections(args.stdin, FRAME)
    for name in FRAME:
        if not frame.get(name):
            raise Refuse(f"'## {name}' is missing or empty on stdin; {FRAME_SHAPE}")
    scope = re.match(r"(?s)\*\*In:\*\*(.*?)\*\*Out:\*\*(.*)", frame["Scope"])
    if not scope or not scope.group(1).strip() or not scope.group(2).strip():
        raise Refuse(f"Scope needs a non-empty **In:** list followed by a non-empty **Out:** list; {FRAME_SHAPE}")
    repo = load(records)
    proj = repo.project(args.project)
    epic_id = proj.meta["bead"]
    if repo.beads[epic_id]["status"] == "closed":
        raise Refuse(f"project {args.project} ({epic_id}) is closed; open a sprint in an open project")

    kids = [k for k in children(repo.beads, epic_id) if k["issue_type"] == "epic"]
    taken = [int(m.group(1)) for r in repo.recs if r.type == "sprint"
             for m in [re.fullmatch(rf"{re.escape(args.project)}-(\d+)", r.name)] if m]
    suffixes = [int(k["id"].rsplit(".", 1)[1]) for k in children(repo.beads, epic_id) if k["id"].rsplit(".", 1)[1].isdigit()]
    n = max(taken + [len(kids)] + [0]) + 1
    path = records / "sprints" / f"{args.project}-{n}.md"
    if path.exists():
        raise Refuse(f"{rel(repo, path)} already exists")
    planned_id = f"{epic_id}.{max(suffixes + [0]) + 1}"
    beads = dict(repo.beads)
    beads[planned_id] = {"id": planned_id, "title": f"Sprint {n}: {title}", "status": "open",
                         "issue_type": "epic", "parent": epic_id,
                         "created_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}
    check_planned(repo, {path: sprint_text(yaml_str(title), planned_id, frame)}, beads)

    out = bd(repo.root, "create", "--type=epic", f"--parent={epic_id}", f"--title=Sprint {n}: {title}", "--json")
    sprint_id, undo = created(out)
    text = sprint_text(yaml_str(title), sprint_id, frame)
    try:
        check_planned(repo, {path: text}, {**repo.beads, sprint_id: show_beads(repo.root, [sprint_id])[sprint_id]})
    except (Refuse, RecordError) as e:
        raise Refuse(f"{e}; undo the Beads step with: {undo}")
    return apply_writes(repo, {path: text},
                        f"opened sprint {n} of {args.project}: epic {sprint_id}, record {rel(repo, path)}", undo)


def require_committed(repo: Repo, record: Path) -> str:
    """The records commit a close names; the closed sprint's or project's own record must be committed. Other
    uncommitted files in the store, such as another session's edit in progress, do not block the close."""
    dirty = uncommitted(repo.records, [record])
    if dirty:
        raise Refuse(f"{', '.join(dirty)} has uncommitted changes; commit it first with pm commit -m \"…\" <path> so "
                     "the close names the exact state it delivered")
    return git(repo.records, "rev-parse", "--short", "HEAD")


MERGED_AS = re.compile(r"merged as ([0-9a-f]{7,40})\b")  # the close reason of a PR review once its PR is on main


def merge_stamp(text: str, stamp: str) -> str:
    """A sprint record's text with `stamp` as a paragraph after the Outcome's verdict paragraph, which stays first
    (it is the epic's close reason)."""
    start, end = section_range(text, "Delivery report")
    lines = text.split("\n")
    n = next(i for i in range(start, end) if lines[i] == "### Outcome") + 1
    while n < end and (not lines[n].strip() or lines[n].startswith(">")):
        n += 1
    while n < end and lines[n].strip() and not lines[n].startswith("#"):
        n += 1
    lines[n:n] = ["", stamp] + ([""] if n < end and lines[n].startswith("#") else [])
    return "\n".join(lines)


def cmd_sprint_close(args, records: Path) -> str:
    """Close a sprint epic once its report is written and every task is closed. Each PR review naming the sprint
    must be closed as 'merged as <sha>' (a dismissed one is skipped); the merges are stamped into the Outcome after its verdict, committed on the
    records branch, and the epic's close reason names that commit. A sprint with no review closes with no stamp."""
    repo = load(records)
    rec = repo.sprint(args.sprint_id)
    epic = repo.beads[args.sprint_id]
    if epic["status"] == "closed":
        raise Refuse(f"sprint {args.sprint_id} is already closed")
    head = require_committed(repo, rec.path)
    line = outcome(rec)
    if not line:
        raise Refuse(f"{rel(repo, rec.path)}: the committed Delivery report Outcome is still "
                     f"'{NOT_CLOSED}'; write it (done, partial or voided, plus one sentence, then optional bullets) and commit")
    if report_part(rec, 'Against "Done when"') in ("", NOT_CLOSED):
        raise Refuse(f"{rel(repo, rec.path)}: the committed Delivery report 'Against \"Done when\"' is still "
                     f"'{NOT_CLOSED}'; write each item with its evidence and commit")
    open_kids = [k for k in children(repo.beads, args.sprint_id) if k["status"] != "closed"]
    reviews = sprint_reviews(repo.beads, args.sprint_id)
    open_kids += [i for i, _ in reviews if i["status"] != "closed" and i not in open_kids]
    if open_kids:
        listing = ", ".join(f"{k['id']} ({k['status']})" for k in open_kids)
        raise Refuse(f"open tasks in the sprint: {listing}; close each with pm task close or move it "
                     "with pm task move (a PR review closes with pm action done <id> --reason \"merged as <sha>\" once "
                     "the PR is on main)")
    merges = []
    for i, review in reviews:
        if dismissed(i):  # a replaced PR's review, or a [TEST] one: it delivers nothing
            continue
        m = MERGED_AS.match(i.get("close_reason") or "")
        if not m:
            raise Refuse(f"PR review {i['id']} ({review['pr']}) is closed as {i.get('close_reason') or 'no reason'!r}, "
                         "not 'merged as <sha>'; a sprint closes only once each PR delivering it is on main, so the "
                         "close stamps its merge commit into the Outcome")
        pr = review["pr"]
        merges.append((m.group(1)[:7], pr_label(pr) if "/pull/" in pr else pr))
    text = rec.path.open(newline="").read()
    stamp = " ".join(f"Merged as {sha} ({pr})." for sha, pr in merges)
    writes = {rec.path: merge_stamp(text, stamp)} if merges and stamp not in text else {}
    proj = project_of(rec, repo.recs, repo.beads)
    n = re.fullmatch(rf"{re.escape(proj.name)}-(\d+)", rec.name)
    message = (f"[SPRINT] {proj.name} sprint {n.group(1) if n else rec.name}: closed, merged as "
               f"{', '.join(sha for sha, _ in merges)}")
    planned = {**epic, "status": "closed", "close_reason": f"{line} (records commit {head})"}
    check_planned(repo, writes, {**repo.beads, args.sprint_id: planned})
    if writes:
        apply_writes(repo, writes, message, prefix="")
        head = git(repo.records, "rev-parse", "--short", "HEAD")
    reason = f"{line} (records commit {head})"
    try:
        bd(repo.root, "close", args.sprint_id, f"--reason={reason}")
    except RecordError as e:
        raise Refuse(f"{e}; the stamp is committed as {head}, so run pm sprint close {args.sprint_id} again"
                     if writes else str(e))
    return f"closed {args.sprint_id}: {reason}" + (f"; stamped the Outcome: {stamp}" if writes else "")


def cmd_project_close(args, records: Path) -> str:
    repo = load(records)
    proj = repo.project(args.name)
    epic_id = proj.meta["bead"]
    if repo.beads[epic_id]["status"] == "closed":
        raise Refuse(f"project {args.name} ({epic_id}) is already closed")
    head = require_committed(repo, proj.path)
    text = section_text(proj.body, "Outcome")
    if not text or text == NOT_CLOSED:
        raise Refuse(f"{rel(repo, proj.path)}: the committed ## Outcome is still '{NOT_CLOSED}'; "
                     "write it and commit")
    open_sprints = [k for k in children(repo.beads, epic_id) if k["status"] != "closed"]
    if open_sprints:
        listing = ", ".join(f"{k['id']} ({k['title']})" for k in open_sprints)
        raise Refuse(f"open sprints in the project: {listing}; close them with pm sprint close first")
    reason = f"{first_sentence(text, 200)} (commit {head})"
    check_planned(repo, {}, {**repo.beads, epic_id: {**repo.beads[epic_id], "status": "closed", "close_reason": reason}})
    bd(repo.root, "close", epic_id, f"--reason={reason}")
    return f"closed {epic_id}: {reason}"


def check_records(recs: list[Record], beads: dict[str, dict], name: str, summaries: dict | None = None) -> int:
    """The check of pm check and pm commit: every page of the site renders from `recs` and `beads`, and every answered
    decision need is cited or marked no-decision; it writes nothing. The number of pages checked."""
    return len(render_pages(recs, beads, name, summaries=summaries))


def cmd_check(args, records: Path) -> str:
    repo = load(records, working=True)
    n = check_records(repo.recs, repo.beads, repo.root.name, read_summaries(records))
    return f"checked the records in {records}: all {n} pages render"



def cfg() -> config.Config:
    """The repo's .pm/config.toml; main() has checked it before any command runs."""
    return config.load(Path.cwd())


def port() -> int:
    """The site port: the PORT environment variable for one run, else the installed service's port, else config's
    `port`."""
    return service.port_for(service_main(), cfg().port)


def public_url() -> str:
    """The site's public base URL, the repo's `site_url` (a tunnel to the pm service), or ""."""
    return cfg().site_url


def site_url() -> str:
    """The base of links printed for the owner; anything that connects to the pm service uses 127.0.0.1:$PORT instead."""
    return public_url() or f"http://localhost:{port()}"


def reply_hosts() -> tuple[str, ...]:
    """The pm service binds 127.0.0.1; any other Host is a rebinding domain, except the configured public URL's host."""
    public = urllib.parse.urlsplit(public_url()).hostname
    return ("127.0.0.1", "localhost") + ((public,) if public else ())


def check_reply(form: dict[str, list[str]], token: str, host: str | None,
                beads: dict[str, dict]) -> tuple[int, str, str]:
    """Check the owner's reply to an open request from the site's form against `beads`, the server's last read, so
    the check reads nothing. Returns the HTTP status, then on success (303) the issue id and the reply's text, else
    the reason it was refused and ""."""
    if (host or "").rsplit(":", 1)[0] not in reply_hosts():
        return 403, (f"refused: Host {host!r} is neither this machine nor the configured site_url; "
                     f"open the site at {site_url()}"), ""
    if not hmac.compare_digest(form.get("token", [""])[0].encode(), token.encode()):
        return 403, ("refused: the reply carries no valid token; reload the page (the server may have restarted) "
                     "and send it again"), ""
    issue_id = form.get("id", [""])[0]
    text = form.get("text", [""])[0].replace("\r\n", "\n").strip()
    issue = beads.get(issue_id)
    if issue is None or HUMAN not in (issue.get("labels") or []):
        return 400, f"refused: {issue_id!r} is not a request waiting on the owner", ""
    if issue["status"] == "closed":
        return 400, f"refused: {issue_id} is already closed ({issue.get('close_reason') or 'no reason'})", ""
    if not text:
        return 400, "refused: the reply is empty", ""
    return 303, issue_id, text


def store_reply(root: Path, issue_id: str, text: str) -> None:
    """Store a checked reply: one Beads comment by REPLY_AUTHOR, which waits for the agent while the request has more
    site replies than its picked_up count (reply_waiting); the request stays open for the agent to act on."""
    with tempfile.NamedTemporaryFile("w", suffix=".txt") as f:  # a file, so a reply starting with '-' is no flag
        f.write(text)
        f.flush()
        bd(root, "comments", "add", issue_id, f"--file={f.name}", f"--author={REPLY_AUTHOR}")


# The reply spool: the pm service appends each checked reply here, fsynced, before it answers the POST, and a "done" line
# once the reply is in Beads, so a reply taken survives a crash or kill and a restart delivers it. It lives in the
# clone's git dir, next to the store and never committed; it is emptied whenever no reply in it is pending.
REPLY_SPOOL = "pm-replies.jsonl"


def spool_path(records: Path) -> Path:
    return Path(store_git(records, "rev-parse", "--path-format=absolute", "--git-common-dir")) / REPLY_SPOOL


@contextlib.contextmanager
def flocked(path: Path):
    """An exclusive flock on `path`, created if missing: the spool for its own edits, its .lock for a delivery."""
    fd = os.open(path, os.O_RDWR | os.O_CREAT, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)
        yield
    finally:
        os.close(fd)


def spool_read(spool: Path) -> tuple[dict[str, dict], set[str]]:
    """The spool's replies by reply id, and the ids of those already in Beads."""
    entries, done = {}, set()
    for n, line in enumerate(spool.read_text().splitlines() if spool.exists() else [], 1):
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            raise RecordError(f"{spool}:{n} is not a JSON line: {line!r}; fix or remove that line")
        if "done" in e:
            done.add(e["done"])
        else:
            entries[e["rid"]] = e
    return entries, done


def spool_add(spool: Path, line: dict, drain: bool = False) -> bool:
    """Append `line` to the spool and fsync it, unless it is a reply whose id the spool already holds (a resubmit);
    with `drain`, empty the spool once none of its replies is pending. True when the line was added."""
    with flocked(spool):
        entries, done = spool_read(spool)
        if "rid" in line and line["rid"] in entries:
            return False
        with spool.open("a") as f:
            f.write(json.dumps(line) + "\n")
            f.flush()
            os.fsync(f.fileno())
        if drain and set(entries) <= done | {line.get("done")}:
            os.truncate(spool, 0)
        return True


def deliver_reply(root: Path, spool: Path, entry: dict) -> None:
    """Store a spooled reply in Beads effectively once, then mark it done: under the spool's delivery lock (so two
    servers never race on it), skip it if it is marked done, and add its comment, ending in its REPLY_MARK, only if
    no comment on the request carries that mark yet (a crash between the write and the mark)."""
    with flocked(spool.with_suffix(".lock")):
        if entry["rid"] in spool_read(spool)[1]:
            return
        mark = REPLY_MARK.format(entry["rid"])
        comments = json.loads(bd(root, "comments", entry["id"], "--json") or "[]") or []
        if not any(mark in (c.get("text") or "") for c in comments):
            store_reply(root, entry["id"], f"{entry['text']}\n\n{mark}")
        spool_add(spool, {"done": entry["rid"]}, drain=True)


SERVE_CHECK = 1.0  # seconds between the pm service's looks for a change in the records, the store's HEAD or Beads


@dataclass(frozen=True)
class Snapshot:
    """What the pm service serves: the records and Beads as read at `as_of` (epoch seconds) or later, so they are current
    as of then; `error` when they do not render. `pages` caches each page rendered from them, with its digest."""
    as_of: float
    texts: dict
    stamp: tuple
    dolt: tuple | None
    recs: list | None = None
    beads: dict | None = None
    dates: dict | None = None
    error: str | None = None
    pages: dict = field(default_factory=dict)
    summaries: dict | None = None  # generated day summaries (read_summaries)


def pin_moved(path: Path) -> str:
    """Why the pm service must stop, its config at `path` no longer pinning the version it runs; "" while it does."""
    try:
        pin = tomllib.loads(path.read_text()).get("version")
    except (OSError, tomllib.TOMLDecodeError) as e:
        return f"the pm service cannot read its pin in {path} ({e}); stopping"
    if pin != __version__:
        return (f"{path} now pins pm {pin}, but this service runs pm {__version__}; stopping, so the supervisor "
                f"starts it again and the pm uv tool runs pm {pin}")
    return ""


def cmd_serve(args, records: Path) -> str:
    """`pm service run`, the pm service's process: serve the site on localhost:$PORT and push (cmd_push) every
    pushjob.INTERVAL seconds, the first that long after start. A request never waits on a writer: it renders from the last snapshot of the
    records and Beads, which a background thread keeps current, and every page states the time its data is current
    as of, refreshes itself when a newer snapshot changes it, and says when it is more than SERVE_BEHIND seconds
    behind. The thread looks every SERVE_CHECK seconds (sooner after a request or a reply) at the record files, the
    store's HEAD and today's date, and at the Beads database's fingerprint (dolt_state); when one moved it reads them
    again without the store lock (taking it only to confirm a state that fails to render), rereading Beads with bd
    only when the fingerprint moved (every time when bd uses a Dolt server) and the design pages' dates with git only when the records, HEAD or day changed. A state that
    fails to render is served as the error, never as an old page. A reply is checked against the snapshot, appended
    to the reply spool before the POST is answered, and stored in Beads by a second background thread, which retries
    a failed write with backoff; replies left pending by a crash or kill are stored at the next start. Once stored,
    the thread pushes it into the inbox of the session that raised the request (push_undelivered) and marks it
    delivered only if that worked. A third thread, at start and every MERGE_POLL seconds, looks at open reviews' PRs
    and hands a merge to main to the second, which stores and pushes it the same way, then has the second sweep the
    open requests for anything their running session has not received and push it again. A card shows its reply saving, then saved
    and whether it was delivered, or the error, then among its replies from Beads. Each request,
    refresh and reply write logs one line to stderr: what, total ms, ms waiting for the store lock, in git, rendering,
    and in each bd call. Every look also reads the config's pin again: once it pins another version (a pull after pm
    upgrade), the service stops with an error, and the supervisor starts the pm uv tool again, which runs the new pin."""
    import html
    import http.server
    import queue
    import threading
    import traceback
    import urllib.parse
    import uuid

    root = code_root(Path.cwd(), records)
    pin_file = cfg().path  # read again every look: a pull that moves the pin stops this service
    build = tool.running()  # sent with every reply, so a probe sees a service left on another build
    stopped: list[str] = []  # why the refresher stopped the server
    noms = dolt_store(root)
    token = secrets.token_urlsafe(32)  # embedded in the served pages' reply forms; a POST without it is refused
    heads = head_files(records)
    now = {"snap": None}  # the snapshot requests read; only the refresh thread replaces it
    replies: dict[str, dict] = {}  # replies on their way to Beads by issue id: state saving, sent or failed
    spool = spool_path(records)
    writes: queue.Queue = queue.Queue()  # replies to store, and merges to store, both then pushed to the session
    merges: set[str] = set()  # reviews whose merge is queued or stored, so the watcher queues each once
    swept: dict[str, tuple] = {}  # by request: the sweep's last outcome, and the inbox it found not running
    gh = shutil.which("gh") is not None
    wake = threading.Event()
    timing = threading.local()  # this thread's (what, ms) so far, logged as one line when its work ends

    def spent(what: str, since: float) -> None:
        timing.ms.append((what, round((time.monotonic() - since) * 1000)))

    def log(head: str, since: float) -> None:
        ms = {k: sum(v for w, v in timing.ms if w == k) for k in ("lock", "git", "render")}
        calls = ",".join(f"{w[3:]}:{v}ms" for w, v in timing.ms if w.startswith("bd ")) or "-"
        print(f"{head} total={round((time.monotonic() - since) * 1000)}ms "
              + " ".join(f"{k}={v}ms" for k, v in ms.items()) + f" bd={calls}", file=sys.stderr, flush=True)

    def read(old: Snapshot | None) -> Snapshot:
        """The records and Beads as of now, reusing what did not move since `old`; `error` set when they do not
        render."""
        as_of, texts, stamp = time.time(), record_texts(records), (read_files(heads), date.today())
        dolt = dolt_state(noms) if noms else None  # taken before bd reads, so a write in between rereads
        keep = old if old and old.error is None else None
        try:
            recs = keep.recs if keep and texts == keep.texts else parse_records(records, texts)
            t = time.monotonic()
            dates = (keep.dates if keep and (texts, stamp) == (keep.texts, keep.stamp)
                     else design_dates(records, recs))
            spent("git", t)
            t = time.monotonic()
            beads = keep.beads if keep and dolt is not None and dolt == keep.dolt else load_beads(root)
            if not keep or beads is not keep.beads:
                spent("bd list", t)
            summaries = read_summaries(records)
            if (keep and texts == keep.texts and beads == keep.beads and dates == keep.dates
                    and summaries == keep.summaries):  # a noisy move
                return dataclasses.replace(keep, as_of=as_of, stamp=stamp, dolt=dolt)
            check_needs_answered(recs, beads)
            return Snapshot(as_of, texts, stamp, dolt, recs, beads, dates, summaries=summaries)
        except Exception as e:
            return Snapshot(as_of, texts, stamp, dolt,
                            error=f"error: {e}" if isinstance(e, RecordError) else traceback.format_exc())

    def refresh(old: Snapshot | None) -> Snapshot:
        """`old` with a newer as_of when nothing moved since it was read, else a new read. The read takes no lock, so
        a writer never delays it; pm writes each file atomically, but between a pm write's steps the records and
        Beads may not render together (a need closed before its decision is written), so a read that fails is
        repeated under the store's shared lock, which a pm write holds exclusively across its steps: an error
        shown is a real one."""
        as_of, texts, stamp = time.time(), record_texts(records), (read_files(heads), date.today())
        dolt = dolt_state(noms) if noms else None
        if old and dolt is not None and (texts, stamp, dolt) == (old.texts, old.stamp, old.dolt):
            return dataclasses.replace(old, as_of=as_of)
        timing.ms, start = [], time.monotonic()
        snap = read(old)
        if snap.error:
            fd = os.open(records, os.O_RDONLY)
            try:
                t = time.monotonic()
                fcntl.flock(fd, fcntl.LOCK_SH)
                spent("lock", t)
                snap = read(old)
            finally:
                os.close(fd)
        log("refresh", start)
        return snap

    def refresher() -> None:
        while True:
            wake.wait(SERVE_CHECK)
            wake.clear()
            moved = pin_moved(pin_file)
            if moved:  # the supervisor starts the service again, which then runs the pm uv tool's version or fails
                stopped.append(moved)
                print(f"error: {moved}", file=sys.stderr, flush=True)
                server.shutdown()
                return
            try:
                now["snap"] = refresh(now["snap"])
            except Exception:  # a file vanished mid-look (a git or Dolt rewrite): keep the snapshot, look again
                traceback.print_exc()

    def writer() -> None:
        """Deliver the spooled replies to Beads one at a time, so a reply's POST never waits for bd; a failed one
        stays in the spool and is tried again after a backoff of 1, 2, 4 … up to 60 seconds."""
        while True:
            entry, tries = writes.get()
            issue_id = entry["id"]
            timing.ms, start = [], time.monotonic()
            if "sweep" in entry:
                sweep()
                continue
            if "merge" in entry:  # a merge the watcher saw: stored, then pushed like a reply
                try:
                    bd(root, "update", issue_id, f"--set-metadata={MERGED}={entry['merge']}")
                    log(f"merge {issue_id} {entry['merge']} {push_undelivered(root, issue_id)}", start)
                except Exception:
                    traceback.print_exc()
                    merges.discard(issue_id)  # the next look tries it again
                wake.set()
                continue
            try:
                deliver_reply(root, spool, entry)
                replies[issue_id] = {"state": "sent", "rid": entry["rid"], "text": entry["text"]}
            except Exception as e:
                replies[issue_id] = {"state": "failed", "text": entry["text"], "rid": entry["rid"],
                                     "error": str(e) if isinstance(e, RecordError) else repr(e)}
                retry = threading.Timer(min(60, 2 ** tries), writes.put, ((entry, tries + 1),))
                retry.daemon = True
                retry.start()
            spent("bd comments", start)
            delivery = ""
            if replies[issue_id]["state"] == "sent":
                try:
                    delivery = push_undelivered(root, issue_id)
                except Exception as e:  # the reply is stored; it stays undelivered, the sweep tries it again
                    delivery = f"failed: {e!r}"
                replies[issue_id]["delivery"] = delivery.split(":")[0]
            log(f"reply {issue_id} {replies[issue_id]['state']}", start)
            if delivery:
                print(f"push {issue_id}: {delivery}", file=sys.stderr, flush=True)
            wake.set()

    def sweep() -> None:
        """On the writer's thread: push again what an open request's session has not received (a push that failed,
        or a crash between storing a reply and pushing it). A session that is gone costs a stat per look: a request
        whose push found it not running is skipped until its inbox path appears or changes. Logs only a change."""
        snap = now["snap"]
        for i in list((snap.beads or {}).values()) if snap else []:
            meta = i.get("metadata") if isinstance(i.get("metadata"), dict) else {}
            path = meta.get("inbox")
            if i["status"] == "closed" or HUMAN not in (i.get("labels") or []) or not path:
                continue
            try:
                due = reply_waiting(i) or merge_waiting(i)
                st = os.stat(path)
            except (RecordError, OSError):
                continue  # a bad count (pm show reports it) or no socket: the session is not running
            key = (path, st.st_ino, st.st_mtime)
            if not due or swept.get(i["id"], ("", None))[1] == key:
                continue
            try:
                got = push_undelivered(root, i["id"])
            except Exception as e:
                got = f"failed: {e!r}"
            if got != swept.get(i["id"], ("", None))[0]:
                print(f"push {i['id']} (sweep): {got}", file=sys.stderr, flush=True)
            swept[i["id"]] = (got, key if got.startswith("not running") else None)
        wake.set()

    def ticker() -> None:
        """At start and every MERGE_POLL seconds: look at each open review's PR without a seen merge (with gh), and
        hand one merged to main to the writer, which stores it and pushes it into the review's session; then queue
        a sweep. A failing look is logged, and the next tick looks again."""
        while True:
            try:
                snap = now["snap"]
                for i in list((snap.beads or {}).values()) if snap and gh else []:
                    pr = review_pr(i)
                    meta = i.get("metadata") if isinstance(i.get("metadata"), dict) else {}
                    if pr and not meta.get(MERGED) and i["id"] not in merges:
                        if sha := merged_on_main(root, pr):
                            merges.add(i["id"])
                            writes.put(({"id": i["id"], "merge": sha}, 0))
            except Exception:
                traceback.print_exc()
            writes.put(({"id": "sweep", "sweep": True}, 0))
            time.sleep(MERGE_POLL)

    def pusher() -> None:
        """Every INTERVAL seconds: push Beads data, summarize today and push the records branch, logging each step's
        line; a run that raises is logged, and the next one tries again."""
        while True:
            time.sleep(pushjob.INTERVAL)
            try:
                print(cmd_push(args, None)[1], file=sys.stderr, flush=True)
            except Exception:
                traceback.print_exc()

    def page(path: str) -> tuple[int, str, Snapshot]:
        """The status and HTML of the page at `path` from the current snapshot, its reply slots filled and its status
        slot left for fill_status, and that snapshot."""
        snap = now["snap"]
        if snap.error:
            return 500, (f'<!doctype html><meta charset="utf-8"><title>Render failed</title>'
                         f'<link rel="stylesheet" href="/style.css"><main>{STATUS_SLOT}<h1>The site did not render</h1>'
                         f"<pre>{html.escape(snap.error)}</pre><p>Fix the cause; this page offers a reload once it renders."
                         f"</p></main>"), snap
        if path not in snap.pages:
            t = time.monotonic()
            try:
                snap.pages[path] = 200, render_page(path, snap.recs, snap.beads, root.name, snap.dates,
                                                       snap.summaries)
            except Exception as e:
                err = f"error: {e}" if isinstance(e, RecordError) else traceback.format_exc()
                snap.pages[path] = 500, (f'<!doctype html><meta charset="utf-8"><title>Render failed</title>'
                                         f'<link rel="stylesheet" href="/style.css"><main>{STATUS_SLOT}'
                                         f"<h1>The page did not render</h1><pre>{html.escape(err)}</pre>"
                                         f"<p>Fix the cause; this page offers a reload once it renders.</p></main>")
            spent("render", t)
        code, text = snap.pages[path]
        if text is None:  # a page that does not exist yet offers a reload once a newer snapshot has it
            return 404, (f'<!doctype html><meta charset="utf-8"><title>No such page</title>'
                         f'<link rel="stylesheet" href="/style.css"><main>{STATUS_SLOT}<h1>No page {html.escape(path)}'
                         f"</h1><p>This page offers a reload once the site has it.</p></main>"), snap
        # The push banner is read on every request, before the digest: the job changes it, not the records.
        if path == "index.html" or path.startswith("projects/"):
            text = text.replace("</nav>", "</nav>" + pushjob.banner(main_of(records), records, cfg().remote), 1)
        # A sent reply stays on its card until the snapshot's Beads data holds its comment, which the thread then shows:
        # a refresh that finds Dolt unchanged keeps an older read under a newer as_of, so the time alone is no proof.
        shown = {i: r for i, r in list(replies.items())
                 if not (r["state"] == "sent" and reply_in_beads(snap.beads, i, r["rid"]))}
        return code, fill_replies(text, token, shown), snap

    def digest(text: str | None) -> str:
        return hashlib.sha1((text or "").encode()).hexdigest()[:16]

    def reply(form: dict[str, list[str]], host: str | None) -> tuple[int, str]:
        snap = now["snap"]
        if snap.error:
            return 500, f"error: the reply was not taken, since the site does not render: {snap.error}"
        code, said, text = check_reply(form, token, host, snap.beads)
        if code != 303:
            return code, said
        rid = form.get("rid", [""])[0] or str(uuid.uuid4())  # a page without the form's script sends none
        if not REPLY_ID.fullmatch(rid):
            return 400, f"refused: {rid!r} is not a reply id"
        entry = {"rid": rid, "id": said, "text": text, "at": time.time()}
        if spool_add(spool, entry):  # a resubmit of a reply id the spool holds is already on its way
            replies[said] = {"state": "saving", "text": text}
            writes.put((entry, 0))
        return code, said

    class Handler(http.server.BaseHTTPRequestHandler):
        def handle_one_request(self):
            timing.ms, start, self.code = [], time.monotonic(), None
            super().handle_one_request()
            if self.code is not None:
                log(f"{self.command} {self.path} {self.code}", start)

        def log_request(self, code="-", size="-"):  # instead of the default access line: handle_one_request logs
            self.code = int(code)

        def path_of(self, url: str) -> str:
            path = urllib.parse.unquote(urllib.parse.urlsplit(url).path).lstrip("/")
            return path + "index.html" if path == "" or path.endswith("/") else path

        def do_GET(self):
            wake.set()  # a reload asks for a look at once; it does not wait for it
            url = urllib.parse.urlsplit(self.path)
            if url.path == "/style.css":
                return self.reply(200, "text/css; charset=utf-8", STYLE.read_bytes())
            if url.path == "/version":
                target = urllib.parse.parse_qs(url.query).get("page", ["/"])[0]
                _, text, snap = page(self.path_of(target))
                body = json.dumps({"asof": snap.as_of, "page": digest(text)})
                return self.reply(200, "application/json", body.encode())
            code, text, snap = page(self.path_of(self.path))
            text = fill_status(text, snap.as_of, digest(text), time.time())
            self.reply(code, "text/html; charset=utf-8", text.encode())

        def do_HEAD(self):  # GET's status and headers without the body, so curl -I and probes work
            self.do_GET()

        def do_POST(self):
            """A reply from a card's form: checked, spooled for Beads, and back to the card, which shows it saving."""
            if urllib.parse.urlsplit(self.path).path != "/reply":
                return self.reply(404, "text/plain; charset=utf-8", b"only /reply takes a POST\n")
            length = self.headers.get("Content-Length") or "0"
            if not length.isdigit():
                return self.reply(400, "text/plain; charset=utf-8", b"refused: bad Content-Length\n")
            raw = self.rfile.read(int(length)).decode("utf-8", "replace")
            code, said = reply(urllib.parse.parse_qs(raw, keep_blank_values=True), self.headers.get("Host"))
            if code != 303:
                return self.reply(code, "text/plain; charset=utf-8", f"{said}\n".encode())
            ref = urllib.parse.urlsplit(self.headers.get("Referer") or "")
            back = (ref.path or "/") if ref.netloc == self.headers.get("Host") else "/"
            self.send_response(303)
            self.send_header("Location", f"{back}#need-{said}")
            self.send_header("Content-Length", "0")
            self.end_headers()

        def reply(self, code: int, ctype: str, body: bytes) -> None:
            self.send_response(code)
            self.send_header("Content-Type", ctype)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-store")
            self.send_header(service.SERVE_HEADER, str(records.resolve()))  # lets a probe tell this service apart
            self.send_header(service.VERSION_HEADER, build)
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(body)

    server = http.server.ThreadingHTTPServer(("127.0.0.1", port()), Handler)
    now["snap"] = refresh(None)  # the first load has data to serve
    entries, done = spool_read(spool)
    for rid, entry in entries.items():  # replies a crash or kill left pending
        if rid not in done:
            replies[entry["id"]] = {"state": "saving", "text": entry["text"]}
            writes.put((entry, 0))
    work = [refresher, writer, ticker, pusher]
    if not gh:
        print("note: gh is not installed, so the pm service does not watch reviews' PRs for their merge", file=sys.stderr,
              flush=True)
    for w in work:
        threading.Thread(target=w, daemon=True).start()
    beads = f"Beads reread when {noms} changes" if noms else "Beads reread every look (Dolt server)"
    print(f"Serving http://localhost:{server.server_address[1]}; each page states the age of its data, at most "
          f"{SERVE_BEHIND} s behind unless it says so; {beads}; pushing every {pushjob.INTERVAL // 60} min",
          flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    if stopped:
        raise Refuse(stopped[0])
    return "stopped"  # replies still pending stay in the spool for the next start


# ---------------------------------------------------------------- reply delivery and pm reply read

MERGE_POLL = 60  # seconds between the pm service's looks at GitHub for open reviews' merges to main
PUSH_TIMEOUT = 5  # seconds the pm service gives a session's inbox to take a message
GH_TIMEOUT = 60  # seconds a gh or git call of the merge watch may take before it counts as not merged yet


def review_pr(issue: dict) -> str | None:
    """The PR an open review waits on, from its metadata.review."""
    meta = issue.get("metadata") if isinstance(issue.get("metadata"), dict) else {}
    review = meta.get("review") if isinstance(meta.get("review"), dict) else {}
    return review.get("pr") if issue.get("status") != "closed" and isinstance(review.get("pr"), str) else None


def merged_on_main(root: Path, pr: str) -> str | None:
    """The merge commit of `pr` once it is merged and on the remote's main branch (a PR merged into a stacked base is not), else
    None; a gh or git failure is reported on stderr and counts as not merged yet, so the wait goes on."""
    try:
        view = subprocess.run(["gh", "pr", "view", pr, "--json", "state,mergeCommit"], cwd=root, capture_output=True,
                              text=True, stdin=subprocess.DEVNULL, timeout=GH_TIMEOUT)
    except subprocess.TimeoutExpired:
        print(f"warning: gh pr view {pr} took over {GH_TIMEOUT}s, still waiting", file=sys.stderr, flush=True)
        return None
    if view.returncode != 0:
        print(f"warning: gh pr view {pr} failed, still waiting: {view.stderr.strip()}", file=sys.stderr, flush=True)
        return None
    info = json.loads(view.stdout)
    sha = (info.get("mergeCommit") or {}).get("oid")
    if info.get("state") != "MERGED" or not sha:
        return None
    remote, main = cfg().remote, cfg().main_branch
    try:
        fetch = subprocess.run(["git", "fetch", "--quiet", remote, main], cwd=root, capture_output=True, text=True,
                               stdin=subprocess.DEVNULL, timeout=GH_TIMEOUT)
    except subprocess.TimeoutExpired:
        print(f"warning: git fetch {remote} {main} took over {GH_TIMEOUT}s, still waiting", file=sys.stderr, flush=True)
        return None
    if fetch.returncode != 0:
        print(f"warning: git fetch {remote} {main} failed, still waiting: {fetch.stderr.strip()}", file=sys.stderr,
              flush=True)
        return None
    on_main = subprocess.run(["git", "merge-base", "--is-ancestor", sha, f"{remote}/{main}"], cwd=root, capture_output=True,
                             stdin=subprocess.DEVNULL)
    return sha if on_main.returncode == 0 else None


def pull_main(root: Path) -> str:
    """The command that fast-forwards this clone's main checkout, which hooks, rules and the ~/.claude links read."""
    return f"git -C {shlex.quote(str(main_of(store_path(root))))} pull --ff-only {cfg().remote} {cfg().main_branch}"


def new_replies(root: Path, issue: dict) -> tuple[list[dict], int]:
    """A request's site replies not delivered yet (those after its picked_up count), and how many it has in all."""
    if not issue.get("comment_count"):
        return [], 0
    replies = site_replies(json.loads(bd(root, "comments", issue["id"], "--json") or "[]") or [])
    return replies[picked_up(issue):], len(replies)


def reply_text(root: Path, issue: dict, replies: list[dict]) -> str:
    """The message for a request's undelivered site replies, and what the agent does next."""
    iid, k = issue["id"], kind(issue)
    said = "\n".join(f"  [{c.get('created_at', '')}] " + reply_body(c["text"]).replace("\n", "\n  ") for c in replies)
    if issue["status"] == "closed":
        nxt = "it is closed already; check the reply is handled"
    elif k == "decision":
        nxt = (f"record the answer: pm decision add --need {iid} --level … with it as the body if it sets a rule, "
               f"else pm decision close {iid} --reason \"<why it sets no rule>\" with the answer on stdin")
    else:
        nxt = f"check the evidence, then pm action done {iid} --reason \"<what you saw>\", or ask again if it falls short"
        if review_pr(issue):
            nxt += f"; once its PR is on main, update the main checkout: {pull_main(root)}"
    return f"pm: owner reply to {k} {iid} ({issue['title']}), relayed from the site:\n{said}\nnext: {nxt}"


def merge_text(root: Path, issue: dict, sha: str) -> str:
    meta = issue.get("metadata") if isinstance(issue.get("metadata"), dict) else {}
    pr = (meta.get("review") or {}).get("pr") or "?"
    n = re.search(r"/pull/(\d+)", pr)
    return (f"pm: PR {'#' + n.group(1) if n else pr} of review {issue['id']} ({issue['title']}) merged to "
            f"{cfg().main_branch} as {sha}\nnext: pm action done {issue['id']} --reason \"merged as {sha}\", then update the main checkout: "
            f"{pull_main(root)}")


def undelivered(root: Path, issue: dict) -> tuple[str, list[str]]:
    """What of a request has not reached its session: the site replies since its last pickup and a merge not
    reported. The message, and the metadata (key=value) that marks it delivered."""
    parts, marks = [], []
    replies, total = new_replies(root, issue)
    if replies:
        parts.append(reply_text(root, issue, replies))
        marks.append(f"{PICKED}={total}")
    if sha := merge_waiting(issue):
        parts.append(merge_text(root, issue, sha))
        marks.append(f"{MERGE_REPORTED}={sha}")
    return "\n".join(parts), marks


def mark_delivered(root: Path, issue_id: str, marks: list[str]) -> None:
    for m in marks:
        bd(root, "update", issue_id, f"--set-metadata={m}")


def push_inbox(issue: dict, text: str) -> str | None:
    """Write `text` as one user message into the inbox socket of the session that raised `issue` (its metadata.inbox),
    connecting only once the line is ready (the socket drops a connection that sends no line within 30 s), and only
    on the machine that stored it. None once the line is written; else why not: "not running: …" (no inbox stored,
    another machine, the socket gone or refusing) or "failed: …" (any other error)."""
    meta = issue.get("metadata") if isinstance(issue.get("metadata"), dict) else {}
    path = meta.get("inbox")
    if not path:
        return "not running: the request stores no session inbox"
    if meta.get("inbox_host") != socket.gethostname():
        return f"not running: the session's inbox is on {meta.get('inbox_host') or 'an unknown host'}, not this machine"
    line = json.dumps({"type": "user", "message": {"role": "user", "content": text}}) + "\n"
    try:
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
            s.settimeout(PUSH_TIMEOUT)
            s.connect(path)
            s.sendall(line.encode())
    except (FileNotFoundError, ConnectionRefusedError):
        return "not running: the session is not running"
    except OSError as e:
        return f"failed: {e!r}"
    return None


def push_undelivered(root: Path, issue_id: str) -> str:
    """The one delivery path of the pm service, for replies and merges alike: push what of the request has not reached its
    session into that session's inbox, and mark it delivered only once the push went through. Returns "delivered",
    "nothing to deliver" (read already), or why not ("not running: …", "failed: …"); the request then stays flagged by
    pm show, and the pm service's sweep tries it again."""
    issue = show_beads(root, [issue_id])[issue_id]
    text, marks = undelivered(root, issue)
    if not text:
        return "nothing to deliver"
    why = push_inbox(issue, text)
    if why:
        return why
    mark_delivered(root, issue_id, marks)
    return "delivered"


def cmd_reply_read(args, records: Path) -> str:
    """Print the requests' site replies not yet delivered and their reviews' merges not yet reported, with what to do
    next, and mark them delivered. Without ids, this session's open requests; with ids, those, closed ones too."""
    root = code_root(Path.cwd(), records)
    if args.ids:
        ids = list(dict.fromkeys(args.ids))
        beads = show_beads(root, ids)
        for iid in ids:
            if iid not in beads or HUMAN not in (beads[iid].get("labels") or []):
                raise Refuse(f"{iid} is not a request to the owner (a Beads issue labelled {HUMAN})")
        beads = {i: beads[i] for i in ids}
    else:
        sid = os.environ.get(SESSION_ENV, "").strip()
        if not sid:
            raise Refuse(f"name the requests to read: pm reply read <id>…; without {SESSION_ENV} pm cannot tell "
                         "which requests this session raised")
        found = bd(root, "list", "--label", HUMAN, "--metadata-field", f"session={sid}", "--limit", "0", "--json")
        beads = {i["id"]: i for i in sorted(json.loads(found), key=lambda i: i["id"])}  # open ones only
    out = []
    for iid, issue in beads.items():
        text, marks = undelivered(root, issue)
        if text:
            out.append(text)
            mark_delivered(root, iid, marks)
    return "\n".join(out) or f"nothing undelivered on {', '.join(beads) or 'any open request of this session'}"


# ---------------------------------------------------------------- pm show

def decisions_of(rec: Record, level: str) -> list[dict]:
    return [{"date": str(a.get("date", "")), "source": a.get("source", ""), "level": level,
             "record": rec.rel, "text": first_sentence(body, 100)} for a, body in decisions(rec.text)]


def show_data(repo: Repo) -> dict:
    b = repo.beads
    sprint_recs = {r.meta["bead"]: r for r in repo.recs if r.type == "sprint"}
    projects = []
    for p in (r for r in repo.recs if r.type == "project"):
        epic = p.meta["bead"]
        if b[epic]["status"] == "closed":
            continue
        decisions = [dict(d, order=i) for i, d in enumerate(decisions_of(p, "project"))]
        sprints = []
        for sp in (k for k in children(b, epic) if k["issue_type"] == "epic"):
            rec = sprint_recs.get(sp["id"])
            name = short(sp["id"], epic)
            if rec is not None:
                n = re.search(r"-(\d+)$", rec.name)
                name = f"sprint {n.group(1) if n else rec.name}"
                decisions += [dict(d, order=len(decisions) + i) for i, d in enumerate(decisions_of(rec, name))]
            if sp["status"] == "closed":
                continue
            tasks = children(b, sp["id"])
            done_when = section_text(rec.body, "Done when") if rec else ""
            items = len(re.findall(r"(?m)^(?:[-*]|\d+\.) ", done_when))
            sprints.append({
                "id": sp["id"], "name": name, "title": sp["title"], "state": state(sp, b),
                "record": rec.rel if rec else None, "url": f"{site_url()}/{rec.out}" if rec else None,
                "done": sum(t["status"] == "closed" for t in tasks), "total": len(tasks),
                "goal": first_sentence(section_text(rec.body, "Goal")) if rec else "",
                "done_when_items": items,
                "done_when": "" if items else first_sentence(done_when),
                "tasks": [{"id": t["id"], "title": t["title"], "state": state(t, b), "status": t["status"],
                           "human": HUMAN in (t.get("labels") or []), "kind": kind(t),
                           "blocked_by": [x for x in blockers(t) if b.get(x, {}).get("status") != "closed"],
                           "holder": holder(t)}
                          for t in tasks if t["status"] != "closed"],
            })
        decisions.sort(key=lambda d: (d["date"], d["order"]))
        fb = feedback_docs(repo.recs, p.name)
        projects.append({
            "name": p.name, "bead": epic, "title": p.title, "url": f"{site_url()}/{p.out}",
            "goal": first_sentence(section_text(p.body, "Goal")),
            "sprints": sprints,
            "needs": [dict(zip(("sprint", "task"), request_place(i, b, epic)), id=i["id"], title=i["title"],
                           kind=kind(i), session=session_of(i), replied=reply_waiting(i) or bool(merge_waiting(i)))
                      for i in sorted(owner_tasks(b, epic), key=lambda i: i["id"])],
            "decisions": [{k: v for k, v in d.items() if k != "order"} for d in reversed(decisions[-3:])],
            "feedback": [{"entries": len(FEEDBACK_ENTRY.findall(r.body)), "url": f"{site_url()}/{r.out}"} for r in fb],
        })
    today = date.today().isoformat()
    summary = read_summaries(repo.records).get(today)
    return {"site": site_url(), "projects": projects, "push": pushjob.flags(main_of(repo.records), repo.records, cfg().remote),
            "today": {"date": today, "page": f"days/{today}.html",
                      "summary": first_sentence(summary_line(summary["text"]), 160) if summary else None,
                      "generated_at": summary["generated_at"] if summary else None}}


def place(need: dict, under: str, names: dict[str, str]) -> str:
    """Where a request line's request sits, compactly: its sprint by name, then its task by short id."""
    if not need["sprint"]:
        return ""
    task = f", task {short(need['task'], under)}" if need["task"] else ""
    return f"  ({names.get(need['sprint'], short(need['sprint'], under))}{task})"


def show_text(data: dict) -> str:
    out = []
    if data["push"]:
        out.append("warning: the pm service's push needs attention (pm service status; pm service logs):")
        out += [f"  {l}" for l in data["push"]]
    me = current_session()
    others = [(t["id"], t["holder"]) for p in data["projects"] for sp in p["sprints"] for t in sp["tasks"]
              if t["holder"] and t["holder"]["live"] and t["holder"]["session"] != me]
    if others:
        out.append("warning: other live sessions hold these tasks; do not start or delegate them:")
        out += [f"  {i}  {holder_text(h)}" for i, h in others]
    # Most important first: `pm prime` cuts the end at its cap, so the task lists and past decisions go last.
    t = data["today"]
    out.append(f"today {t['date']}: " + (f"{t['summary']} (generated {t['generated_at']})" if t["summary"]
                                         else "no summary yet; the pm service generates it from today's activity"))
    out.append(f"site: {data['site']} (the pm service); a record's page is <site>/<its path under records/, without .md>"
               ".html; pm record link <target> prints one")
    out.append('feedback: when pm gets in your way, run pm feedback add --project <p> --text "…"')
    for p in data["projects"]:
        e = p["bead"]
        sprint_names = {sp["id"]: sp["name"] for sp in p["sprints"]}
        out.append(f"{p['name']}  {e}  {p['goal']}")
        for k in ("decision", "action"):
            needs = [n for n in p["needs"] if n["kind"] == k]
            out.append(f"{k}s await you ({len(needs)}):")
            out += [f"  {short(n['id'], e)}  {n['title']}{place(n, e, sprint_names)}  -> bd show {n['id']}"
                    + (f"  [undelivered reply: pm reply read {n['id']}]" if n["replied"] else "") for n in needs]
        out += [f"feedback: {f['entries']} entries -> {f['url']}" for f in p["feedback"]]
    for p in data["projects"]:
        e = p["bead"]
        if not (p["sprints"] or p["decisions"]):
            continue
        out.append(f"{p['name']}  {e}  sprints and decisions:")
        quiet = []
        for sp in p["sprints"]:
            if not sp["tasks"]:
                quiet.append(f"{short(sp['id'], e)} {sp['title']}")
                continue
            out.append(f"{sp['title']}  {short(sp['id'], e)}  {sp['state']}  {sp['done']}/{sp['total']} done")
            held = sorted({h["session"][:8] if h["session"] else f"{h['assignee']} (no session)"
                           for h in (t["holder"] for t in sp["tasks"]) if h})
            if held:
                out.append(f"  held by: {', '.join(held)}")
            if sp["goal"]:
                out.append(f"  goal: {sp['goal']}")
            if sp["done_when_items"]:
                out.append(f"  done when: {sp['done_when_items']} items (pm show --sprint {sp['id']})")
            elif sp["done_when"]:
                out.append(f"  done when: {sp['done_when']}")
            for t in sp["tasks"]:
                st = {"running": "in_progress"}.get(t["state"], t["state"])
                tail = f"  [human {t['kind']}]" if t["human"] else ""
                if t["blocked_by"]:
                    tail += f"  (by {', '.join(short(x, e) for x in t['blocked_by'])})"
                if t["holder"]:
                    tail += f"  [{holder_text(t['holder'])}]"
                out.append(f"  {st:<11}  {short(t['id'], e)}  {t['title']}{tail}")
        if quiet:
            out.append(f"open sprints without tasks: {'; '.join(quiet)}")
        if p["decisions"]:
            out.append(f"decisions (last {len(p['decisions'])}):")
            out += [f"  {d['date']} {d['source']} {d['level']}  {d['text']}" for d in p["decisions"]]
    return "\n".join(out)


def sprint_detail(repo: Repo, sprint_id: str) -> str:
    rec = repo.sprint(sprint_id)
    render_record(rec, repo.recs, repo.beads)
    sp = repo.beads[sprint_id]
    out = [f"{sp['title']}  {sprint_id}  {state(sp, repo.beads)}  {rel(repo, rec.path)}"]
    for name in FRAME + ["Findings"]:
        out += [f"## {name}", section_text(rec.body, name)]
    out.append("## Tasks")
    for t in children(repo.beads, sprint_id):
        h = holder(t)
        out.append(f"  {state(t, repo.beads):<7}  {t['id']}  {t['title']}" + (f"  [{holder_text(h)}]" if h else ""))
    return "\n".join(out)


def record_section(rec: Record, name: str) -> str:
    """One section of a record, from its heading through the next heading of the same or a higher level.
    An unknown or ambiguous name is refused with the record's section names."""
    lines, heads = rec.body.split("\n"), headings(rec.body)
    found = [(lvl, line) for lvl, text, line in heads if text == name]
    if len(found) != 1:
        names = "\n".join("  " * (lvl - 2) + "  " + text for lvl, text, _ in heads)
        why = "has no section" if not found else f"has {len(found)} sections"
        raise Refuse(f"records/{rec.rel}.md {why} named {name!r}; its sections:\n{names}")
    lvl, start = found[0]
    end = next((line for l2, _, line in heads if line > start and l2 <= lvl), len(lines))
    return "\n".join(lines[start:end]).strip()


def cmd_show(args, records: Path) -> str:
    if args.record or args.section:
        if not (args.record and args.section) or args.sprint or args.json:
            raise Refuse("--record and --section go together, without --sprint or --json")
        return record_section(link_target(read_records(records), records, args.record), args.section)
    repo = load(records, working=True)
    if args.sprint:
        if args.json:
            raise Refuse("--sprint prints text only; drop --json")
        return sprint_detail(repo, args.sprint)
    if args.refresh_inbox:
        refresh_inbox(repo)
    data = show_data(repo)
    return json.dumps(data, indent=1) if args.json else show_text(data)


# ---------------------------------------------------------------- pm record link

def link_target(recs: list[Record], records: Path, target: str) -> Record:
    """The record a target names: a record path (records/ and .md optional), a sprint or project Beads id, a
    project name or a design slug. A target that fits several records is refused, never guessed."""
    if "/" in target or target.endswith(".md"):
        given = Path(target)
        if given.is_absolute():
            try:
                given = given.resolve().relative_to(records.resolve())
            except ValueError:
                raise Refuse(f"{target} is not in the records store {records}")
        rel = given.as_posix().removeprefix("records/").removesuffix(".md")
        found = [r for r in recs if r.rel == rel]
    else:
        found = [r for r in recs if (r.type in ("sprint", "project") and r.meta.get("bead") == target)
                 or (r.type in ("project", "design") and r.name == target)]
    if not found:
        raise Refuse(f"no record matches {target!r}; give a sprint id, a project name, a design slug or a record "
                     "path such as records/sprints/<name>.md")
    if len(found) > 1:
        raise Refuse(f"{target!r} names {' and '.join('records/' + r.rel + '.md' for r in found)}; "
                     "give the record path instead")
    return found[0]


def cmd_record_link(args, records: Path) -> str:
    """The rendered page's URL once the pm service for this store answers on $PORT; otherwise the command that fixes it."""
    rec = link_target(read_records(records), records, args.target)
    fix = "pm service restart" if service.installed(service_main()) else "pm service install"
    served = service.answering(port())
    if served is None:
        raise Refuse(f"no site is served on :{port()}; start it with {fix}, then run this again")
    if not served[0]:
        raise Refuse(f"the server on :{port()} is not the pm service, so its pages may be stale; stop the old server on "
                     f":{port()}, then {fix}")
    if Path(served[0]) != records.resolve():
        raise Refuse(f"the pm service on :{port()} renders another store ({served[0]}); stop it, then {fix}")
    return f"{site_url()}/{rec.out}"


# ---------------------------------------------------------------- the store

def setup_beads(main: Path) -> list[str]:
    """Connect the clone to Beads, as `bd init` does for a new repo: the database (cloned from the remote's
    refs/dolt/data by `bd bootstrap`), the maintainer role, the team-maintainer agent profile, and the git hooks at .beads/hooks. Each step runs
    only when it is missing, so a set-up clone is left as it is."""
    out = []
    plan = json.loads(bd(main, "bootstrap", "--dry-run", "--json"))
    if plan["action"] not in ("none", "sync"):
        # Anything but cloning the remote's refs/dolt/data (an old issues.jsonl, a fresh database) would fork
        # the project's issues; fail instead.
        raise Refuse(f"bd bootstrap would {plan['action']}, not clone the remote's refs/dolt/data: "
                     f"{plan.get('reason', 'no reason given')}; fetch {cfg().remote}'s refs/dolt/data and run {SETUP} again")
    if plan["action"] == "sync":
        bd(main, "bootstrap", "--yes")
        out.append(f"set up the Beads database ({plan['action']}): {plan.get('beads_dir', main / '.beads')}")
    beads_dir = main / ".beads"
    if beads_dir.stat().st_mode & 0o777 != 0o700:
        beads_dir.chmod(0o700)  # bd warns on every call otherwise
        out.append(f"made {beads_dir} private (0700), as bd asks")
    if not store_git(main, "config", "--get", "--default=", "beads.role"):
        store_git(main, "config", "beads.role", "maintainer")  # bd init's non-interactive default
        out.append("set beads.role to maintainer")
    # The harness commits records itself; bd's default conservative profile tells agents never to commit or push.
    if json.loads(bd(main, "config", "get", "agent.profile", "--json"))["value"] != "team-maintainer":
        bd(main, "config", "set", "agent.profile", "team-maintainer")
        out.append("set the Beads agent profile to team-maintainer")
    hooks = beads_dir / "hooks"
    current = store_git(main, "config", "--get", "--default=", "core.hooksPath")
    if not current or (main / current).resolve() != hooks.resolve():
        bd(main, "hooks", "install", "--beads")
        out.append(f"installed the git hooks: core.hooksPath={hooks}")
    return out


def setup_clone() -> str:
    """Make the clone and this worktree ready, the part of pm init's clone half that the post-checkout hook runs too:
    connect Beads and install the git hooks, check out the store if it is missing, link this worktree's records/ to
    it, and let Codex's sandbox write the store and commit to it. pm init then installs the pm service."""
    cwd = Path.cwd()
    store = store_path(cwd)
    main = main_of(store)
    out = setup_beads(main)
    excluded = setup_exclude(main)
    if excluded:
        out.append(excluded)
    if not store.exists() and legacy.old_store(main).is_dir():
        raise Refuse(f"the records store is still at {legacy.old_store(main)}, where the project-management harness "
                     f"kept it; pm init moves it to {store} and migrates the rest of the old setup: run pm init")
    if not store.exists():
        remote = cfg().remote
        if subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"refs/heads/{BRANCH}"],
                          cwd=cwd, capture_output=True).returncode != 0:
            if subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"refs/remotes/{remote}/{BRANCH}"],
                              cwd=cwd, capture_output=True).returncode != 0:
                raise Refuse(f"no {BRANCH} branch here or on {remote}; fetch it, or create it once with "
                             f"git subtree split --prefix=records -b {BRANCH}")
            store_git(cwd, "branch", "--track", BRANCH, f"{remote}/{BRANCH}")
        store_git(cwd, "worktree", "add", "--quiet", str(store), BRANCH)
        out.append(f"checked out branch {BRANCH} at {store}")
    find_store(cwd)
    top = Path(store_git(cwd, "rev-parse", "--show-toplevel"))
    if top.resolve() == store.resolve():
        raise Refuse(f"{top} is the store itself; run {SETUP} from a code worktree")
    # main's copy of records/ is tracked on any branch made from it, now or after a later pull; keep it out of
    # this worktree always, or git replaces the ignored link with the copy the moment the branch tracks it.
    if store_git(top, "config", "--get", "--default=", "core.sparseCheckout") != "true":
        store_git(top, "sparse-checkout", "set", "--no-cone", "/*", "!/records/")
        store_git(top, "config", "--worktree", "sparse.expectFilesOutsideOfPatterns", "true")
        out.append(f"excluded records/ from {top} with sparse checkout")
    link = top / "records"
    if link.is_symlink() and link.resolve() == store.resolve():
        pass
    elif link.exists() or link.is_symlink():
        raise Refuse(f"{link} exists and is not a link to {store}; move it away and run {SETUP} again")
    else:
        link.symlink_to(store)
        out.append(f"linked {link} -> {store}")
    claude = setup_claude(top, store)
    if claude:
        out.append(claude)
    done = "\n".join(out) or f"already set up: {link} -> {store}"
    codex = setup_codex(main)
    return f"{done}\n{codex}" if codex else done


def check_hooks_path(top: Path, main: Path) -> None:
    """Refuse a core.hooksPath other than Beads' .beads/hooks (relative, or this worktree's or the main checkout's
    absolute path); unset is fine, as pm init has bd install it. Any other is refused since pm's git hooks live in Beads' hook files."""
    current = store_git(top, "config", "--get", "--default=", "core.hooksPath")
    if current and (top / current).resolve() not in ((top / ".beads/hooks").resolve(), (main / ".beads/hooks").resolve()):
        raise Refuse(f"core.hooksPath is {current}, not .beads/hooks; pm's git hooks live in Beads' hook files, so pm "
                     f"init works only with Beads' hooks path (other hook managers are not supported)")


def init_settings(top: Path, site_url: str | None, port: int) -> install.Settings:
    """A new repo's settings: the remote origin, which must exist, and its default branch (origin/HEAD), else the
    branch checked out; `port` is the site port."""
    remote = "origin"
    if subprocess.run(["git", "remote", "get-url", remote], cwd=top, capture_output=True).returncode != 0:
        raise Refuse(f"this repo has no remote {remote}; pm keeps the records branch and Beads data there, so add it "
                     f"(git remote add {remote} URL) and run pm init again")
    head = subprocess.run(["git", "symbolic-ref", "--quiet", "--short", f"refs/remotes/{remote}/HEAD"], cwd=top,
                          capture_output=True, text=True).stdout.strip()
    branch = head.removeprefix(f"{remote}/") if head else store_git(top, "symbolic-ref", "--short", "HEAD")
    url = (site_url or "").strip().rstrip("/")
    if url:
        check_site_url(url)
    return install.Settings(remote, branch, port, url)


def cmd_init(args) -> str:
    """Install pm: the repo's half only when the repo has no .pm/config.toml yet (a first install; pm doctor reports
    and pm upgrade rewrites a changed piece after that), then the clone's and this worktree's half every time, the pm
    service last. Session start runs it, so every worktree an agent works in is ready. The repo's pieces are written,
    never committed: the output names the commit to make, also when a later step fails."""
    cwd = Path.cwd()
    store = store_path(cwd)
    main = main_of(store)
    top = Path(store_git(cwd, "rev-parse", "--show-toplevel"))
    if top.resolve() == store.resolve():
        raise Refuse(f"{top} is the records store; run pm init from a code worktree")
    check_hooks_path(top, main)
    fresh = not (top / config.REL).is_file()
    if top.resolve() != main.resolve():
        if fresh:  # pm's files come from the main branch; a linked worktree without them is on a branch from before
            raise Refuse(f"this worktree's branch has no {config.REL}: it was cut before pm was installed in this "
                         f"repo, and pm init installs the repo's files only in the main checkout; merge the main "
                         f"branch into this one, or run pm init in the main checkout {main}; pm init wrote nothing")
        # the service runs in the main checkout, under its pin, with the one pm uv tool the hooks run too; this
        # worktree's setup does not depend on either, so it runs first and only the tool and the service are refused
        try:
            pin = config.read(main).version
        except config.ConfigError as e:
            pin, why = None, (f"the pm service runs in the main checkout {main}, under its branch's pin, and that "
                              f"branch has none ({e}); run pm init there, or check out a branch there that pins pm "
                              f"{__version__}")
        else:
            why = (f"the main checkout {main} pins pm {pin}, and the pm service and the one pm uv tool follow it; run "
                   f"pm init with pm {pin}, or move main's pin with pm upgrade there first")
        if pin != __version__:
            raise Refuse(f"{why}. pm init set up this worktree and left the pm uv tool and the service alone:\n"
                         f"{setup_clone()}")
    if fresh:  # the site port: $PORT, else the clone's unit's, else the first free one no pm unit here names
        s = init_settings(top, args.site_url, service.port_for(main, service.free_port()))
    else:
        c = cfg()
        s = install.Settings(c.remote, c.main_branch, c.port, c.site_url)
        if args.site_url and args.site_url.strip():
            check_site_url(args.site_url.strip().rstrip("/"))  # before anything is written
    old = legacy.old_store(main)
    if old.is_dir() and store.exists():
        raise Refuse(f"both the old store {old} and the store {store} exist; keep the one holding your records, remove "
                     f"the other (git worktree remove), and run pm init again")
    why = legacy.store_unsettled(old, s.remote, BRANCH) if old.is_dir() else ""
    if why:  # refused before anything changes; migrate_clone checks again once the old push job is stopped
        raise Refuse(f"pm init moves the records store from {old} to {store}, but {why}; then run pm init again")
    if not (args.session_start and service.installed(main)):  # session start leaves an installed service alone
        try:  # the service pm init ends with must be able to serve: refused before anything is written
            service.check_port(main, s.port if fresh else service.port_for(main, s.port))
        except RecordError as e:
            raise Refuse(str(e))
    before = worktree_changes(top)
    out = []
    if fresh:
        try:  # read-only: a refusal comes before bd init, which writes and commits
            install.plan(top, s, repo_legacy(top)[0])
        except install.InstallError as e:
            raise Refuse(str(e))
    written: list[str] = []
    try:
        done = init_steps(args, top, main, s, fresh, out, written)
    except (Refuse, RecordError) as e:  # a failure after writing still names what to commit, then what to run
        hint = commit_hint(top, before, written)
        if not hint:
            raise
        raise Refuse("\n".join([str(e), *out, hint, "then fix the error above and run pm init again"])) from e
    return "\n".join(l for l in (done, commit_hint(top, before, written)) if l)


def init_steps(args, top: Path, main: Path, s: install.Settings, fresh: bool, out: list[str],
               written: list[str]) -> str:
    """pm init's writes, in order; `out` and `written` say how far it got when one fails. The repo's half (bd init,
    pm's pieces, the records branch) runs only when `fresh`."""
    tooled = tool.ensure()  # the service's unit and the hooks run the pm uv tool, not this pm (uvx's, say)
    if tooled:
        out.append(tooled)
    if fresh and not (top / ".beads").exists():
        bd(top, "init", "--non-interactive")
        out.append("ran bd init: Beads set up its database, its files and its git hooks (and commits them itself)")
    try:  # the clone first: it can still refuse, and the repo's writes (.gitignore losing /.records/) must not precede it
        out += [f"legacy: {l}" for l in legacy.migrate_clone(main, main / config.STORE, s.remote, BRANCH,
                                                            codex_remove)]
    except RecordError as e:
        raise Refuse(str(e))
    if fresh:
        try:
            overlay, found = repo_legacy(top)  # again: bd init wrote files pm's pieces share
            planned = install.plan(top, s, overlay)
            if (subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"refs/heads/{BRANCH}"], cwd=top,
                               capture_output=True).returncode != 0
                    and not install.remote_has_branch(top, s.remote, BRANCH)):
                out.append(install.create_records_branch(top, s.remote, BRANCH))
            out += [f"legacy: {f}" for f in found]
            written += write_legacy(top, overlay, {piece.rel for piece, *_ in planned})
            written += install.write(planned)
        except install.InstallError as e:
            raise Refuse(str(e))
        out += [f"wrote {rel}" for rel in written if (top / rel).exists()]
    elif args.site_url is not None:
        site = setup_site_url(args.site_url)
        if site:
            out.append(site)
    out.append(setup_clone())
    if args.session_start and service.installed(main):
        # A restart may take service.RESTART_WAIT (15 s) of session start's budget, and parallel session starts
        # would each make one: session start only reports a stale or down service, and pm service restart fixes it.
        ok, line = service.health(main, main / config.STORE)
        if not ok:
            out.append(f"left the installed pm service as it is (session start never restarts it): "
                       f"{line.split(')  ', 1)[-1]}")
        return "\n".join(out)
    said = service.install(main, service.port_for(main, cfg().port))
    if said:
        out.append(said)
    return "\n".join(out)


def repo_legacy(top: Path) -> tuple[dict[str, str | None], list[str]]:
    """The tracked files holding legacy pieces, each without them (None: the file goes), and what goes; read-only."""
    return legacy.repo_overlay(top, {rel: install.events(rel) for rel in legacy.SETTINGS})


def write_legacy(top: Path, overlay: dict[str, str | None], pieces: set[str]) -> list[str]:
    """Write the files that lose a legacy piece and that no pm piece rewrites (that write starts from the overlay);
    the paths written or removed."""
    for rel, text in overlay.items():
        if rel in pieces:
            continue
        if text is None:
            (top / rel).unlink()
        else:
            (top / rel).write_text(text)
    return [rel for rel in overlay if rel not in pieces]


def codex_remove(roots: list[str]) -> str:
    """Take `roots` out of Codex's writable_roots, keeping every other byte; what changed, or empty."""
    path = codex_home() / "config.toml"
    if not path.is_file():
        return ""
    new = remove_codex_roots(path, roots)
    if new == path.read_text():
        return ""
    write_atomic(path, new)
    return f"removed {', '.join(roots)} from writable_roots in {path}"


def commit_hint(top: Path, before: dict[str, str], written: list[str]) -> str:
    """The commit to make of what pm init changed in `top` since `before`; empty when nothing changed."""
    after = worktree_changes(top)
    changed = list(dict.fromkeys(written + [p for p in after if after[p] != before.get(p)]))
    if not changed:  # pm's pieces, and what bd changed meanwhile (bd init's files, the agent profile in .beads/config.yaml)
        return ""
    branch = store_git(top, "rev-parse", "--abbrev-ref", "HEAD")
    return (f"pm commits nothing on {branch}; commit what pm init changed there: git add -- {' '.join(changed)} "
            f'&& git commit -m "Install pm {__version__}"')


def worktree_changes(top: Path) -> dict[str, str]:
    """Each path git status shows in `top` (untracked files one by one), with its status and, for a file, a hash of
    its bytes, so a file changed again under the same status still differs."""
    res = subprocess.run(["git", "status", "--porcelain=v1", "-z", "--untracked-files=all"], cwd=top,
                         capture_output=True, text=True)  # not store_git: its strip would eat a leading status space
    if res.returncode != 0:
        raise Refuse(f"git status failed in {top}: {res.stderr.strip()}")
    raw = res.stdout
    out, entries = {}, iter(raw.split("\0"))
    for e in entries:
        if not e:
            continue
        code, rel = e[:2], e[3:]
        if "R" in code or "C" in code:
            next(entries, None)  # a rename's or copy's source path
        path = top / rel
        digest = hashlib.sha1(path.read_bytes()).hexdigest() if path.is_file() and not path.is_symlink() else ""
        out[rel] = f"{code} {digest}"
    return out


def code_top(cwd: Path, store: Path, what: str) -> Path:
    """The code worktree a repo-level command acts on; refused inside the store."""
    top = Path(store_git(cwd, "rev-parse", "--show-toplevel"))
    if top.resolve() == store.resolve():
        raise Refuse(f"{top} is the records store; run pm {what} from a code worktree")
    return top


def settings_of(c: config.Config) -> install.Settings:
    return install.Settings(c.remote, c.main_branch, c.port, c.site_url)


def sparse_patterns(top: Path) -> list[str]:
    if store_git(top, "config", "--get", "--default=", "core.sparseCheckout") != "true":
        return []
    res = subprocess.run(["git", "sparse-checkout", "list"], cwd=top, capture_output=True, text=True)
    return res.stdout.split() if res.returncode == 0 else []


PM_SPARSE = ["/*", "!/records/"]  # what setup_clone sets


def doctor_setup(top: Path, main: Path, store: Path) -> list[str]:
    """How the clone and this worktree differ from what pm init makes, one line each."""
    out = []
    try:
        find_store(top)
    except RecordError as e:
        out.append(f"store: {e}")
    link = top / "records"
    if not (link.is_symlink() and link.resolve() == store.resolve()):
        out.append(f"records link: {link} is not a link to {store}; run pm init")
    if "!/records/" not in sparse_patterns(top):
        out.append(f"sparse checkout: {top} does not exclude records/; run pm init")
    hooks_path = store_git(main, "config", "--get", "--default=", "core.hooksPath")
    if not hooks_path or (main / hooks_path).resolve() != (main / ".beads/hooks").resolve():
        fix = ("run pm init" if not hooks_path else
               "pm works only with Beads' hooks path: move any hooks there into .beads/hooks (outside Beads' and "
               "pm's marked sections), run git config --unset core.hooksPath, then pm init")
        out.append(f"hooks path: core.hooksPath is {hooks_path or 'unset'}, not .beads/hooks; {fix}")
    out += [f"service: {d}" for d in service.drift(main, service.port_for(main, cfg().port))]
    if codex_home().is_dir() and store.is_dir():
        path = codex_home() / "config.toml"
        missing = [r for r in codex_roots(main) if r not in codex_config(path)[2]]
        if missing:
            out.append(f"codex: {path} lacks writable_roots {', '.join(missing)}; run pm init")
    path = exclude_path(main)
    missing = [l for l in PM_EXCLUDE if l not in exclude_lines(path)]
    if missing:
        out.append(f"git exclude: {path} lacks {', '.join(missing)}; run pm init")
    if Path(os.environ.get("CLAUDE_CONFIG_DIR") or Path.home() / ".claude").is_dir():
        path = top / ".claude/settings.local.json"
        if str(store) not in claude_dirs(path):
            out.append(f"claude: {path} does not list {store} in permissions.additionalDirectories; run pm init")
    return out


def cmd_doctor(args) -> tuple[int, str]:
    """Compare every managed piece with what this pm (the pinned version, as main checked) writes, and the clone and
    worktree with what pm init makes; non-zero on any difference."""
    cwd = Path.cwd()
    store = store_path(cwd)
    main = main_of(store)
    top = code_top(cwd, store, "doctor")
    diffs = [f"repo: {d}; run pm upgrade to rewrite it" for d in install.drift(top, settings_of(cfg()))]
    diffs += doctor_setup(top, main, store)
    try:
        repo_found = repo_legacy(top)[1]
    except install.InstallError as e:  # a file pm cannot read: no legacy piece is known, the file needs a hand fix
        repo_found = []
        diffs.append(f"repo: cannot look for the pre-package harness's pieces: {e}")
    codex = codex_home() / "config.toml"
    diffs += [f"legacy: {d}; run pm upgrade" for d in repo_found]  # pm init leaves an installed repo's files alone
    diffs += [f"legacy: {d}; run pm init" for d in
              legacy.clone_pieces(main, store, codex_config(codex)[2] if codex.is_file() else [])]
    if diffs:
        return 1, "\n".join(diffs)
    return 0, (f"pm {__version__} ({launch.how()}): every managed piece and the clone's setup match what pm init "
               "makes")


def cmd_upgrade(args) -> str:
    """Move the pin to the running pm and rewrite every managed piece as it writes them; commits nothing."""
    to = args.to or __version__
    if to != __version__:
        raise Refuse(f"pm upgrade --to {to} must run pm {to}, but pm {__version__} is running; install it with "
                     f"{config.INSTALL.format(v=to)}, then run pm upgrade")
    cwd = Path.cwd()
    store = store_path(cwd)
    top = code_top(cwd, store, "upgrade")
    try:
        c = config.read(top)
    except config.ConfigError as e:
        raise Refuse(str(e))
    try:
        overlay, found = repo_legacy(top)  # the pre-package harness's pieces go too
        planned = install.rewrite(top, settings_of(c), overlay)
    except install.InstallError as e:
        raise Refuse(str(e))
    written = write_legacy(top, overlay, {piece.rel for piece, *_ in planned}) + install.write(planned)
    if not written:
        return f"pm {__version__}: every managed piece is current; nothing to commit"
    branch = store_git(top, "rev-parse", "--abbrev-ref", "HEAD")
    moved = f"moved the pin from {c.version} to {__version__}" if c.version != __version__ else f"pin stays {__version__}"
    return "\n".join([moved, *(f"legacy: {f}" for f in found), *(f"wrote {rel}" for rel in written),
                      f"pm commits nothing on {branch}; commit pm's files there: git add -- {' '.join(written)} && "
                      f'git commit -m "Upgrade pm to {__version__}"'])


def claude_dirs(path: Path) -> list:
    try:
        data = json.loads(path.read_text()) if path.exists() else {}
    except (OSError, ValueError) as e:
        raise Refuse(f"cannot parse {path}: {e}; fix it by hand")
    perms = data.get("permissions", {}) if isinstance(data, dict) else None
    dirs = perms.get("additionalDirectories", []) if isinstance(perms, dict) else None
    if not isinstance(dirs, list):
        raise Refuse(f"{path}: permissions.additionalDirectories is not a list of paths; fix it by hand")
    return dirs


def remove_claude(top: Path, store: Path) -> str:
    """Take the store out of this worktree's .claude/settings.local.json; the file goes when nothing else is left."""
    path = top / ".claude/settings.local.json"
    if str(store) not in claude_dirs(path):
        return ""
    data = json.loads(path.read_text())
    dirs = data["permissions"]["additionalDirectories"]
    dirs.remove(str(store))
    if not dirs:
        del data["permissions"]["additionalDirectories"]
        if not data["permissions"]:
            del data["permissions"]
    if data:
        write_atomic(path, install.dump_json(data))
    else:
        path.unlink()
        with contextlib.suppress(OSError):
            path.parent.rmdir()  # only when empty
    return f"removed {store} from permissions.additionalDirectories in {path}"


def remove_codex_roots(path: Path, roots: list[str]) -> str:
    """`path`'s text without pm's writable roots, by a minimal edit undoing add_codex_roots; refused when the edit
    would change anything else."""
    text, data, have = codex_config(path)
    gone = [r for r in roots if r in have]
    if not gone:
        return text
    header = CODEX_HEADER.search(text)
    items = ", ".join(json.dumps(r, ensure_ascii=False) for r in gone)
    if header is None:
        raise Refuse(f"{path} sets {CODEX_TABLE} without a [{CODEX_TABLE}] header; remove these from its writable_roots "
                     f"by hand: {items}")
    nxt = re.compile(r"^[ \t]*\[", re.M).search(text, header.end())
    end = nxt.start() if nxt else len(text)
    section = text[header.end():end]
    for r in gone:
        item = json.dumps(r, ensure_ascii=False)
        for form in (item + ", ", ", " + item, item):
            if form in section:
                section = section.replace(form, "", 1)
                break
    new = text[:header.end()] + section + text[end:]
    expected = copy.deepcopy(data)
    kept = [r for r in have if r not in gone]
    expected[CODEX_TABLE]["writable_roots"] = kept
    if not kept:  # what add_codex_roots made from nothing goes too: the key, then the table
        block = f"[{CODEX_TABLE}]\nwritable_roots = []\n"
        if set(data[CODEX_TABLE]) == {"writable_roots"} and new.endswith(block):
            new = new[:-len(block)]
            new = new[:-1] if new.endswith("\n\n") else new
            del expected[CODEX_TABLE]
        elif section.startswith("\nwritable_roots = []"):
            new = text[:header.end()] + section[len("\nwritable_roots = []"):] + text[end:]
            del expected[CODEX_TABLE]["writable_roots"]
    try:
        edited = tomllib.loads(new)
    except tomllib.TOMLDecodeError as e:  # a writable_roots spread over lines, say: its commas stay behind
        raise Refuse(f"editing {path} would leave it invalid TOML ({e}); remove these from its writable_roots by hand: "
                     f"{items}")
    if edited != expected:
        raise Refuse(f"editing {path} would change more than writable_roots; remove these by hand: {items}")
    return new


def worktrees(main: Path) -> list[Path]:
    """The clone's worktrees whose directory exists: one deleted without git worktree remove is still listed
    (prunable), and git cannot run in it."""
    out = []
    for block in store_git(main, "worktree", "list", "--porcelain").split("\n\n"):
        lines = block.splitlines()
        if lines and lines[0].startswith("worktree ") and not any(l.startswith("prunable") for l in lines):
            tree = Path(lines[0].removeprefix("worktree "))
            if tree.is_dir():
                out.append(tree)
    return out


def cmd_uninstall(args) -> str:
    """Remove pm's pieces from this worktree and pm's setup from the clone and the machine; the records branch,
    records/ on the main branch and Beads stay. Refused before anything changes when the store holds uncommitted
    records or a piece cannot be removed without touching what is not pm's."""
    cwd = Path.cwd()
    store = store_path(cwd)
    main = main_of(store)
    top = code_top(cwd, store, "uninstall")
    try:
        removals = install.removals(top, settings_of(cfg()))
    except install.InstallError as e:
        raise Refuse(str(e))
    trees = [t for t in worktrees(main) if t.resolve() != store.resolve()]
    roots: list[str] = []
    if store.is_dir():
        dirty = store_git(store, "status", "--porcelain")
        if dirty:
            raise Refuse(f"the store {store} holds uncommitted records; commit them with pm commit or revert them, "
                         f"then run pm uninstall again:\n{dirty}")
        roots = clone_codex_roots(main)  # uv's cache stays: other clones' setup may have added or need it
    codex_path = codex_home() / "config.toml"
    codex_new = remove_codex_roots(codex_path, roots) if roots and codex_path.exists() else None
    for t in trees:
        claude_dirs(t / ".claude/settings.local.json")  # refuse an unreadable one before changing anything
    sparse = {t: sparse_patterns(t) == PM_SPARSE for t in trees}  # read before the first change, which may not fail
    out = []
    said = service.uninstall(main)
    if said:
        out.append(said)
    for t in trees:
        link = t / "records"
        if link.is_symlink() and link.resolve() == store.resolve():
            link.unlink()
            out.append(f"removed the link {link}")
        if sparse[t]:
            store_git(t, "sparse-checkout", "disable")
            subprocess.run(["git", "config", "--worktree", "--unset", "sparse.expectFilesOutsideOfPatterns"], cwd=t,
                           capture_output=True)
            out.append(f"turned off the sparse checkout of {t}")
        claude = remove_claude(t, store)
        if claude:
            out.append(claude)
    if store.is_dir():
        store_git(main, "worktree", "remove", str(store))
        out.append(f"removed the store checkout {store}; the {BRANCH} branch stays")
    excluded = remove_exclude(main)
    if excluded:
        out.append(excluded)
    if codex_new is not None and codex_new != codex_path.read_text():
        write_atomic(codex_path, codex_new)
        out.append(f"removed this clone's writable_roots from {codex_path}; uv's cache stays, as other clones share it")
    changed = []
    for path, text in removals:
        if text is None:
            path.unlink()
        else:
            path.write_text(text)
        changed.append(path.relative_to(top).as_posix())
    for base in dict.fromkeys([top.resolve(), main.resolve()]):
        pmdir = base / ".pm"
        if base == top.resolve() and pmdir.exists():
            shutil.rmtree(pmdir)
            changed.append(".pm")
        elif pmdir.exists():  # another branch's checkout: only the clone's own state, never its tracked files
            for d in ("store", "run"):
                shutil.rmtree(pmdir / d, ignore_errors=True)
    out += [f"removed pm's part of {rel}" if (top / rel).exists() else f"removed {rel}" for rel in dict.fromkeys(changed)]
    if changed:
        branch = store_git(top, "rev-parse", "--abbrev-ref", "HEAD")
        out.append(f"pm commits nothing on {branch}; commit the removal there: git add -A -- "
                   f"{' '.join(dict.fromkeys(changed))} && git commit -m \"Uninstall pm\"")
    return "\n".join(out) or "pm is not installed here; nothing to remove"


# the clone's own state under .pm/ and each worktree's records/ link, in the common git dir's info/exclude: a branch
# made before pm has neither .pm/.gitignore nor pm's .gitignore block, and there git add -A would stage the store as an
# embedded repo and the link as a file
PM_EXCLUDE = ["/.pm/store/", "/.pm/run/", "/records"]


def exclude_path(main: Path) -> Path:
    return main / ".git/info/exclude"  # store_path checked that the common git dir is main's .git


def exclude_lines(path: Path) -> list[str]:
    return path.read_text().splitlines() if path.exists() else []


def setup_exclude(main: Path) -> str:
    """Append the PM_EXCLUDE lines missing from the clone's info/exclude, keeping the rest byte for byte."""
    path = exclude_path(main)
    missing = [l for l in PM_EXCLUDE if l not in exclude_lines(path)]
    if not missing:
        return ""
    text = path.read_text() if path.exists() else ""
    sep = "" if not text or text.endswith("\n") else "\n"
    write_atomic(path, text + sep + "".join(f"{l}\n" for l in missing))
    return f"added {', '.join(missing)} to {path}, so no branch stages the clone's pm state"


def remove_exclude(main: Path) -> str:
    """Take the PM_EXCLUDE lines out of the clone's info/exclude, keeping every other byte."""
    path = exclude_path(main)
    if not path.exists():
        return ""
    lines = path.read_text().splitlines(keepends=True)
    kept = [l for l in lines if l.rstrip("\r\n") not in PM_EXCLUDE]
    if kept == lines:
        return ""
    gone = [l for l in PM_EXCLUDE if l in exclude_lines(path)]
    write_atomic(path, "".join(kept))
    return f"removed {', '.join(gone)} from {path}"


def setup_claude(top: Path, store: Path) -> str:
    """Add the store to permissions.additionalDirectories in this worktree's .claude/settings.local.json, keeping
    the rest of the file's data: records/ resolves outside the worktree, so without it Claude Code asks before each
    write through the link. The path is absolute and per machine, so it goes in the local file, not settings.json.
    Without a Claude Code config dir it does nothing."""
    if not Path(os.environ.get("CLAUDE_CONFIG_DIR") or Path.home() / ".claude").is_dir():
        return ""
    path = top / ".claude/settings.local.json"
    try:
        data = json.loads(path.read_text()) if path.exists() else {}
    except (OSError, ValueError) as e:
        raise Refuse(f"cannot parse {path}: {e}; fix it by hand and run {SETUP} again")
    perms = data.setdefault("permissions", {}) if isinstance(data, dict) else None
    dirs = perms.setdefault("additionalDirectories", []) if isinstance(perms, dict) else None
    if not isinstance(dirs, list):
        raise Refuse(f"{path}: permissions.additionalDirectories is not a list of paths; fix it by hand")
    if str(store) in dirs:
        return ""
    dirs.append(str(store))
    path.parent.mkdir(exist_ok=True)
    write_atomic(path, install.dump_json(data))
    return f"added {store} to permissions.additionalDirectories in {path}, so Claude Code writes records through records/ without asking"


def check_site_url(url: str) -> None:
    parts = urllib.parse.urlsplit(url)
    if parts.scheme not in ("http", "https") or not parts.hostname or parts.path or parts.query or parts.fragment:
        raise Refuse(f"--site-url {url!r} is not an http(s) base URL like https://pm.example.com")


def setup_site_url(url: str) -> str:
    """Store the repo's public site URL as `site_url` in .pm/config.toml ("" removes it), for the owner to commit;
    "" when nothing changed."""
    url = url.strip().rstrip("/")
    if url:
        check_site_url(url)
    old = public_url()
    if url == old:
        return ""
    config.write_site_url(cfg(), url)
    if url:
        return f"site URL set to {url}" + (f" (was {old})" if old else "") + f" in {cfg().path}; commit it"
    return f"site URL cleared (was {old}) in {cfg().path}; commit it; links use http://localhost:{port()}"


# Codex's workspace-write sandbox keeps .git read-only even inside the workspace, leaves the store outside a
# worktree's workspace along with the Beads database pm reads through bd, and blocks uv's cache, so pm cannot
# commit records until all are writable roots. A writable root's own git dir stays read-only unless listed itself,
# so the store's (.git/worktrees/<name>) is listed too.
CODEX_TABLE = "sandbox_workspace_write"
CODEX_HEADER = re.compile(rf"^[ \t]*\[[ \t]*{CODEX_TABLE}[ \t]*\][ \t]*(?:#.*)?$", re.M)


def codex_home() -> Path:
    return Path(os.environ.get("CODEX_HOME") or Path.home() / ".codex")


def clone_codex_roots(main: Path) -> list[str]:
    """The roots only this clone needs: its git dir, the store, the store's git dir and the Beads dir, as absolute
    paths."""
    store = main / config.STORE
    store_gitdir = Path(store_git(store, "rev-parse", "--absolute-git-dir"))
    return [str(p.resolve()) for p in (main / ".git", store, store_gitdir, main / ".beads")]


def codex_roots(main: Path) -> list[str]:
    """The clone's own roots and uv's cache, which every clone on the machine shares."""
    try:
        res = subprocess.run(["uv", "--color", "never", "cache", "dir"], capture_output=True, text=True)
    except FileNotFoundError:
        raise Refuse("uv is not installed; pm is a uv tool and needs it")
    if res.returncode != 0 or not res.stdout.strip():
        raise Refuse(f"uv cache dir failed: {res.stderr.strip()}")
    return clone_codex_roots(main) + [str(Path(res.stdout.strip()).resolve())]


def codex_config(path: Path) -> tuple[str, dict, list[str]]:
    """The config's text, its parsed data, and the writable roots it already lists; refuses what it cannot read."""
    text = path.read_text() if path.exists() else ""
    try:
        data = tomllib.loads(text)
    except tomllib.TOMLDecodeError as e:
        raise Refuse(f"cannot parse {path}: {e}; fix it by hand and run {SETUP} again")
    table = data.get(CODEX_TABLE, {})
    have = table.get("writable_roots", []) if isinstance(table, dict) else None
    if not isinstance(have, list) or not all(isinstance(r, str) for r in have):
        raise Refuse(f"{path}: {CODEX_TABLE}.writable_roots is not a list of paths; fix it by hand")
    return text, data, have


def add_codex_roots(path: Path, text: str, data: dict, have: list[str], missing: list[str]) -> str:
    """`text` with `missing` added to the writable roots by a minimal edit; every other byte is kept."""
    items = ", ".join(json.dumps(r, ensure_ascii=False) for r in missing)
    expected = copy.deepcopy(data)
    header = CODEX_HEADER.search(text)
    if CODEX_TABLE not in data:
        sep = "" if not text or text.endswith("\n\n") else "\n" if text.endswith("\n") else "\n\n"
        new = f"{text}{sep}[{CODEX_TABLE}]\nwritable_roots = [{items}]\n"
        expected[CODEX_TABLE] = {"writable_roots": missing}
    elif header is None:
        raise Refuse(f"{path} sets {CODEX_TABLE} without a [{CODEX_TABLE}] header; add these to its writable_roots "
                     f"by hand: {items}")
    else:
        nxt = re.compile(r"^[ \t]*\[", re.M).search(text, header.end())
        section = text[header.end():nxt.start() if nxt else len(text)]
        key = re.search(r"^[ \t]*writable_roots[ \t]*=[ \t]*\[", section, re.M)
        if "writable_roots" not in data[CODEX_TABLE]:
            new = f"{text[:header.end()]}\nwritable_roots = [{items}]{text[header.end():]}"
        elif key is None:
            raise Refuse(f"cannot find writable_roots under [{CODEX_TABLE}] in {path}; add these by hand: {items}")
        else:
            at = header.end() + key.end()
            rest = text[at:]
            new = f"{text[:at]}{items}{'' if rest.lstrip().startswith(']') else ', '}{rest}"
        expected[CODEX_TABLE]["writable_roots"] = missing + have
    if tomllib.loads(new) != expected:  # the edit must add the roots and change nothing else
        raise Refuse(f"editing {path} would change more than writable_roots; add these by hand: {items}")
    return new


def setup_codex(main: Path) -> str:
    """Add the roots pm needs to the user's Codex config, keeping the rest of the file byte for byte."""
    home = codex_home()
    if not home.is_dir():
        return f"Codex: no {home}, so Codex is not used here; left its sandbox config alone"
    path = home / "config.toml"
    text, data, have = codex_config(path)
    missing = [r for r in codex_roots(main) if r not in have]
    if not missing:
        return ""
    write_atomic(path, add_codex_roots(path, text, data, have, missing))
    return f"added to {CODEX_TABLE}.writable_roots in {path}, so Codex can commit records: {', '.join(missing)}"


def cmd_push(args, records: None) -> tuple[int, str]:
    """One run of the push, as the pm service makes every 10 minutes; its exit code is non-zero when either store failed. It finds the store
    itself, so a missing store is recorded as the records step's failure rather than leaving no state."""
    cwd = Path.cwd()

    def summarize() -> tuple[bool, str]:
        try:
            return True, summarize_day(find_store(cwd))
        except (Refuse, RecordError) as e:
            return False, str(e)
    return pushjob.push(main_of(store_path(cwd)), cfg().remote, lambda: find_store(cwd), summarize)


def service_main() -> Path:
    """The main checkout, where the service runs and keeps its state: the git common dir's parent."""
    return main_of(store_path(Path.cwd()))


def cmd_service_install(args, records: Path) -> str:
    main = service_main()
    said = service.install(main, port())
    return said or f"already installed and current\n{service.health(main, records)[1]}"


def cmd_service_status(args, records: Path) -> tuple[int, str]:
    return service.status(service_main(), records, cfg().remote)


def cmd_service_restart(args, records: Path) -> str:
    return service.restart(service_main())


def cmd_service_logs(args, records: Path) -> str:
    return service.logs(service_main(), args.lines)


def cmd_where(args, records: Path) -> str:
    """`pm where records`: the store's path alone, for scripts."""
    return str(records)


def where_all() -> str:
    """Every location an agent or the owner needs, each with its state; works before setup, to show what is missing."""
    cwd = Path.cwd()
    store = store_path(cwd)
    main = main_of(store)

    def g(where: Path, *args: str) -> str:
        res = subprocess.run(["git", *args], cwd=where, capture_output=True, text=True)
        return res.stdout.strip() if res.returncode == 0 else ""

    out = [f"pm        {__version__}  {launch.how()}"]
    if not store.is_dir():
        out.append(f"store     {store}  missing; run {SETUP}")
    else:
        branch = g(store, "rev-parse", "--abbrev-ref", "HEAD")
        line = f"store     {store}  branch {branch}" + ("" if branch == BRANCH else f" (must be {BRANCH})")
        upstream = f"{cfg().remote}/{BRANCH}"
        counts = g(store, "rev-list", "--left-right", "--count", f"HEAD...{upstream}").split()
        line += (f", {counts[0]} ahead, {counts[1]} behind {upstream} (as of the last fetch)" if counts
                 else f", no {upstream}")
        out.append(line)

    top = Path(g(cwd, "rev-parse", "--show-toplevel"))
    top = main if top.resolve() == store.resolve() else top
    link = top / "records"
    if link.is_symlink() and link.resolve() == store.resolve():
        state_ = "records link set up"
    elif not link.exists() and not link.is_symlink():
        state_ = f"no records link; run {SETUP}"
    elif not link.is_symlink() and g(top, "ls-files", "--", "records"):
        state_ = f"records/ is main's tracked copy, not the link; run {SETUP}"  # setup hides it
    else:
        state_ = f"records is not a link to the store; move it away and run {SETUP}"
    out.append(f"checkout  {top}  branch {g(top, 'rev-parse', '--abbrev-ref', 'HEAD')}, {state_}")

    try:
        res = subprocess.run(["bd", "context", "--json"], cwd=main, capture_output=True, text=True)
    except FileNotFoundError:
        raise RecordError("bd is not installed; the work layer is required")
    ctx = json.loads(res.stdout) if res.returncode == 0 and res.stdout.strip().startswith("{") else None
    # bd context answers from config alone; an embedded database exists only once bootstrapped.
    if ctx and (ctx.get("dolt_mode") != "embedded" or (Path(ctx["beads_dir"]) / "embeddeddolt").is_dir()):
        profile = json.loads(bd(main, "config", "get", "agent.profile", "--json"))["value"]
        profile += "" if profile == "team-maintainer" else f"; run {SETUP}"
        out.append(f"beads     {ctx['beads_dir']}  database {ctx['database']}, remote {ctx.get('sync_remote') or 'none'}"
                   f"; ahead/behind not reported by bd; agent profile {profile}")
    else:
        out.append(f"beads     {main / '.beads'}  no database; run {SETUP}")

    hooks_path = g(main, "config", "--get", "core.hooksPath")
    if hooks_path:
        hooks = (main / hooks_path).resolve()
        have = ", ".join(f"{h} {'installed' if os.access(hooks / h, os.X_OK) else 'missing'}"
                         for h in ("post-checkout", "pre-commit"))
        out.append(f"hooks     {hooks}  {have}")
    else:
        out.append(f"hooks     core.hooksPath unset; run {SETUP}")

    home = codex_home()
    if not home.is_dir():
        out.append(f"codex     {home}  missing, so Codex is not used here")
    elif not store.is_dir():
        out.append(f"codex     {home / 'config.toml'}  writable_roots not checked without the store; run {SETUP}")
    else:
        path = home / "config.toml"
        try:
            have = codex_config(path)[2]
            missing = [r for r in codex_roots(main) if r not in have]
            state_ = ("writable_roots set" if not missing
                      else f"writable_roots missing {', '.join(missing)}; run {SETUP}")
        except Refuse as e:
            state_ = f"unreadable: {e}"
        out.append(f"codex     {path}  {state_}")

    out.append(service.health(main, store)[1])
    out += pushjob.describe(main)
    out.append(f"site      {site_url()} (served by the pm service)")
    return "\n".join(out)


def cmd_commit(args, records: Path) -> str:
    """Commit the caller's own hand edits in the store, named by path, once the records branch with them committed
    renders. Other changes in the store, such as another session's edit in progress, stay uncommitted and are not
    checked, so they neither block the commit nor can it depend on them."""
    message = args.message.strip()
    if not message:
        raise Refuse("-m is empty; say what the hand edit changed")
    dirty = subprocess.run(["git", "status", "--porcelain", "--untracked-files=all"], cwd=records, check=True,
                           capture_output=True, text=True).stdout.rstrip("\n")  # keep each line's status columns
    if not dirty:
        raise Refuse(f"nothing to commit in {records}")
    if not args.paths:
        listing = "\n".join(f"  {line[:3]}records/{line[3:]}" for line in dirty.splitlines())
        raise Refuse("name the records you edited: pm commit -m \"…\" <path>…; other sessions' edits may be in "
                     f"the store too. Uncommitted now:\n{listing}")
    changed = {line[3:].split(" -> ")[-1].strip('"') for line in dirty.splitlines()}
    store = records.resolve()
    paths = []
    for name in args.paths:
        given = Path(name)
        top, _, under = name.partition("/")
        if not given.is_absolute() and top == "records":
            given = records / under  # records/... from any worktree, its link may not exist yet
        path = (Path.cwd() / given).resolve() if not given.is_absolute() else given.resolve()
        try:
            inside = path.relative_to(store).as_posix()
        except ValueError:
            raise Refuse(f"{name} is not in the records store {records}")
        if inside == ".":
            raise Refuse(f"{name} is the records store itself; name the records you edited: pm commit -m \"…\" "
                         "records/<…>.md…")
        if inside not in changed:
            raise Refuse(f"{name} has no uncommitted change")
        paths.append(path)
    # Check the records branch as this commit leaves it: HEAD plus the named records, nothing else uncommitted.
    root = code_root(Path.cwd(), records)
    changes = {p: (p.open(newline="").read() if p.exists() else None) for p in paths}
    for p in paths:
        if p.name.endswith(".summary.json") and p.exists():
            read_summary(p)
    check_records(committed_records(records, changes), load_beads(root), root.name)
    head = commit(records, message, paths)
    return f"committed {', '.join('records/' + p.relative_to(store).as_posix() for p in paths)} as {head}: {message}"


# ---------------------------------------------------------------- entry point

# Commands that read stdin; main reads it before the lock. Others leave stdin unread, so an open one never blocks them.
READS_STDIN = {"decision add", "decision need", "decision close", "action need", "doc new", "project open",
               "sprint open", "task add", "task move", "feedback add"}
WRITES = {"finding add", "feedback add", "decision add", "decision need", "decision close", "action need", "action done",
          "doc new", "design new", "postmortem new", "project open", "sprint open", "sprint close", "task add", "task close",
          "task move", "commit"}

# `pm service --help`: the service's whole context, which pm prime only points at.
SERVICE_HELP = f"""\
The pm service: one supervised background process per clone, `pm service run` in the main checkout. It
serves the site live from the records store and Beads (a page is at most {SERVE_BEHIND} s behind them), delivers
the owner's site replies and reviewed PRs' merges (GitHub polled every {MERGE_POLL} s) into the sessions that
raised them, and pushes Beads data (bd dolt push), today's summary and the records branch every
{pushjob.INTERVAL} s, the first push {pushjob.INTERVAL} s after it starts. Sessions push neither.

Supervisor: it starts the service at login and again after a crash. A launchd agent with KeepAlive on
macOS (~/Library/LaunchAgents/local.pm.<dir>.<hash>.plist); a systemd user service with Restart=always
on Linux ($XDG_CONFIG_HOME/systemd/user/local.pm.<dir>.<hash>.service, ~/.config by default). On a
machine with neither there is no service, and install refuses.

Unit: runs the pm uv tool's interpreter (`<tool python> -m pm.cli service run`; pm init installs the
tool) in the main checkout, with the PATH install ran with (bd, git and uv must be on it) and PORT. The tool
runs the version the main checkout pins, through uv when it is another.

Port: $PORT, else the installed unit's port, else `port` in {config.REL}. Installing again keeps the unit's
port. A second clone of the repo on this machine needs its own: PORT=<n> pm service install.

State and logs, in <main checkout>/{config.RUN}/ (never committed): service.log (pm service logs), push.json
(each push step's last outcome), push.log (a line per step per run), push.lock (one push at a time).

Health (pm service status, pm where): the supervisor holds the unit, and the site answers on its port
with {service.SERVE_HEADER} naming this clone's store and {service.VERSION_HEADER} naming this pm's build. Status also
flags a push step that failed or has not succeeded for {pushjob.OVERDUE} s.

Stale build: every {SERVE_CHECK:g} s the service rereads the pin in {config.REL}; once it pins another version (a
pull after pm upgrade) the service exits and the supervisor starts the pm uv tool again, which runs the new
pin. A service that answers on another build than the running pm, the pinned one, is stale in pm where, pm
service status and pm doctor. Fix: pm init (it installs the pm uv tool at this build unless the tool launched
it, then the service); when the tool already runs this build, pm service install or pm service restart.

Agents: pm prime carries pm where's service line and push state, and pm show warns when a push needs
attention. When the service is down, run pm service restart; if that fails, raise an action for the
owner (pm action need) and add a bug task (pm task add). pm init installs the service; pm uninstall stops
it and removes its unit."""


def parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(prog="pm", description=__doc__.strip().splitlines()[0],
                                 epilog=f"The pm uv tool runs the pm version the repo pins in {config.REL}, through uv "
                                        "when it is another; pm where names the version running and why.")
    sub = ap.add_subparsers(dest="cmd", required=True)

    s = sub.add_parser("show", help="compact status of open projects for agents")
    s.add_argument("--json", action="store_true", help="the same data as one JSON object")
    s.add_argument("--sprint", metavar="ID", help="one sprint's frame, findings and tasks")
    s.add_argument("--record", metavar="PATH", help="with --section: the record to read (a path, sprint id, "
                   "project name or design slug)")
    s.add_argument("--section", metavar="NAME", help="with --record: print that one section, heading included")
    s.add_argument("--refresh-inbox", action="store_true", help="first point this session's open requests at its "
                   f"current inbox (${INBOX_ENV}); the session-start hook passes it, since a resumed session binds a "
                   "new one")
    s.set_defaults(func=cmd_show)

    day = sub.add_parser("day", help="day pages: generated from the day's activity; nobody writes a day record").add_subparsers(
        dest="sub", required=True)
    s = day.add_parser("summarize", help="generate today's Today summary with claude -p; the pm service runs it",
                       description="Summarize today's activity (what the day page shows, plus the records committed "
                                   f"today) with claude -p --model {SUMMARY_MODEL} into records/days/<today>.summary.json"
                                   ", committed on the records branch; the day page shows it as Today, labelled with "
                                   "the time it was generated. Skips when nothing happened today or the activity's "
                                   "digest is unchanged since the last summary. Fails, writing nothing, when claude "
                                   "is missing or fails. The pm service runs it every 10 minutes.")
    s.add_argument("--dry-run", action="store_true", help="print the summary it would write; write nothing")
    s.set_defaults(func=cmd_day_summarize)

    finding = sub.add_parser("finding", help="sprint findings").add_subparsers(dest="sub", required=True)
    s = finding.add_parser("add", help="append a bullet to a sprint's Findings")
    s.add_argument("--sprint", required=True, metavar="ID", help="the sprint's Beads id")
    s.add_argument("text", nargs="+")
    s.set_defaults(func=cmd_finding_add)

    feedback = sub.add_parser("feedback", help="feedback on pm itself").add_subparsers(dest="sub", required=True)
    s = feedback.add_parser(
        "add", help="append an entry to the project's pm feedback doc; text with --text or on stdin",
        description="When pm got in the way (a confusing refusal, a missing command, a rule that cost time), say "
                    "once what happened and what would have helped. Appends a dated entry with this session's id to "
                    "records/docs/<date of first use>-<project>-feedback.md, creating it on first use.")
    s.add_argument("--project", required=True, metavar="NAME", help="the project the feedback doc belongs to")
    s.add_argument("--sprint", metavar="ID", help="the sprint the feedback is about")
    s.add_argument("--task", metavar="ID", help="the task the feedback is about")
    s.add_argument("--text", help="the feedback; default: stdin")
    s.add_argument("--session", metavar="ID", help="the session to record; default: this session's id from the environment")
    s.set_defaults(func=cmd_feedback_add)

    decision = sub.add_parser("decision", help="decisions: record one, or ask the owner for one").add_subparsers(
        dest="sub", required=True)
    s = decision.add_parser(
        "add", help="append a decision to a project's or sprint's Decisions; body on stdin",
        description="Append a ::: decision block, dated today, to the Decisions of the named project or "
                    "sprint. The body on stdin states the decision and, on the next line, its reason. "
                    f"Choosing --level: {LEVEL_RULE}. Source is agent unless --need or --confirmed. With --need, "
                    "the body is the owner's answer to that decision need: it ends 'Answers `<need-id>`.', and the "
                    "need is closed with bd human respond and the same text unless the owner already closed it.")
    s.add_argument("--level", choices=["project", "sprint"], help=f"required, no default: {LEVEL_RULE}")
    s.add_argument("--project", metavar="NAME", help="the project record name (with --level project)")
    s.add_argument("--sprint", metavar="ID", help="the sprint's Beads id (with --level sprint)")
    s.add_argument("--until", help="a known condition to revisit the decision")
    owner = s.add_mutually_exclusive_group()
    owner.add_argument("--need", metavar="ID", help="source=owner: the decision answers this decision need, and "
                                                    "closes it if it is open")
    owner.add_argument("--confirmed", action="store_true", help="source=owner: the owner confirmed it")
    s.set_defaults(func=cmd_decision_add)
    s = decision.add_parser(
        "need", help="ask the owner for a decision under a sprint or task; description on stdin",
        description="Raise a decision need: a Beads task labelled human under a sprint or task. The description "
                    "comes from stdin, one part per line: one 'Question:', one or more 'Fact:', two or more "
                    "'Option <label>:' each followed by its 'Cost:' line, and one 'Default:' that names the label "
                    "taken if the owner does not answer, then its reason. Send it with a quoted heredoc (<<'EOF'), "
                    "so code spans stay. pm writes the description in one Markdown layout and refuses an option "
                    "without a cost, a default that names no option, and a sentence of more than 25 words. Record "
                    "the answer with pm decision add --need, or close a small answer with pm decision close.")
    s.add_argument("--title", required=True)
    s.add_argument("--parent", required=True, metavar="ID", help="the sprint or task the decision belongs to")
    s.set_defaults(func=cmd_decision_need)
    s = decision.add_parser(
        "close", help="close a decision need whose answer sets no rule, with no decision record; the answer on stdin",
        description="Close the need with bd human respond, the owner's answer and the reason, and label it "
                    "no-decision; on a need the owner already closed, only the label and the reason (as a comment) "
                    "are added, and no answer is needed. An answer that sets a rule is recorded with pm decision add "
                    "--need instead. Answers that set no rule: a name, a port, which of two equal files. If you are not "
                    "sure, record a decision. If labelling fails, run it again; it does not repeat the reason.")
    s.add_argument("need_id", help="the decision need's Beads id")
    s.add_argument("--reason", required=True, help="why the answer sets no rule, in a sentence")
    s.set_defaults(func=cmd_decision_close)

    action = sub.add_parser("action", help="actions: ask the owner to do something (run, apply), or to review a PR (--pr)"
                            ).add_subparsers(dest="sub", required=True)
    s = action.add_parser(
        "need", help="ask the owner to do something under a sprint or task, or to review a PR (--pr); "
                     "description on stdin",
        description="Raise an action: a Beads task labelled human and action under a sprint or task. The "
                    "description on stdin says what to do and why. With --pr it is a PR review instead: every sprint "
                    "named must be open with its delivery report written (Outcome and 'Against \"Done when\"') and "
                    "committed; the review goes under the first and blocks its close until the PR merges and you "
                    "close the review with pm action done <id> --reason \"merged as <sha>\"; "
                    "the site's card links the PR, each sprint's record and delivery report, and the design "
                    "pages named with --design plus those the sprints' records list, and shows the focus; stdin "
                    "then holds optional extra context. Close it with pm action done once you see it done.")
    s.add_argument("--title", help="required without --pr; with --pr, default: Review PR #<n>")
    s.add_argument("--parent", metavar="ID", help="the sprint or task the action belongs to (required without --pr; "
                   "not allowed with it)")
    s.add_argument("--pr", metavar="URL", help="the pull request to review; requires --sprint and --focus")
    s.add_argument("--sprint", action="append", metavar="ID",
                   help="with --pr: an open sprint the PR delivers, its report written (repeatable)")
    s.add_argument("--focus", help="with --pr: what to look at first: risky changes, open choices")
    s.add_argument("--design", action="append", metavar="SLUG",
                   help="with --pr: a design page behind the PR (repeatable, optional)")
    s.set_defaults(func=cmd_action_need)
    s = action.add_parser("done", help="close an action once you see the owner did it")
    s.add_argument("need_id", help="the action's Beads id")
    s.add_argument("--reason", required=True, help="what showed you it is done (a merged PR, a command's output)")
    s.set_defaults(func=cmd_action_done)

    reply = sub.add_parser("reply", help="the owner's replies from the site").add_subparsers(dest="sub", required=True)
    s = reply.add_parser(
        "read", help="print the owner's replies and reviews' merges that did not reach your session",
        description="Print the requests' site replies not yet delivered and their reviews' PR merges not yet "
                    "reported, with what to do next, and mark them delivered; it does not wait. The pm service pushes "
                    "each reply and merge into the inbox of the session that raised the request when it can; what "
                    "it could not deliver (the session had ended, or has no inbox) waits here and is flagged by pm "
                    f"show. Without ids, this session's open requests (${SESSION_ENV}); with ids, those, closed "
                    "ones too.")
    s.add_argument("ids", nargs="*", metavar="ID", help="a decision need, action or review (default: this session's)")
    s.set_defaults(func=cmd_reply_read)

    doc = sub.add_parser("doc", help="free-form dated docs").add_subparsers(dest="sub", required=True)
    s = doc.add_parser("new", help="create records/docs/<today>-<slug>.md; body on stdin")
    s.add_argument("slug", help="what the doc is for, lowercase words joined by '-'")
    s.add_argument("--title", required=True)
    target = s.add_mutually_exclusive_group(required=True)
    target.add_argument("--bead", metavar="ID", help="the sprint or task the doc belongs to")
    target.add_argument("--project", metavar="NAME", help="the project the doc belongs to")
    s.set_defaults(func=cmd_doc_new)

    design = sub.add_parser("design", help="design pages").add_subparsers(dest="sub", required=True)
    s = design.add_parser("new", help="create records/design/<slug>.md with every template section",
                          description="Create a design page with every section of the template, each with "
                                      "its prompt line and \"None yet.\"; then edit it by hand. A page covers one "
                                      "area and holds its final state; decisions and plans go in the project or "
                                      "sprint record. Put no date in the slug. When a page grows to cover several "
                                      "areas, split it into sub pages and keep a short summary per area linking them.")
    s.add_argument("slug", help="what the design is, lowercase words joined by '-'")
    s.add_argument("--title", required=True)
    s.add_argument("--project", required=True, metavar="NAME", help="the project the design belongs to")
    s.set_defaults(func=cmd_design_new)

    postmortem = sub.add_parser("postmortem", help="incident postmortems").add_subparsers(dest="sub", required=True)
    s = postmortem.add_parser("new", help="create records/postmortems/<today>-<slug>.md with every template section",
                              description="Create a postmortem with every section of the template, each with its "
                                          "prompt line and \"None yet.\"; then write it by hand. Due for an "
                                          "incident that cost more than a day, or broke other sessions or the "
                                          "owner's view. Write it once the incident is fixed.")
    s.add_argument("slug", help="what broke, lowercase words joined by '-'")
    s.add_argument("--title", required=True)
    target = s.add_mutually_exclusive_group(required=True)
    target.add_argument("--sprint", metavar="ID", help="the sprint the incident hit, open or closed")
    target.add_argument("--project", metavar="NAME", help="the project the incident hit")
    s.set_defaults(func=cmd_postmortem_new)

    project = sub.add_parser("project", help="projects").add_subparsers(dest="sub", required=True)
    s = project.add_parser("open", help="create a project epic and record; Goal on stdin",
                           description="Create a project epic and its record, with the Goal from stdin. The owner "
                                       "confirms the goal in their own words before you open the project.")
    s.add_argument("name")
    s.add_argument("--title", required=True)
    s.set_defaults(func=cmd_project_open)
    s = project.add_parser("close", help="close a project epic at the committed records",
                           description="Close a project epic at the committed records. Close every sprint first. "
                                       "Write the record's '## Outcome' first, by hand: the results against the goal "
                                       "in numbers, what was learned, what was retired, and links to the sprints' "
                                       "delivery reports.")
    s.add_argument("name")
    s.set_defaults(func=cmd_project_close)

    sprint = sub.add_parser("sprint", help="sprints").add_subparsers(dest="sub", required=True)
    s = sprint.add_parser("open", help="create a sprint epic and record; frame on stdin "
                                       "('## Goal', '## Scope' with **In:**/**Out:**, '## Done when')")
    s.add_argument("project", help="project record name, e.g. pm-harness")
    s.add_argument("--title", required=True)
    s.set_defaults(func=cmd_sprint_open)
    s = sprint.add_parser("close", help="close a sprint epic once its report is written, every task is closed and "
                                        "each PR review is closed as merged",
                          description="Close a sprint epic at the committed records. Refuses until the Delivery report "
                                      "is written and every task is closed, and each PR review naming the sprint is "
                                      "closed with pm action done <id> --reason \"merged as <sha>\" once its PR is on "
                                      "main; it does not ask GitHub. With reviews, it stamps 'Merged as <sha> (PR #N).' "
                                      "into the Outcome after the verdict and commits it on the records branch; the "
                                      "close reason names the records commit. Only the Outcome's first paragraph "
                                      "becomes the close reason.")
    s.add_argument("sprint_id", help="the sprint's Beads id")
    s.set_defaults(func=cmd_sprint_close)

    task = sub.add_parser("task", help="tasks inside open sprints").add_subparsers(dest="sub", required=True)
    s = task.add_parser("add", help="create a task in an open sprint; description on stdin (optional)")
    s.add_argument("--sprint", required=True, metavar="ID", help="the open sprint's Beads id")
    s.add_argument("--title", required=True)
    s.set_defaults(func=cmd_task_add)
    s = task.add_parser("close", help="close a task with a reason naming its commit",
                        description="Close a non-epic task with bd close. The reason ends with "
                                    "'(commit <hash>)': HEAD if it was committed after the task started, "
                                    "or the commit given with --commit; with neither, a warning.")
    s.add_argument("task_id", help="the task's Beads id")
    s.add_argument("--reason", help="what was done (default: Done)")
    s.add_argument("--commit", metavar="REF", help="the commit holding the work (default: HEAD, if newer than the task)")
    s.set_defaults(func=cmd_task_close)
    s = task.add_parser("claim", help="claim a task for this agent session",
                        description="Claim an open task with bd update --claim and record the session "
                                    f"(${SESSION_ENV}, else ${CODEX_SESSION_ENV}) and the time in its metadata "
                                    "(claimed_by, claimed_at). Refuses when another live session holds it: one whose "
                                    f"transcript was written in the last {LIVE_WINDOW // 60} minutes. A subagent "
                                    "shares its session's id, so it may claim what its session holds.")
    s.add_argument("task_id", help="the task's Beads id")
    s.add_argument("--session", metavar="ID", help="the session to record when no session id is in the environment")
    s.set_defaults(func=cmd_task_claim)
    s = task.add_parser("move", help="move a task to another open sprint; reason on stdin",
                        description="Move an open task to another open sprint with bd update --parent and "
                                    "record the scope change as a source=agent decision in the sprint it "
                                    "leaves. The reason on stdin states why, on at least two lines.")
    s.add_argument("task_id", help="the task's Beads id")
    s.add_argument("--to", required=True, metavar="SPRINT_ID", help="the open sprint the task moves to")
    s.set_defaults(func=cmd_task_move)

    record = sub.add_parser("record", help="records on the served site").add_subparsers(dest="sub", required=True)
    s = record.add_parser(
        "link", help="print a record's page URL on the served site; the only link to give for a record",
        description="Print the URL of a record's page on the site the pm service serves on "
                    "localhost:$PORT (default: port in .pm/config.toml), printed with the repo's site_url "
                    "instead when .pm/config.toml sets one. The pm service's pages follow the records within 10 s and state "
                    "their data's age, so no render step is needed. Fails with the command that fixes it when nothing serves there, "
                    "or when what answers is not the pm service for this store.")
    s.add_argument("target", help="a sprint or project Beads id, a project name, a design slug, or a record path "
                                  "(records/<…>.md; records/ and .md optional)")
    s.set_defaults(func=cmd_record_link)

    s = sub.add_parser("check", help="check that every record renders with Beads, writing nothing; the check before a "
                                     "commit, and the one pm commit runs")
    s.set_defaults(func=cmd_check)

    svc = sub.add_parser("service", help="the pm service: one background process per clone serves the site and pushes "
                                         "Beads data and the records branch every 10 minutes",
                         description=SERVICE_HELP, formatter_class=argparse.RawDescriptionHelpFormatter
                         ).add_subparsers(dest="sub", required=True)
    s = svc.add_parser("install", help="install and start this clone's service (launchd on macOS, systemd on Linux), "
                                       "or update it; a no-op once installed and current",
                       description="Install the pm service under the machine's supervisor, which starts it at login "
                                   "and restarts it after a crash: a launchd agent with KeepAlive on macOS, a systemd "
                                   "user service on Linux; refused on a machine with neither. The unit runs the pm uv "
                                   "tool's interpreter (refused unless the tool runs this pm's build: run pm init) "
                                   "with the current PATH (bd and git must be on it) and serves on $PORT, else the "
                                   "port it was installed with, else the port in .pm/config.toml; give a second clone "
                                   "of the repo its own port once with PORT=<n> pm service install. Installing again "
                                   "rewrites a changed unit and restarts a service on another build. It waits "
                                   f"{service.RESTART_WAIT} s for the site to answer for this store and fails when it "
                                   "does not (another clone's service on the port, say). pm init runs it.")
    s.set_defaults(func=cmd_service_install)
    s = svc.add_parser("status", help="whether the service is up and its site answers for this store, and its pushes; "
                                      "non-zero when either needs attention",
                       description="Print the service's health line (as pm where does), each push step's last "
                                   "outcome, every push problem and the log's path. Up means the supervisor holds the "
                                   f"unit and the site answers on its port with {service.SERVE_HEADER} naming this "
                                   f"store and {service.VERSION_HEADER} naming this pm's build; a push problem is a "
                                   f"step whose last run failed or that has not succeeded for {pushjob.OVERDUE} s. "
                                   "Exits non-zero when the service is down, stale or a push needs attention; each "
                                   "line names the command that fixes it.")
    s.set_defaults(func=cmd_service_status)
    s = svc.add_parser("restart", help="restart the service and wait for its site to answer; when it fails, raise an "
                                       "action for the owner and add a bug task",
                       description="Restart the installed service, or load it when the supervisor does not hold it, "
                                   f"then wait {service.RESTART_WAIT} s for the site to answer for this store. The "
                                   "fix when pm where shows the service down. Refused when it is not installed (run "
                                   "pm service install); when it fails, read pm service logs, raise an action for the "
                                   "owner (pm action need) and add a bug task (pm task add).")
    s.set_defaults(func=cmd_service_restart)
    s = svc.add_parser("logs", help="print the end of the service's log, <main checkout>/.pm/run/service.log",
                       description="Print the end of <main checkout>/.pm/run/service.log, where the service writes "
                                   "stdout and stderr: a line per request, refresh and reply write with its timings, "
                                   "and any error that stopped it. The push's own log is .pm/run/push.log beside it.")
    s.add_argument("-n", "--lines", type=int, default=50, help="how many lines (default 50)")
    s.set_defaults(func=cmd_service_logs)
    s = svc.add_parser("run", help="the service's process, run by its supervisor: serve the site on localhost:$PORT "
                                   "and push every 10 minutes",
                       description="Serve the site on localhost:$PORT (default: port in .pm/config.toml): a page is at "
                                   "most 10 s behind the records and Beads and states its data's age; an open page "
                                   "never reloads itself but shows within ~10 s that newer data exists, loaded on "
                                   "reload. Every 10 minutes, the first 10 after start, run pm push. Exits once "
                                   ".pm/config.toml pins another pm version, so the supervisor starts the pm uv tool "
                                   "again, which runs the new pin. The supervisor runs it; run it by hand only to debug, or on another PORT.")
    s.set_defaults(func=cmd_serve)

    s = sub.add_parser("doctor", help="compare every managed piece with what this pm writes, and the clone and "
                                      "worktree with what pm init makes; report each difference, exit 1 on any")
    s.set_defaults(func=cmd_doctor)

    s = sub.add_parser("upgrade", help="move the pin in .pm/config.toml to this pm and rewrite every managed piece "
                                       "as it writes them (the fix pm doctor names for a changed one), removing the "
                                       "pre-package harness's; writes files and prints the commit to make, never commits")
    s.add_argument("--to", metavar="X", help="the version to move to: the running pm's (the default), or another, "
                                             "which the pm uv tool runs to make the move")
    s.set_defaults(func=cmd_upgrade)

    s = sub.add_parser("uninstall", help="remove pm's pieces from this worktree (hook entries, workflows, .gitignore "
                                         "block, pm's sections in .beads/hooks, .pm/) and the clone's and machine's "
                                         "setup (the store checkout, records/ links and sparse checkouts in every "
                                         "worktree, the pm service, this clone's Codex writable_roots (uv's cache, shared by every clone, stays), pm's lines in .git/info/exclude); keeps the records branch, "
                                         "records/ on the main branch and Beads; never commits")
    s.set_defaults(func=cmd_uninstall)

    s = sub.add_parser("init", help="install pm: the repo's files on first install (--site-url sets the public site "
                                    "link), then this clone, this worktree and the pm service (PORT=<n> sets its "
                                    "port, and on a first install the config's; PORT=<n> pm service install moves "
                                    "only the service's); session start runs it; never commits on the code branch",
                       description="Install pm, doing only what is missing. Repo, on first install only (no "
                                   ".pm/config.toml yet): write .pm/ (config.toml, README.md, .gitignore), pm's hook "
                                   "entries in .claude/settings.json and .codex/hooks.json, pm's marked sections in "
                                   ".beads/hooks/post-checkout and pre-commit, the workflows "
                                   ".github/workflows/pm-records-{guard,copy}.yml and pm's .gitignore lines; run bd "
                                   "init when the repo has no .beads/; create the records branch with an empty store "
                                   "and push it when the remote has none; print the commit to make. After that pm init "
                                   "leaves the repo's files alone: pm doctor reports a changed or missing piece and pm "
                                   "upgrade rewrites it. Clone and worktree, every run: install the pm uv tool at this "
                                   "build when it runs another, connect Beads (bd bootstrap), "
                                   "install the git hooks (core.hooksPath .beads/hooks), list .pm/store/ and .pm/run/ "
                                   "in .git/info/exclude, check out the records store at <main "
                                   "checkout>/.pm/store/records if missing, link this worktree's records/ to it and "
                                   "keep records/ out of its sparse checkout, add the clone's .git, the store, .beads "
                                   "and uv's cache to the writable roots of $CODEX_HOME/config.toml and the store to "
                                   "this worktree's .claude/settings.local.json, then install the pm service (pm "
                                   "service install). Session start runs it in every worktree; once all is set up it "
                                   "prints 'already set up'. Refuses a core.hooksPath other than .beads/hooks. Site "
                                   "port: a new repo's config gets $PORT, else the first free port from 8000 up that "
                                   "no pm service unit on this machine names; a clone's service serves on $PORT, else "
                                   "its unit's port, else the config's. Refuses, writing nothing, when another server "
                                   "holds that port, and names a free one: PORT=<n> pm init. .pm/config.toml, tracked: "
                                   "version (the pm every session must run; pm upgrade moves it), remote and "
                                   "main_branch (origin and its default branch), port (the site port) and site_url "
                                   "(--site-url).")
    s.add_argument("--session-start", action="store_true",
                   help="what session start runs, without $PORT: install the pm service only when it is missing, "
                        "and report an installed one that is stale or down instead of restarting it (pm service "
                        "restart does)")
    s.add_argument("--site-url", metavar="URL",
                   help="the site's public base URL (a tunnel to the pm service), written to site_url in "
                        ".pm/config.toml for you to commit: every link pm prints (pm record link, pm show, pm where) "
                        "uses it instead of http://localhost:<port>, and the site accepts the owner's replies from "
                        "its host besides localhost; '' clears it")
    s.set_defaults(func=cmd_init)

    s = sub.add_parser("push", help="push Beads data (bd dolt push), summarize today and push the records branch; "
                                    "what the pm service runs every 10 minutes, not a session command",
                       description="Push Beads data with bd dolt push, then summarize today (pm day summarize "
                                   "only when today's activity changed), "
                                   "then push the records branch when the store is "
                                   "ahead of <remote>/records: fetch, rebase onto it if it moved (under "
                                   "the store lock; a rebase that stops is aborted, leaving the store as it was), "
                                   f"push. Each step has a {pushjob.TIMEOUT}s timeout; a second run while one holds "
                                   "the lock exits at once. Each step's outcome goes to <clone>/.pm/run/push.json "
                                   "(read by pm show, pm where, pm service status and the site) and "
                                   "<clone>/.pm/run/push.log, a line per step.")
    s.set_defaults(func=cmd_push)

    s = sub.add_parser("where", help="list every location with its state: the store, this checkout, Beads, the "
                                     "hooks, the Codex sandbox roots, the pm service, the last push, and the site; 'pm where records' prints only the store's path")
    s.add_argument("what", nargs="?", choices=["records"], help="print only this location's path")
    s.set_defaults(func=cmd_where)

    s = sub.add_parser("prime", help="pm's rules, then pm init, pm where and pm show: the context a session starts "
                                     "with; the SessionStart hooks run --rules 1 to --rules "
                                     f"{len(hooks.STARTS)} and --state, and an agent may run it by hand")
    part = s.add_mutually_exclusive_group()
    part.add_argument("--rules", dest="part", type=int, choices=range(1, len(hooks.STARTS) + 1), metavar="N",
                      help=f"only chunk N of the rules and the command list, under a title naming its sections: "
                           f"one hook each on SessionStart and SubagentStart, since Claude Code passes a hook's text "
                           f"inline only up to 10,000 characters")
    part.add_argument("--state", dest="part", action="store_const", const="state",
                      help="only pm init, pm where and pm show, cut at a line to 10,000 characters: the last SessionStart hook")
    part.add_argument("--subagent", dest="part", action="store_const", const="subagent",
                      help="only the line naming the Beads agent profile: the last SubagentStart hook, beside the rules chunks")
    s.add_argument("--hook-json", action="store_true", help="read the SessionStart or SubagentStart input on stdin "
                   "and print the hook's JSON envelope, as Claude Code and Codex read it")

    hook = sub.add_parser("hook", help="what a runtime hook runs: the hook input JSON on stdin; installs nothing"
                          ).add_subparsers(dest="sub", required=True)
    hook.add_parser("stop", help="Stop: block once while records this session's tool calls name are uncommitted "
                                 "in the store")
    hook.add_parser("owner-request", help="Stop: block once while the reply asks the owner for something no open "
                                          "need or action of this session covers")
    s = hook.add_parser("git-post-checkout", help="git post-checkout (pm's section in .beads/hooks/post-checkout): in a "
                                                  "new worktree, run pm init's clone and worktree half, all but the pm service")
    s.add_argument("git_args", nargs="*", help="the hook's arguments: previous HEAD, new HEAD, branch flag")
    hook.add_parser("git-pre-commit", help="git pre-commit (pm's section in .beads/hooks/pre-commit): refuse staged "
                                           "records/ changes on a code branch, unless a merge is in progress")

    s = sub.add_parser("commit", help="commit your hand edits in the store, named by path, on the records branch",
                       description="Commit only the named records, so other sessions' uncommitted edits in the "
                                   "shared store are left alone. With no path, it lists what is uncommitted and "
                                   "commits nothing. A record uses only these fenced blocks: ::: decision, "
                                   "::: result and ```mermaid; put a one-line reading under each diagram or large table.")
    s.add_argument("-m", "--message", required=True, help="what the hand edit changed")
    s.add_argument("paths", nargs="*", metavar="PATH", help="a record you edited: records/<…>.md, a path in the "
                                                            "store, or relative to it from inside it")
    s.set_defaults(func=cmd_commit)
    return ap


def hook_git_post_checkout(git_args: list[str]) -> int:
    """`pm hook git-post-checkout`: in a new worktree (previous HEAD all zeros), run setup_clone. The store's own
    checkout, which setup_clone itself makes, is skipped. A failure is printed and exits 1, which git reports without
    undoing the checkout."""
    if not git_args or set(git_args[0]) != {"0"}:
        return 0
    try:
        cwd = Path.cwd()
        if Path(store_git(cwd, "rev-parse", "--show-toplevel")).resolve() == store_path(cwd).resolve():
            return 0
        print(setup_clone())
    except (Refuse, RecordError) as e:
        print(f"pm hook git-post-checkout: {e}", file=sys.stderr)
        return 1
    return 0


def main(argv: list[str] | None = None) -> int:
    argv = sys.argv[1:] if argv is None else argv
    code = launch.launch(argv)  # the pm uv tool runs the repo's pinned version: exec'd, unless it is this one
    if code is not None:
        return code
    args = parser().parse_args(argv)
    name = f"{args.cmd} {getattr(args, 'sub', '')}".strip()
    try:  # every command, hooks included, fails hard without the repo's config or on another pinned version; pm init
        # writes a missing one, and pm upgrade moves the pin
        if args.cmd != "upgrade" and not (args.cmd == "init" and not (config.root(Path.cwd()) / config.REL).is_file()):
            config.load(Path.cwd())
    except config.ConfigError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    if args.cmd == "prime":  # past the config check, hooks fail open and need no store, so they run before pm looks for one
        return hooks.cmd_prime(args.part, args.hook_json)
    if args.cmd == "hook" and args.sub == "git-post-checkout":
        return hook_git_post_checkout(args.git_args)
    if args.cmd == "hook":
        return hooks.HOOKS[args.sub]()
    try:
        if args.func in (cmd_init, cmd_upgrade, cmd_uninstall):
            print(args.func(args))
            return 0
        if args.func is cmd_doctor:
            code, said = cmd_doctor(args)
            print(said)
            return code
        if args.func is cmd_push:
            code, said = cmd_push(args, None)  # finds the store itself: a missing store is a recorded failure
            print(said)
            return code
        if args.func is cmd_where and args.what is None:
            print(where_all())
            return 0
        records = find_store(Path.cwd())
        reads = name in READS_STDIN and not (name == "feedback add" and args.text is not None)
        args.stdin = stdin_text() if reads else ""
        if name in WRITES:
            fd = locked(records)
            try:
                out = args.func(args, records)
            finally:
                os.close(fd)
        else:
            out = args.func(args, records)
    except (Refuse, RecordError) as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    code, out = out if isinstance(out, tuple) else (0, out)  # a command with its own exit code returns both
    print(out)
    return code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
