---
type: design
title: Sprint lifecycle
project: pm-harness
covers:
  - "internal/cli/writes.go"
  - "internal/cli/sprint.go"
  - "internal/cli/project.go"
  - "internal/cli/move.go"
  - "internal/cli/review.go"
  - "internal/cli/needs.go"
  - "internal/cli/design.go"
  - "internal/design/**"
---

## Problem

> What are we solving, and why now?

Part of the [Project management harness](pm-harness.md) design. A sprint
passes through Beads, the records store, a code branch, a GitHub PR and the
owner's review before it closes. Each step needs one obvious command and one
place that shows where the sprint stands, or agents invent their own state
and the owner cannot tell which sprints wait on them. This page is the
end-to-end flow; the sprint record's sections are under
[Record layer](record-layer.md).

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

- One `pm` command per lifecycle step that touches both Beads and a record.
- Every state of a sprint is visible from Beads and the sprint page; no
  record holds hand-made status.
- A sprint closes only once its work is on main, with the merge commit in
  its report.
- Owner input never stops the work: needs are raised and work continues.

**Non-goals**

- `pm` reading a PR's merge state from GitHub; the agent carries the merge evidence.
- A model judging whether a design page's text matches the code; the gate
  checks only that the page was edited or waived.
- Enforcing how code is branched or reviewed beyond the review action.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- A sprint closes only once its PR is on main; a PR merged into a stacked
  base does not count.
- `pm` never asks GitHub whether a PR merged: the review action's close
  reason ("merged as <sha>") is the merge evidence. It asks `gh` for two
  things only: a GitHub PR's base branch when a review is raised, and the
  files a PR changes while some design page has `covers:` (Design pages
  follow the code).
- A stacked PR (its base another PR's branch) merges into that branch, not
  main, so a review raised on it could never close as merged; one closed
  anyway before its work reached main held its sprint's close (pm-codex
  sprint 2).
- A rule in prose does not keep design pages current: pm-codex sprint 1
  changed reply delivery, session claims and the owner-request hook without
  editing their pages, although `prime.md` said to.
- Records live in the `records` branch store, shared by every worktree; each
  `pm` write commits there, never on a code branch.
- The scheduled `pm push` publishes Beads data and records every 10 minutes;
  sessions push neither.
- Hooks, rules and `~/.claude` links read the main checkout, so it must
  follow `origin/main` after a merge.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

```mermaid
stateDiagram-v2
  [*] --> Open: pm sprint open
  Open --> Plan: pm task add
  Plan --> Work: pm task claim
  Work --> Report: report written, pm commit
  Report --> Review: pm action need --pr
  Review --> Work: changes requested
  Review --> Close: PR on main, pm action done
  Work --> Close: no PR, or voided
  Close --> [*]: pm sprint close
```

A sprint moves through six stages; review can send it back to work, and a
sprint without a PR goes from work straight to close.

### Open

`pm sprint open --title … <project>` reads Goal, Scope (**In** and **Out**)
and Done when from its body (`--text-file - <<'EOF'`), creates the Beads epic and the sprint record, and
commits the record on the records branch.

### Plan

`pm task add` creates tasks under the epic. Sub-tasks (`bd create --parent`)
and dependencies (`bd dep add`) are plain `bd`. A scope change is a sprint
decision, not a renamed task or rewritten description.

### Work

`pm task claim` records the claiming session and refuses a task another live
session holds. Code goes on a branch off main. Findings go in with
`pm finding add`, decisions with `pm decision add`. Owner input is raised as
a need (`pm decision need` or `pm action need`), and work continues on other
ready tasks meanwhile. `pm task close` stamps the commit in its reason; a task
that will not be done closes with `pm task close --dropped` and its reason.

### Report

Before review, the agent writes the full delivery report by hand in the
store and commits it with `pm commit`: Outcome (done, partial or voided plus
one sentence, optional bullets) and Against "Done when" (each item met or
not, with evidence). The report carries no draft marker; the open review
action is what says the sprint is not closed.

### Review

The agent opens the PR and raises the review with
`pm action need --pr URL --sprint ID --focus …`, which refuses a sprint
whose report is not written, a GitHub PR whose base is not the main branch
(raise the review on the PR that reaches main, or retarget this one with
`gh pr edit <url> --base main`), or a PR the design-page gate holds (below).
A URL that is not a GitHub pull request skips the base check.
The review's text links each design page covering the changed code. The review is an action under the sprint and
blocks its close. The sprint page renders the sprint's own open needs in the
generated "Decisions await you" and "Actions await you" sections; the review
card (PR link, focus, reply box) is what shows the sprint awaits a merge. In
Claude Code a PostToolUse hook runs `pm reply wait` in the background and
wakes the agent on an owner reply or when the PR merges to main; the wake for
a merged review tells the agent to fast-forward the main checkout with
`git -C <main checkout> pull --ff-only origin main`.

### Close

