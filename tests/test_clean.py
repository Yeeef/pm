"""pm clean on a temp clone with a bare origin: each worktree listed with keep or remove and the reason, and --apply
removing exactly the remove set. The worktrees: dirty, unpushed, merged, squash-merged, pushed, locked by a live or a
dead process, used by a live session; the main checkout, the store and the caller's worktree are always kept. A
worktree's records workspace goes with it, and keeps it while the workspace has uncommitted edits."""

import json
import os
import re
import subprocess
import sys
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from conftest import PM

pytestmark = pytest.mark.integration  # a bare origin, pushed to and fetched from

LINE = re.compile(r"^(keep|remove) +(\S+)  \((.*?)\): (.*)$")


@pytest.fixture
def clone(repo, tmp_path):
    """The repo with a bare origin holding its main branch."""
    origin = tmp_path / "origin.git"
    repo.git("init", "-q", "--bare", "-b", "main", str(origin))
    repo.git("remote", "add", "origin", str(origin))
    repo.git("push", "-q", "-u", "origin", "main")
    return repo


def agent_tree(repo, name: str) -> Path:
    """An agent worktree as Claude Code makes one: .claude/worktrees/<name> on a new branch from origin/main."""
    path = repo.root / ".claude/worktrees" / name
    repo.git("worktree", "add", "-q", "-b", name, str(path), "origin/main")
    return path


def commit(repo, where: Path, name: str, msg: str = "") -> str:
    (where / name).write_text(f"{name}\n")
    repo.git("add", name, cwd=where)
    repo.git("commit", "-qm", msg or f"add {name}", cwd=where)
    return repo.git("rev-parse", "HEAD", cwd=where).strip()


def clean(repo, *args: str, cwd: Path | None = None, **env: str) -> dict[str, tuple[str, str]]:
    """pm clean's verdicts by worktree directory name: (keep or remove, reason); the last line is its count."""
    res = subprocess.run([*PM, "clean", *args], cwd=cwd or repo.root, env=dict(repo.env, **env),
                         capture_output=True, text=True, check=False)
    assert res.returncode == 0, res.stderr
    *lines, last = res.stdout.splitlines()
    verdicts = {}
    for line in lines:
        m = LINE.match(line)
        assert m, f"not a verdict line: {line!r}"
        verdicts[Path(m[2]).name] = (m[1], m[4])
    verdicts["_count"] = ("", last)
    return verdicts


def worktrees(repo) -> list[str]:
    return sorted(Path(line.split(" ", 1)[1]).name for line in repo.git("worktree", "list", "--porcelain").splitlines()
                  if line.startswith("worktree "))


def branches(repo) -> list[str]:
    return sorted(repo.git("branch", "--format=%(refname:short)").split())


