#!/usr/bin/env python3
"""A stand-in for `bd` in tests: serves issues from $FAKE_BD_STATE (a JSON list) and logs each call to $FAKE_BD_LOG.
A call whose arguments start with the JSON list in $FAKE_BD_FAIL fails without changing anything, until the path in
$FAKE_BD_HEAL exists; one whose arguments start with the list in $FAKE_BD_HOLD ([prefix, path]) waits until that path
exists. Comments are kept as bd 1.3.1 shows them (`bd comments <id> --json`), outside `bd list`; `bd human respond` adds "Response: <text>"."""

import json
import os
import subprocess
import sys
import time
from pathlib import Path

state_path, log_path = os.environ["FAKE_BD_STATE"], os.environ["FAKE_BD_LOG"]
args = sys.argv[1:]
with open(log_path, "a") as f:
    f.write(json.dumps(args) + "\n")
hold, gate = json.loads(os.environ.get("FAKE_BD_HOLD") or "[null, null]")
while hold and args[:len(hold)] == hold and not os.path.exists(gate):
    time.sleep(0.02)
issues = json.load(open(state_path))
opts = dict(a[2:].split("=", 1) for a in args if a.startswith("--") and "=" in a)
fail = json.loads(os.environ.get("FAKE_BD_FAIL") or "null")
heal = os.environ.get("FAKE_BD_HEAL")
if fail and args[:len(fail)] == fail and not (heal and os.path.exists(heal)):
    print(f"fake bd: failing {args} on purpose", file=sys.stderr)
    sys.exit(1)


def main_checkout() -> Path:
    """The clone's main checkout, whose .beads real bd uses from any worktree."""
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], check=True,
                            capture_output=True, text=True).stdout.strip()
    return Path(common).parent


def save() -> None:
    """Write the issues back; when the test made a Dolt store (Repo.dolt), grow its journal as a Dolt write does."""
    json.dump(issues, open(state_path, "w"))
    journal = main_checkout() / ".beads/embeddeddolt/demo/.dolt/noms/journal"
    if journal.parent.is_dir():
        with open(journal, "a") as f:
            f.write(json.dumps(args) + "\n")


def comment(issue_id: str, text: str, author: str = "t") -> None:
    for i in issues:
        if i["id"] == issue_id:
            i["comments"] = i.get("comments", []) + [{"issue_id": issue_id, "author": author, "text": text,
                                                      "created_at": "2026-10-03T12:00:00Z"}]


def flag(name: str) -> str | None:
    """The value of --name, given as --name=value or --name value."""
    for n, a in enumerate(args):
        if a == f"--{name}" and n + 1 < len(args):
            return args[n + 1]
        if a.startswith(f"--{name}="):
            return a.split("=", 1)[1]
    return None


def shown(i: dict) -> dict:
    """An issue as bd list and bd show print it: comments only as their count."""
    return {**{k: v for k, v in i.items() if k != "comments"}, "comment_count": len(i.get("comments", []))}


if args[:1] == ["list"]:
    # Like bd: closed issues only with --all; --label and --metadata-field key=value narrow the rows.
    label, field = flag("label"), flag("metadata-field")
    key, _, value = (field or "").partition("=")
    print(json.dumps([shown(i) for i in issues if ("--all" in args or i["status"] != "closed")
                      and (label is None or label in (i.get("labels") or []))
                      and (field is None or (i.get("metadata") or {}).get(key) == value)]))
elif args[:1] == ["show"] and args[-1:] == ["--json"]:
    # Like bd: a list of the known ids; it fails only when none is known.
    found = [shown(i) for i in issues if i["id"] in args[1:-1]]
    if not found:
        print(f"Issue {args[1]} not found", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(found))
elif args == ["export"]:
    # Like bd: one JSON line per issue, comments included.
    for i in issues:
        print(json.dumps({**i, "comments": i.get("comments", [])}))
elif args[:1] == ["comments"] and args[2:] == ["--json"]:
    print(json.dumps(next(i for i in issues if i["id"] == args[1]).get("comments", [])))
elif args[:1] == ["create"]:
    parent = opts.get("parent")
    if parent:
        n = 1 + sum(1 for i in issues if i.get("parent") == parent)
        new_id = f"{parent}.{n}"
    else:
        new_id = f"new-{len(issues) + 1}"
    issue = {"id": new_id, "title": opts["title"], "status": "open", "issue_type": opts.get("type", "task"),
             "created_at": "2026-10-03T12:00:00Z"}
    if parent:
        issue["parent"] = parent
    if opts.get("labels"):
        issue["labels"] = opts["labels"].split(",")
    if opts.get("description"):
        issue["description"] = opts["description"]
    if opts.get("metadata"):
        issue["metadata"] = json.loads(opts["metadata"])
    issues.append(issue)
    save()
    print(json.dumps(issue))
elif args[:1] == ["close"]:
    for i in issues:
        if i["id"] == args[1]:
            i["status"] = "closed"
            i["close_reason"] = opts.get("reason")
    save()
