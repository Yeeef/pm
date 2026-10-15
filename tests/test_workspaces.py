"""Records workspaces (the record-drafts design page, step 2): session start's pm init makes each code worktree's
records/ a worktree of its own branch drafts/<name>; pm commit there publishes journals on the records branch, merging
disjoint edits and refusing an overlap with the other commit named; the site serves each workspace as a version."""
from __future__ import annotations

import urllib.request

import pytest

from conftest import wait_for
from test_hooks import init_ready

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
