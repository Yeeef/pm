# pm rules

A project lives in two layers. Beads is the work layer: items, holders, status and dependencies. The Markdown records are the record layer: goals, sprint frames, decisions and findings. `pm` is the orchestration layer on top of both. It writes records, and it runs each project action that touches both layers or needs a check. The site shows both layers to the owner.

**pm is self-contained.** It orchestrates the work, and it gives the context about itself, level by level. `pm prime` prints this file at session start: the model and the procedures. `pm show` prints the state. Each command's `--help` holds that command's, or its subsystem's, detail. This file names a command and its refusals; for more, run `pm <noun> [cmd] --help`.

# What

## 1. The layers

| Layer | Holds | Written by |
|---|---|---|
| Work tracking: Beads | What exists, who holds it, its status, what blocks what: epics, tasks, needs. | Agents, through `bd` and `pm` |
| Record layer: Markdown under `records/` | Why and Context: goals, sprint frames, decisions, findings, designs, reports. | Agents, through `pm` and hand edits that `pm commit` commits |
| `pm` cli | The orchestration: it writes records, runs the actions that touch both layers, and checks each write. | |
| `pm` service | `pm serve` serves the site from the records and Beads. A scheduled job runs `pm push`: it pushes Beads data and the `records` branch and generates the day summary. Sessions push neither. `pm render` checks every record. `pm --help` describes each; `pm push --help` holds the push's detail. | |
| Interface | **The owner's interface is the site.** The owner reads status there and answers there. **The agent's interface is `pm` and the records.** | Nobody; it is rendered |

Each fact has one home: Beads holds the status, the records hold the why. Two copies of one fact drift. `pm` wraps an action only when it touches both Beads and a record, or needs a check beyond Beads. Other task work stays plain `bd`.

## 2. The objects

| Object | In Beads | Record | What it is |
|---|---|---|---|
| Project | epic | `records/projects/<name>.md` | A standing goal that takes more than one sprint. It has no deadline. |
| Sprint | child epic of the project | `records/sprints/<project>-<n>.md` | One step toward the project's goal, framed by a goal, a scope and a done-when. It takes hours to days, never weeks. Its PR is where work goes back to the owner. |
| Task | task under the sprint, or a sub-task | none | One unit of work with one holder. |
| Need | issue labelled `human`; an action also `action` | none | What waits on the owner. A decision need: only they can choose. An action: only they can do the step. A PR review: an action of its own form. |
| Decision | none | `::: decision` block in a project or sprint record | A ruling that changes what the team does, with its reason. |
| Finding | none | bullet in a sprint's Findings | What the work showed, with its numbers. |
| Design page | none | `records/design/<name>.md` | The reference for one area of a design, in its final state. |
| Doc | none | `records/docs/<date>-<slug>.md` | A free-form result or explainer, tied to a bead or a project. |
| Postmortem | none | `records/postmortems/<date>-<slug>.md` | A costly incident: timeline, cost, root cause, what changed, what would have caught it earlier. |
| Day page | none | generated; nobody writes it | What moved on one date, across all projects. |

### 7. Records

**Where records live.** Each clone has one store: the `records` branch, checked out at `<main checkout>/.records`. Each worktree's `records/` is a git-ignored link to it, made by `pm setup`. So a write from any branch or worktree shows everywhere at once, as Beads does.
- Code branches never commit `records/`.
- A GitHub Action copies the store into `main`'s `records/` on each push to `main`.
- Many sessions write in one store. Commit only the paths that you edited.

**Format.** A record is Markdown with a small header and fixed `##` sections. The header holds ids only, never a status. Each section opens with a `>` prompt line that says what goes there. A record has all its sections from the start. An empty section holds "None yet.". A close-time section holds "Not closed yet.".

| Record | Sections | 
|---|---|---|
| Project | Goal; Progress (generated); Decisions; Design pages; Outcome (at close) |
| Sprint | Goal; Scope; Done when; Design pages; Progress (generated); Decisions; Findings; Delivery report, with `### Outcome` and `### Against "Done when"` |
| Design page | Problem; Goals and non-goals; Constraints and key facts; Design (free `###` subsections); Alternatives considered; Prior art (optional); Open questions  |
| Doc | Free | 
| Postmortem | Summary; Timeline; Cost; Root cause; What changed; What would have caught it earlier | 
| Day page | all generated | 

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
- Draw real diagrams (Mermaid, or inline SVG), never ASCII art.
- Use one term for each concept, and define it once. Use plain table headers.
- Keep task ids and digests out of the prose.

