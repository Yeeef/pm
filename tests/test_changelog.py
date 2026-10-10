"""release/changelog.py: the checks CHANGELOG.md's preamble states, on CHANGELOG.md and the entry files in
changelog.d/, the notes the release workflow publishes, the release section it writes, and the PR check. The expected
results come from those rules, on small changelogs built here."""

from __future__ import annotations

import importlib.util
import os
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

PM_DIR = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("changelog", PM_DIR / "release" / "changelog.py")
changelog = sys.modules["changelog"] = importlib.util.module_from_spec(spec)  # dataclasses look it up
spec.loader.exec_module(changelog)

REPO = "https://github.com/Yeeef/pm"

GUIDE = "### Upgrade guide\n\n1. Install it.\n2. Run `pm upgrade\n   --to X`.\n"
RELEASE_2 = """## [0.2.1] - 2026-10-09

Texts name the work
store.

""" + GUIDE + """
### Breaking changes

- Something moved (step 2).

### Added

- A new command.
"""
RELEASE_1 = "## [0.2.0] - 2026-10-08\n\nThe first Go release.\n\n" + GUIDE + "\n### Changed\n\n- One binary.\n"
GUIDE_OUT = "### Upgrade guide\n\n1. Install it.\n2. Run `pm upgrade --to X`."
LINKS = (f"[0.2.1]: {REPO}/compare/pm-v0.2.0...pm-v0.2.1\n"
         f"[0.2.0]: {REPO}/compare/pm-v0.1.6...pm-v0.2.0\n")


def text(releases=(RELEASE_2, RELEASE_1), links=LINKS):
    return "# Changelog\n\nThe preamble.\n\n" + "\n".join(releases) + "\n" + links


def refused(t: str) -> str:
    with pytest.raises(changelog.ChangelogError) as e:
        changelog.parse(t)
    return str(e.value)


def entry_refused(name: str, t: str) -> str:
    with pytest.raises(changelog.ChangelogError) as e:
        changelog.parse_entry(name, t)
    return str(e.value)


def guide(version: str) -> str:
    return (f"### Upgrade guide\n\n1. On each machine, install the release: "
            f"`curl -fsSL {REPO}/releases/download/pm-v{version}/install.sh | sh`.\n"
            f"2. In the repo, run `pm upgrade --to {version}`, then commit what it changes and merge it, as an "
            "ordinary PR.")


def test_a_valid_changelog_passes_and_gives_each_release_its_notes_unwrapped():
    log = changelog.parse(text())
    assert [s.name for s in log.sections] == ["0.2.1", "0.2.0"]
    assert changelog.notes(log, [], "0.2.1") == (
        "Texts name the work store.\n\n" + GUIDE_OUT + "\n\n"
        "### Breaking changes\n\n- Something moved (step 2).\n\n"
        "### Added\n\n- A new command.")
    assert changelog.notes(log, [], "0.2.0") == "The first Go release.\n\n" + GUIDE_OUT + "\n\n### Changed\n\n- One binary."


def test_a_release_with_no_section_has_no_notes():
    log = changelog.parse(text())
    with pytest.raises(changelog.ChangelogError, match=r"no '## \[0.3.0\] - <date>' section"):
        changelog.notes(log, [changelog.parse_entry("a.md", "### Fixed\n\n- A fix.\n")], "0.3.0")


def test_an_unknown_category_is_refused():
    improvements = RELEASE_2.replace("### Added", "### Improvements")
    assert "unknown category 'Improvements'" in refused(text(releases=(improvements, RELEASE_1)))


def test_categories_out_of_order_are_refused():
    swapped = RELEASE_1.replace("### Changed\n\n- One binary.\n", "### Changed\n\n- One binary.\n\n### Added\n\n- X.\n")
    assert "category 'Added' comes after 'Changed'" in refused(text(releases=(RELEASE_2, swapped)))