def test_clean_keeps_dirty_unpushed_and_not_agent_worktrees_and_apply_removes_merged_squashed_and_pushed(clone):
    repo = clone
    merged = agent_tree(repo, "merged")
    (merged / "records").symlink_to(repo.store)  # pm init's link: removing the worktree must leave the store
    (agent_tree(repo, "dirty") / "scratch.txt").write_text("not committed\n")
    commit(repo, agent_tree(repo, "unpushed"), "a.txt")
    pushed = agent_tree(repo, "pushed")
    commit(repo, pushed, "b.txt")
    repo.git("push", "-q", "-u", "origin", "pushed", cwd=pushed)
    plain = agent_tree(repo, "plain")  # pushed without -u: its upstream stays origin/main, as worktree add set it
    commit(repo, plain, "d.txt")
    repo.git("push", "-q", "origin", "plain", cwd=plain)
    tip = commit(repo, agent_tree(repo, "squashed"), "c.txt")
    commit(repo, repo.root, "c.txt", "Add c (#7)")  # the PR squash-merged: one new commit on main, not the branch's
    repo.git("push", "-q", "origin", "main")
    gh = Path(repo.env["FAKE_GH_STATE"])
    gh.write_text(json.dumps({"https://github.com/o/r/pull/7": {"number": 7, "state": "MERGED",
                                                                 "headRefName": "squashed", "baseRefName": "main",
                                                                 "headRefOid": tip}}))
    repo.worktree("outside")
    before = (worktrees(repo), branches(repo))

    v = clean(repo)
    assert v == {
        "repo": ("keep", "the main checkout"),
        "records": ("keep", "the records store"),
        "merged": ("remove", "merged: its commits are on origin/main"),
        "dirty": ("keep", "uncommitted changes (1 paths)"),
        "unpushed": ("keep", "commits not on origin/main, and the branch was never pushed"),
        "pushed": ("remove", "nothing beyond its pushed branch origin/pushed; the branch stays, as it is not merged"),
        "plain": ("remove", "nothing beyond its pushed branch origin/plain; the branch stays, as it is not merged"),
        "squashed": ("remove", "squash-merged: PR #7 merged this branch at its tip"),
        "outside": ("keep", "not an agent worktree (outside .claude/worktrees/)"),
        "_count": ("", "dry run: 4 of 9 worktrees to remove; pm clean --apply removes them"),
    }
    assert (worktrees(repo), branches(repo)) == before  # a dry run changes nothing

    v = clean(repo, "--apply")
    assert v.pop("_count") == ("", "removed 4 of 9 worktrees")
    removed = sorted(n for n, (word, _) in v.items() if word == "remove")
    assert removed == ["merged", "plain", "pushed", "squashed"]
    assert worktrees(repo) == ["dirty", "outside", "records", "repo", "unpushed"]
    assert not merged.exists() and not pushed.exists()
    assert branches(repo) == ["dirty", "main", "outside", "plain", "pushed", "records", "unpushed"]  # merged ones deleted
    assert (repo.store / ".git").exists() and repo.git("status", "--porcelain", cwd=repo.store) == ""

    def reset():  # each folded case starts from the main checkout and the records store alone
        for line in repo.git("worktree", "list", "--porcelain").splitlines():
            if line.startswith("worktree ") and Path(line[9:]).name not in ("repo", "records"):
                repo.git("worktree", "remove", "--force", "--force", line[9:])

    # folded: test_clean_keeps_a_branch_not_on_main_when_gh_cannot_check_a_squash_merge
    reset()
    tip = commit(repo, agent_tree(repo, "squashed"), "e.txt")  # main holds c.txt now
    Path(repo.env["FAKE_GH_STATE"]).write_text(json.dumps({"u": {"number": 7, "state": "MERGED",
                                                                  "headRefName": "squashed", "baseRefName": "main",
                                                                 "headRefOid": tip}}))
    v = clean(repo, FAKE_GH_STATE=str(repo.root.parent / "no-such-file.json"))  # gh fails
    word, reason = v["squashed"]
    assert word == "keep"
    assert reason.startswith("commits not on origin/main, and the branch was never pushed (gh could not check for a "
                             "squash merge: ")
    assert clean(repo)["squashed"][0] == "remove"  # gh answering: the squash merge counts

    # folded: test_clean_keeps_a_worktree_a_live_process_locks_and_removes_one_whose_lock_is_stale
    reset()
    me = os.getpid()
    start = f" start {proc_start(me)}" if sys.platform == "linux" else ""
    locks = {"live": f"claude agent live (pid {me}{start})",
             "gone": f"claude agent gone (pid {dead_pid()} start 1)",
             "foreign": "on a disk that is unplugged"}
    if sys.platform == "linux":  # macOS locks are checked by pid alone
        locks["reused"] = f"claude agent reused (pid {me} start 1)"
    for name, reason in locks.items():
        repo.git("worktree", "lock", "--reason", reason, str(agent_tree(repo, name)))

    v = clean(repo)
    assert v["live"] == ("keep", f"locked by live process pid {me}")
    assert v["foreign"] == ("keep", "locked: on a disk that is unplugged")
    assert v["gone"][0] == "remove" and re.fullmatch(r"merged: its commits are on origin/main; stale lock: pid \d+ is "
                                                     r"gone", v["gone"][1])
    if sys.platform == "linux":
        assert v["reused"] == ("remove", f"merged: its commits are on origin/main; stale lock: pid {me} is another "
                                         f"process now (started {proc_start(me)}, not 1)")

    clean(repo, "--apply")
    assert worktrees(repo) == ["foreign", "live", "records", "repo"]

    # folded: test_clean_keeps_a_worktree_a_live_session_used_and_the_worktree_it_runs_in
    reset()
    trees = {n: agent_tree(repo, n) for n in ("bypath", "byrel", "bycwd", "shown", "old", "by", "caller")}
    # a subagent runs in its parent's directory, the main checkout, and reaches its worktree by path
    transcript(repo, "-repo/sess-live/subagents/agent-a.jsonl",
               entry("assistant", repo.root, call=f"cd {trees['bypath']} && make test"),
               entry("assistant", repo.root, call="git -C .claude/worktrees/byrel status"),
               entry("user", trees["bycwd"] / "internal"),
               entry("user", repo.root, shown=f"{trees['shown']} {trees['old']}"),
               entry("assistant", repo.root, call=f"ls {trees['old']}", age_s=3600))

    v = clean(repo, "--apply", cwd=trees["caller"])
    assert v["caller"] == ("keep", "the worktree pm clean runs in")
    for n in ("bypath", "byrel", "bycwd"):
        word, reason = v[n]
        assert word == "keep" and re.fullmatch(r"live session sess-live used it \d+m ago", reason), (n, v[n])
    for n in ("shown", "old", "by"):  # named only in output, too long ago, or only as the start of a longer name
        assert v[n][0] == "remove", (n, v[n])
    assert worktrees(repo) == ["bycwd", "bypath", "byrel", "caller", "records", "repo"]

    # a live Codex session, as a Claude Code transcript: a rollout's cwd (its session_meta's, then each
    # turn_context's) in the worktree, or a tool call naming its path; a subagent's use is its root thread's
    reset()
    trees = {n: agent_tree(repo, n) for n in ("cxmeta", "cxturn", "cxcall", "cxrel", "cxshown", "cxold", "cxcaller")}
    rollout(repo, ROOT_THREAD, ROOT_THREAD, trees["cxmeta"])
    rollout(repo, CHILD_THREAD, ROOT_THREAD, repo.root,
            ("turn_context", {"turn_id": "t1", "cwd": str(trees["cxturn"])}),
            ("turn_context", {"turn_id": "t2", "cwd": str(repo.root)}),
            exec_call("make test", trees["cxcall"]),
            ("response_item", {"type": "function_call", "name": "exec_command", "call_id": "c2",
                               "arguments": json.dumps({"cmd": "git -C .claude/worktrees/cxrel status"})}),
            ("response_item", {"type": "custom_tool_call_output", "call_id": "c2", "output": str(trees["cxshown"])}))
    rollout(repo, "01a12bf0-0000-7000-8000-000000000000", "01a12bf0-0000-7000-8000-000000000000", repo.root,
            exec_call(f"ls {trees['cxold']}", repo.root), age_s=3600)
    v = clean(repo, "--apply", cwd=trees["cxcaller"])
    for n in ("cxmeta", "cxturn", "cxcall", "cxrel"):
        word, reason = v[n]
        assert word == "keep" and re.fullmatch(rf"live session {ROOT_THREAD} used it \d+m ago", reason), (n, v[n])
    for n in ("cxshown", "cxold"):  # named only in a tool's output, or too long ago
        assert v[n][0] == "remove", (n, v[n])
    assert worktrees(repo) == ["cxcall", "cxcaller", "cxmeta", "cxrel", "cxturn", "records", "repo"]



