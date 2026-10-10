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
