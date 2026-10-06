# /// script
# requires-python = ">=3.12"
# dependencies = ["markdown-it-py>=3", "mdit-py-plugins>=0.4", "pyyaml>=6"]
# ///
"""Render project records (Markdown) plus live Beads status into a static site.

Usage: uv run render.py OUT_DIR

Every record is a Markdown file with a YAML header. Its `type` decides how it
renders: project, sprint, day, design or doc. Fenced-div blocks are limited to a
fixed vocabulary; anything else is an error. Records are read from the store
(`<main checkout>/.records`), found from the current directory as pm finds it.
Status always comes from Beads (`bd list --all --json`), never from the records. Parsing, validation and
rendering live in the `harness` package beside this script, shared with pm.py.
"""

from __future__ import annotations

import sys
from pathlib import Path

from harness.beads import load_beads
from harness.records import RecordError, read_records, read_summaries
from harness.site import render_pages, write_site
from harness.store import code_root, design_dates, find_store


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print(__doc__.strip().splitlines()[2], file=sys.stderr)
        return 2
    site = Path(argv[1]).resolve()
    try:
        records = find_store(Path.cwd())
        repo = code_root(Path.cwd(), records)
        recs = read_records(records)
        pages = render_pages(recs, load_beads(repo), repo.name, dates=design_dates(records, recs),
                             summaries=read_summaries(records))
    except RecordError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    write_site(pages, site)
    print(f"rendered {len(pages)} pages into {site}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
