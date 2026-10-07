"""The pm flows agents and the owner depend on, end to end against a temp repo and a fake bd: setup, the sprint loop,
needs and replies, tasks, the shared records store, the pm service and its push."""

from __future__ import annotations

import contextlib
import json
import os
import re
import shlex
import shutil
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

import pytest

from conftest import write_config, HARNESS, PM, fake_bd_env

sys.path.insert(0, str(HARNESS))
from pm.beads import reply_body  # noqa: E402

FRAME = "## Goal\n\nShip the thing.\n\n## Scope\n\n**In:** the thing.\n\n**Out:** other things.\n\n## Done when\n\n- It ships.\n"


def committed(repo, messages):
    """The latest store commits carry `messages`, newest first, and nothing is left uncommitted anywhere."""
    return (repo.store_log()[:len(messages)] == messages and repo.git("status", "--porcelain", cwd=repo.store) == ""
            and repo.git("status", "--porcelain") == "")


def refused(repo, *args, stdin="", match):
    """Run a command that must refuse: non-zero exit, the message, no file and no Beads change."""
    heads = lambda: [repo.git("rev-parse", "HEAD", cwd=d) for d in (repo.root, repo.store)]
    before, head = repo.snapshot(), heads()
    res = repo.pm(*args, stdin=stdin)
    assert res.returncode != 0, res.stdout
    assert re.search(match, res.stderr), res.stderr
    assert repo.snapshot() == before
    assert heads() == head
    assert repo.bd_writes() == []
    return res


# ---------------------------------------------------------------- sprints: open, review, close


PR = "https://github.com/o/r/pull/12"


def report(repo, outcome, against="- It ships: met, see the log.", name="demo-1"):
    """Write a sprint's delivery report over whatever it holds and commit it."""
    text = (repo.records / f"sprints/{name}.md").read_text()
    head, _, _ = text.partition("### Outcome\n")
    text = (head + "### Outcome\n\n> Done, partial or voided, plus one sentence; then, optionally, bullets of what "
            f"shipped.\n\n{outcome}\n\n### Against \"Done when\"\n\n> Each item, met or not, with its evidence.\n\n"
            f"{against}\n")
    repo.write(f"sprints/{name}.md", text)
    repo.commit("delivery report")


SHA = "8f5c6180a1b2c3d4e5f60718293a4b5c6d7e8f90"


def store_head(repo):
    return repo.git("rev-parse", "--short", "HEAD", cwd=repo.store).strip()



def test_sprint_open_creates_epic_and_record(repo):
    res = repo.pm("sprint", "open", "demo", "--title", "Third: the end", stdin=FRAME)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["create", "--type=epic", "--parent=demo", "--title=Sprint 3: Third: the end", "--json"]]
    assert repo.issues()["demo.3"]["parent"] == "demo"
    # After the create it reads only the new epic, not every issue again.
    assert [c for c in repo.bd_calls() if c[:1] in (["list"], ["show"])] == [["list", "--all", "--json"], ["show", "demo.3", "--json"]]
    text = (repo.records / "sprints/demo-3.md").read_text()
    assert text.startswith('---\ntype: sprint\ntitle: "Third: the end"\nbead: demo.3\n---\n')
    assert "Ship the thing." in text and "**Out:** other things." in text and "- It ships." in text
    # Every section and prompt line matches an existing sprint record (the fixture mirrors the real ones).
    skeleton = lambda t: [l for l in t.splitlines() if l.startswith(("#", ">"))]
    assert skeleton(text) == skeleton((repo.records / "sprints/demo-1.md").read_text())
    assert text.count("None yet.") == 3 and text.count("\n\nNot closed yet.\n") == 2


def test_sprint_closes_after_its_pr_merges(repo):
    """The whole loop: a written report, a review under the sprint, a refusal while the review is open, the review
    closed as merged once the PR is on main, then a close that stamps the merge and names its records commit."""
    repo.set_issue("demo.1.2", status="closed", close_reason="Dismissed")
    report(repo, "Done: shipped the thing.")
    res = repo.pm("action", "need", "--pr", PR, "--sprint", "demo.1", "--focus", "F")
    assert res.returncode == 0, res.stderr
    assert repo.issues()["demo.1.3"]["parent"] == "demo.1" and "under demo.1;" in res.stdout
    repo.log.write_text("")
    refused(repo, "sprint", "close", "demo.1", match=r"open tasks in the sprint: demo.1.3 \(open\)")
    assert repo.pm("action", "done", "demo.1.3", "--reason", f"merged as {SHA}").returncode == 0
    repo.log.write_text("")
    res = repo.pm("sprint", "close", "demo.1")
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["[SPRINT] demo sprint 1: closed, merged as 8f5c618"])
    assert repo.bd_writes() == [["close", "demo.1", f"--reason=Done: shipped the thing. (records commit {store_head(repo)})"]]
    assert repo.pm("check").returncode == 0
    page = repo.page("sprints/demo-1.html")
    assert "<p>Done: shipped the thing.</p>\n<p>Merged as 8f5c618 (PR #12).</p>" in page


# ---------------------------------------------------------------- needs: answer with a decision or close

BODY = "Use the small parser.\nIt is enough for the record set and adds no dependency.\n"
TODAY = __import__("datetime").date.today().isoformat()