### the invariants rules

- Split the work along independence, so that projects and sprints run in parallel. Where one feeds another, record the dependency in Beads.
- Records point at Beads ids and never copy a status. The site looks up the status when it builds a page. There is no hand-kept status file.
- Every change has a task in a sprint. Work outside any sprint becomes a small new sprint, so that it shows in Beads.
- A task has one holder, a session. A subagent shares its session's id, so it holds what its session holds.
- A sprint's frame exists before anything runs. A scope change is a sprint decision, never a task renamed or rewritten.
- A sprint that changes code is done when its PR is on main. A sprint with no code change has no PR; it is done when its report is written. A sprint that a finding voided is voided, not failed.
- A decision has a source, a date and a reason. Without the reason, nobody can tell later if it still holds.
- A decision lives where it governs. Project level when later sprints must follow it; sprint level when it ends with the sprint. `until` names only a known condition to revisit it.
- A choice that is cheap to reverse, and that nobody will ask about, needs no record. The commit message is enough.
- The source is `agent` for your own decisions. It is `owner` only when it answers a need or the owner confirmed it.
- Owner decisions are closed. Do not ask again about a decided question.
- Anything the owner must decide or do is a need, never text in a record and never only in chat.
- An answered decision need is cited by a decision, or marked as setting no rule. So a missed rule is caught.
- A design page holds one area's final state and the alternatives not chosen. The trail of findings stays in sprint records. Decisions and plans stay in project and sprint records.
- A postmortem is due when an incident cost more than a day, or broke other sessions or the owner's view.
- A day page is generated. Nobody writes one.
- Session start runs `pm setup`, then injects `pm where` and `pm show` beside `bd prime`. When it says setup failed or timed out, run `pm setup` by hand. Never run `bd init`, which makes a new database.
- **Never leave a request only in chat.** Raise a need first for each request that sprint work waits on. Examples: a decision on a sprint's scope or design, a PR review or merge, an action a task waits on.

- When the design changes, edit the page to the new state. Do not add a trail of findings. Put no date in the name or the header. Git keeps the history, and the site shows the created and updated dates.
- A design page covers one area. When it grows to several areas, make each area a sub design page. The main page keeps a short summary per area that links its sub page.

- **The sprint frame.** Goal: what is true when the sprint ends, and why now. Scope: an **In:** and an **Out:** list, at a high level; detail goes in a design page. Done when: a check and its expected result, written before the run. The evidence then shows the goal met, or the finding that voids it. For a one-shot or costly run, the expected result is written first, so the run cannot be read backwards.
- If you are not sure, record a decision. The record check fails on an answered decision need that no decision cites and that has no `no-decision` label.

# How

| To do this | Use |
|---|---|
| See what is open, who holds what, and what waits on the owner | `pm show`. Session start injects it, stamped with its UTC time: orient from that copy. Other sessions change the state, so run it again before you tell the owner the project state. |
| See one sprint's frame, findings and tasks | `pm show --sprint ID` |
| Read one section of a record | `pm show --record <path, sprint id, project name or design slug> --section <name>`. Do not read the whole file for one part. |
| Find ready work | `bd ready --exclude-type=epic` |
| See a task, its holder and its needs | `pm show`, then `bd show <id>` |
| Keep a small operational fact (a command, a path, a gotcha) | `bd remember`; decisions go in records, not there |
| See every location and its state | `pm where`; `pm where records` prints the store's path |
| Report where pm got in your way | `pm feedback add --project NAME`, once, with what happened and what would have helped |

- check current projects state
 -  `pm show`: See an overview of what is open, who holds what, and what waits on the owner
 -  drill down for more details with pm show --sprint, pm show --record,
 - Find ready work: `bd ready --exclude-type=epic`
 - See a task, its holder and its needs : `bd show <id>`
