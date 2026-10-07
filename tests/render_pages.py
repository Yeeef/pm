"""Print every page of the site, as the pm service renders it, as one JSON object by path: what the tests of the
pages' HTML read, since pm writes no site to disk. Run from a checkout, with bd on PATH."""

import json
from pathlib import Path

from pm import cli
from pm.records import read_summaries
from pm.site import render_pages
from pm.store import design_dates, find_store

records = find_store(Path.cwd())
repo = cli.load(records, working=True)
print(json.dumps(render_pages(repo.recs, repo.beads, repo.root.name, dates=design_dates(records, repo.recs),
                              summaries=read_summaries(records))))