NEED = ("Question: Which parser should we use?\nFact: Records hold no tables yet.\n"
        "Option small: The small parser.\nCost: no tables.\nOption full: The full parser.\nCost: a new dependency.\n"
        "Default: small. It is cheap.\n")


ACTION = "Restart the site on port 8767, which the new proxy expects.\n"


def test_decision_add_need_closes_need_and_records_decision(repo):
    path = repo.records / "projects/demo.md"
    res = repo.pm("decision", "add", "--need", "demo.1.2", "--level", "project", "--project", "demo", stdin=BODY)
    assert res.returncode == 0, res.stderr
    text = BODY + "Answers `demo.1.2`."
    assert repo.bd_writes() == [["human", "respond", "demo.1.2", f"--response={text}"]]
    assert repo.issues()["demo.1.2"]["status"] == "closed"
    assert path.read_text().split("## Design pages")[0].rstrip().endswith(
        f"::: decision {{source=owner date={TODAY}}}\n{text}\n:::")
    assert repo.pm("check").returncode == 0


ANSWER = "Port 8767."


def test_decision_close_closes_small_answer_without_record(repo):
    """A small answer closes the need with the answer and the reason, labelled no-decision, and writes no record;
    the render accepts it although no decision cites it."""
    heads = repo.store_log()
    res = repo.pm("decision", "close", "demo.1.2", "--reason", "It picks a port and sets no rule.", stdin=ANSWER)
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [
        ["human", "respond", "demo.1.2", f"--response={ANSWER}\n\nNo decision record: It picks a port and sets no rule."],
        ["update", "demo.1.2", "--add-label=no-decision"]]
    need = repo.issues()["demo.1.2"]
    assert (need["status"], need["labels"]) == ("closed", ["human", "no-decision"])
    assert repo.store_log() == heads and repo.git("status", "--porcelain", cwd=repo.store) == ""
    assert repo.pm("check").returncode == 0


# ---------------------------------------------------------------- pm task add


def test_task_add_creates_task_in_sprint(repo):
    res = repo.pm("task", "add", "--sprint", "demo.1", "--title", "Write the parser", stdin="Small and fast.\n")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes() == [["create", "--type=task", "--parent=demo.1", "--title=Write the parser",
                                 "--description=Small and fast.", "--json"]]
    assert "created task demo.1.3 in sprint demo.1" in res.stdout
    assert repo.issues()["demo.1.3"]["parent"] == "demo.1"
    assert repo.git("status", "--porcelain") == ""
    res = repo.pm("task", "add", "--sprint", "demo.2", "--title", "No description")
    assert res.returncode == 0, res.stderr
    assert repo.bd_writes()[-1] == ["create", "--type=task", "--parent=demo.2", "--title=No description", "--json"]


# ---------------------------------------------------------------- pm task close

def add_task(repo, issue_id="demo.1.3", **fields):
    issues = json.loads(repo.state.read_text()) + [dict(
        {"id": issue_id, "title": "Write the parser", "status": "in_progress", "issue_type": "task",
         "parent": "demo.1", "created_at": "2026-10-01T12:00:00Z", "started_at": "2026-10-01T12:00:00Z"}, **fields)]
    repo.state.write_text(json.dumps(issues))


def test_task_close_names_head(repo):
    add_task(repo)
    head = repo.git("rev-parse", "--short", "HEAD").strip()
    res = repo.pm("task", "close", "demo.1.3", "--reason", "Parser written.")
    assert res.returncode == 0, res.stderr
    assert res.stderr == ""
    assert repo.bd_writes() == [["close", "demo.1.3", f"--reason=Parser written. (commit {head})"]]
    assert repo.issues()["demo.1.3"]["status"] == "closed"


# ---------------------------------------------------------------- the store: pm setup

