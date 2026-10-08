"""Work-store items from bd issues: how the tests read work data from the Python pm, whose store is (fake) bd.

The mapping is the work store's import from bd, as the work-store design page's "Field mapping" gives it; the Go
importer must agree with it. Every item carries every field, null or empty where it does not apply. Like the import,
it fails hard on what it cannot map, and maps nothing by guess. One case the import has no row for: Python pm shows
an in_progress issue without `claimed_by` (a claim made without pm) as held without a session, so its holder here
has session null."""

from __future__ import annotations

import re

FIELDS = ("id", "type", "title", "description", "status", "resolution", "close_reason", "number", "parent",
          "blocked_by", "labels", "holder", "started_at", "created_at", "updated_at", "closed_at", "closed_by",
          "comments", "need")
MAPPED = {"id", "issue_type", "title", "description", "notes", "status", "close_reason", "parent", "dependencies",
          "labels", "metadata", "external_ref", "started_at", "created_at", "updated_at", "closed_at", "comments"}
DROPPED = {"priority", "owner", "created_by", "assignee", "lease_expires_at", "heartbeat_at", "dependency_count",
           "dependent_count", "comment_count", "design", "acceptance_criteria", "_type"}
METADATA = {"claimed_by", "claimed_at", "session", "inbox", "inbox_host", "picked_up", "review", "merged",
            "merge_reported", "probe"}
NEED_ONLY = {"session", "inbox", "inbox_host", "picked_up", "review", "merged", "merge_reported"}
FLAG_LABELS = {"human", "action", "no-decision"}  # fields now: type, need.kind, resolution
STATUS = {"open": "open", "in_progress": "open", "closed": "closed"}


class MapError(ValueError):
    pass


def items(issues: list[dict]) -> dict[str, dict]:
    """Every bd issue (as `bd export` or the fake bd's state holds it) as a work-store item, by id."""
    by_id = {i["id"]: i for i in issues}
    out = {i["id"]: item(i, by_id) for i in issues}
    for i in out.values():
        for ref in ([i["parent"]] if i["parent"] else []) + i["blocked_by"]:
            if ref not in out:
                raise MapError(f"{i['id']}: dangling reference {ref}")
    return out


def item(issue: dict, by_id: dict[str, dict]) -> dict:
    iid = issue["id"]
    if unknown := set(issue) - MAPPED - DROPPED:
        raise MapError(f"{iid}: unknown bd keys {sorted(unknown)}")
    meta = issue.get("metadata") or {}
    if unknown := set(meta) - METADATA:
        raise MapError(f"{iid}: unknown metadata keys {sorted(unknown)}")
    deps = issue.get("dependencies") or []
    if odd := {d["type"] for d in deps} - {"parent-child", "blocks"}:
        raise MapError(f"{iid}: unknown dependency types {sorted(odd)}")
    parent = issue.get("parent") or next((d["depends_on_id"] for d in deps if d["type"] == "parent-child"), None)
    labels = set(issue.get("labels") or [])
    bd_type = issue["issue_type"]
    if bd_type == "epic":
        up = by_id.get(parent, {}) if parent else {}
        if parent and (up.get("issue_type") != "epic" or up.get("parent")):
            raise MapError(f"{iid}: an epic under {parent}, which is no project")
        kind = "sprint" if parent else "project"
    elif bd_type in ("task", "bug"):
        kind = "need" if "human" in labels else "task"
    else:
        raise MapError(f"{iid}: unknown issue_type {bd_type}")
    if issue["status"] not in STATUS:
        raise MapError(f"{iid}: unknown status {issue['status']}")
    status = STATUS[issue["status"]]
    if kind in ("task", "need") and not parent and status == "open":
        raise MapError(f"{iid}: an open {kind} with no parent")
    reason = issue.get("close_reason")
    resolution = None
    if status == "closed":
        resolution = ("no-decision" if "no-decision" in labels else "answered" if (reason or "").startswith("Responded")
                      else "dismissed" if (reason or "").startswith("Dismissed") else "done")
    number = None
    if kind == "sprint":
        m = re.match(r"Sprint (\d+): ", issue["title"])
        if not m:
            raise MapError(f"{iid}: a sprint whose title does not start 'Sprint N: '")
        number = int(m.group(1))
    if kind != "need" and (used := set(meta) & NEED_ONLY):
        raise MapError(f"{iid}: need metadata {sorted(used)} on a {kind}")
    description = issue.get("description") or ""
    if issue.get("notes"):
        description = f"{description}\n\n## Notes\n\n{issue['notes']}" if description else f"## Notes\n\n{issue['notes']}"
    holder = None
    if issue["status"] == "in_progress":
        holder = {"session": meta.get("claimed_by"), "host": None, "claimed_at": meta.get("claimed_at")}
    out = {
        "id": iid, "type": kind, "title": issue["title"], "description": description, "status": status,
        "resolution": resolution, "close_reason": reason, "number": number, "parent": parent,
        "blocked_by": [d["depends_on_id"] for d in deps if d["type"] == "blocks"],
        "labels": sorted(labels - FLAG_LABELS | ({"bug"} if bd_type == "bug" else set())),
        "holder": holder, "started_at": issue.get("started_at"), "created_at": issue.get("created_at"),
        "updated_at": issue.get("updated_at"), "closed_at": issue.get("closed_at"),
        "closed_by": meta.get("claimed_by") if status == "closed" else None,
        "comments": [comment(c) for c in issue.get("comments") or []],
        "need": need(issue, meta, labels) if kind == "need" else None,
    }
    assert tuple(out) == FIELDS
    return out


def comment(c: dict) -> dict:
    reply = c["author"] == "owner (site reply)"
    return {"id": c.get("id"), "kind": "reply" if reply else "note", "author": "owner" if reply else c["author"],
            "text": c["text"], "created_at": c.get("created_at")}


def need(issue: dict, meta: dict, labels: set[str]) -> dict:
    kind = "review" if "review" in meta else "action" if "action" in labels else "decision"
    review = None
    if kind == "review":
        if odd := set(meta["review"]) - {"pr", "sprints", "designs", "focus"}:
            raise MapError(f"{issue['id']}: unknown review keys {sorted(odd)}")
        review ={"pr": None, "sprints": [], "designs": [], "focus": None, **meta["review"],
                  "merged": meta.get("merged"), "merge_reported": meta.get("merge_reported")}
        if issue.get("external_ref") not in (None, review["pr"]):
            raise MapError(f"{issue['id']}: external_ref {issue['external_ref']} is not the review's PR {review['pr']}")
    elif "merged" in meta or "merge_reported" in meta or issue.get("external_ref"):
        raise MapError(f"{issue['id']}: review data on a {kind} need")
    raised_by = ({"session": meta["session"], "inbox": meta.get("inbox"), "host": meta.get("inbox_host")}
                 if meta.get("session") else None)
    return {"kind": kind, "raised_by": raised_by, "delivered": int(meta.get("picked_up") or 0), "review": review}
