---
type: doc
title: pm feedback
date: 2026-10-10
project: pm-quality
---

Where pm got in the way, one entry per `pm feedback add`, newest last.

### 2026-10-10 04:31 UTC, session `d2440053-39b8-519d-8887-8ecd90d4282a`

There is no `pm sprint move`. Moving pm-harness sprints 34, 56, 105 and 110 to pm-quality and pm-codex took, per sprint, a new `pm sprint open` with the frame copied by hand, one `pm task move` per task, a hand-written Delivery report, `pm commit` and `pm sprint close`.
The move loses the sprint number: pm-harness sprint 105 is now pm-quality sprint 2, and links or chat that name the old number go stale. Findings and decisions do not follow; I copied one finding by hand.
What would have helped: `pm sprint move <id> --to <project>` that reparents the epic with its tasks, needs, findings and decisions, renames the record, and records the move as a decision in both projects.
`pm commit` also refused a path given relative to the store (sprints/x.md); it wants records/sprints/x.md. Accepting either would have saved a retry.

### 2026-10-10 04:39 UTC, session `44850292-f394-5fbf-8a0e-4d0c75e1d559`

About sprint `pm-bfli.1`.

Moving pm-harness sprint 92 to the new pm-site project took six steps: `pm sprint open` with the frame copied out of the old record by hand, `pm task move` per task, `pm decision add` to carry each decision, a hand-written voided delivery report, `pm commit`, `pm sprint close`. The first `pm sprint open` timed out when the service restarted mid-move, leaving the move half done until checked by hand.
What would have helped: one `pm sprint move <sprint> --to <project>` that moves the frame, tasks, decisions and findings and leaves the old id pointing at the new one.

### 2026-10-10 13:25 UTC, session `ffa2f713-c138-5cf7-8379-c247d20c2e2a`

Moving a repo's pin (0.3.0 to 0.4.0, PR #14) while ten agent worktrees were mid-sprint: once the main checkout pulls the new pin and the service restarts, every worktree still on the old pin gets each pm command refused by the version handshake, until it rebases onto main. With uncommitted work, that means a WIP commit or a stash first, in every worktree.
What would have helped: `pm upgrade`'s output, or the release's upgrade guide, naming this (rebase each open worktree after the pin move), and `pm where` in a worktree on another pin saying so before the restart rather than after.

### 2026-10-10 13:31 UTC, session `ffa2f713-c138-5cf7-8379-c247d20c2e2a`

About sprint `pm-d2k5.1`.

pm finding add --sprint pm-d2k5.1 --text="…" wrote the finding with a leading "--text=" as part of its text: finding add takes the text as a positional argument, and an unknown --text=… was taken as that positional instead of being refused. Every other body command takes --text, so the brief's form looked right. What would have helped: finding add accepting --text/--text-file like the other body commands, or refusing an argument that starts with "--".