GIT_ENV = dict(__import__("os").environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.com",
               GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.com")


def git_in(cwd, *args):
    return subprocess.run(["git", *args], cwd=cwd, env=GIT_ENV, check=True, capture_output=True, text=True).stdout


def setup_in(cwd, *args):
    """pm setup with the fake bd, whose calls land in <tmp>/bd.log next to the clone."""
    return subprocess.run([*PM, "setup", *args], cwd=cwd,
                          env=fake_bd_env(cwd.parent, GIT_ENV),
                          capture_output=True, text=True)


def schedule_line(clone: Path, home: Path, state: str) -> str:
    """pm where's schedule line for a clone, on this platform's scheduler as the fakes present it."""
    name = f"local.pm-push.{clone.name}.{__import__('hashlib').sha1(str(clone.resolve()).encode()).hexdigest()[:8]}"
    at = (home / f"Library/LaunchAgents/{name}.plist" if sys.platform == "darwin"
          else home / f".config/systemd/user/{name}.timer")
    return f"schedule  {'launchd' if sys.platform == 'darwin' else 'systemd'} {name} ({at})  {state}"


def sched_calls(tmp_path) -> list[list[str]]:
    log = tmp_path / "sched.log"
    return [json.loads(l) for l in log.read_text().splitlines()] if log.exists() else []


def bd_writes_in(tmp_path):
    """The bd calls that change something: not the --json reads (the bootstrap dry run, bd context)."""
    calls = [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]
    return [c for c in calls if "--json" not in c]


def service_line(clone: Path, home: Path, state: str) -> str:
    """pm where's service line for a clone, on this platform's supervisor as the fakes present it."""
    name = f"local.pm.{clone.name}.{__import__('hashlib').sha1(str(clone.resolve()).encode()).hexdigest()[:8]}"
    at = (home / f"Library/LaunchAgents/{name}.plist" if sys.platform == "darwin"
          else home / f".config/systemd/user/{name}.service")
    return f"service   {'launchd' if sys.platform == 'darwin' else 'systemd'} {name} ({at})  {state}"


@pytest.fixture
def origin(tmp_path):
    """A clone's origin as this repo was cut over: records split onto the records branch, then untracked on main."""
    o = tmp_path / "origin"
    (o / "records/sprints").mkdir(parents=True)
    (o / "records/sprints/demo-1.md").write_text("one\n")
    git_in(tmp_path, "init", "-q", "-b", "main", str(o))
    git_in(o, "add", "-A")
    git_in(o, "commit", "-qm", "records")
    git_in(o, "subtree", "split", "--prefix=records", "-b", "records")
    (o / ".gitignore").write_text("/records\n")
    write_config(o)
    git_in(o, "rm", "-rq", "records")
    git_in(o, "add", ".gitignore", ".pm")
    git_in(o, "commit", "-qm", "records live on the records branch")
    return o


@pytest.mark.slow
def test_setup_on_fresh_clone_checks_out_store_and_links_records(tmp_path, origin):
    git_in(tmp_path, "clone", "-q", str(origin), "clone")
    clone = tmp_path / "clone"
    res = setup_in(clone)
    assert res.returncode == 0, res.stderr
    assert bd_writes_in(tmp_path) == [["bootstrap", "--yes"], ["config", "set", "agent.profile", "team-maintainer"],
                                     ["hooks", "install", "--beads"]]
    assert git_in(clone, "config", "core.hooksPath").strip() == str(clone / ".beads/hooks")
    assert git_in(clone, "config", "beads.role").strip() == "maintainer"
    assert (clone / ".beads").stat().st_mode & 0o777 == 0o700
    assert git_in(clone / ".pm/store/records", "rev-parse", "--abbrev-ref", "HEAD").strip() == "records"
    assert (clone / "records").is_symlink() and (clone / "records/sprints/demo-1.md").read_text() == "one\n"
    assert git_in(clone, "status", "--porcelain") == ""
    config = (clone / ".git/config").read_text()
    where = subprocess.run([*PM, "where"], cwd=clone,
                           env=fake_bd_env(tmp_path, GIT_ENV), capture_output=True, text=True).stdout.splitlines()
    assert where[0] == f"store     {clone}/.pm/store/records  branch records, 0 ahead, 0 behind origin/records (as of the last fetch)"
    assert where[1] == f"checkout  {clone}  branch main, records link set up"
    # pm init installs the service; setup leaves the machine's supervisor alone
    assert service_line(clone, tmp_path / "home", "not installed; run pm service install") in where
    assert f"push      {clone}/.pm/run/push.log  no push recorded yet" in where
    assert not [c for c in sched_calls(tmp_path) if c[1:2] in (["bootstrap"], ["kickstart"]) or c[2:3] in (["enable"], ["restart"])]
    again = setup_in(clone)
    assert again.returncode == 0 and again.stdout.startswith("already set up"), again.stderr
    assert bd_writes_in(tmp_path) == [["bootstrap", "--yes"], ["config", "set", "agent.profile", "team-maintainer"],
                                     ["hooks", "install", "--beads"]], "no bd change"
    assert (clone / ".git/config").read_text() == config


# ---------------------------------------------------------------- the store: shared across worktrees

def test_write_on_one_branch_is_visible_on_another_without_merge(repo):
    wt = repo.worktree("feature-x")
    res = repo.pm("finding", "add", "--sprint", "demo.1", "Seen from every branch.", cwd=wt)
    assert res.returncode == 0, res.stderr
    assert committed(repo, ["pm: added a finding to records/sprints/demo-1.md"])
    assert repo.git("status", "--porcelain", cwd=wt) == ""
    # The main checkout is on main, which merged nothing.
    assert repo.git("rev-parse", "--abbrev-ref", "HEAD").strip() == "main"
    assert repo.git("log", "--format=%s").splitlines() == ["code"]
    assert "- Seen from every branch." in repo.pm("show", "--sprint", "demo.1").stdout
    assert repo.pm("check").returncode == 0
    assert "Seen from every branch." in repo.page("sprints/demo-1.html")


def test_concurrent_writes_from_two_worktrees_land_as_separate_commits(repo):
    wts = [repo.worktree("feature-a"), repo.worktree("feature-b")]
    procs = [subprocess.Popen([*PM, "finding", "add", "--sprint", "demo.1",
                               f"From {wt.name}."], cwd=wt, env=repo.env, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, text=True) for wt in wts]
    for p in procs:
        assert p.wait() == 0, p.stderr.read()
    assert committed(repo, ["pm: added a finding to records/sprints/demo-1.md"] * 2)
    assert repo.store_log()[2] == "records"
    text = (repo.store / "sprints/demo-1.md").read_text()
    assert "- From feature-a." in text and "- From feature-b." in text
    for rev in ("HEAD", "HEAD~1"):
        assert repo.git("show", "--name-only", "--format=", rev, cwd=repo.store).split() == ["sprints/demo-1.md"]


def test_pm_commit_refuses_a_hand_edit_that_does_not_render_and_commits_one_that_does(repo):
    assert repo.pm("where", "records").stdout == f"{repo.store}\n"
    path = repo.store / "sprints/demo-1.md"
    text = path.read_text()
    path.write_text(text.replace("## Findings", "## Finds"))
    refused(repo, "commit", "-m", "Break it", "records/sprints/demo-1.md",
            match=r"error: sprints/demo-1: sprint record needs a '## Findings'")
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M sprints/demo-1.md\n"
    path.write_text(text.replace("Ship it.", "Ship it soon."))
    res = repo.pm("commit", "-m", "Sharpen the sprint 1 goal", "records/sprints/demo-1.md")
    assert res.returncode == 0, res.stderr
    assert "committed records/sprints/demo-1.md as " in res.stdout
    assert committed(repo, ["Sharpen the sprint 1 goal"])
    refused(repo, "commit", "-m", "Nothing", "records/sprints/demo-1.md", match=r"error: nothing to commit in .*\.pm/store/records")


def test_commit_commits_only_its_callers_records_beside_another_sessions_edit(repo):
    """Two sessions share the store: A has an edit in progress, B commits its own. B's commit holds B's file only,
    and pm commit with no path lists both instead of sweeping A's edit in."""
    a = repo.store / "sprints/demo-1.md"
    a.write_text(a.read_text().replace("Ship it.", "Ship it, session A."))
    (repo.store / "docs").mkdir()
    new = repo.store / "docs/scratch.md"
    new.write_text("session A, not a record yet")
    b = repo.store / "projects/demo.md"
    b.write_text(b.read_text().replace("A demo project.", "A demo project, session B."))
    wt = repo.worktree("session-b")
    res = refused(repo, "commit", "-m", "Session B", match=r"error: name the records you edited: pm commit")
    for line in (" M records/projects/demo.md", " M records/sprints/demo-1.md", "?? records/docs/scratch.md"):
        assert line in res.stderr
    refused(repo, "commit", "-m", "Session B", "records/sprints/demo-2.md", match=r"error: records/sprints/demo-2.md has no uncommitted change")
    refused(repo, "commit", "-m", "Session B", str(repo.root / ".gitignore"), match=r"is not in the records store")
    # A's half-written file does not render, but B's commit is checked as the records branch will hold it.
    res = repo.pm("commit", "-m", "Session B", "records/projects/demo.md", cwd=wt)
    assert res.returncode == 0, res.stderr
    assert repo.git("show", "--name-only", "--format=%s", "HEAD", cwd=repo.store).split("\n")[:3] == [
        "Session B", "", "projects/demo.md"]
    assert repo.git("status", "--porcelain", cwd=repo.store) == " M sprints/demo-1.md\n?? docs/\n"
    assert "session A" in a.read_text()


SERVE_BEHIND = 10  # seconds a served page may be behind, as pm/site.py has it


def asof(page: str) -> float:
    """The time, in epoch seconds, a page served by the pm service states its data is current as of."""
    m = re.search(r'<p class="asof[^"]*" data-asof="([\d.]+)"', page)
    assert m, "the page states no age"
    return float(m.group(1))


def until_shown(get, shown, written: float | None) -> str:
    """Load get() until shown(page), which must happen within SERVE_BEHIND s of `written`, the epoch time a change
    was done; a page that does not show it yet must state data from before the change. None for a change the server
    makes in the background, whose time the test does not know: then only the bound from now applies."""
    deadline = (written or time.time()) + SERVE_BEHIND
    while True:
        page = get()
        if shown(page):
            return page
        assert written is None or asof(page) < written, "the page states data as of after a change it does not show"
        assert time.time() < deadline, "the change did not show within the stated bound"
        time.sleep(0.05)


def load(url: str) -> str:
    """The page at `url`, whatever its status."""
    import urllib.error
    try:
        with urllib.request.urlopen(url) as r:
            return r.read().decode()
    except urllib.error.HTTPError as e:
        return e.read().decode()


@pytest.mark.slow
def test_serve_shows_each_change_within_its_stated_age(repo):
    """The pm service shows a pm write and a Beads change within SERVE_BEHIND s, every page stating data from before a
    change it does not show yet, and a failing render as the error."""
    import urllib.error
    import urllib.request

    repo.dolt()
    srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root,
                           env=dict(repo.env, PORT="0"), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    try:
        url = re.search(r"http://localhost:\d+", srv.stdout.readline()).group(0)

        def get(path):
            try:
                with urllib.request.urlopen(f"{url}/{path}") as r:
                    return r.status, r.headers["Cache-Control"], r.read().decode()
            except urllib.error.HTTPError as e:
                return e.code, e.headers["Cache-Control"], e.read().decode()

        assert "Served fresh." not in get("sprints/demo-1.html")[2]
        assert repo.pm("finding", "add", "Served fresh.", "--sprint", "demo.1").returncode == 0
        until_shown(lambda: get("sprints/demo-1.html")[2], lambda p: "Served fresh." in p, time.time())
        assert get("sprints/demo-1.html")[:2] == (200, "no-store")

        assert "1 of 2 tasks done" in get("")[2]
        repo.set_issue("demo.1.1", status="open")
        until_shown(lambda: get("")[2], lambda p: "0 of 2 tasks done" in p, time.time())
        assert get("style.css")[0] == 200 and get("nope.html")[0] == 404

        repo.set_issue("demo.1.2", status="closed")  # an answered need no decision cites: the render fails
        until_shown(lambda: get("")[2], lambda p: "no decision cites it" in p, time.time())
        code, _, body = get("")
        assert code == 500 and "need demo.1.2 (Ask the owner) is closed but no decision cites it" in body
    finally:
        srv.terminate()
        srv.wait()


# ---------------------------------------------------------------- the site: pm serve


@pytest.fixture
def served(repo):
    """The pm service for the repo's store on a free port; the repo's env carries that PORT from here on."""
    repo.dolt()
    srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root,
                           env=dict(repo.env, PORT="0"), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    try:
        port = re.search(r"http://localhost:(\d+)", srv.stdout.readline()).group(1)
        repo.env = dict(repo.env, PORT=port)
        yield f"http://localhost:{port}"
    finally:
        srv.terminate()
        srv.wait()


# ---------------------------------------------------------------- replies on the site

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def post_reply(url: str, form: dict[str, str], headers: dict[str, str] | None = None) -> tuple[int, str, str]:
    """POST a card's form as the browser does; the status, the Location header and the body."""
    import urllib.error
    import urllib.parse

    req = urllib.request.Request(f"{url}/reply", data=urllib.parse.urlencode(form).encode(), headers=headers or {})
    try:
        with urllib.request.build_opener(NoRedirect).open(req) as r:
            return r.status, r.headers.get("Location", ""), r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.headers.get("Location", ""), e.read().decode()


def page_token(url: str, page: str = "") -> str:
    with urllib.request.urlopen(f"{url}/{page}") as r:
        m = re.search(r'name="token" value="([^"]+)"', r.read().decode())
    assert m, "the served page has no reply form"
    return m.group(1)


@pytest.mark.slow
def test_reply_on_a_card_is_stored_on_its_issue_as_one_comment(repo, served):
    """A decision's answer and an action's evidence each land on their own issue, which stays open for the agent;
    each reply is one Beads write, the comment, and no label."""
    assert repo.pm("action", "need", "--title", "Restart the site", "--parent", "demo.1", stdin=ACTION).returncode == 0
    day = until_shown(lambda: urllib.request.urlopen(f"{served}/days/2026-10-01.html").read().decode(),
                      lambda p: p.count('<form class="reply" method="post" action="/reply"') == 2, time.time())
    assert '<input type="hidden" name="id" value="demo.1.2">' in day
    token = page_token(served, "days/2026-10-01.html")
    repo.log.write_text("")

    for issue_id, text in (("demo.1.2", "-Pick small.\r\nBecause tables can wait."), ("demo.1.3", "Merged as abc123.")):
        code, where, _ = post_reply(served, {"token": token, "id": issue_id, "text": text},
                                    {"Referer": f"{served}/days/2026-10-01.html"})
        assert (code, where) == (303, f"/days/2026-10-01.html#need-{issue_id}")
    deadline = time.monotonic() + 10  # the comments are written in the background
    while len(repo.bd_writes()) < 2 and time.monotonic() < deadline:
        time.sleep(0.05)
    issues = repo.issues()
    for issue_id, text in (("demo.1.2", "-Pick small.\nBecause tables can wait."), ("demo.1.3", "Merged as abc123.")):
        assert [(c["author"], reply_body(c["text"])) for c in issues[issue_id]["comments"]] == [("owner (site reply)",
                                                                                                text)]
        assert re.fullmatch(r"(?s).*\n\n<!-- pm-reply [0-9a-f-]{36} -->", issues[issue_id]["comments"][0]["text"])
        assert issues[issue_id]["labels"] == ["human"] + (["action"] if issue_id == "demo.1.3" else [])
        assert issues[issue_id]["status"] == "open"
    assert [c[:3] for c in repo.bd_writes()] == [["comments", "add", "demo.1.2"], ["comments", "add", "demo.1.3"]]
    until_shown(lambda: urllib.request.urlopen(f"{served}/").read().decode(),
                lambda p: p.count('<span class="replied">not delivered') == 2 and "Merged as abc123." in p, None)

    assert repo.pm("check").returncode == 0  # the static site keeps the slot and shows no form, but the replies
    static = repo.page("index.html")
    assert "<!--pm-reply demo.1.2 decision-->" in static and "<form" not in static
    assert "<p>Merged as abc123.</p>" in static and "<!-- pm-reply" not in static


@contextlib.contextmanager
def serving(repo, **env):
    """The pm service for the repo's store on a free port with `env` added; its URL, then its stderr once it stopped."""
    log = repo.root.parent / "serve.log"
    with log.open("w") as err:
        srv = subprocess.Popen([*PM, "service", "run"], cwd=repo.root,
                               env=dict(repo.env, PORT="0", **env), stdout=subprocess.PIPE, stderr=err, text=True)
    try:
        yield re.search(r"http://localhost:\d+", srv.stdout.readline()).group(0), log
    finally:
        srv.terminate()
        srv.wait()


@pytest.mark.slow
@pytest.mark.parametrize("token, form, headers, code, said", [
    ("forged", {"id": "demo.1.2", "text": "Small."}, {}, 403, "no valid token"),
    ("page", {"id": "demo.1.2", "text": "Small."}, {"Host": "evil.example:80"}, 403, "is neither this machine"),
])
def test_reply_is_refused_without_the_token_or_from_another_host(repo, served, token, form, headers, code, said):
    form = dict(form, token=page_token(served) if token == "page" else token)
    res = post_reply(served, form, headers)
    assert res[0] == code and said in res[2]
    assert repo.bd_writes() == []


@contextlib.contextmanager
def session_inbox(path: str | None = None):
    """A stand-in for a Claude Code session's inbox: a Unix socket that collects each JSON line written to it. Its
    path is short (a socket path is limited to about 100 bytes), so it lives in its own temp dir, or at `path`."""
    import threading
    d = None if path else inbox_dir()
    path = path or os.path.join(d, "s")
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(path)
    srv.listen()
    lines: list[dict] = []

    def accept():
        while True:
            try:
                conn, _ = srv.accept()
            except OSError:
                return
            with conn:
                data = b"".join(iter(lambda: conn.recv(65536), b""))
            lines.extend(json.loads(line) for line in data.decode().splitlines())
    threading.Thread(target=accept, daemon=True).start()
    try:
        yield path, lines
    finally:
        srv.close()
        os.unlink(path)
        if d:
            shutil.rmtree(d)


def inbox_dir() -> str:
    import tempfile
    return tempfile.mkdtemp(prefix="pm-inbox-")


def wait_for(cond, what: str, timeout: float = 30):
    deadline = time.time() + timeout
    while not (got := cond()):
        assert time.time() < deadline, f"not within {timeout}s: {what}"
        time.sleep(0.05)
    return got


def inbox_text(line: dict) -> str:
    """The text of a message written to a session's inbox, checked to be the user-message line Claude Code takes."""
    assert line["type"] == "user" and line["message"]["role"] == "user"
    return line["message"]["content"]


@pytest.mark.slow
def test_every_reply_is_pushed_into_the_session_that_asked(repo, served):
    """The three replies lost on 2026-10-06, each pushed by the pm service: several requests raised in one command, one
    raised with its output piped through grep, and a second reply to a request already delivered."""
    with session_inbox() as (inbox, lines):
        env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET=inbox, NEED=NEED)
        pm = shlex.join(PM)
        raise_ = f'printf %s "$NEED" | {pm} decision need --parent demo.1'
        script = (f"{raise_} --title A >/dev/null && {raise_} --title B >/dev/null && "
                  f"{raise_} --title C | grep -o 'raised decision need [^ ;]*'")
        res = subprocess.run(["bash", "-c", script], cwd=repo.root, env=env, capture_output=True, text=True)
        assert (res.returncode, res.stdout) == (0, "raised decision need demo.1.5\n"), res.stderr
        until_shown(lambda: load(f"{served}/"), lambda p: 'value="demo.1.5"' in p, time.time())
        token = page_token(served)

        def reply(issue_id, text):
            assert post_reply(served, {"token": token, "id": issue_id, "text": text})[0] == 303

        reply("demo.1.3", "First answer.")
        wait_for(lambda: len(lines) == 1, "the first reply pushed")
        assert inbox_text(lines[0]).startswith("pm: owner reply to decision demo.1.3 (A), relayed from the site:\n")
        assert "First answer." in inbox_text(lines[0]) and "pm decision add --need demo.1.3" in inbox_text(lines[0])
        wait_for(lambda: repo.issues()["demo.1.3"]["metadata"].get("picked_up") == "1", "the reply marked delivered")
        for issue_id, text in (("demo.1.4", "Answer to B."), ("demo.1.5", "Answer to C."),
                               ("demo.1.3", "On second thought, no.")):
            reply(issue_id, text)
        wait_for(lambda: len(lines) == 4, "every later reply pushed")
        texts = [inbox_text(line) for line in lines[1:]]
        assert [next(i for i in ("demo.1.4", "demo.1.5", "demo.1.3") if f" {i} " in x) for x in texts] == \
            ["demo.1.4", "demo.1.5", "demo.1.3"]
        assert "Answer to B." in texts[0] and "Answer to C." in texts[1]
        assert "On second thought, no." in texts[2] and "First answer." not in texts[2], "no reply delivered twice"
        wait_for(lambda: repo.issues()["demo.1.3"]["metadata"].get("picked_up") == "2", "the second reply marked")
        page = until_shown(lambda: load(f"{served}/"), lambda p: "On second thought, no." in p
                           and "Reply saved" not in p, None)
        assert "delivered to the agent&#x27;s session" in page or "delivered to the agent's session" in page
    assert "pm reply read" not in repo.pm("show").stdout


