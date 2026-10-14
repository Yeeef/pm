"""AGENTS.md stays a map every runtime reads whole: at most 20 KiB (the owner's cap, well under the 32 KiB Codex reads
of the instruction files from the repo root down), and CLAUDE.md is a link to it, so both runtimes read one file."""

from __future__ import annotations

import os
from pathlib import Path

PM_DIR = Path(__file__).resolve().parent.parent
CAP = 20_480


def test_agents_md_fits_the_cap():
    size = (PM_DIR / "AGENTS.md").stat().st_size
    assert size <= CAP, f"AGENTS.md is {size:,} bytes, over the {CAP:,}-byte cap: move detail next to its code"


def test_claude_md_links_to_agents_md():
    claude = PM_DIR / "CLAUDE.md"
    assert claude.is_symlink() and os.readlink(claude) == "AGENTS.md", "CLAUDE.md must be a symlink to AGENTS.md"
