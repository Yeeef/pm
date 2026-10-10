---
type: postmortem
title: The site lost its styles after main moved pm into a package
date: 2026-10-07
sprint: yeeef-agents-9va.52
---

## Summary

> What broke, for whom, and how was it noticed?

The owner's site at http://localhost:8000 showed pages without styles after this session fast-forwarded the main checkout to PR #52's merge. The owner noticed it on the index page and reported "page is cooked".

## Timeline

> What happened when? Times with their zone, from the first cause to the fix.

- 2026-10-07 23:28 EDT (Oct 6): `make docs` started `uv run skills/project-management/harness/pm.py serve` in the main checkout.
- 2026-10-07 19:19 UTC: this session ran `git pull --ff-only origin main`, which brought PR #52 (35ef486); it moves the harness into the `pm/` package, so `harness/pm.py` and its `style.css` left the disk.
- Shortly after: the running server, still the old process, could no longer read `style.css`; `/style.css` failed, so every page rendered unstyled.
- About 19:27 UTC: the owner reported it; the session found the server was the old process and restarted the site with `make docs`, now `pm serve` from the package; `/style.css` returned 200.

## Cost

> What did it cost: time lost, sessions or people blocked, work redone?

A few minutes of an unstyled site for the owner; no records or Beads data were lost, and no other session was blocked.

## Root cause

> Why did it happen? The cause under the trigger.

A long-running server read its assets from files that a pull of main can remove. Fast-forwarding the main checkout to a commit that moves pm's code changes the files under any `pm serve` started before it, and nothing restarts it.

## What changed

> What was fixed, and what changed so it does not recur? Name the commits and PRs.

- The site was restarted from the package (`make docs`, now `uv run --project pm pm serve`).
- Sprint 45's pm service (PR #65) replaces `make docs`: a supervised process whose build is checked against the pin, so a pull that changes pm restarts it.
- The migration after PR #65's merge stops the `make docs` process before `pm init`, since the pm service it installs cannot take port 8000 while the old server holds it; the service then serves the site.

## What would have caught it earlier

> Which test, check or rule would have caught it before it cost anything?

A step in the sprint-close procedure: after fast-forwarding the main checkout to a commit that changes pm's code, restart the site, or check that `/style.css` answers. The pm service's version check covers this once it runs here.