@pytest.mark.slow
def test_a_reply_to_an_ended_session_is_flagged_and_read_with_pm_reply_read(repo, served):
    """No socket at the stored inbox: the session ended. The reply stays undelivered, its card says so, pm show
    flags it for the next session, and pm reply read prints it once and marks it delivered."""
    repo.env = dict(repo.env, CLAUDE_CODE_SESSION_ID="sess-1", CLAUDE_CODE_MESSAGING_SOCKET="/nonexistent/pm/s")
    assert repo.pm("decision", "need", "--title", "Parser?", "--parent", "demo.1", stdin=NEED).returncode == 0
    until_shown(lambda: load(f"{served}/"), lambda p: 'value="demo.1.3"' in p, time.time())
    assert post_reply(served, {"token": page_token(served), "id": "demo.1.3", "text": "Small, until tables."})[0] == 303
    page = until_shown(lambda: load(f"{served}/"), lambda p: "Small, until tables." in p and "Saving" not in p, None)
    assert "the session that asked is not running" in page
    assert "picked_up" not in repo.issues()["demo.1.3"]["metadata"]
    assert "Parser?  (sprint 1)  -> bd show demo.1.3  [undelivered reply: pm reply read demo.1.3]" \
        in repo.pm("show").stdout
    res = repo.pm("reply", "read")  # no ids: this session's open requests
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith("pm: owner reply to decision demo.1.3 (Parser?), relayed from the site:\n  [")
    assert "Small, until tables." in res.stdout
    assert repo.issues()["demo.1.3"]["metadata"]["picked_up"] == "1"
    assert "pm reply read" not in repo.pm("show").stdout
    assert repo.pm("reply", "read", "demo.1.3").stdout == "nothing undelivered on demo.1.3\n"
    repo.log.write_text("")  # refused() checks that nothing at all was written to Beads
    refused(repo, "reply", "read", "demo.1.1", match=r"demo.1.1 is not a request to the owner")
    repo.env.pop("CLAUDE_CODE_SESSION_ID")
    refused(repo, "reply", "read", match=r"name the requests to read")


