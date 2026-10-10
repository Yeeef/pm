"""release/changelog.py: the checks CHANGELOG.md's preamble states, the notes the release workflow publishes, and the
PR check. The expected results come from those rules, on small changelogs built here."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

import pytest

PM_DIR = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("changelog", PM_DIR / "release" / "changelog.py")
changelog = sys.modules["changelog"] = importlib.util.module_from_spec(spec)  # dataclasses look it up
spec.loader.exec_module(changelog)

pytestmark = pytest.mark.impl("python", reason="tests the release tooling, not a pm implementation")
REPO = "https://github.com/Yeeef/pm"

GUIDE = "### Upgrade guide\n\n1. Install it.\n2. Run `pm upgrade\n   --to X`.\n"
UNRELEASED = "## [Unreleased]\n\n" + GUIDE + "\n### Fixed\n\n- A fix not yet\n  released.\n"
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
LINKS = (f"[Unreleased]: {REPO}/compare/pm-v0.2.1...HEAD\n"
         f"[0.2.1]: {REPO}/compare/pm-v0.2.0...pm-v0.2.1\n"
         f"[0.2.0]: {REPO}/compare/pm-v0.1.6...pm-v0.2.0\n")


def text(unreleased=UNRELEASED, releases=(RELEASE_2, RELEASE_1), links=LINKS):
    return "# Changelog\n\nThe preamble.\n\n" + "\n".join([unreleased, *releases]) + "\n" + links


def refused(t: str) -> str:
    with pytest.raises(changelog.ChangelogError) as e:
        changelog.parse(t)
    return str(e.value)


def test_a_valid_changelog_passes_and_gives_each_release_its_notes_unwrapped():
    log = changelog.parse(text())
    assert [s.name for s in log.sections] == ["Unreleased", "0.2.1", "0.2.0"]
    assert changelog.notes(log, "0.2.1") == (
        "Texts name the work store.\n\n" + GUIDE_OUT + "\n\n"
        "### Breaking changes\n\n- Something moved (step 2).\n\n"
        "### Added\n\n- A new command.")
    assert changelog.notes(log, "0.2.0") == "The first Go release.\n\n" + GUIDE_OUT + "\n\n### Changed\n\n- One binary."


def test_a_release_with_no_section_has_no_notes():
    log = changelog.parse(text())
    with pytest.raises(changelog.ChangelogError, match=r"no '## \[0.3.0\] - <date>' section"):
        changelog.notes(log, "0.3.0")


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


def test_a_pre_release_takes_its_version_s_section_else_unreleased_which_must_not_be_empty():
    log = changelog.parse(text())
    assert changelog.notes(log, "0.2.1-rc.1") == changelog.notes(log, "0.2.1")
    assert changelog.notes(log, "0.3.0-rc.1") == GUIDE_OUT + "\n\n### Fixed\n\n- A fix not yet released."
    empty = changelog.parse(text(unreleased="## [Unreleased]\n"))
    with pytest.raises(changelog.ChangelogError, match=r"\[Unreleased\] section is empty"):
        changelog.notes(empty, "0.3.0-rc.1")
    unguided = changelog.parse(text(unreleased=UNRELEASED.replace(GUIDE + "\n", "")))
    with pytest.raises(changelog.ChangelogError, match=r"\[Unreleased\] section has no '### Upgrade guide'"):
        changelog.notes(unguided, "0.3.0-rc.1")


def test_versions_dates_headings_and_links_are_checked():
    assert "newest first" in refused(text(releases=(RELEASE_1, RELEASE_2)))
    assert "2026-13-01 is not a valid date" in refused(text(releases=(RELEASE_2.replace("2026-10-09", "2026-13-01"),
                                                                      RELEASE_1)))
    assert "a release heading is" in refused(text(releases=(RELEASE_2.replace("## [0.2.1] - ", "## 0.2.1 "),
                                                            RELEASE_1)))
    assert "'## [Unreleased]' must come before" in refused(text(unreleased="", releases=(RELEASE_2, RELEASE_1)))
    assert "[Unreleased] must be the first" in refused(text(releases=(RELEASE_2, UNRELEASED, RELEASE_1)))
    assert "only '#' title" in refused(text(releases=(RELEASE_2 + "\n#### Detail\n", RELEASE_1)))
    assert "[0.2.1] has no link reference" in refused(text(links=LINKS.replace(
        f"[0.2.1]: {REPO}/compare/pm-v0.2.0...pm-v0.2.1\n", "")))
    assert "links" in refused(text(links=LINKS.replace("pm-v0.2.1...HEAD", "pm-v0.2.0...HEAD")))


def test_a_change_that_ships_needs_an_unreleased_entry():
    base, head = text(), text(unreleased=UNRELEASED + "\n### Security\n\n- New.\n")
    shipping = ["internal/work/dolt.go", "tests/test_pm.py"]
    assert changelog.needs_entry(shipping, base, base) == ["internal/work/dolt.go"]
    assert changelog.needs_entry(shipping, base, head) == []
    assert changelog.needs_entry(["tests/test_pm.py", "internal/work/dolt_test.go", "AGENTS.md",
                                  "internal/service/testdata/units/x", "src/pm/cli.py"], base, base) == []
    assert changelog.needs_entry(["prime.md", "src/pm/prompts/day_summary.txt", "go.sum"], base, base) == [
        "prime.md", "src/pm/prompts/day_summary.txt", "go.sum"]
    assert changelog.needs_entry(shipping, None, base) == []  # the change adds the changelog


def test_the_real_changelog_passes():
    log = changelog.parse((PM_DIR / "CHANGELOG.md").read_text())
    assert log.sections[0].name == "Unreleased"
