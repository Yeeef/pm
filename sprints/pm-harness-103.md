---
type: sprint
title: Triage the five unsprinted Python-pm tasks against Go pm
bead: yeeef-agents-9va.113
---

## Goal

> What should be true when this sprint ends, and why now?

Each of the five pm-harness tasks outside any sprint (.26, .54, .58, .59, .104) is either closed as obsolete, with evidence that Go pm has no such code path, or fixed in Yeeef/pm. They were filed against Python pm (bd, `pm serve`, `harness/beads.py`, `tests/test_pm.py`); since this repo runs Go pm 0.2.0, an open task that no longer applies misleads anyone reading `pm task ready`.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** checking each task's defect against Go pm's source in Yeeef/pm at its current main; closing obsolete ones; fixing a defect that Go pm still has, with a test, in a Yeeef/pm PR.

**Out:** fixing Python pm (this repo's `pm/` copy is deleted after the soak); other pm-harness sprints.

## Done when

> What evidence will show the goal is met?

- Each of the five tasks is closed, and its reason names the Go pm file or search that shows it obsolete, or the commit that fixes it.
- Any fix has a test that fails before it and passes after, and its PR is on Yeeef/pm's main.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- All five Python-pm tasks (.26 .54 .58 .59 .104) are obsolete on Go pm
  (Yeeef/pm a32ce1b); none needed a code change. Go pm show reads the whole
  work store in one SQL query: pm show --project pm-harness 0.11-0.18 s (3
  runs) against 0.48 s per bd level before.

- Seen in passing, not filed: Go pm's records walk Stamp()
  (internal/cli/serve.go:186-201) errors when a .md file disappears between
  WalkDir and ReadFile, serving an error page for one 1 s look; the same
  skip-if-not-exist guard as work.Fingerprint would fix it. Python pm in
  Yeeef/pm (src/pm/beads.py:87) still has the .59 stat race, which matters
  only for a Python pin.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: all five Python-pm tasks are closed as obsolete on Go pm, and none needed a code change.

- Checked against Yeeef/pm at a32ce1b.
- `.26`, `.54`, `.58`, `.59` and `.104` are closed. Each close reason names the Go file or search that shows the defect gone.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Each of the five tasks is closed with evidence: met.**

  | Task | Defect | Evidence on Go pm |
  |---|---|---|
  | `.26` | one bd call (0.48 s) per tree level | no bd calls; `internal/work/dolt.go` reads all items in one SQL query; `pm show --project pm-harness` 0.11-0.18 s (3 runs) |
  | `.54` | crash test leaks a `pm serve` child | test gone (grep: no match); the Go test runs the server in process; shared tests kill the server pid itself; 30 runs, no orphan |
  | `.58` | card note reads delivery state before the push | `writeReply` (`internal/service/run.go:701-723`) sets the note from the push result |
  | `.59` | noms file vanishes between scan and stat | `work.Fingerprint` skips it (`internal/work/service.go:45`) |
  | `.104` | test reads fake bd state mid-write | Go pm runs no bd; `tests/fake_bd.py:52` writes by `os.replace` |

- **Any fix has a test and its PR on Yeeef/pm main: not applicable.** No task needed a fix.