REVIEWED_PR = "https://github.com/o/r/pull/7"


def review_with_origin(repo) -> str:
    """An open review of REVIEWED_PR under demo.1, and an origin remote whose main is one commit ahead:
    the sha returned."""
    issues = json.loads(repo.state.read_text()) + [
        {"id": "demo.1.3", "title": "Review PR #7", "status": "open", "issue_type": "task", "parent": "demo.1",
         "labels": ["human", "action"], "created_at": "2026-10-01T12:00:00Z",
         "metadata": {"review": {"pr": REVIEWED_PR, "sprints": ["demo.1"], "focus": "f", "designs": []}}}]
    repo.state.write_text(json.dumps(issues))
    origin = repo.root.parent / "origin.git"
    repo.git("init", "-q", "--bare", "-b", "main", str(origin))
    repo.git("remote", "add", "origin", str(origin))
    repo.git("push", "-q", "origin", "main")
    other = repo.root.parent / "other"
    repo.git("clone", "-q", str(origin), str(other))
    repo.git("-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "merge",
             cwd=other)
    repo.git("push", "-q", "origin", "HEAD:main", cwd=other)
    return repo.git("rev-parse", "HEAD", cwd=other).strip()


def pull_main(repo) -> str:
    """The pull a merged review's wake names: the main checkout as git reports it (resolved, as on macOS /private)."""
    common = repo.git("rev-parse", "--path-format=absolute", "--git-common-dir").strip()
    return f"git -C {Path(common).parent} pull --ff-only origin main"


