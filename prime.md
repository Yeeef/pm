# pm rules

A project lives in two layers. The work store is the work layer: items, holders, status and dependencies. The Markdown records are the record layer: goals, sprint frames, decisions and findings. `pm` is the orchestration layer on top of both. It writes records, and it runs each project action that touches both layers or needs a check. The site shows both layers to the owner.

**pm is self-contained.** It orchestrates the work, and it gives the context about itself, level by level. `pm prime` prints this file at session start: the model and the procedures. `pm show` prints the state. Each command's `--help` holds that command's, or its subsystem's, detail. This file names a command and its refusals; for more, run `pm <noun> [cmd] --help`.

# What

## 1. The layers

| Layer | Holds | Written by |
|---|---|---|
| Work tracking: the work store | What exists, who holds it, its status, what blocks what: projects, sprints, tasks, needs. A Dolt database per clone at `<main checkout>/.pm/store/work`, synced through the remote. | Agents, through `pm` |
| Record layer: Markdown under `records/` | Why and Context: goals, sprint frames, decisions, findings, designs, reports. | Agents, through `pm` and hand edits that `pm commit` commits |
| `pm` cli | The orchestration: it writes records, runs the actions that touch both layers, and checks each write. | |
| `pm` service | One background process per clone (`pm service --help`). It holds the work store, which every `pm` command reaches through it; with the service down, `pm` refuses and names `pm service restart`. It serves the site from the records and the work store, delivers the owner's replies to the sessions that asked, and syncs the records and the work store. | |
| Interface | **The owner's interface is the site.** The owner reads status there and answers there. **The agent's interface is `pm` and the records.** | Nobody; it is rendered |

Each fact has one home: the work store holds the status, the records hold the why. Two copies of one fact drift. Every write to either goes through `pm`.

## 2. The objects

| Object | In the work store | Record | What it is |
|---|---|---|---|
| Project | project | `records/projects/<name>.md` | A standing goal that takes more than one sprint. It has no deadline. |
| Sprint | sprint under the project | `records/sprints/<project>-<n>.md` | One step toward the project's goal, framed by a goal, a scope and a done-when. It takes hours to days, never weeks. Its PR is where work goes back to the owner. |
| Task | task under the sprint, or a sub-task | none | One unit of work with one holder. |
| Need | need: a decision, an action or a review | none | What waits on the owner. A decision need: only they can choose. An action: only they can do the step. A PR review: an action of its own form. |
| Decision | none | `::: decision` block in a project or sprint record | A ruling that changes what the team does, with its reason. |
| Finding | none | bullet in a sprint's Findings | What the work showed, with its numbers. |
| Design page | none | `records/design/<name>.md` | The reference for one area of a design, in its final state. |
| Doc | none | `records/docs/<date>-<slug>.md` | A free-form result or explainer, tied to a bead or a project, or the repo's one feedback doc. |
| Postmortem | none | `records/postmortems/<date>-<slug>.md` | A costly incident: timeline, cost, root cause, what changed, what would have caught it earlier. |
| Day page | none | generated; nobody writes it | What moved on one date, across all projects. |

## 3. Records

**Where records live.** Each clone has one store: the `records` branch, checked out at `<main checkout>/.pm/store/records`. Each worktree's `records/` is a git-ignored link to it, made by `pm init`. So a write from any branch or worktree shows everywhere at once, as the work store does.
- No code branch tracks `records/`, `main` included: people read records on the site or on the `records` branch.
- Many sessions write in one store. Commit only the paths that you edited.

**Format.** A record is Markdown with a small header and fixed `##` sections. The header holds ids only, never a status. Each section opens with a `>` prompt line that says what goes there. A record has all its sections from the start. An empty section holds "None yet.". A close-time section holds "Not closed yet.".