def test_an_empty_category_or_release_section_is_refused():
    assert "empty 'Added' category" in refused(text(releases=(RELEASE_2.replace("- A new command.\n", ""), RELEASE_1)))
    assert "[0.2.0] needs an '### Upgrade guide'" in refused(text(releases=(RELEASE_2,
                                                                           "## [0.2.0] - 2026-10-08\n\nOnly words.\n")))
    assert "[0.2.0] needs a summary" in refused(text(releases=(RELEASE_2, RELEASE_1.replace("The first Go release.\n",
                                                                                            ""))))


def test_a_release_without_an_upgrade_guide_is_refused():
    assert "[0.2.1] needs an '### Upgrade guide'" in refused(text(releases=(RELEASE_2.replace(GUIDE, ""), RELEASE_1)))


def test_an_upgrade_guide_that_is_not_a_numbered_list_is_refused():
    bullets = RELEASE_2.replace(GUIDE, "### Upgrade guide\n\n- Install it.\n")
    assert "is a numbered list" in refused(text(releases=(bullets, RELEASE_1)))
    skipped = RELEASE_2.replace("2. Run", "3. Run")
    assert "step 2 should start '2. '" in refused(text(releases=(skipped, RELEASE_1)))
    empty = RELEASE_2.replace(GUIDE, "### Upgrade guide\n\n")
    assert "empty 'Upgrade guide' category" in refused(text(releases=(empty, RELEASE_1)))


def test_an_upgrade_guide_after_another_category_is_refused():
    late = RELEASE_1.replace(GUIDE + "\n### Changed\n\n- One binary.\n", "### Changed\n\n- One binary.\n\n" + GUIDE)
    assert "category 'Upgrade guide' comes after 'Changed'" in refused(text(releases=(RELEASE_2, late)))


def test_versions_dates_headings_and_links_are_checked():
    assert "newest first" in refused(text(releases=(RELEASE_1, RELEASE_2)))
    assert "2026-13-01 is not a valid date" in refused(text(releases=(RELEASE_2.replace("2026-10-09", "2026-13-01"),
                                                                      RELEASE_1)))
    assert "a release heading is" in refused(text(releases=(RELEASE_2.replace("## [0.2.1] - ", "## 0.2.1 "),
                                                            RELEASE_1)))
    assert "only '#' title" in refused(text(releases=(RELEASE_2 + "\n#### Detail\n", RELEASE_1)))
    assert "[0.2.1] has no link reference" in refused(text(links=LINKS.replace(
        f"[0.2.1]: {REPO}/compare/pm-v0.2.0...pm-v0.2.1\n", "")))
    assert "links" in refused(text(links=LINKS.replace("pm-v0.2.0...pm-v0.2.1", "pm-v0.1.6...pm-v0.2.1")))


def test_an_unreleased_section_in_changelog_md_is_refused_naming_the_entry_files():
    unreleased = "## [Unreleased]\n\n### Fixed\n\n- A fix.\n"
    assert "an unreleased change is its own file, changelog.d/<slug>.md" in refused(
        text(releases=(unreleased, RELEASE_2, RELEASE_1)))
    assert "link reference [Unreleased] names no release section" in refused(text(
        links=f"[Unreleased]: {REPO}/compare/pm-v0.2.1...HEAD\n" + LINKS))


def test_an_entry_file_holds_only_categories_and_their_items():
    entry = changelog.parse_entry("fix-x.md", "### Added\n\n- A command.\n\n### Fixed\n\n- A fix\n  wrapped.\n")
    assert entry.where == "changelog.d/fix-x.md"
    assert list(entry.categories) == ["Added", "Fixed"]
    assert "its first line is a category heading" in entry_refused("a.md", "A fix.\n\n### Fixed\n\n- A fix.\n")
    assert "at least one '### ' category" in entry_refused("a.md", "\n")
    assert "an entry holds only '### ' category headings" in entry_refused("a.md", "## [0.3.0] - 2026-10-10\n")
    assert "unknown category 'Improvements'" in entry_refused("a.md", "### Improvements\n\n- X.\n")
    assert "changelog.d/a.md:3: a category holds only '- ' bullets" in entry_refused("a.md", "### Fixed\n\nA fix.\n")
    assert "empty 'Fixed' category" in entry_refused("a.md", "### Fixed\n\n")
    assert "category 'Added' comes after 'Fixed'" in entry_refused("a.md", "### Fixed\n\n- F.\n\n### Added\n\n- A.\n")
    assert "step 1 should start '1. '" in entry_refused("a.md", "### Upgrade guide\n\n3. Restart.\n")
    assert "the slug lowercase letters" in entry_refused("Fix X.md", "### Fixed\n\n- A fix.\n")
    assert "the slug lowercase letters" in entry_refused("README", "### Fixed\n\n- A fix.\n")