@pytest.mark.slow
def test_a_reviewed_prs_merge_is_pushed_into_the_session_once(repo):
    repo.dolt()
    sha = review_with_origin(repo)
    repo.set_pr(REVIEWED_PR, "MERGED", sha)
    with session_inbox() as (inbox, lines):
        repo.set_issue("demo.1.3", metadata={**repo.issues()["demo.1.3"]["metadata"], "session": "sess-1",
                                             "inbox": inbox, "inbox_host": socket.gethostname()})
        with serving(repo):
            wait_for(lambda: lines, "the merge pushed")
            wait_for(lambda: repo.issues()["demo.1.3"]["metadata"].get("merge_reported") == sha, "the merge marked")
        assert inbox_text(lines[0]) == (f"pm: PR #7 of review demo.1.3 (Review PR #7) merged to main as {sha}\n"
                                    f"next: pm action done demo.1.3 --reason \"merged as {sha}\", then update the "
                                    f"main checkout: {pull_main(repo)}")
        assert repo.issues()["demo.1.3"]["metadata"]["merged"] == sha
        assert repo.issues()["demo.1.3"]["status"] == "open", "the agent closes the review, not the pm service"
        with serving(repo) as (url, _):  # a restart: the merge is stored, so it is neither looked up nor pushed again
            until_shown(lambda: load(f"{url}/"), lambda p: "Review PR #7" in p, None)
            deadline = time.time() + 2
            while time.time() < deadline:
                assert len(lines) == 1
                time.sleep(0.1)
    assert repo.pm("reply", "read", "demo.1.3").stdout == "nothing undelivered on demo.1.3\n"


