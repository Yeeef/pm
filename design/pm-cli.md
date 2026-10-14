---
type: design
title: pm CLI
project: pm-harness
covers:
  - "internal/cli/cli.go"
  - "internal/cli/commands*.go"
  - "internal/cli/help_golden_test.go"
  - "internal/cli/testdata/help.golden"
  - "internal/cli/doc_test.go"
  - "internal/cli/parse/**"
  - "internal/cli/fsutil/**"
  - "internal/textwrap/**"
  - "internal/cli/agent.go"
  - "internal/cli/agentcmds.go"
  - "internal/cli/writes*.go"
  - "internal/cli/finding.go"
  - "internal/cli/feedback.go"
  - "internal/cli/document.go"
  - "internal/cli/project.go"
  - "internal/cli/sprint.go"
  - "internal/cli/task.go"
  - "internal/cli/move.go"
  - "internal/cli/commit.go"
  - "internal/cli/show*.go"
  - "internal/cli/where.go"
  - "internal/cli/check.go"
---

## Problem

> What are we solving, and why now?

Part of the [Project management harness](pm-harness.md) design. Records that agents edit freely drift from their format, and an
action that touches both Beads and a record (opening a sprint, answering a
need) takes several steps an agent can get wrong. Agents also need project
state without reading every record.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

- Agents read project state in a few hundred tokens (`pm show`) and write
  through commands that check every write.
- One command for each action that changes both Beads and a record, or that
  needs a check Beads cannot do.

**Non-goals**

- Wrapping all of `bd`: `pm` wraps only actions that touch both layers or need
  a check Beads cannot do.
- Editing or deleting an existing entry; that is done by hand.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- A repo holds many projects, each with several sprints that can be open at
  once, so nothing can be inferred from "the only project" or "the open sprint".
- Agents run under the owner's git identity, so git and Beads cannot tell an
  agent's change from the owner's.
- Some tasks commit nothing, so a task close cannot require a commit.
- An agent's shell may give a command stdin as a socket that never closes, so
  a read of stdin can hang forever. A body passed as
  `--text="$(cat <<'EOF' … EOF)"` breaks on macOS bash 3.2 when it holds an
  apostrophe or a lone `)`, and a backtick inside double quotes executes.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

`pm` is the write path for records and for project actions that touch both Beads and a record. Finding, claiming and linking tasks stay plain `bd`. Every `pm` write validates the whole record set before and after, and writes nothing if the result would not render.

A command that takes a body ("in the body" below) takes it with `--text` for one plain line, or with
`--text-file PATH`; the two are exclusive. `--text-file -` reads stdin, and only when it is a pipe or a file,
so agents give multi-line text as `--text-file - <<'EOF'` … `EOF`. Any other stdin (a tty, a socket, a
device) is refused before reading. No other command reads stdin.

### Commands