The agent confirms the merge commit is on `origin/main`, closes the review
with `pm action done <review> --reason "merged as <sha>"`, and edits the
report by hand only if review changed the work. Open tasks are closed or
moved (`pm task move`) first. `pm sprint close <id>` then requires every task
closed and each review closed as merged (a dismissed one, a replaced PR's or a [TEST] review, is skipped), appends "Merged as <sha> (PR #N)." to the
Outcome, commits the record on the records branch and closes the epic. It
runs the design-page gate again on each review's PR, since a PR can change
after its review was raised; with `--merged SHA` it runs it on `--pr`'s files,
or without `--pr` on the files a merge commit brought against its first
parent. A commit with one parent (a squash or rebase merge, a direct commit)
does not say where its delivery starts, so there it refuses and asks for
`--pr`. Then it holds the covers of the sprint's pages to the remote's main
(Covers hold on main at the close, below). A sprint with no PR,
or a voided one, closes without a review or a stamp.

A review closed without "merged as <sha>" (its PR merged into a stacked
base, which later reached main inside another merge) is restamped with the
same command, `pm action done <review> --reason "merged as <sha>"`: the sha
must be on the remote's main, checked as `--merged` checks it, and the old
reason stays after the new one ("…; first closed as: <old>"). A review
already closed as merged, a dismissed one and any other closed action are
refused.

### Design pages follow the code

A design page names the code its area is about in its header, `covers:`
(see [Record layer](record-layer.md)). The gate takes the files the PR
changes from GitHub's pull request files API (`gh api --paginate`, every
page, a renamed file's old path as well as its new one; a PR URL that is not
GitHub's is refused, since its files cannot be read) and, for each design page whose covers match one,
passes the page when one of the sprints the PR delivers either:

| Way out | What it takes | What the gate reads |
|---|---|---|
| Updated | The page brought to the new state, committed with `pm commit`, and linked in the sprint record's Design pages | A commit to the page on the records branch since the sprint's epic was created, and a `../design/<slug>.md` link in Design pages |
| Waived | `pm design unchanged <slug> --sprint ID --reason "…"` | A sprint decision of source agent whose first line is "The design page <slug> stays as it is for this sprint's code changes." and whose second is the reason |

Otherwise it refuses, naming each page, a file it covers and both ways out.
With no page carrying `covers:` there is nothing to gate and `gh` is not
asked. The waiver is a decision in the sprint record, not a work-store field:
decisions live in records, and the owner reads it on the sprint page next to
the PR.

### Covers hold on main at the close

A sprint updates its page before its PR merges, and the records store is
shared across branches, so a page's `covers:` may name a file that only the
sprint's branch has. `pm check` lists such a pattern as waiting on unmerged
work and passes ([Record layer](record-layer.md)). The close is where main
must have the file: `pm sprint close`, in both forms, fetches the remote's
main (the ref `--merged` checks) and reads its tree, then refuses while a
page holds a covers pattern of the sprint's that matches no file there.

| Pages held | Why |
|---|---|
| Each design page the sprint record's Design pages links | The sprint brought it to the new state |
| Each design page the gate names for the sprint's PRs, updated or waived | The PR changed code it covers |

Pages are shared, so a held page may carry another sprint's pending
pattern, for a file that sprint's open PR adds. Only the sprint's own
patterns hold it: those the page did not hold at the last records commit
before the sprint's epic was created.

| Case | Held against the sprint |
|---|---|
| A pattern on the page before the sprint opened, left as it was | No: another sprint's to keep live |
| A pattern added while the sprint was open | Yes |
| A pattern edited while the sprint was open | Yes: the edited text is a new pattern |
| A page created while the sprint was open, or no records commit before it opened | Every pattern |
| A pattern another session added to the page while the sprint was open | Yes: a records commit names no sprint |

The last row is the rule's known cost: two sprints open at once that both
edit one page's covers hold each other's pending patterns, as before. The
refusal says so: it names each page and pattern and the two ways out, fix
the pattern to the file main has and commit the page, or merge the PR that
adds the file first, which may be another sprint's. A page without covers is
not held, and with no pattern of the sprint's nothing is fetched.

## Alternatives considered

> What else was considered and not adopted, and why not?

- **`pm check` failing on a covers pattern that matches no tracked file:**
  every session's check failed while other sessions' branches were in flight
  (27 patterns on main at once), since the gate makes a
  page name a file before main has it. The close is the first point where
  main must have the file.
- **Holding every pattern of a held page at the close:** a sprint waited on
  any other session's open PR whose pattern sat on a shared page (the CLI
  split's patterns on the pm CLI page held every sprint that touched the
  CLI), although the sprint did nothing to the pattern.
- **Attributing each records commit to a session or a sprint:** records
  commits carry neither, a subagent shares its parent's session (one session
  runs several sprints at once), and the owner commits with none; adding a
  required sprint to every `pm commit` costs every edit for a case the time
  window already covers but for concurrent edits to one page.

- **A draft paragraph in the report** (`Draft until its PR merges.`, shown
  as a "Draft — closes when PR #N merges" banner): it duplicated the open
  review action's state and cost two hand steps per sprint (delete the line,
  commit), and the forced post-merge edit never made anyone re-check
  evidence.
- **A `Draft:` prefix before the verdict:** older `pm` rejects an Outcome
  that does not start with done, partial or voided.
- **`pm` checking GitHub at close:** adds a `gh` dependency to every close;
  the review's close reason already carries the evidence. The design-page
  gate asks `gh` for a PR's files only, and only where pages have `covers:`;
  raising a review asks it for the PR's base.
- **Reopening a review closed too early, then closing it again:** the work
  store has no reopen, and a reopened review would block the sprint again
  for no new evidence; a restamp changes only the reason.
- **Warning on a stacked PR instead of refusing:** the review would still
  close before its work reached main, the case the restamp repairs.
- **The design-page rule in prose only** (`prime.md`: edit the page to the
  new state): broken in pm-codex sprint 1; the gate makes it structural.
- **The waiver in the work store:** a decision with its reason is what the
  record layer holds; a work-store field would be a second home for it.
- **`pm` pulling the main checkout itself:** the agent runs a `--ff-only`
  pull instead, so a diverged checkout refuses visibly.

## Prior art

> Optional. What existing work did we learn from, and what did we take?

None yet.

## Open questions

> What is still unresolved?

None yet.