def test_entries_assemble_by_category_in_name_order_with_the_guide_s_steps_after_the_two_every_release_has():
    b = changelog.parse_entry("b.md", "### Upgrade guide\n\n1. Restart the\n   service.\n\n### Fixed\n\n- Fix B.\n")
    a = changelog.parse_entry("a.md", "### Added\n\n- New A.\n\n### Fixed\n\n- Fix A.\n")
    c = changelog.parse_entry("c.md", "### Upgrade guide\n\n1. Rerun init.\n2. Commit it.\n")
    entries = sorted([b, a, c], key=lambda e: e.where)
    assert changelog.notes(changelog.parse(text()), entries, "0.3.0-rc.1") == (
        guide("0.3.0-rc.1") + "\n3. Restart the service.\n4. Rerun init.\n5. Commit it.\n\n"
        "### Added\n\n- New A.\n\n### Fixed\n\n- Fix A.\n- Fix B.")
    with pytest.raises(changelog.ChangelogError, match=r"changelog.d/ holds no entries; pre-release 0.3.0-rc.1"):
        changelog.notes(changelog.parse(text()), [], "0.3.0-rc.1")


def test_a_pre_release_takes_its_version_s_section_when_there_is_one():
    log = changelog.parse(text())
    entries = [changelog.parse_entry("a.md", "### Fixed\n\n- A fix.\n")]
    assert changelog.notes(log, entries, "0.2.1-rc.1") == changelog.notes(log, entries, "0.2.1")


def test_release_writes_x_s_section_above_the_newest_with_its_link():
    entries = [changelog.parse_entry("a.md", "### Fixed\n\n- Fix A.\n"),
               changelog.parse_entry("b.md", "### Added\n\n- New B.\n")]
    out = changelog.release(text(), entries, "0.3.0", "2026-10-10", "Two changes.")
    log = changelog.parse(out)
    assert [s.name for s in log.sections] == ["0.3.0", "0.2.1", "0.2.0"]
    assert log.links["0.3.0"][1] == f"{REPO}/compare/pm-v0.2.1...pm-v0.3.0"
    assert changelog.notes(log, [], "0.3.0") == (
        "Two changes.\n\n" + guide("0.3.0") + "\n\n### Added\n\n- New B.\n\n### Fixed\n\n- Fix A.")
    assert out.index("## [0.3.0] - 2026-10-10") < out.index("## [0.2.1]")
    with pytest.raises(changelog.ChangelogError, match=r"already has \[0.2.1\]"):
        changelog.release(text(), entries, "0.2.1", "2026-10-10", "S.")
    with pytest.raises(changelog.ChangelogError, match="holds no entries"):
        changelog.release(text(), [], "0.3.0", "2026-10-10", "S.")
    with pytest.raises(changelog.ChangelogError, match="newest first"):
        changelog.release(text(), entries, "0.1.9", "2026-10-10", "S.")
    with pytest.raises(changelog.ChangelogError, match="summary paragraph"):
        changelog.release(text(), entries, "0.3.0", "2026-10-10", " ")