def proc_start(pid: int) -> str:
    """/proc/<pid>/stat's start time, as Claude Code writes it in a worktree lock."""
    stat = Path(f"/proc/{pid}/stat").read_text()
    return stat[stat.rindex(")") + 1:].split()[19]


def dead_pid() -> int:
    p = subprocess.Popen(["true"])
    p.wait()
    return p.pid


def transcript(repo, rel: str, *entries: dict) -> None:
    """A Claude Code transcript under the test's config dir, written now."""
    p = Path(repo.env["CLAUDE_CONFIG_DIR"]) / "projects" / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    with p.open("a") as f:
        for e in entries:
            f.write(json.dumps(e) + "\n")


def entry(kind: str, cwd: Path, *, call: str | None = None, shown: str | None = None, age_s: float = 0) -> dict:
    """A transcript entry: an assistant's tool call (call), or a tool's output it was shown (shown)."""
    at = (datetime.now(UTC) - timedelta(seconds=age_s)).isoformat(timespec="milliseconds")
    e = {"type": kind, "timestamp": at.replace("+00:00", "Z"), "cwd": str(cwd), "sessionId": "sess-live"}
    if call is not None:
        e["message"] = {"role": "assistant", "content": [{"type": "tool_use", "name": "Bash",
                                                          "input": {"command": call}}]}
    if shown is not None:
        e["message"] = {"role": "user", "content": [{"type": "tool_result", "content": shown}]}
    return e