| Record | Sections |
|---|---|
| Project | Goal; Progress (generated); Decisions; Design pages; Outcome (at close) |
| Sprint | Goal; Scope; Done when; Design pages; Progress (generated); Decisions; Findings; Delivery report, with `### Outcome` (starts with `done`, `partial` or `voided` and one sentence; a bullet list of what shipped may follow; only the first paragraph becomes the sprint's close reason) and `### Against "Done when"` (each item met or not, with its evidence: a page, a command, a number) |
| Design page | Problem; Goals and non-goals; Constraints and key facts; Design (free `###` subsections); Alternatives considered; Prior art (optional); Open questions |
| Doc | Free |
| Postmortem | Summary; Timeline; Cost; Root cause; What changed; What would have caught it earlier |
| Day page | all generated |

**The sprint frame.** Goal: what is true when the sprint ends, and why now. Scope: an **In:** and an **Out:** list, at a high level; detail goes in a design page or a doc. Done when: a check and its expected result. The evidence then shows the goal met, or the finding that voids it.

**Blocks.** Use only fenced blocks in a record, e.g.:

- `::: decision {source=owner|agent date=YYYY-MM-DD until="…"}`, which `pm decision add` writes.
- `::: result {title="…"}`: a titled table with a reading line under it.
- A `` ```mermaid `` diagram, with a one-line reading under it.
Use raw HTML only for what Markdown cannot show, such as a mock-up.

**Writing style guide.** The site gives every page one shared stylesheet; a record never carries its own CSS.

- Write an engineer's reference: dense, with no marketing and no decoration.
- Put facts in tables: dimensions, parameters, results. Write formulas out.
- Give only numbers that evidence backs, and state their scope.
- A date or a fact that the records do not have is "not recorded". Do not guess it.
- Draw real diagrams (Mermaid, SVG), never ASCII art. A figure is an image file beside its record: `![…](fig.png)`.
- Use one term for each concept, and define it once. Use plain table headers.
- Keep task ids and digests out of the prose. Give the page link, not a file path.
- Prefer to use bullet points rather than a long paragraph.

## 4. Invariants

- Split the work along independence, so that projects and sprints run in parallel. Where one feeds another, record the dependency: `pm dep add`.
- Records point at work-store ids and never copy a status. The site looks up the status when it builds a page. There is no hand-kept status file.
- **Change code only in a worktree of your own**, never in the main checkout or another session's worktree, so sessions never mix edits, branches or stashes. Read-only work and records writes may run anywhere: `records/` is the shared store. A subagent works in its parent's worktree. `pm task claim` in the main checkout is refused with how to make a worktree.
- Every change has a task in a sprint. Work outside any sprint becomes a small new sprint, so that it shows in the work store.
- A task has one holder, a session. A subagent shares its session's id, so it holds what its session holds.
- A sprint's frame exists before anything runs. A scope change is a sprint decision, never a task renamed or rewritten.
- A sprint that changes code is done when its PR is on main. A sprint with no code change has no PR; it is done when its report is written. A sprint that a finding voided is voided, not failed.
- Agents push a sprint's branch and open its PR without asking. Only the PR review and the merge wait on the owner, raised with `pm action need --pr`.
- A decision has a source, a date and a reason. Without the reason, nobody can tell later if it still holds.
- A decision lives where it governs. Project level when later sprints must follow it; sprint level when it ends with the sprint. `until` names only a known condition to revisit it.
- A choice that is cheap to reverse, and that nobody will ask about, needs no record. The commit message is enough.
- The source is `agent` for your own decisions. It is `owner` only when it answers a need or the owner confirmed it.
- Owner decisions are closed. Do not ask again about a decided question.
- Anything the owner must decide or do is a need, never text in a record.
- An answered decision need is cited by a decision, or marked as setting no rule. So a missed rule is caught.
- A design page holds one area's final state and the alternatives not chosen. The trail of findings stays in sprint records. Decisions and plans stay in project and sprint records.
- When the design changes, edit the page to the new state. Do not add a trail of findings. Put no date in the name or the header. Git keeps the history, and the site shows the created and updated dates.
- A design page covers one area. When it grows to several areas, make each area a sub design page. The main page keeps a short summary per area that links its sub page.
- A postmortem is due when an incident cost more than a day, or broke other sessions or the owner's view.
- A day page is generated. Nobody writes one.
- Session start runs `pm init --session-start`, then injects `pm where` and `pm show`. When it says init failed or timed out, run `pm init` by hand.
- **Never leave a request only in chat.** Ask the owner only for what you cannot do yourself: as a need under its task or sprint (open one if none holds it; if one holds it, do not ask again), or with AskUserQuestion when they are plainly in the chat. The owner-request hook blocks any other ask and says how.

# How

**Bodies.** Some commands below take a body: a goal, a frame, a description, a reason, an answer. Give it as a quoted heredoc, `--text-file - <<'EOF'` … `EOF`, or as `--text="…"` for one plain line without backticks, `$` or quotes. pm reads stdin only for `--text-file -`, and only from a heredoc or a pipe; any other stdin is refused at once. Hooks read their JSON input from stdin.

- check current projects state
  - `pm show`: the top level. It shows what other live sessions hold, each open owner request and undelivered reply, a failed push, and one line per project. Session start injects it, stamped with its UTC time: orient from that copy. Other sessions change the state, so run it again before you tell the owner the project state.
  - drill down one level at a time, only as far as your work needs: `pm show --project NAME` (its sprints, tasks and last decisions), then `pm show --sprint ID`, then `pm show --record <path, sprint id, project name or design slug> --section <name>`. Do not read the whole file for one part.
  - Find ready work: `pm task ready`
  - See a task, its holder and its needs: `pm show <id>`
  - See every location and its state: `pm where`; `pm where records` prints the store's path.
- open a project: `pm project open <name> --title "…"`; body: a one-paragraph Goal, once the owner confirmed the goal in their own words.
- open a sprint: `pm sprint open <project> --title "…"`; body: the frame as `## Goal`, `## Scope` and `## Done when`. It refuses a frame with a required section missing.
- open tasks: `pm task add --sprint ID --title "…"`; body (optional): the description. A dependency: `pm dep add <id> --on <blocker>`. A sub-task: `pm task add --parent <task> --title "…"`.
- claim a task: `pm task claim <id>`, from your own worktree. It records your session and refuses a task that another live session holds, or a claim from the main checkout. Do not take or brief work that another live session holds.
- close a task: `pm task close <id> --reason "…"`. The reason names the commit: HEAD when newer than the task, else `--commit REF` (a commit here, `OWNER/REPO@SHA` or a PR URL). A task not done closes with `--dropped --reason "<why>"` and no commit. It refuses a task another live session holds.
- move a task: `pm task move <id> --to SPRINT_ID`; body: the reason, two lines or more. It records the scope change as a decision in the sprint it leaves, or, for a task directly under a project, in the project's sprint it joins.
- need a decision from owner: `pm decision need --title "…" --parent ID`, with each part as a flag, one line each, in single quotes:
  - one `--question '…'`;
  - one or more `--fact '…'`;
  - two or more `--option LABEL '<what it does>'`, each with its `--cost LABEL '<what it costs>'`;
  - one `--default LABEL '<why>'`.
- need an action from owner: `pm action need --title "…" --parent ID`; body: what to do and why. Examples: run a command, apply a setting. An action is done: check the evidence, then run `pm action done <id> --reason "<what showed it>"`.
- need a pr review from owner: `pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`; body (optional): extra context. The review blocks the sprint close until the PR is on main; close it then with `pm action done <id> --reason "merged as <sha>"`.
- Read a reply from owner with `pm reply read <id>` first. Each close below refuses while the request holds a reply that has not reached a session.
- add a decision: `pm decision add --level project --project NAME`, or `--level sprint --sprint ID`, with `--decision '…'` and `--reason '…'`, one line each, in single quotes. With `--need <id>` it cites the answered need and closes it. Use `--confirmed` instead for an answer that the owner gave in chat.
- close a decision need that sets no rule: `pm decision close <id> --reason "<why>"`. Body: the answer. The answer and the reason stay in the work store, and the need's resolution is `no-decision`.
- add a finding: `pm finding add --sprint ID "<text>"`, as it occurs, with its numbers. A large result table is a `::: result` block in the record.
- create a design page record: `pm design new <slug> --title "…" --project NAME` writes every section with its prompt line; then edit it by hand and `pm commit`. Put no date in the slug.
- create a free-form doc record: `pm doc new <slug> --title "…" --bead ID\|--project NAME`; body: the doc's text, or `--text-file PATH` to read a file; later edits by hand and `pm commit`.
- close a sprint: `pm sprint close <id>`. It refuses an unwritten report, any open task or review, and a review closed without `merged as <sha>`. It skips a dismissed review, such as a replaced PR's.
- close a project: Close every sprint. Write Outcome by hand: the results against the goal in numbers, what was learned and what was retired. Link the sprints' delivery reports. Commit it with `pm commit`, then run `pm project close <name>`.
- create a postmortem: `pm postmortem new <slug> --title "…" --sprint ID\|--project NAME` writes every section; then by hand and `pm commit`. Write it once the incident is fixed, under the sprint it hit.
- find a link: Give the owner a record's URL from `pm record link <target>`; never a `records/…` path or a URL you built.
- **Hand edits.** Goal, Scope, Done when, the delivery report, design pages, docs and postmortems are edited by hand in `records/`. Then commit them: `pm commit -m "…" <path>…`. It checks the whole store. With no path, it lists what is uncommitted and commits nothing. `pm check` checks the store without a commit.
- Report where pm got in your way: `pm feedback add [--project NAME]`, once, with what happened and what would have helped. It appends to the repo's one feedback doc, `records/docs/pm-feedback.md`.
- check pm service status: `pm service status`; `pm service --help` holds the detail.
- setup pm: `pm init` installs pm in the repo, clone and worktree, doing only what is missing: the repo's files and hooks, the records store and its link, and the pm service.
- set the site link or port: `pm init --site-url URL` sets the public site link. `PORT=<n> pm service install` moves this clone's site port; on a first install, run `PORT=<n> pm init`.

For anything this reference does not cover, run `pm <noun> --help`.
