"""Records workspaces (the record-drafts design page, steps 2 to 4): session start's pm init makes each code worktree's
records/ a worktree of its own branch drafts/<name>; pm commit there publishes journals on the records branch, merging
disjoint edits and refusing an overlap with the other commit named, and drafts design pages, which pm sprint close
publishes; the site serves each workspace as a version."""
from __future__ import annotations

import urllib.request

import pytest

from conftest import wait_for
from test_hooks import init_ready
from test_pm import design_page, report

pytestmark = pytest.mark.integration  # pm init in each worktree, and the site from the installed pm service

SPRINT = "records/sprints/demo-1.md"


def get(repo, path: str) -> str:
    with urllib.request.urlopen(f"http://127.0.0.1:{repo.env['PORT']}/{path}") as r:
        return r.read().decode()


def replace(path, old: str, new: str) -> None:
    text = path.read_text()
    assert old in text, (path, old)
    path.write_text(text.replace(old, new, 1))


def test_two_worktrees_publish_disjoint_edits_refuse_an_overlap_and_show_as_versions(repo):
    init_ready(repo)
    trees = [repo.worktree(name) for name in ("one", "two")]
    for wt in trees:
        res = repo.pm("init", "--session-start", cwd=wt)
        assert res.returncode == 0, res.stderr
        assert f"made {wt / 'records'} a records workspace on branch drafts/{wt.name}" in res.stdout, res.stdout
    one, two = (wt / "records/sprints/demo-1.md" for wt in trees)
    replace(one, "Ship it.", "Ship it, one.")
    replace(two, "- It works.", "- It works, two.")

    # the site: the published page is unchanged, and each workspace is a version of it, labelled with its worktree
    def versions():
        page = get(repo, "sprints/demo-1.html")
        return "worktree one on one" in page and "worktree two on two" in page and page
    page = wait_for(versions, "both workspaces listed as versions")
    assert "Ship it, one." not in page and "uncommitted edits" in page
    assert "Ship it, one." in get(repo, "drafts/one/sprints/demo-1.html")
    assert "It works, two." in get(repo, "drafts/two/sprints/demo-1.diff.html")

    # disjoint edits from both reach records
    for wt, msg in zip(trees, ("one's goal", "two's done when"), strict=True):
        res = repo.pm("commit", "-m", msg, SPRINT, cwd=wt)
        assert res.returncode == 0, res.stderr
    published = repo.git("show", "records:sprints/demo-1.md")
    assert "Ship it, one." in published and "- It works, two." in published

    # an overlap is refused at the second commit, naming the first, with markers only in the second workspace
    replace(one, "Ship it, one.", "Ship it, one again.")
    replace(two, "Ship it, one.", "Ship it, two.")
    assert repo.pm("commit", "-m", "one again", SPRINT, cwd=trees[0]).returncode == 0
    first = repo.git("rev-parse", "--short", "records").strip()
    res = repo.pm("commit", "-m", "two's goal", SPRINT, cwd=trees[1])
    assert res.returncode == 1, res.stdout
    assert f"error: {SPRINT} has conflict markers, which records changed under your edit in {first} one again" \
        in res.stderr, res.stderr
    assert "<<<<<<< workspace" in two.read_text()
    assert "<<<<<<<" not in one.read_text() and "<<<<<<<" not in (repo.store / "sprints/demo-1.md").read_text()
    assert repo.git("rev-parse", "--short", "records").strip() == first
    located = repo.pm("where", cwd=trees[1]).stdout
    assert (f"records workspace on drafts/two, 1 uncommitted files (conflict markers in {SPRINT}), on the records "
            "tip") in located, located


def test_a_design_page_drafted_in_two_worktrees_publishes_at_the_sprint_close(repo):
    """Two worktrees draft one design page for one sprint: the site's page stays the published one, each draft a
    version of it, until pm sprint close publishes both drafts in one records commit."""
    init_ready(repo)
    (repo.records / "design").mkdir()
    repo.write("design/parser.md", design_page("[]").replace("## Design\n\nNone yet.", "## Design\n\nA.\n\nB."))
    replace(repo.records / "sprints/demo-1.md", "## Design pages\n\n> Where is the detail?\n\nNone yet.",
            "## Design pages\n\n> Where is the detail?\n\n- [The parser](../design/parser.md)")
    repo.commit("the parser page, linked by the sprint")
    trees = [repo.worktree(name) for name in ("one", "two")]
    for wt, old, new in zip(trees, ("A.", "B."), ("A, by one.", "B, by two."), strict=True):
        assert repo.pm("init", "--session-start", cwd=wt).returncode == 0
        replace(wt / "records/design/parser.md", old, new)
        res = repo.pm("commit", "-m", f"{wt.name}'s parser", "--sprint", "repo-demo.1", "records/design/parser.md",
                      cwd=wt)
        assert res.returncode == 0 and "pm sprint close repo-demo.1 publishes it" in res.stdout, res
    assert "A, by one." not in repo.git("show", "records:design/parser.md")

    def versions():
        page = get(repo, "design/parser.html")
        return "worktree one on one" in page and "worktree two on two" in page and page
    page = wait_for(versions, "both drafts listed as versions")
    assert "A, by one." not in page and "B, by two." not in page and "committed" in page
    assert "B, by two." in get(repo, "drafts/two/design/parser.html")

    repo.set_issue("repo-demo.1.2", status="closed", close_reason="Dismissed", closed_at="2026-10-02T12:00:00Z")
    report(repo, "Done: published the parser.")
    before = repo.git("rev-parse", "records").strip()
    res = repo.pm("sprint", "close", "repo-demo.1", cwd=trees[0])
    assert res.returncode == 0 and "published design/parser.md" in res.stdout, res
    assert repo.git("rev-list", "--count", f"{before}..records").strip() == "1"
    published = repo.git("show", "records:design/parser.md")
    assert "A, by one." in published and "B, by two." in published
    wait_for(lambda: "B, by two." in get(repo, "design/parser.html"), "the published page with both drafts")
