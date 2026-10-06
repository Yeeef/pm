"""The work layer: `bd` subprocess calls and status derived from Beads JSON."""

from __future__ import annotations

import json
import os
import re
import subprocess
from pathlib import Path

from .records import RecordError


def bd(repo: Path, *args: str) -> str:
    """Run `bd` in the repo and return stdout; a failure is a RecordError carrying bd's message."""
    try:
        res = subprocess.run(["bd", *args], cwd=repo, capture_output=True, text=True)
    except FileNotFoundError:
        raise RecordError("bd is not installed; the work layer is required")
    if res.returncode != 0:
        raise RecordError(f"bd {' '.join(args)} failed: {(res.stderr or res.stdout).strip()}")
    return res.stdout


def load_beads(repo: Path) -> dict[str, dict]:
    try:
        out = subprocess.run(["bd", "list", "--all", "--json"], cwd=repo, check=True,
                             capture_output=True, text=True).stdout
    except FileNotFoundError:
        raise RecordError("bd is not installed; the work layer is required to render status")
    except subprocess.CalledProcessError as e:
        raise RecordError(f"bd list --all --json failed: {(e.stderr or e.stdout).strip()}")
    beads = {i["id"]: i for i in json.loads(out)}
    attach_comments(repo, beads)
    return beads


# Comments read for open requests, by (issue id, comment count): comments on a request are only ever added, so a
# long-lived pm serve rereads them (one `bd export`, since bd list carries only the count) only when a count moved.
def reply_in_beads(beads: dict[str, dict] | None, issue_id: str, rid: str) -> bool:
    """Whether `beads` (a snapshot's issues) already shows site reply `rid` on its card: a comment on the issue ends
    in its REPLY_MARK, or the issue is gone or closed, so its card shows no thread to wait for."""
    issue = (beads or {}).get(issue_id)
    if issue is None or issue["status"] == "closed":
        return True
    mark = REPLY_MARK.format(rid)
    return any(mark in (c.get("text") or "") for c in issue.get("comments") or [])


_COMMENTS: dict[tuple[str, int], list[dict]] = {}


def attach_comments(repo: Path, beads: dict[str, dict]) -> None:
    """Give each open `human` issue with comments its `comments`, oldest first, for its card on the site."""
    want = {i["id"]: i["comment_count"] for i in beads.values()
            if HUMAN in (i.get("labels") or []) and i["status"] != "closed" and i.get("comment_count")}
    if any((iid, n) not in _COMMENTS for iid, n in want.items()):
        rows = {r["id"]: r for r in map(json.loads, bd(repo, "export").splitlines()) if r.get("id") in want}
        for iid, n in want.items():
            _COMMENTS[(iid, n)] = sorted(rows.get(iid, {}).get("comments") or [],
                                         key=lambda c: c.get("created_at") or "")
    for iid, n in want.items():
        beads[iid]["comments"] = _COMMENTS[(iid, n)]


def show_beads(repo: Path, ids: list[str]) -> dict[str, dict]:
    """Only the given issues, by id, read with one `bd show`; an id bd does not know is absent (all unknown fails)."""
    return {i["id"]: i for i in json.loads(bd(repo, "show", *ids, "--json"))}


def dolt_store(repo: Path) -> Path | None:
    """The chunk store (`.dolt/noms`) of the embedded Dolt database bd uses from `repo`, or None when bd talks to a
    Dolt server, whose files are not here to watch."""
    ctx = json.loads(bd(repo, "context", "--json"))
    if ctx.get("dolt_mode") != "embedded":
        return None
    noms = Path(ctx["beads_dir"]) / "embeddeddolt" / ctx["database"] / ".dolt" / "noms"
    if not noms.is_dir():
        raise RecordError(f"bd uses an embedded Dolt database, but its store {noms} does not exist")
    return noms


def dolt_state(noms: Path) -> tuple:
    """A fingerprint of the Beads database that changes on every write and costs a few stat calls: Dolt appends each
    write to its chunk journal, so a file grows, and a garbage collection rewrites the manifest. Sizes only, because
    a bd read also touches the files' mtimes."""
    sizes = sorted((e.path, e.stat().st_size) for d in (noms, noms / "oldgen") for e in os.scandir(d) if e.is_file())
    return (noms / "manifest").read_bytes(), tuple(sizes)

def blockers(issue: dict) -> list[str]:
    return [d["depends_on_id"] for d in issue.get("dependencies") or [] if d.get("type") == "blocks"]


def ancestors(beads: dict[str, dict], issue_id: str) -> list[str]:
    """Parent, grandparent, … of an issue, nearest first."""
    out, p = [], beads.get(issue_id, {}).get("parent")
    while p and p not in out:
        out.append(p)
        p = beads.get(p, {}).get("parent")
    return out