# ---------------------------------------------------------------- pm task claim

def transcript(repo, sid: str, age_s: float, where: str = "-repo") -> None:
    """Session sid's Claude Code transcript under the test's config dir, last written age_s seconds ago."""
    p = Path(repo.env["CLAUDE_CONFIG_DIR"]) / "projects" / where / f"{sid}.jsonl"
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text("{}\n")
    t = time.time() - age_s
    os.utime(p, (t, t))


def claim(repo, task: str, sid: str | None, *extra: str):
    env = dict(repo.env, **({"CLAUDE_CODE_SESSION_ID": sid} if sid else {}))
    return subprocess.run([*PM, "task", "claim", task, *extra], cwd=repo.root, env=env,
                          capture_output=True, text=True)


def test_task_claim_refuses_a_task_another_live_session_holds(repo):
    repo.set_issue("demo.1.2", labels=[])
    assert claim(repo, "demo.1.2", "sess-a").returncode == 0
    i = repo.issues()["demo.1.2"]
    assert i["status"] == "in_progress" and i["metadata"]["claimed_by"] == "sess-a"
    transcript(repo, "sess-a", 60, where="-other-worktree")
    repo.log.write_text("")
    res = claim(repo, "demo.1.2", "sess-b")
    assert res.returncode == 1
    assert "held by live session sess-a" in res.stderr and "last 30 minutes" in res.stderr
    assert not repo.bd_writes()
    # the same session (a subagent shares it) may claim again
    assert claim(repo, "demo.1.2", "sess-a").returncode == 0