elif args[:1] == ["update"] and "--claim" in args:
    # Like bd: assignee and in_progress, and each --set-metadata=key=value merged into the metadata.
    for i in issues:
        if i["id"] == args[1]:
            i.update(status="in_progress", assignee="t")
            i["metadata"] = {**(i.get("metadata") or {}),
                             **dict(a.split("=", 2)[1:] for a in args if a.startswith("--set-metadata="))}
    save()
elif args[:1] == ["update"] and "parent" in opts:
    for i in issues:
        if i["id"] == args[1]:
            i["parent"] = opts["parent"]
    save()
elif args[:1] == ["update"] and ("add-label" in opts or "remove-label" in opts):
    for i in issues:
        if i["id"] == args[1]:
            labels = [l for l in i.get("labels", []) if l != opts.get("remove-label")]
            i["labels"] = labels + ([opts["add-label"]] if "add-label" in opts else [])
    save()
elif args[:1] == ["update"] and "set-metadata" in opts:
    key, value = opts["set-metadata"].split("=", 1)
    for i in issues:
        if i["id"] == args[1]:
            i["metadata"] = {**(i.get("metadata") or {}), key: value}
    save()
elif args[:2] == ["comments", "add"]:
    comment(args[2], Path(opts["file"]).read_text() if "file" in opts else args[3], opts.get("author", "t"))
    save()
elif args[:2] == ["human", "respond"]:
    for i in issues:
        if i["id"] == args[2]:
            i["status"] = "closed"
            i["close_reason"] = "Responded"
    comment(args[2], f"Response: {opts['response']}")
    save()
elif args[:1] == ["bootstrap"]:
    # The database is a marker in the clone's git dir; bootstrap creates it and .beads/, as cloning refs/dolt/data does.
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], check=True,
                            capture_output=True, text=True).stdout.strip()
    db = Path(common) / "fake-bd-db"
    if "--dry-run" in args:
        action = "none" if db.exists() else os.environ.get("FAKE_BD_BOOTSTRAP", "sync")
        print(json.dumps({"action": action, "beads_dir": str(Path.cwd() / ".beads"), "reason": "fake"}))
    else:
        (Path.cwd() / ".beads").mkdir(exist_ok=True)
        db.touch()
elif args[:2] == ["dolt", "push"]:
    print("Pushing to Dolt remote...\nPush complete.")
elif args[:2] == ["context", "--json"]:
    print(json.dumps({"beads_dir": str(main_checkout() / ".beads"), "database": "demo", "dolt_mode": "embedded",
                      "sync_remote": "git+https://example.com/demo.git"}))
elif args[:2] == ["config", "get"]:
    # The agent profile is kept beside the database marker; bd's default is conservative.
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], check=True,
                            capture_output=True, text=True).stdout.strip()
    stored = Path(common) / f"fake-bd-{args[2]}"
    print(json.dumps({"key": args[2], "value": stored.read_text() if stored.exists() else "conservative"}))
elif args[:2] == ["config", "set"]:
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], check=True,
                            capture_output=True, text=True).stdout.strip()
    (Path(common) / f"fake-bd-{args[2]}").write_text(args[3])
    with open(main_checkout() / ".beads/config.yaml", "a") as f:  # bd keeps its config in the tracked config.yaml
        f.write(f"{args[2]}: {args[3]}\n")
elif args[:1] == ["init"]:
    # As bd init does: .beads/ with its config and its git hooks (Beads' marked section), core.hooksPath, the
    # database, and its SessionStart hook in .claude/settings.json (bd writes JSON with sorted keys). It does not
    # commit, unlike bd 1.3.1.
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], check=True,
                            capture_output=True, text=True).stdout.strip()
    (Path.cwd() / ".beads/hooks").mkdir(parents=True)
    (Path.cwd() / ".beads/config.yaml").write_text("# fake\n")
    for name in ("post-checkout", "pre-commit"):
        hook = Path.cwd() / ".beads/hooks" / name
        hook.write_text("#!/usr/bin/env sh\n# --- BEGIN BEADS INTEGRATION v1.3.1 ---\n# beads' part\n"
                        "# --- END BEADS INTEGRATION v1.3.1 ---\n")
        hook.chmod(0o755)
    subprocess.run(["git", "config", "core.hooksPath", str(Path.cwd() / ".beads/hooks")], check=True)
    (Path(common) / "fake-bd-db").touch()
    settings = Path.cwd() / ".claude/settings.json"
    if not settings.exists():
        settings.parent.mkdir(exist_ok=True)
        settings.write_text(json.dumps({"hooks": {"SessionStart": [{"hooks": [
            {"command": "bd prime --hook-json", "type": "command"}], "matcher": ""}]}}, indent=2) + "\n")
elif args[:2] == ["hooks", "install"] and "--beads" in args:
    subprocess.run(["git", "config", "core.hooksPath", str(Path.cwd() / ".beads/hooks")], check=True)
else:
    print(f"fake bd: unsupported {args}", file=sys.stderr)
    sys.exit(1)