def children(beads: dict[str, dict], parent: str) -> list[dict]:
    return sorted((i for i in beads.values() if i.get("parent") == parent), key=lambda i: i["id"])


def state(issue: dict, beads: dict[str, dict]) -> str:
    """One of done, running, blocked, ready."""
    if issue["status"] == "closed":
        return "done"
    if issue["status"] == "in_progress":
        return "running"
    if issue["issue_type"] == "epic":
        # An open epic is running once any of its children has started or finished.
        kids = [i for i in beads.values() if i.get("parent") == issue["id"]]
        if any(k["status"] in ("in_progress", "closed") for k in kids):
            return "running"
    if any(beads.get(b, {}).get("status") != "closed" for b in blockers(issue)):
        return "blocked"
    return "ready"


HUMAN = "human"              # waits on the owner; `bd human list` shows it
ACTION = "action"            # with human: the owner does something, and the agent closes it once it sees it done
NO_DECISION = "no-decision"  # on a closed human decision: answered, and the answer sets no rule to record
REPLY_AUTHOR = "owner (site reply)"  # the author of the Beads comment that holds a site reply
PICKED = "picked_up"         # metadata on a human issue: how many of its site replies reached its session
MERGED = "merged"            # metadata on a review: the merge commit on main pm serve saw for its PR
MERGE_REPORTED = "merge_reported"  # metadata on a review: the merge commit that reached its session
REPLY_MARK = "<!-- pm-reply {} -->"  # the last line of a site reply's comment: its reply id, so a retry never doubles it
REPLY_ID = re.compile(r"[A-Za-z0-9-]{1,64}")  # a reply id as the site's form sends it: crypto.randomUUID()


def reply_body(text: str) -> str:
    """A site reply's comment text as the owner wrote it, without its REPLY_MARK line."""
    return re.sub(r"\s*<!-- pm-reply [A-Za-z0-9-]+ -->\s*$", "", text)


def kind(issue: dict) -> str:
    """What a `human` issue waits for: an action by the owner, or (the default, and every older need) a decision."""
    return "action" if ACTION in (issue.get("labels") or []) else "decision"


def session_of(issue: dict) -> str | None:
    """The agent session that raised a `human` issue, as `pm decision need` and `pm action need` store it."""
    meta = issue.get("metadata")
    sid = meta.get("session") if isinstance(meta, dict) else None
    return sid if isinstance(sid, str) and sid else None


def picked_up(issue: dict) -> int:
    meta = issue.get("metadata")
    value = meta.get(PICKED, 0) if isinstance(meta, dict) else 0
    if isinstance(value, int) or (isinstance(value, str) and value.isdigit()):
        return int(value)
    raise RecordError(f"{issue['id']} ({issue['title']}): metadata.{PICKED} is {value!r}, not a comment count; fix "
                      f"it with bd update {issue['id']} --set-metadata={PICKED}=<the comments already picked up>")


def merge_waiting(issue: dict) -> str | None:
    """The merge commit of a review's PR that pm serve saw but that has not reached the session, else None."""
    meta = issue.get("metadata") if isinstance(issue.get("metadata"), dict) else {}
    sha = meta.get(MERGED)
    return sha if sha and meta.get(MERGE_REPORTED) != sha else None


def site_replies(comments: list[dict]) -> list[dict]:
    """The owner's site replies among a request's comments, oldest first: those pm serve wrote, by REPLY_AUTHOR. Any
    other comment (pm's own as it closes a request, an agent's bd comments add) is no reply."""
    return [c for c in comments if c.get("author") == REPLY_AUTHOR]


def reply_waiting(issue: dict) -> bool:
    """An open request holds a site reply not delivered yet: more site replies than its picked_up count. It needs
    the request's comments (attach_comments gives every open request its own)."""
    if issue["status"] == "closed":
        return False
    seen = picked_up(issue)  # read even without comments, so a bad count is an error wherever it is
    if not issue.get("comment_count"):
        return False
    if "comments" not in issue:
        raise RecordError(f"{issue['id']}: its comments were not read, so pm cannot tell whether a reply waits")
    return len(site_replies(issue["comments"])) > seen


def owner_tasks(beads: dict[str, dict], under: str) -> list[dict]:
    """Open issues under a project bead that are flagged for a human (`human` label,
    as `bd human list` reads it): what awaits the owner, decisions and actions alike.
    Assignment alone does not count, because agents claim work under the owner's identity."""
    def inside(i):
        p = i.get("parent")
        while p:
            if p == under:
                return True
            p = beads.get(p, {}).get("parent")
        return False
    return [i for i in beads.values()
            if i["status"] != "closed" and HUMAN in (i.get("labels") or []) and inside(i)]
