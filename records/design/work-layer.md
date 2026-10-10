---
type: design
title: "Work layer: Beads"
project: pm-harness
---

## Problem

> What are we solving, and why now?

Part of the [Project management harness](pm-harness.md) design. Two of its four problems are about tracking work:

- **Work tracking is free-form.** Each agent writes status its own way; the
  format drifts until neither people nor tools can rely on it.
- **Dependencies live in prose.** Every session rereads the plan to work out
  what is blocked and what is ready, spending tokens and possibly getting a
  different answer each time.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

- Work has fixed fields and statuses: what exists, who owns it, its status,
  and what blocks what.
- Agents declare dependencies once; `bd ready` computes what can start.
- Each of our concepts (project, sprint, task, need) maps to one Beads object
  by a fixed convention.

**Non-goals**

- Our own task model: Beads is used as is.
- Status in records: records point at Beads ids and never copy status.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- Beads stores issues in Dolt; `.beads/issues.jsonl` is an export only, and
  `bd init` edits agent config and git hooks unless told not to.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

Beads, used as is. It owns what exists, who owns it, its status, and what blocks what. Records never store any of these; they point at Beads ids.

[pm's boundary with Beads](work-layer-bd-boundary.md) maps what pm uses of bd, where bd constrains pm, and the options of staying layered, forking bd or replacing it.

### How our concepts map to Beads

| Our concept | Beads object | Example id | Convention |
|---|---|---|---|
| Project | Epic | `bd-a3f8` | One epic per project record |
| Sprint | Child epic of the project | `bd-a3f8.9` | One child epic per sprint record |
| Work trunk | Task under the sprint | `bd-a3f8.9.1` | Closed with `pm task close`, which names the commit; closing gates are the agent-sdlc (a yeeef-agents record) project's work |
| Task | Task or sub-task | `bd-a3f8.9.1.2` | A sprint's task is created, closed and moved with `pm task add`, `pm task close` and `pm task move`; a sub-task is created with `bd create --parent <task>` and closed with `pm task close` |
| Need | Issue labelled `human` (what `bd human list` shows); an action is also labelled `action` | `bd-a3f8.12` | A decision has options and the cost of each in `description`, and the owner answers with `bd human respond`, which comments and closes; an action says what to do, and the agent closes it once it sees it done |
| Dependency | Dependency link |  | Declared when work is created; `bd ready` computes what can start |

Ids are placeholders. Real ids take the repo name as prefix (`yeeef-agents-9va`, `yeeef-agents-9va.1.5`).
### Who decides what

| Fact | Source of truth |
|---|---|
| Open, in progress or closed | Beads |
| Owner and assignee | Beads |
| Blocked or ready | Beads dependency graph |
| Goal, sprint frame, outcome, decisions | Record layer |
| What was learned | Sprint record findings, plus the Beads audit trail |
| Small operational facts (commands, paths, gotchas) | `bd remember`; `bd prime` loads all of it every session, so keep it short. Decisions go in records, not here |

### Which tool agents use

| Action | Tool | Why |
|---|---|---|
| Find work, claim it, link dependencies | `bd` directly | Beads already does this well. Use `bd ready --exclude-type=epic`: plain `bd ready` lists epics too |
| Create, close or move a task | `pm task add`, `pm task close`, `pm task move` | Keeps every task in an open sprint, names the commit on close, and records a move as a sprint decision |
| Open or close a sprint; add a finding, decision, doc or design page | `pm` | Changes Beads and the record together and enforces the gates |
| Raise, answer or close a need | `pm decision need`, `pm decision add --need`, `pm decision close`; `pm action need` (with `--pr` for a PR review), `pm action done` | Creates the owner task in the agreed shape; an answer that sets a rule becomes a decision, a small one is marked `no-decision`; an action closes with what showed it done |
| Read state at session start | `pm show` | Combines Beads status with record context in one summary |
| Link a record for the owner | `pm record link` | Prints the rendered page's URL only when `pm serve` for this store answers, so a link is never raw Markdown or dead |

## Alternatives considered

> What else was considered and not adopted, and why not?

None yet.

## Prior art

> Optional. What existing work did we learn from, and what did we take?

### Beads, the chosen work layer
[Beads](https://github.com/steveyegge/beads) (`bd`, by Steve Yegge) is an issue tracker built for coding agents, and the chosen work layer. Facts from its README and source, read on 29–30 Sep 2026.

| Aspect | Beads |
|---|---|
| Model | Issues with hash ids prefixed by the repo name (`yeeef-agents-9va`; README examples show `bd-a1b2`), type (task, epic, …), status, priority, assignee, dependencies. Text fields: `description`, `design`, `acceptance_criteria`, `notes`. Epics nest as `bd-a3f8.1` |
| Storage | Dolt, a SQL database with git-like versioning; `.beads/issues.jsonl` is an export only |
| Commands | `ready` (claimable work), `update --claim`, `show`, `close`, `prime` (session-start context), `remember` (project memory); all can print JSON |
| Agent setup | `bd init` edits `AGENTS.md` and installs Claude and Codex hooks unless `--skip-agents`. In practice (sprint 1) it also commits on its own, takes over git hooks, writes Codex and Cursor files, and edits `CLAUDE.md` |
| Human view | None built in |

| Our concept | In Beads | Gap our record layer fills |
|---|---|---|
| Project | Epic | Goal, decisions |
| Sprint | Child epic | Goal, scope, done when, outcome, delivery report |
| Trunk, task | Task, sub-task | Closing gates (tests, cleanup, review; designed in the agent-sdlc (a yeeef-agents record) project) |
| Need | Issue labelled `human` (`bd human list`), plus `action` for an action | Shown as two lists, decisions with options and cost, actions with what to do |
| Day | None | Entirely ours |

**Costs we accept:** a Go binary and a Dolt database, schema migrations across clones, and its setup editing agent config.

## Open questions

> What is still unresolved?

None yet.