# ---------------------------------------------------------------- pm push: what the service runs

DAY2 = "---\ntype: day\ndate: 2026-10-02\n---\n\n## Today\n\n> What are we chasing today, and why now?\n\nMore.\n"


@pytest.fixture
def pushed(repo, tmp_path):
    """The repo with a bare origin holding its records branch, and a second clone of origin (repo.other) that can
    move it."""
    origin = tmp_path / "origin.git"
    repo.git("init", "-q", "--bare", str(origin))
    repo.git("remote", "add", "origin", str(origin))
    repo.git("push", "-q", "origin", "records", cwd=repo.store)
    repo.git("fetch", "-q", "origin")
    other = tmp_path / "other"
    repo.git("clone", "-q", "-b", "records", str(origin), str(other))
    repo.git("config", "user.email", "o@example.com", cwd=other)
    repo.git("config", "user.name", "o", cwd=other)
    repo.other = other
    return repo


def remote_records(repo) -> str:
    return repo.git("rev-parse", "refs/heads/records", cwd=repo.root.parent / "origin.git").strip()


def push_state(repo) -> dict:
    return json.loads((repo.root / ".pm/run/push.json").read_text())


def move_remote(repo, rel: str, text: str) -> None:
    (repo.other / rel).parent.mkdir(parents=True, exist_ok=True)
    (repo.other / rel).write_text(text)
    repo.git("add", "-A", cwd=repo.other)
    repo.git("commit", "-qm", "remote", cwd=repo.other)
    repo.git("push", "-q", "origin", "records", cwd=repo.other)


@pytest.mark.slow
def test_push_pushes_beads_and_new_records_commits(pushed):
    repo = pushed
    first = repo.pm("push").stdout
    assert first.count(" ok: ") == 3 and "summary ok: summarized" in first, first
    repo.write("days/2026-10-02.md", DAY2)
    repo.commit("a day")
    res = repo.pm("push")
    assert res.returncode == 0, res.stdout + res.stderr
    assert ["dolt", "push"] in repo.bd_calls()
    assert remote_records(repo) == repo.git("rev-parse", "HEAD", cwd=repo.store).strip()
    state = push_state(repo)
    assert state["beads"]["ok"] and state["records"]["ok"] and state["records"]["message"] == "pushed 2 commit(s)", "the day file and its new summary"
    assert state["records"]["last_ok"] == state["records"]["at"]
    assert len((repo.root / ".pm/run/push.log").read_text().splitlines()) == 6
    assert state["summary"]["message"].startswith("summarized"), "the new day file changed the activity"


@pytest.mark.slow
def test_push_rebases_onto_a_moved_remote(pushed):
    repo = pushed
    move_remote(repo, "docs/remote.md", "x\n")
    repo.write("days/2026-10-02.md", DAY2)
    repo.commit("local")
    res = repo.pm("push")
    assert res.returncode == 0, res.stdout + res.stderr
    assert repo.store_log()[1:3] == ["local", "remote"] and repo.store_log()[0].startswith("pm: summarized ")
    assert remote_records(repo) == repo.git("rev-parse", "HEAD", cwd=repo.store).strip()
    assert push_state(repo)["records"]["message"] == "pushed 2 commit(s) after rebasing onto 1 new on origin/records"


# ---------------------------------------------------------------- day pages and pm day summarize

def now_z() -> str:
    from datetime import datetime, timezone
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def claude_calls(repo) -> list[dict]:
    log = Path(repo.env["FAKE_CLAUDE_LOG"])
    return [json.loads(l) for l in log.read_text().splitlines()] if log.exists() else []


def summary(repo) -> dict:
    return json.loads((repo.records / f"days/{TODAY}.summary.json").read_text())


def test_day_summarize_skips_unchanged_activity_and_regenerates_on_change(repo):
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stderr
    assert committed(repo, [f"pm: summarized {TODAY} in records/days/{TODAY}.summary.json"])
    first = summary(repo)
    assert first["text"] == "Summary 1." and first["date"] == TODAY
    call = claude_calls(repo)[0]
    assert call["args"][:4] == ["-p", "--model", "haiku", "--tools"]
    assert "records commit: records [" in call["stdin"], "today's records commits are the activity"
    res = repo.pm("day", "summarize")
    assert res.returncode == 0 and "unchanged" in res.stdout and len(claude_calls(repo)) == 1
    repo.set_issue("demo.1.2", status="in_progress", started_at=now_z())
    res = repo.pm("day", "summarize")
    assert res.returncode == 0, res.stderr
    assert summary(repo)["text"] == "Summary 2." and summary(repo)["digest"] != first["digest"]
    assert "started: Ask the owner" in claude_calls(repo)[1]["stdin"]
    assert repo.pm("check").returncode == 0
    page = repo.page(f"days/{TODAY}.html")
    assert re.search(r"<span>generated at \d\d:\d\d</span>.*Summary 2\.", page, re.S), page
    assert "Summary 2." in repo.page("index.html")
    assert f"today {TODAY}: Summary 2. (generated " in repo.pm("show").stdout