ROOT_THREAD, CHILD_THREAD = "01a12bee-db2f-7050-ba39-116b615a19b8", "01a12bef-027d-7c33-8fb3-8fd52363eaf7"


def rollout(repo, thread: str, root: str, cwd: Path, *entries: tuple[str, dict], age_s: float = 0) -> None:
    """A Codex 0.154 rollout of `thread` (a subagent of `root` when they differ) under the test's CODEX_HOME, written
    now: its session_meta in `cwd`, then entries, each a line's type and payload, written age_s seconds ago."""
    at = (datetime.now(UTC) - timedelta(seconds=age_s)).isoformat(timespec="milliseconds").replace("+00:00", "Z")
    meta = {"id": thread, "session_id": root, "cwd": str(cwd), "originator": "codex_exec"}
    p = Path(repo.env["CODEX_HOME"]) / "sessions/2026/10/11" / f"rollout-2026-10-11T19-07-16-{thread}.jsonl"
    p.parent.mkdir(parents=True, exist_ok=True)
    lines = [{"timestamp": at, "type": "session_meta", "payload": meta}]
    lines += [{"timestamp": at, "type": t, "payload": payload} for t, payload in entries]
    p.write_text("".join(json.dumps(line) + "\n" for line in lines))


def exec_call(command: str, workdir: Path) -> tuple[str, dict]:
    """Codex's exec tool call, as Codex 0.154 writes it."""
    return "response_item", {"type": "custom_tool_call", "name": "exec", "call_id": "c1", "status": "completed",
                             "input": f"text(await tools.exec_command({{cmd:{json.dumps(command)},"
                                      f"workdir:{json.dumps(str(workdir))}}}));\n"}


def test_clean_keeps_what_a_removal_would_delete_with_it(clone):
    """git worktree remove deletes the whole directory: a worktree inside it, and untracked files git status hides."""
    repo = clone
    (repo.root / ".git/info/exclude").write_text("**/.claude/worktrees/\n")  # as on a clone, so the outer one is clean
    outer = agent_tree(repo, "outer")
    inner = outer / ".claude/worktrees/inner"
    repo.git("worktree", "add", "-q", "-b", "inner", str(inner), "origin/main")
    (inner / "work.txt").write_text("not committed\n")
    hidden = agent_tree(repo, "hidden")
    repo.git("config", "status.showUntrackedFiles", "no")
    (hidden / "notes.txt").write_text("untracked, and status hides it\n")
    tip = commit(repo, agent_tree(repo, "stacked"), "s.txt")  # its PR merged into another branch, not main
    Path(repo.env["FAKE_GH_STATE"]).write_text(json.dumps({"u": {"number": 8, "state": "MERGED", "headRefName": "stacked",
                                                                  "baseRefName": "feature", "headRefOid": tip}}))
    gone = agent_tree(repo, "gone")
    away = repo.worktree("away")
    for d in (gone, away):  # a directory deleted by hand, or on a disk not mounted now
        subprocess.run(["rm", "-rf", str(d)], check=True)

    v = clean(repo, "--apply")
    assert v["outer"] == ("keep", f"holds the worktree {inner}")
    assert v["inner"] == ("keep", "uncommitted changes (1 paths)")
    assert v["hidden"] == ("keep", "uncommitted changes (1 paths)")
    assert v["stacked"] == ("keep", "commits not on origin/main, and the branch was never pushed")
    assert v["gone"] == ("remove", "merged: its commits are on origin/main; its directory is gone")
    assert v["away"] == ("keep", "not an agent worktree (outside .claude/worktrees/)")
    assert (inner / "work.txt").exists() and (hidden / "notes.txt").exists()
    assert worktrees(repo) == ["away", "hidden", "inner", "outer", "records", "repo", "stacked"]  # no global prune