| Command | Arguments | Records | Beads | Refuses when |
|---|---|---|---|---|
| `pm show` | `[--json]` | Reads; prints the top level: push-failure and held-task warnings, one fixed-size `site:` line naming the site and how a record maps to its page, the open questions no task holds, then one line per open project with one line per owner request (shown below); `--json` prints every level's data as one object, the day summary too, with `site` and a `url` per project and open sprint | Reads `bd list --all --json` | Records do not validate (prints the errors) |
| `pm show --project` | `NAME` (a project name or its Beads id) | Reads; prints that project's level: goal, decision, action and question lists, feedback, open sprints with their tasks, sprints with no tasks, last decisions | Reads `bd list --all --json` | No open project matches (lists the open ones); `--json` or `--sprint` given with it |
| `pm show --sprint` | `ID` | Reads; prints one sprint's frame, findings and tasks | Reads `bd list --all --json` | `--json` or `--project` given with it |
| `pm show --record --section` | `--record <target>` (a record path, sprint or project id, project name or design slug), `--section <name>` (a heading's text, any level), both required together | Reads one record; prints the section from its heading through the next heading of the same or a higher level, prompt lines included. Headings are found as the renderer parses them, so a `#` line in a code fence or raw HTML block is not one | None | No record or several match; no heading or several headings have that name (prints the record's section names, indented by level); `--sprint`, `--project` or `--json` given with it |
| `pm record link` | `<target>`: sprint or project id, project name, design slug or record path (`records/` and `.md` optional) | Reads; prints the page's URL on the served site, with no render step since `pm serve` renders on every load | None | No record or several match; nothing answers on `127.0.0.1:$PORT` (names `make docs`); the server does not send `X-PM-Store` naming this store (names: stop it, then `make docs`) |
| `pm day summarize` | `[--dry-run]` | Builds a digest of today's activity; when it changed, asks the runtime `summary_runtime` names (`claude -p` on Haiku by default, or `codex exec` on `gpt-5.6-luna`) for an ASD-STE100 summary and commits `records/days/<date>.summary.json`; `--dry-run` prints it and writes nothing. Run by the scheduled `pm push` as its `summary` step, every 10 minutes; it first regenerates yesterday's summary when yesterday's digest changed | `claude -p` or `codex exec` | the runtime's CLI missing, failing or returning nothing (writes nothing; no fallback to the other) |
| `pm finding add` | `"<text>"` `--sprint ID` (required) | Appends a bullet to that sprint's Findings, replacing "None yet." | None | `--sprint` missing, unknown or closed |
| `pm decision add` | `--level project\|sprint` (required); `--project NAME` for project level or `--sprint ID` for sprint level (required); `[--until "…"]`; `--decision "…"` and `--reason "…"` (required, one line each; stdin is not read); `[--need ID \| --confirmed]` | Appends a `::: decision` block to that project's or sprint's Decisions, `date` = today, replacing "None yet.": the decision line, then the reason line. `source=agent`, or `source=owner` with `--need` (the block then ends "Answers `<need-id>`.") or `--confirmed`; there is no `--source` flag | With `--need` on an open need: `bd human respond <id>` with the same text, which comments and closes; it runs first, and a failed record write prints `bd reopen <id>` to undo it. A need the owner already closed is only recorded | No `--level`; the target for that level is missing, unknown, closed or given with the other level's flag; `--decision` or `--reason` missing (argparse), empty, more than one line, or starting with `:::`; `--need` is not an issue labelled `human`, is an action or a question (naming `pm need dismiss`), or was dismissed |
| `pm decision need` | `--title "…"` `--parent ID` (a sprint or task, both required); `--question`, repeatable `--fact`, repeatable `--option LABEL TEXT` with one `--cost LABEL TEXT` each, `--default LABEL REASON` (all required, one line each; stdin is not read). [Decision need layout](decision-need-layout.md) has the rules | None | `bd create --type=task --parent=ID --labels=human` with the description, and `metadata.session` naming the raising session when `$CLAUDE_CODE_SESSION_ID` is set (also for `pm action need`) | `--parent` missing, unknown, closed or outside every project with a record; a part missing, empty or malformed, as the [Decision need layout](decision-need-layout.md) refusals list |
| `pm decision close` | `<need-id>` `--reason "<why it sets no rule>"`, the answer in the body | None | Open need: `bd human respond <id>` with the answer and "No decision record: <why>", then `bd update <id> --add-label=no-decision`. Need the owner already closed: `bd comments add` with the reason, then the label | The reason empty; an open need with no answer; an action, a question (naming `pm need dismiss`), a dismissed need or one already marked |
| `pm action need` | `--title "…"` `--parent ID` (both required without `--pr`), what to do and why in the body; `--sprint`, `--focus` and `--design` refused without `--pr` | None | `bd create --type=task --parent=ID --labels=human,action` with the description | As `pm decision need`; description empty, refused with the expected shape and an example |
| `pm action need --pr URL` | `--sprint ID` (repeatable) and `--focus "…"` required, `--parent` not allowed; `--design SLUG` (repeatable), `--title` (default `Review PR #<n>`); extra context in the body | None | `bd create --type=task --parent=<first sprint> --labels=human,action --external-ref=<pr>` with `metadata.review` holding the PR, sprints, focus and named design pages; the review blocks that sprint's close. The card adds the sprints' delivery reports and the design pages the sprint records list | `--pr` not a URL; focus empty; a sprint without a record, closed, or whose committed report is not written (Outcome or Against "Done when" still "Not closed yet."); a GitHub PR (`https://github.com/OWNER/REPO/pull/N`) whose base branch, as `gh pr view` gives it, is not the main branch (a stacked PR), or whose base `gh` cannot give, naming the fix: raise the review on the PR that reaches main, or `gh pr edit <url> --base main`; a design slug with no page; a design page whose `covers:` match a file the PR changes (GitHub's pull request files API; a rename's old path too), neither updated and listed by one of the sprints nor waived ([Sprint lifecycle](sprint-lifecycle.md), Design pages follow the code) |
| `pm need edit` | `<need-id>`; an action's description in the body, or a decision need's parts as `pm decision need` takes them | None | The need's description, which the store refuses to change, in the write's transaction, once the need holds a reply or is closed | The need unknown, not a need, closed or holding a reply; a PR review; a question, whose card pm builds from the ask tool's call (naming `pm need dismiss`); a decision need given a body, or an action given parts; the parts malformed as `pm decision need` refuses them; a body that asks for a PR review; the body unchanged |
| `pm action done` | `<id>` `--reason "…"` (required) | None | `bd close <id> --reason=…`; on a PR review closed without `merged as <sha>`, a reason `merged as <sha>` with the sha on the remote's main (fetched first) restamps it: the new reason, then `; first closed as: <old>` | The issue is unknown, not labelled `human` or not an action (a question names `pm need dismiss`); closed, unless it is that restamp; a restamp whose reason lacks `merged as <sha>` or whose sha is not on the remote's main; the reason empty |
| `pm reply wait` | `<ID>` (a decision need, action or review), `--timeout DURATION` (default 30m) | None | Reads the need every 2 s until it holds a reply or merge that reached no session (printed with the next step and marked delivered, as `pm reply read` does) or replies stamped since the wait began (printed as they are) | The id is not a need; the need closes with no new reply; at the timeout, saying the need is still open. For a session the service cannot push into (Codex); see [Reply delivery](reply-delivery.md) |
| `pm doc new` | `<slug>` `--title "…"`, exactly one of `--bead ID` or `--project NAME`, body in the body | Creates `records/docs/<today>-<slug>.md` with header `type: doc`, `title`, `date` and `bead` or `project`; the sprint, day, project and root pages list it by query, no edit | None | The slug is not lowercase words joined by `-`; the file exists; neither or both of `--bead` and `--project`; the bead is unknown or outside every project with a record, or the project is unknown; title or body empty |
| `pm design new` | `<slug>` `--title "…"` `--project NAME` | Creates `records/design/<slug>.md` with header `type: design`, `title`, `project` and every template section, each with its prompt line and "None yet."; the page is then written by hand | None | The slug is not lowercase words joined by `-`; the file exists; the project is unknown; title empty |
| `pm design unchanged` | `<slug>` `--sprint ID` `--reason "…"` (all required) | Appends a sprint decision (source agent): "The design page <slug> stays as it is for this sprint's code changes." then the reason, the form the design-page gate reads | None | The sprint is closed or has no record; no design page has the slug; the reason empty or not one line; the sprint already waived the page |
| `pm postmortem new` | `<slug>` `--title "…"`, exactly one of `--sprint ID` or `--project NAME` | Creates `records/postmortems/<today>-<slug>.md` with header `type: postmortem`, `title`, `date` and `sprint` or `project`, and every section (Summary, Timeline, Cost, Root cause, What changed, What would have caught it earlier), each with its prompt line and "None yet."; the text is then written by hand; the sprint, project and root pages list it by query | None | The slug is not lowercase words joined by `-`; the file exists; neither or both of `--sprint` and `--project`; the sprint has no record (open or closed both work), or the project is unknown; title empty |
| `pm project open` | `<name>` `--title "…"`, Goal in the body | Creates `records/projects/<name>.md` with every section, the rest as placeholders | `bd create --type epic` | The name exists; Goal is empty |
| `pm sprint open` | `<project>` `--title "…"`, frame in the body (Goal, Scope with **In**/**Out**, Done when) | Creates `records/sprints/<project>-<n>.md` with every section, the rest as placeholders | `bd create --type epic --parent <project epic>` | Goal, Scope (In and Out) or Done when missing or empty; the project epic is closed |
| `pm sprint close` | `<sprint-id>` `[--merged SHA [--pr URL]]` | Appends "Merged as <sha> (PR #N)." to the Outcome after its verdict paragraph and commits the record on the records branch as `[SPRINT] …`: the sha from each review's close reason, or with `--merged`, for a PR merged with no review, the commit given (and "Merged as <sha>." without `--pr`); a sprint with neither gets no stamp | `bd close <epic> --reason="<outcome> (records commit <records HEAD>)"` | Uncommitted changes in the store; the committed Outcome or Against "Done when" still "Not closed yet.", or Outcome not done/partial/voided; a child task open, a review included; a review not closed as `merged as <sha>` (a dismissed review, such as a replaced PR's or a [TEST] one, is skipped); with `--merged`: a sha that is not 7 to 40 hex digits or not on the remote's main after `git fetch`, `--pr` without `--merged`, and a sprint that holds a review that is not dismissed; the design-page gate on each review's PR, or on `--merged`'s (`--pr`'s files, else a merge commit's against its first parent; a one-parent commit without `--pr` is refused); in both forms, a page the sprint's Design pages list or the gate names whose `covers:` has a pattern the sprint added or changed (not on the page at the last records commit before the sprint opened) that matches no file on the remote's main after `git fetch` |
| `pm sprint edit` | `<sprint-id>` `--title "…"` (without `Sprint <n>: `), reason in the body (two lines at least) | Sets the record's `title:` header and appends a `::: decision {source=agent …}` naming the old and new title and the reason, in one records commit | The work store first: the title, its `Sprint <n>: ` prefix kept (the merge keeps it at the sprint's number); a failed records step puts the old title back | The sprint is unknown, not a sprint, closed or has no record; the title empty, unchanged or carrying its own prefix; reason empty or one line |
| `pm sprint move` | `<sprint-id>` `--to PROJECT`, reason in the body (two lines at least) | Renames the record to `sprints/<project>-<m>.md` and appends a `::: decision {source=agent …}` naming the sprint, both numbers and the reason to both projects' records, in one records commit | The work store first: the sprint's parent, its next number in the new project and a move note, in one write through the compare-and-swap ([work store](work-store.md), Moving a sprint); a rerun after a failed records step writes the records step alone | The sprint is unknown, not a sprint, closed or has no record; the project is unknown or closed, or already holds the sprint under its name; reason empty or one line |
| `pm project close` | `<name>` | Reads only | `bd close <project epic> --reason="… (commit <records HEAD>)"` | Uncommitted changes in the store; the committed `## Outcome` still "Not closed yet."; a sprint of the project still open |
| `pm task add` | `--sprint ID` `--title "…"`, description in the body (optional) | None | `bd create --type=task --parent=ID` | `--sprint` missing, unknown, closed, not an epic, or outside every project with a record; title empty |
| `pm task close` | `<id>` `[--reason "…"]` `[--commit REF]` | None | `bd close <id> --reason="<reason> (commit <hash>)"`: HEAD if committed after the task started, or `--commit`; with neither, no hash and a warning. A dirty working tree warns but proceeds | Unknown, closed, an epic (use `pm sprint close`), or labelled `human` (use `pm decision add --need`, `pm decision close` or `pm action done`); `--commit` is not a commit |
| `pm task move` | `<id>` `--to SPRINT_ID`, reason in the body (two lines at least) | Appends a `::: decision {source=agent …}` "Moved <id> to <target>: <reason>" to the sprint the task leaves | `bd update <id> --parent=<target>` first; a failed record write prints `bd update <id> --parent=<source>` to undo it | The task is unknown, closed, an epic, or not directly in a sprint with a record (the source sprint may be closed); the target is the same sprint, unknown or closed; reason empty or one line |
| `pm render` | None | None | Reads | Same as `make render` |
| `pm serve` | `PORT` in the environment (default 8000) | None; every reply carries `X-PM-Store: <store path>` so `pm record link` can tell it from another server | Reads | Never: a failed render is shown on the page as the error |
| `pm setup` | None | Checks out the `records` branch at `<main checkout>/.records` if missing (from `origin/records` on a fresh clone); links this worktree's `records/` to it; turns off an earlier pm's sparse checkout (`/*` `!/records/`) once the worktree's `HEAD` and index track nothing under `records/` | First, each only if missing: `bd bootstrap --yes` (clones the database from the remote's `refs/dolt/data`), `beads.role=maintainer`, `bd hooks install --beads` | `bd` fails; `bd bootstrap` would do anything but clone the remote (import a JSONL, mint a database); no `records` branch locally or on origin; `records/` exists and is not the link; run from the store itself |
| `pm where` | `[records]` | Bare: lists the store (branch, ahead/behind `origin/records`), this checkout (branch, link), Beads, the hooks (post-checkout) and the site URL, each with its state. With `records`: prints only the store's path | Bare: reads `bd context --json` | With `records`: no store |
| `pm commit` | `-m "…"`, the paths of the records edited: `records/<…>`, a path in the store, or one relative to the store from anywhere (`sprints/<…>.md`) | Commits only the named records on the `records` branch; other sessions' uncommitted edits stay out | Reads | No path (it lists what is uncommitted); a path outside the store or without a change; nothing to commit; the record set does not render |

Not yet: work-trunk gates on `pm task close` (the agent-sdlc (a yeeef-agents record) project), and editing or deleting an existing entry.

**Closing a sprint or project points to a commit.** `pm sprint close` and
`pm project close` refuse while records have uncommitted changes, check the
committed delivery report or Outcome, and close the Beads epic with the
commit hash in its reason. The hash lives in Beads, not on the owner's
pages. `pm task close` names the commit for an ordinary task the same way:
HEAD when it was committed after the task started, or `--commit`. Not
chosen: refusing a close without a commit, since some tasks commit nothing.

**Every write names its target.** A repo holds many projects, each with
several sprints that can be open at once, so no command infers its target
from "the only project" or "the open sprint". Sprint-level commands take
`--sprint`, and the project comes from the sprint's parent epic in Beads, so
the two cannot disagree; project-level commands take `--project`. A missing
or ambiguous target is refused, never guessed.

**Choosing a decision's level.** `--level` has no default, so the agent must decide. The rule printed in `--help`, in the refusal for a missing `--level`, and in the rules file: *project if a later sprint must follow it; sprint if it is about this sprint's own work; skip choices cheap to reverse.* Not chosen: inferring the level from keywords, which would be wrong silently.

**Closing a sprint** writes only the merge stamp: the agent writes the delivery report first and closes or moves open tasks with `pm task close` and `pm task move`. Not chosen: an interactive close that writes the report, which hides the most important text behind prompts.

**A sprint closes on merge.** Done means delivered to main (owner decision answering `yeeef-agents-9va.22.5`). Before review the agent writes and commits the full delivery report; `pm action need --pr` refuses a sprint that is closed or whose report is not written, and puts the review under the first sprint, so the open review blocks the close and its card on the sprint page shows the sprint awaits a merge. Once the PR is on main the agent closes the review with `pm action done`, its reason `merged as <sha>`, and runs `pm sprint close`, which refuses an open task or a review not closed as merged, stamps "Merged as <sha> (PR #N)." into the Outcome, commits the record and closes the epic. It does not ask GitHub (owner decision answering `yeeef-agents-9va.23.2`): the review's close reason is the merge evidence, and closing it before the PR is on main is the agent's mistake, which a check would not prevent; a PR merged into a stacked base branch is not yet on main. The stages from open to close: [Sprint lifecycle](sprint-lifecycle.md). Not chosen: a draft paragraph in the report until merge (see [Sprint lifecycle](sprint-lifecycle.md)); `pm sprint close` checking each reviewed PR with `gh pr view` (built in sprint 19 and removed: a `gh` dependency, abandoned-PR and dismissed-review cases and a fake `gh` in tests, and GitHub reports a stacked PR merged before it reaches main); a separate `pr:` header field, which would duplicate the review's metadata; and reviews under the project epic (tried on `cli-disclosure`), which let a sprint close before its PR merged.

### Relationship with `bd`

| Stays plain `bd` | Wrapped by `pm` |
|---|---|
| `bd ready --exclude-type=epic`, `bd show`, `bd update --claim`, `bd dep add`, `bd create --parent <task>` for a sub-task, `bd remember` | Opening and closing a sprint (epic plus record), raising a need (shape check), answering a need (decision plus close), creating a task (open-sprint check), closing a task (commit in the reason), moving a task (decision plus reparent), `pm show` (status joined with records) |

The rule: `pm` wraps an operation only when it changes both layers or enforces a check Beads cannot. `pm` calls `bd` as a subprocess with `--json`; it never writes the Dolt database. Not chosen: wrapping all of `bd`, which doubles the surface and hides Beads features agents already know.

### How records are edited
`pm` only creates files and inserts into a named section: a bullet at the end of Findings, a block at the end of Decisions, replacing the section's "None yet." placeholder on first insert. The insertion point is the section's line range from the parser; the file is otherwise byte-for-byte unchanged. Goal, Scope, Done when, the delivery report and design pages are edited by hand in the store (agents with their editor), then checked and committed by `pm commit -m "…" <path>`. Not chosen: editing or removing blocks by line range, which needs stable block ids and is rare enough to do by hand for now.

### Code structure

- `cli` owns the command tree, session/config checks, dispatch and work/record context. Bodies live in files named for their noun: project, sprint, task, finding, feedback, decision, request, review and reply; document creation, moves and commits have their own files.
- `cli/parse` builds Cobra routes and uses pflag to parse options, in pm's own grammar: switches (one may set a named value, as `pm prime --state` does), single, repeated and two-value options (`--option LABEL TEXT`), required, optional and trailing positionals, required options, alternative-option groups (`--text` or `--text-file`) and choices. An option is read only by its whole name; a prefix of one is an unrecognized argument. Explicit stages join each two-value option into one pflag value, collect option values in command-line order, assign positionals, refuse unrecognized arguments, then check required arguments and alternative groups. The help output (frozen in `internal/cli/testdata/help.golden`) and the error strings stay fixed. Not chosen: Cobra's own required-flag and flag-group checks, whose messages differ from pm's.
- `cli/setup` owns init, doctor, upgrade, uninstall and git-hook commands. Validation runs before writes; uninstall separates preparation, clone removal, managed-piece removal and reporting. Setup consumes parsed arguments through its `Arguments` interface.
- `cli/fsutil` shares filesystem predicates and symlink-aware path resolution. No command or setup package imports the other.
- CI caps every Go file under `internal/cli` at 800 lines. The package-size ratchet is 7,457 non-test lines; every Go function is checked globally for cyclomatic complexity above 30.


One parser and one validator serve both commands, so `pm` cannot accept a record the renderer rejects. Each write: take an exclusive lock on the store directory, which every worktree shares, build the new text in memory, validate every record with the new text substituted, write the file atomically (temp file and rename), then commit exactly the files written on the `records` branch before releasing the lock. For commands that also call `bd`, the `bd` step runs first and the record write only after it succeeds; if the record write then fails validation, `pm` prints the `bd` command that undoes it. Not chosen: one large script, which the renderer already strains at 534 lines.

### How a repo runs it
The installed `pm` binary, which runs the repo's pinned version ([pm as an installable product](pm-product.md), Version pin; [pm in Go](pm-go.md), Distribution). `pm check` validates the records and `pm service` serves the site. Not chosen: a make target per command, which cannot take stdin and arguments cleanly.

### What `pm show` prints
`pm show` prints project state in levels; each level names the command for the next.

| Level | Command | Prints | Size in this repo |
|---|---|---|---|
| Top | `pm show` | A push-failure warning; a warning listing tasks other live sessions hold; the `site:` line; the `feedback:` line (the repo's one feedback doc, `records/docs/pm-feedback.md`: its entry count, its link and how to add to it); under `sessions ask you, outside any task:`, one line per open question with no parent (from a session that held no task): its id, title and session, with the undelivered-reply hint; a pointer to `pm show --project NAME`; one line per open project (name, bead, open and running sprint counts, owner requests), each followed by one line per owner request: kind, short id, title cut at 60 characters, its sprint, and `[undelivered reply: pm reply read ID]` when a site reply has not reached a session | 1,238 characters |
| Project | `pm show --project NAME\|ID` | Goal; the decisions, actions and questions awaiting the owner in full, each with `-> bd show ID`; the same `feedback:` line; each open sprint with its goal, done-when count and open tasks with holders; open sprints without tasks; the last 3 decisions | 6,141 characters for pm-harness |
| Sprint | `pm show --sprint ID` | One sprint's frame, findings and tasks | Not measured |
| Section | `pm show --record TARGET --section NAME` | One record section | Not measured |

Session start injects the top level, and it shares Claude Code's 10,000-character hook cap with the rest of the start context; the single-level output it replaced was about 8,000 characters and was cut. The top level holds what an agent must see before it starts work, so its size grows with open projects and owner requests, not with sprints and tasks.

The top level, for this repo:

```
warning: other live sessions hold these tasks; do not start or delegate them:
  yeeef-agents-9va.84.1  held by 905fe774, 4m, live
site: https://pm.yeeefs.com (the pm service); a record's page is <site>/<its path under records/, without .md>.html; pm record link <target> prints one
feedback: 24 entries -> https://pm.yeeefs.com/docs/pm-feedback.html; when pm gets in your way, run pm feedback add [--project NAME] --text="…"
projects: pm show --project NAME prints one's sprints, tasks, owner requests and last decisions
  agent-setup  yeeef-agents-2sn  1 open sprints, 1 running, 0 owner requests
  pm-harness  yeeef-agents-9va  12 open sprints, 5 running, 5 owner requests
    action .76.2  Review PR #70  (sprint 67)
    action .77.3  Reply with the Cloudflare Access team domain and the AUD ta…  (sprint 68)
    ...
```

The project level, abbreviated:

```
pm-harness  yeeef-agents-9va  Agents and the owner share one project record.
decisions await you (1):
  .66.4  Sprint 57: accept "pm prime under 7,000" as met by nothing being cut?  (sprint 57)  -> bd show yeeef-agents-9va.66.4
actions await you (5):
  .76.2  Review PR #70  (sprint 67)  -> bd show yeeef-agents-9va.76.2
  ...
feedback: 24 entries -> https://pm.yeeefs.com/docs/pm-feedback.html; when pm gets in your way, run pm feedback add [--project NAME] --text="…"
pm-harness  yeeef-agents-9va  sprints and decisions:
Sprint 57: pm show discloses project state level by level  .66  running  0/4 done
  held by: d0e6676f
  goal: An agent sees only top-level project state at session start, and reads deeper levels of `pm show` (a project,…
  done when: 3 items (pm show --sprint yeeef-agents-9va.66)
  in_progress  .66.1  Design the pm show levels and what each prints  [held by d0e6676f, 16m, live]
  ...
decisions (last 3):
  2026-10-07 agent sprint 68  The Access token is required on every public-host request, page reads included, not only on POST /r…
  ...
```

Ids are shortened relative to the project epic. Each decision shows its first sentence; the full text is in the record. `--json` prints every level's data as one object for scripts; `--project` and `--sprint` print text only. Not chosen: reusing `bd prime`, which knows nothing of records; one level cut at the hook cap, which dropped whatever came last.

### Agent hooks
`pm init` writes pm's hook entries into `.claude/settings.json` and `.codex/hooks.json` ([pm as an installable product](pm-product.md)), the same command per event in both runtimes but `pm hook ask`, which runs where each runtime's ask tool can be seen; each runs `pm prime` or `pm hook <name>`, whose logic ships in the binary:

| Event | Command | Does |
|---|---|---|
| SessionStart (Claude Code: every source; Codex: `startup\|resume\|clear\|compact`) | `pm prime --rules N` per chunk, then `pm prime --state` | The rules, then `pm init --session-start`, `pm where` and `pm show --refresh-inbox`, each hook's text within Claude Code's 10,000-character cap; a step that fails is one line saying so, and the session starts anyway |
| SubagentStart | the same chunks, then `pm prime --subagent` | The rules and pm's git rule for agents |
| UserPromptSubmit | `pm hook inbox` | The owner's replies that did not reach this session, as context, then marked delivered ([Reply delivery](reply-delivery.md)); then it closes the session's open Codex questions, with the reply it handed over, else with the prompt, the owner's answer in the chat |
| Claude Code's PermissionRequest on AskUserQuestion (up to a day); Codex's Stop, first | `pm hook ask --runtime claude\|codex` | Raises the session's ask-tool question as a question need under the newest open task the session holds (else none), so the owner can answer it on the site too; in Claude Code it waits beside the dialog and answers the tool with a site reply; the first answer closes the need ([Reply delivery](reply-delivery.md), Questions) |
| Stop | `pm hook owner-request` | Blocks once a reply that asks the owner only in chat ([Owner-request Stop hook](owner-request-hook.md)) |
| Stop; Codex's SubagentStop | `pm hook stop` | Blocks once (never on `stop_hook_active`) while the store holds uncommitted files that this session's tool calls name by their path (Claude Code `tool_use` inputs; Codex `function_call` arguments, `custom_tool_call` inputs and `local_shell_call` actions); another session's file is left alone, and a path only a still-running subagent's prompt names is left out. At Codex's SubagentStop it reads the subagent's own rollout (`agent_transcript_path`), since the parent's holds the subagent's tool inputs encrypted |

`pm hook stop` cannot see a file edited without naming its path (after `cd` into a store folder, or through a glob) and lets the stop through. Agents change code only in a worktree of their own: `pm task claim` refuses in the main checkout.

Not chosen: a Stop check for unclosed claimed tasks (a task stays open across turns by design, so it would be wrong at most stops), PreToolUse guards on generated sections and on `records/` (Bash and `sed` bypass them; `records/` is git-ignored, so no code-branch commit can carry it, and the render check catches a bad record from any tool), PreCompact (it cannot inject context), and needs on every prompt (they show at session start and on the site); see the [hooks survey](../docs/2026-10-04-hooks-for-pm.md).

### Agent identity
Agents run under the owner's git identity, so git and Beads cannot tell them apart. `pm` makes the difference visible where it matters: `pm decision add` writes `source=agent` unless the decision answers a need (`--need`) or the owner confirmed it in the session (`--confirmed`); `bd` itself records `$BEADS_ACTOR` as the actor when it is set. Separate identities for agents are deferred to the agent-sdlc (a yeeef-agents record) project.

### Example: an update during work

```mermaid
sequenceDiagram
  participant A as Agent
  participant CLI as pm CLI
  participant B as Beads
  participant R as Sprint record
  A->>CLI: pm show
  CLI->>B: bd list --all --json
  CLI-->>A: sprint, tasks, needs, today (a few hundred tokens)
  A->>CLI: pm task close yeeef-agents-9va.6.2 --reason "…"
  CLI->>B: bd close with the commit in the reason
  A->>CLI: pm finding add --sprint bd-a3f8.9 "Arm B loss 0.037 vs 0.041"
  CLI->>R: insert bullet in Findings, validate, write
```

*Reading:* the agent reads one summary and sends small commands; the site served by `make docs` shows each write on the next page load.

## Alternatives considered

> What else was considered and not adopted, and why not?

Smaller alternatives sit beside the choice they lost to in Design, marked
"Not chosen".

## Prior art

> Optional. What existing work did we learn from, and what did we take?

None yet.

## Open questions

> What is still unresolved?

None yet.