- open a project: `pm project open <name> --title "…"`
- open a sprint: `pm sprint open <project> --title "…"`. It refuses a frame with the required section missing.
- open tasks: `pm task add --sprint ID --title "…"`, the description on stdin. Dependencies and sub-tasks are plain `bd`: `bd dep add`, `bd create --parent <task>`.
- claim a task: `pm task claim <id>`. It records your session and refuses a task that another live session holds. Do not claim with `bd update --claim`.  Do not take or brief work that another live session holds.
- close a task: `pm task close <id> --reason "…"`, never `bd close`. The reason names the commit: HEAD when newer than the task, else `--commit REF`.
- move a task: `pm task move <id> --to SPRINT_ID`, the reason on stdin, two lines or more. It records the scope change as a decision in the sprint it leaves.
- close a sprint: Write the full delivery report in sprint record by hand, then commit it with `pm commit`.
- Typically a sprint is associated with a PR. Open PR and ask owner to review with `pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`
- close a sprint: `pm sprint close <id>`.It refuses an unwritten report, any open task or review, and a review closed without `merged as <sha>`. It skips a dismissed review, such as a replaced PR's. 
- close a project: Close every sprint. Write Outcome by hand: the results against the goal in numbers, what was learned and what was retired. Link the sprints' delivery reports. Commit it with `pm commit`, then run `pm project close <name>`.
- need a decision from owner: `pm decision need --title "…" --parent ID` Stdin gives one part per line:
  - one `Question:`;
  - one or more `Fact:`;
  - two or more `Option <label>:`, each with a `Cost:` line under it;
  - one `Default: <label>`, then its reason.
- need an action from owner: `pm action need --title "…" --parent ID`, under "Actions await you"): the owner does a step that only they can do. Examples: run a command, apply a setting. The description on stdin says what to do and why. An action is done: check the evidence, then run `pm action done <id> --reason "<what showed it>"`. It needs no decision.

- need a pr review from owner: `pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`): section 6, step 3. Stdin holds optional extra context.
- Read a reply from owner with `pm reply read <id>` first.
- add a finding:
- add a decision: `pm decision add --need <id> --level …`. This cites the need and closes it. Use `--confirmed` instead for an answer that the owner gave in chat.
- close a decision: `pm decision close <id> --reason "<why>"`. The answer goes on stdin. The answer and the reason stay in Beads, and the need gets the label `no-decision`.
- create a design page record:
- create a free-form doc record: `pm doc new <slug> --title "…" --bead ID\|--project NAME`, the body on stdin; later edits by hand and `pm commit`.
- create a postmortem: `pm postmortem new <slug> --title "…" --sprint ID\|--project NAME` writes every section; then by hand and `pm commit`. Write it once the incident is fixed, under the sprint it hit.
- find a link: **Links.** Give the owner a record's URL from `pm record link <target>`; never a `records/…` path or a URL you built. 
- **Hand edits.** Goal, Scope, Done when, the delivery report, design pages, docs and postmortems are edited by hand in `records/`. Then commit them: `pm commit -m "…" <path>…`. It checks the whole store. With no path, it lists what is uncommitted and commits nothing. `pm render` checks the store without a commit.
1. Write the full delivery report by hand, then commit it with `pm commit`.
   - Outcome: start with `done`, `partial` or `voided`, plus one sentence. A bullet list of what shipped may follow. Only the first paragraph becomes the Beads close reason.
   - Against "Done when": give each item as met or not, with its evidence: a page, a command, a number.
- Keep a small operational fact (a command, a path, a gotcha) | `bd remember`; decisions go in records, not there |
- check pm service status:
- setup pm: pm init TODO

- The answer sets a rule that later work must follow: record it with `pm decision add --need <id> --level …`. This cites the need and closes it. Use `--confirmed` instead for an answer that the owner gave in chat.
- The answer is a small input that sets no rule, such as a name or a port. Run `pm decision close <id> --reason "<why>"`. The answer goes on stdin. The answer and the reason stay in Beads, and the need gets the label `no-decision`.
- Each of these closes refuses while the request holds a reply that has not reached a session. Read it with `pm reply read <id>` first.

# Part 3: writing to the owner

Obey these rules in chat replies, needs, actions and the records the owner reads:
- Put the conclusion first.
- Use 4 bullets or fewer.
- Do not use ids, hashes or file names unless the owner asks.
- Give the page link, not a file path.
- Tell what each number measures.
- Write one instruction in each sentence.