def test_clean_removes_the_interactive_mode_of_a_session_idle_for_a_day(clone):
    """A session that crashed leaves its mode file (`pm interactive`): pm clean removes it once no transcript or rollout
    of the session was written for a day, and keeps it for a session idle longer than the 30 minutes that make it not
    live, so an interactive session the owner stepped away from keeps its mode; Claude Code and Codex alike."""
    repo = clone
    modes = repo.root / ".pm/run/sessions"
    modes.mkdir(parents=True)
    for session in ("sess-away", ROOT_THREAD, "sess-gone"):
        (modes / session).write_text("interactive\n")
    ages = {"-repo/sess-away.jsonl": 2 * 3600, "-repo/sess-gone.jsonl": 25 * 3600}
    for rel, age in ages.items():
        transcript(repo, rel, entry("user", repo.root, age_s=age))
        path = Path(repo.env["CLAUDE_CONFIG_DIR"]) / "projects" / rel
        os.utime(path, (path.stat().st_mtime - age,) * 2)
    rollout(repo, ROOT_THREAD, ROOT_THREAD, repo.root, age_s=2 * 3600)
    for p in (Path(repo.env["CODEX_HOME"]) / "sessions").rglob(f"*{ROOT_THREAD}.jsonl"):
        os.utime(p, (p.stat().st_mtime - 2 * 3600,) * 2)

    kept = ("keep", "the session was active within a day")
    for args in ((), ("--apply",)):
        v = clean(repo, *args)
        assert v["sess-away"] == kept and v[ROOT_THREAD] == kept, v
        assert v["sess-gone"] == ("remove", "the session has been idle for a day")
        assert (modes / "sess-gone").exists() == (not args), args
    assert sorted(p.name for p in modes.iterdir()) == sorted(["sess-away", ROOT_THREAD])


def test_clean_removes_a_records_workspace_with_its_worktree_and_keeps_one_with_uncommitted_edits(clone):
    repo = clone
    trees = {name: agent_tree(repo, name) for name in ("gone", "edited")}
    for name, wt in trees.items():  # each worktree's records workspace, as pm init makes it
        repo.git("worktree", "add", "-q", "-B", f"drafts/{name}", str(wt / "records"), "records")
    sprint = trees["edited"] / "records/sprints/demo-1.md"
    sprint.write_text(sprint.read_text() + "edited\n")
    res = subprocess.run([*PM, "clean"], cwd=repo.root, env=repo.env, capture_output=True, text=True, check=False)
    assert res.returncode == 0, res.stderr
    lines = res.stdout.splitlines()
    for name, wt in trees.items():
        assert f"keep    {wt / 'records'}  (drafts/{name}): the records workspace of {wt}, removed with it" in lines, \
            res.stdout
    assert f"remove  {trees['gone']}  (gone): merged: its commits are on origin/main" in lines, res.stdout
    assert f"keep    {trees['edited']}  (edited): its records workspace has uncommitted edits (1 paths)" in lines, \
        res.stdout

    clean(repo, "--apply")
    assert "gone" not in worktrees(repo) and "drafts/gone" not in branches(repo)
    assert "edited" in worktrees(repo) and "drafts/edited" in branches(repo) and sprint.read_text().endswith("edited\n")
    assert (repo.store / "sprints/demo-1.md").is_file(), "the store stays"
