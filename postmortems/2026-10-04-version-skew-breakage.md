---
type: postmortem
title: Version skew broke pm on main
date: 2026-10-04
sprint: yeeef-agents-9va.13
---

## Summary

> What broke, for whom, and how was it noticed?

On 2026-10-04 `pm` on `main`, and on every branch not built on
`owner-requests`, refused every command, `pm show` and `make render`
included, with "need … is closed but no decision cites it". The cause was
data, not code on those branches: needs closed by the newer `pm` on
`owner-requests` (sprint 13) carried labels and close reasons that the older
answered-need check did not know. Every session on another branch, and the
owner's site rendered from `main`, broke at once. Sessions noticed it when
their next `pm` command failed. It happened twice in one evening.

## Timeline

> What happened when? Times with their zone, from the first cause to the fix.

All times EDT (UTC-4), from `bd history`, the store's git log and
`gh pr view 14`.

- 17:44: the sprint 13 live check creates demo needs `yeeef-agents-9va.13.3`
  to `.13.5` and closes them with the new pm: an action (`action` label), an
  answer closed `--no-decision`, and a dismiss with a note. From here `pm` on
  `main` refuses every command.
- 17:49: the demo needs are deleted from Beads (their last history entry);
  `pm` on `main` works again.
- 17:52: the cause is recorded as a sprint 13 finding (version skew).
- 18:26: the context-efficiency session raises action
  `yeeef-agents-2sn.1.3` (review and merge PR #13).
- 18:29: it closes that action with `pm need done` from `owner-requests`:
  the same breakage, now from real data that cannot simply be deleted.
- 18:30: it reopens `2sn.1.3` and re-closes it as a bare dismiss, which old
  `pm` accepts, with a comment saying why.
- 18:32: hotfix PR #14 ("pm on main accepts needs closed by newer pm") is
  opened against `main`.
- 18:33: the sprint 13.6 live check raises need `yeeef-agents-9va.13.9` and,
  on the main agent's instruction, dismisses it with a note ("Dismissed: live
  check"); `main`'s `pm` exempts only a bare "Dismissed", so it refuses again.
  The context-efficiency session reports it later that evening (exact time
  not recorded); the main agent reopens `13.9` and dismisses it bare.
- 22:52: a build agent marks three delivery reports as drafts by starting
  their Outcome with "Draft:" (store commit `2762bb2`), a marker only its
  unmerged code reads; `pm show` on `main` and the owner's site fail.
- 22:55: the store commit is reverted (`8a722bd`); the draft marker is
  redesigned as a separate "Draft until … merges." paragraph that older `pm`
  ignores.
- 22:57: PR #14 is merged into `main` (`700be24`).

## Cost

> What did it cost: time lost, sessions or people blocked, work redone?

- Four occurrences in one day: about 6 minutes of a broken `pm` for every
  checkout the first time (17:44 to 17:49), under a minute the second (18:29
  to 18:30), an unrecorded stretch the third (a context-efficiency session
  was blocked from closing its sprint until it was fixed), and about 3
  minutes the fourth (22:52 to 22:55).
- Every concurrent session on another branch was blocked for those windows,
  as was the owner's view of the site rendered from `main`.
- Work redone: the sprint 13 demo needs were thrown away; `2sn.1.3` lost its
  real close (an action marked done) and was re-closed as a dismiss, its
  record kept only as a comment; a hotfix PR to `main` outside any sprint
  plan.
- Small in time, but it broke other sessions and the owner's view, which
  makes it due for a postmortem.

## Root cause

> Why did it happen? The cause under the trigger.

Beads and the records store are shared by every checkout of the clone, but
`pm`'s code is per branch. A branch's `pm` can write data in a shape only it
understands (new labels, new close reasons), and every older `pm` then reads
that data and validates the whole store on every command. `pm` fails hard on
data it does not expect, which is the intended rule, so one session using
unmerged code broke every other checkout. No test or rule checked that data
written by a branch stays readable by the `pm` on `main`.

## What changed

> What was fixed, and what changed so it does not recur? Name the commits and PRs.

- The demo needs `.13.3` to `.13.5` were deleted.
- `yeeef-agents-2sn.1.3` was re-closed as a bare dismiss.
- Hotfix PR #14 (merged as `700be24`) makes `main`'s answered-need check
  accept the action label, the `no-decision` label and a dismiss with a note,
  with a test that fails without the fix.
- `yeeef-agents-9va.13.9` was re-dismissed bare, and store commit `2762bb2`
  was reverted; build agents are now told to check every real record with
  `main`'s `bin/pm show` before committing it.
- The rule is written down: new kinds of record or label must stay readable
  by the `pm` on `main`, or merge first (sprint 13 finding). This
  postmortem's own record type follows it: the postmortem lands in the store
  only after the code that reads `type: postmortem` is on `main`.

## What would have caught it earlier

> Which test, check or rule would have caught it before it cost anything?

- A test that runs `main`'s `pm show` and `make render` against data written
  by the branch's `pm` (a compatibility check in the branch's `make test`,
  or in CI on every PR that touches the harness).
- Or a rule followed before the first write: a new label, close reason or
  record type is first made readable by the `pm` on `main`, in its own small
  PR, and only then written to the shared stores.