def test_a_change_that_ships_needs_an_entry_file():
    shipping = ["internal/work/dolt.go", "tests/test_pm.py"]
    assert changelog.needs_entry(shipping, shipping) == ["internal/work/dolt.go"]
    assert changelog.needs_entry(shipping + ["changelog.d/fix.md"], shipping + ["changelog.d/fix.md"]) == []
    # deleting another change's entry is not an entry
    assert changelog.needs_entry(shipping + ["changelog.d/old.md"], shipping) == ["internal/work/dolt.go"]
    assert changelog.needs_entry(["CHANGELOG.md"] + shipping, ["CHANGELOG.md"] + shipping) == ["internal/work/dolt.go"]
    assert changelog.needs_entry(["tests/test_pm.py", "internal/work/dolt_test.go", "AGENTS.md",
                                  "internal/service/testdata/units/x", "tests/render-pages/main.go"], []) == []
    assert changelog.needs_entry(["prime.md", "prompts/day_summary.txt", "style.css", "go.sum"], []) == [
        "prime.md", "prompts/day_summary.txt", "style.css", "go.sum"]


def test_two_branches_that_each_add_an_entry_merge_without_conflict_and_both_reach_the_release(tmp_path):
    env = {**os.environ, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t",
           "GIT_COMMITTER_EMAIL": "t@t"}

    def run(*args: str) -> subprocess.CompletedProcess:
        return subprocess.run(args, cwd=tmp_path, env=env, capture_output=True, text=True)

    def git(*args: str):
        r = run("git", *args)
        assert r.returncode == 0, r.stderr + r.stdout

    def change(branch: str, slug: str, entry: str):
        git("checkout", "-q", "-b", branch, "main")
        (tmp_path / "internal").mkdir(exist_ok=True)
        (tmp_path / "internal" / f"{slug}.go").write_text("package x\n")
        git("add", "-A")
        git("commit", "-q", "-m", f"{slug} without its entry")
        r = run(sys.executable, "release/changelog.py", "pr", "main")
        assert r.returncode == 1 and "adds no entry file: add changelog.d/<slug>.md" in r.stderr
        (tmp_path / "changelog.d").mkdir(exist_ok=True)
        (tmp_path / "changelog.d" / f"{slug}.md").write_text(entry)
        git("add", "-A")
        git("commit", "-q", "-m", f"{slug}'s entry")
        r = run(sys.executable, "release/changelog.py", "pr", "main")
        assert r.returncode == 0, r.stderr

    (tmp_path / "release").mkdir()
    shutil.copy(PM_DIR / "release" / "changelog.py", tmp_path / "release" / "changelog.py")
    (tmp_path / "CHANGELOG.md").write_text(text())
    git("init", "-q", "-b", "main")
    git("add", "-A")
    git("commit", "-q", "-m", "base")
    change("fix-a", "fix-a", "### Fixed\n\n- Fix A.\n")
    change("add-b", "add-b", "### Added\n\n- New B.\n\n### Fixed\n\n- Fix B.\n")
    git("checkout", "-q", "main")
    git("merge", "-q", "--no-ff", "-m", "merge fix-a", "fix-a")
    git("merge", "-q", "--no-ff", "-m", "merge add-b", "add-b")  # cut from the same base: no rebase between

    r = run(sys.executable, "release/changelog.py", "check")
    assert r.returncode == 0 and "2 entries" in r.stdout, r.stderr
    r = run(sys.executable, "release/changelog.py", "release", "0.3.0", "--summary", "A and B.", "--date",
            "2026-10-10")
    assert r.returncode == 0, r.stderr
    assert not list((tmp_path / "changelog.d").glob("*.md"))
    r = run(sys.executable, "release/changelog.py", "notes", "0.3.0")
    assert r.returncode == 0, r.stderr
    assert r.stdout == ("A and B.\n\n" + guide("0.3.0") + "\n\n### Added\n\n- New B.\n\n"
                        "### Fixed\n\n- Fix B.\n- Fix A.\n")  # entries in name order: add-b, fix-a


def test_the_real_changelog_and_entries_pass():
    log = changelog.parse((PM_DIR / "CHANGELOG.md").read_text())
    assert log.sections
    changelog.read_entries(PM_DIR)
