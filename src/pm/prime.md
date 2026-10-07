# pm rules

A project lives in two layers. Beads is the work layer: items, holders, status and dependencies. The Markdown records are the record layer: goals, sprint frames, decisions and findings. `pm` is the orchestration layer on top of both. It writes records, and it runs each project action that touches both layers or needs a check. The site shows both layers to the owner.

Each rule has a reason. When a rule and its reason disagree in a new case, follow the reason. Part 1 says what the pieces are. Part 2 says how to operate them, step by step. Part 3 says how to write to the owner. `pm --help` lists the nouns. Run `pm <noun> --help` for its commands and flags.

# Part 1: what

## 1. The layers

| Layer | Holds | Written by |
|---|---|---|
| Work layer: Beads | What exists, who holds it, its status, what blocks what: epics, tasks, needs. | Agents, through `bd` and `pm` |
| Record layer: Markdown under `records/` | Why and how far: goals, sprint frames, decisions, findings, designs, reports. | Agents, through `pm` and hand edits that `pm commit` commits |
| `pm` | The orchestration: it writes records, runs the actions that touch both layers, and checks each write. | |
| The site | Both layers, rendered for the owner, with live status from Beads. | Nobody; it is rendered |

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

**Invariants.** Each holds whatever the step; part 2 names the check that enforces it.

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
- A need stands alone. The owner reads it days later, on the site, without the conversation.
- An answered decision need is cited by a decision, or marked as setting no rule. So a missed rule is caught.
- A design page holds one area's final state and the alternatives not chosen. The trail of findings stays in sprint records. Decisions and plans stay in project and sprint records.
- A postmortem is due when an incident cost more than a day, or broke other sessions or the owner's view.
- A day page is generated. Nobody writes one.

## 3. The interfaces

**The owner's interface is the site.** The owner reads status there and answers there.

- A project page shows the goal, Progress from Beads, the decisions, the design pages and the sprints.
- A sprint page shows the frame, Progress, the findings, the decisions and the delivery report.
- Each open need is a card under "Decisions await you" or "Actions await you", with a reply box.
- A PR review card links the PR, the sprints' reports and the design pages behind them, and shows the focus. So the owner reads the goal, outcome and design before the diff.
- A day page shows what moved on one date, with a generated Today summary.
- Do not post status outside the site: no status cadence, no channel posts.

**The agent's interface is `pm` and the records.** `pm show` gives the state in a few hundred tokens. `pm show --record … --section …` reads one section. Design pages and docs are read as the reference they are. `bd` finds ready work and holds the tasks. Section 5 says how to read; sections 6 to 8 say how to write.

**pm is self-contained.** It orchestrates the work, and it gives the context about itself, level by level. `pm prime` prints this file at session start: the model and the procedures. `pm show` prints the state. Each command's `--help` holds that command's, or its subsystem's, detail. This file names a command and its refusals; for more, run `pm <noun> [cmd] --help`.

**Behind both.** The pm service, one background process per clone, serves the site from the records and Beads and runs `pm push`: it pushes Beads data and the `records` branch and generates the day summary. Sessions push neither. `pm check` checks every record. When `pm where` shows the service down, run `pm service restart`. If that fails, raise an action and add a bug task. For more about the service (ports, logs, health, stale builds), read `pm service --help`. `pm push --help` holds the push's detail. Set the public site link with `pm init --site-url URL`. Move this clone's site port with `PORT=<n> pm service install`; on a repo's first install, `PORT=<n> pm init` also writes it to the config.

## 4. Working with them

**Breaking the work down.**

1. List what must become true for the goal to be met. An item that needs more than one sprint is a project. An item that fits in a sprint is a sprint.
2. Split the work along independence, so that projects and sprints run in parallel. Where one feeds another, record the dependency in Beads.
3. Test each item against the goal. If finishing it would not move the work toward the goal, it is harness: checkers, tools, audits. Build harness only when a named sprint is blocked without it.
4. Plan one sprint ahead. Choose the next sprint from the last result.
5. Run the smallest test that can change the decision. If each result leads to the same next step, do not run it.

**The sprint frame.** Goal: what is true when the sprint ends, and why now. Scope: an **In:** and an **Out:** list, at a high level; detail goes in a design page. Done when: a check and its expected result, written before the run. The evidence then shows the goal met, or the finding that voids it. For a one-shot or costly run, the expected result is written first, so the run cannot be read backwards.

**Sizing.** A sprint takes hours to days. Longer work is a project of several sprints. Each sprint that changes code ends in a PR, so the owner sees work land at that pace.

**Why needs go to the site.** Chat ends with the session; the owner reads the site when they have time. A need is tracked in Beads, blocks what waits on it, and its reply reaches the session that raised it. So raise the need, then continue other ready work. Do not stop, and do not decide silently.

# Part 2: how

## 5. Reading state

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

Session start runs `pm init`, then injects `pm where` and `pm show` beside `bd prime`. When it says init failed or timed out, run `pm init` by hand. Never run `bd init`, which makes a new database.

**Reading pm show.**

| Text | Meaning | Do |
|---|---|---|
| `held by <session>, <age>, live` | A session works on it. | Leave it. |
| `held by <session>, <age>, idle` | Its session wrote nothing for 30 minutes. | `pm task claim` can take it. |
| `held by <name> without a session` | Someone claimed it outside `pm`. | Leave it, unless the owner tells you to take it. |
| `[undelivered reply: pm reply read <id>]` | An owner reply did not get to its session. | Run `pm reply read <id>`. Then do its next step. |
| `warning: the pm service's push needs attention` | The push failed or is late. | Read `pm service status` and `pm service logs`. If you cannot fix it, raise an action and add a bug task. |

## 6. Projects, sprints and tasks

**Open a project.** Once the owner confirmed the goal in their own words: `pm project open <name> --title "…"`, a one-paragraph Goal on stdin. It creates the epic and the record.

**Open a sprint.** `pm sprint open <project> --title "…"`, the frame on stdin as `## Goal`, `## Scope` and `## Done when`, before anything runs. It refuses a frame with a section missing. Then add its tasks: `pm task add --sprint ID --title "…"`, the description on stdin. Dependencies and sub-tasks are plain `bd`: `bd dep add`, `bd create --parent <task>`.

**Work a task.**

- Before you start or delegate a task, read its holders and open needs in `pm show`. Do not take or brief work that another live session holds.
- Claim it first: `pm task claim <id>`. It records your session and refuses a task that another live session holds. Do not claim with `bd update --claim`.
- Tell a subagent that it holds a task only after the claim succeeds. A subagent claims and closes its own task. It gets these rules and the Beads profile line, but no `pm show`.
- Examine a subagent's report against the files before you accept it.
- Work the sprint, push its branch and open its PR without asking. The owner authorizes these steps for every sprint.
- Close it with `pm task close <id> --reason "…"`, never `bd close`. The reason names the commit: HEAD when newer than the task, else `--commit REF`.
- Move it with `pm task move <id> --to SPRINT_ID`, the reason on stdin, two lines or more. It records the scope change as a decision in the sprint it leaves.

**Close a sprint.** A sprint that changes code closes only once its PR is on main. A merge into a stacked base is not on main. A sprint with no code change, such as an investigation or a design page, has no PR. It closes after its report and its tasks, whatever its verdict. `pm sprint close` does not ask GitHub. A review under the sprint blocks the close; close the review only once the PR is on main.

1. Write the full delivery report by hand, then commit it with `pm commit`.
   - Outcome: start with `done`, `partial` or `voided`, plus one sentence. A bullet list of what shipped may follow. Only the first paragraph becomes the Beads close reason.
   - Against "Done when": give each item as met or not, with its evidence: a page, a command, a number.
2. With a PR: push the branch and open the PR without asking.
3. With a PR: raise the review, `pm action need --pr URL --sprint ID --focus "…" [--design SLUG]` (section 8). Every sprint named must be open, with its report committed. The review sits under the first sprint named and blocks its close.
4. Close each other open task with `pm task close`, or move it with `pm task move`.
5. With a PR: once it is on main (section 8 says how you learn it), close the review: `pm action done <review> --reason "merged as <sha>"`.
6. With a PR: fast-forward the main checkout, `git -C <main checkout> pull --ff-only origin main`. Hooks, rules and links read it.
7. Run `pm sprint close <id>`.
   - It refuses an unwritten report, any open task or review, and a review closed without `merged as <sha>`. It skips a dismissed review, such as a replaced PR's.
   - With a review, it stamps `Merged as <sha> (PR #N).` into the Outcome after the verdict and commits that on the `records` branch. A sprint with no review gets no stamp.
   - The epic's close reason names the records commit, so the milestone points at the exact state it delivered.

**Close a project.** Close every sprint. Write Outcome by hand: the results against the goal in numbers, what was learned and what was retired. Link the sprints' delivery reports. Commit it with `pm commit`, then run `pm project close <name>`. Nothing is deleted: closed projects stay on the site.

## 7. Records

**Where records live.** Each clone has one store: the `records` branch, checked out at `<main checkout>/.pm/store/records`. Each worktree's `records/` is a git-ignored link to it, made by `pm init`. So a write from any branch or worktree shows everywhere at once, as Beads does.

- Code branches never commit `records/`. The pre-commit hook and the PR guard refuse it.
- A GitHub Action copies the store into `main`'s `records/` on each push to `main`.
- Many sessions write in one store. Commit only the paths that you edited.

**Format.** A record is Markdown with a small header and fixed `##` sections. The header holds ids only, never a status. Each section opens with a `>` prompt line that says what goes there. A record has all its sections from the start. An empty section holds "None yet.". A close-time section holds "Not closed yet.". A generated section holds only its prompt line: the site fills Progress from Beads. The site also adds Decisions await you, Actions await you, Docs and Postmortems to pages. Never write a heading with one of these names. The record check fails on hand-written text in a generated section.

| Record | Sections | Write |
|---|---|---|
| Project | Goal; Progress (generated); Decisions; Design pages; Outcome (at close) | Open and close: section 6. Decisions: `pm decision add --level project --project NAME`. |
| Sprint | Goal; Scope; Done when; Design pages; Progress (generated); Decisions; Findings; Delivery report, with `### Outcome` and `### Against "Done when"` | Open and close: section 6. Findings: `pm finding add "<text>" --sprint ID`, as they occur, with their numbers; a large result table is a `::: result` block. Decisions: `pm decision add --level sprint --sprint ID`. Design pages and the delivery report: by hand, then `pm commit`. |
| Design page | Problem; Goals and non-goals; Constraints and key facts; Design (free `###` subsections); Alternatives considered; Prior art (optional); Open questions | `pm design new <slug> --title "…" --project NAME` writes every section with its prompt line; then edit by hand and `pm commit`. Constraints, Alternatives and Open questions may hold "None yet.". An explanation or a reference (an architecture, a data format, "explain step 1") goes here or in a doc. |
| Doc | Free | `pm doc new <slug> --title "…" --bead ID\|--project NAME`, the body on stdin; later edits by hand and `pm commit`. |
| Postmortem | Summary; Timeline; Cost; Root cause; What changed; What would have caught it earlier | `pm postmortem new <slug> --title "…" --sprint ID\|--project NAME` writes every section; then by hand and `pm commit`. Write it once the incident is fixed, under the sprint it hit. |
| Day page | all generated | Nobody. The push runs `pm day summarize`; older day records keep their paragraph as history. |

**A decision's body** comes on stdin: the decision on its first line, its reason on the next. When the reason is too long, put it in a doc and link the doc. `--level` has no default. Add `--until "…"` only for a known condition to revisit. A decision that turns out to apply beyond one sprint moves to the project record as soon as that shows.

**Every `pm` write.**

- It names its target with `--sprint ID` or `--project NAME`. A repo holds many projects, so pm never infers "the only project" or "the open sprint".
- It checks the records it writes and those of the issues it changes, as its commit will leave them. Other sessions' uncommitted files are not checked and not relied on.
- It refuses a record with uncommitted changes, so it never carries an edit in progress. Commit that record with `pm commit -m "…" <path>`, or revert it. Then run the write again.
- It commits what it wrote on the `records` branch. If the commit fails, it puts its files back and says so.

**Hand edits.** Goal, Scope, Done when, the delivery report, design pages, docs and postmortems are edited by hand in `records/`. Then commit them: `pm commit -m "…" <path>…`. It checks the whole store. With no path, it lists what is uncommitted and commits nothing. `pm check` checks the store without a commit.

- A Stop hook (`pm hook stop`) blocks your turn once while records that your tool calls named are uncommitted. Commit or revert them before you hand back. Leave a file that another session is writing.

**Blocks.** Use only these fenced blocks in a record.

- `::: decision {source=owner|agent date=YYYY-MM-DD until="…"}`, which `pm decision add` writes.
- `::: result {title="…"}`: a titled table with a reading line under it.
- A `` ```mermaid `` diagram, with a one-line reading under it.

Use raw HTML only for what Markdown cannot show, such as a mock-up.

**Design pages.**

- When the design changes, edit the page to the new state. Do not add a trail of findings.
- Put no date in the name or the header. Git keeps the history, and the site shows the created and updated dates.
- A page covers one area. When it grows to several areas, make each area a sub page. The main page keeps a short summary per area that links its sub page.
- A sprint's Design pages section links the sub pages that its work changed, not only the main page.

**Writing style guide.** The site gives every page one shared stylesheet; a record never carries its own CSS.

- Write an engineer's reference: dense, with no marketing and no decoration.
- Put facts in tables: dimensions, parameters, results. Write formulas out.
- Give only numbers that evidence backs, and state their scope.
- A date or a fact that the records do not have is "not recorded". Do not guess it.
- When the repository's own docs are stale, say so on the page. Do not correct them silently.
- Draw real diagrams (Mermaid, or inline SVG), never ASCII art.
- Use one term for each concept, and define it once. Use plain table headers.
- Keep task ids and digests out of the prose. Put them in an evidence table when you need them.

**Links.** Give the owner a record's URL from `pm record link <target>`; never a `records/…` path or a URL you built. When it fails, run the command that it names. Open the page and look at it before you send it, in light and dark mode and at phone width. A page that nobody looked at is not done. Make one accuracy pass on the page. No edit for looks may change a number.

## 8. Needs and actions

**Raise a need** under the sprint or task, then continue other ready work. Read the Decisions first, project then sprint. Then the reply asks for it in plain words, with no id.

- **Decision** (`pm decision need --title "…" --parent ID`, under "Decisions await you"): the owner chooses. Stdin gives one part per line:
  - one `Question:`;
  - one or more `Fact:`;
  - two or more `Option <label>:`, each with a `Cost:` line under it;
  - one `Default: <label>`, then its reason.
- pm writes the card in one layout. It refuses an option without a cost, a default that names no option, and a sentence over 25 words. Send stdin with a quoted heredoc (`<<'EOF'`), so code spans stay.
- **Action** (`pm action need --title "…" --parent ID`, under "Actions await you"): the owner does a step that only they can do. Examples: run a command, apply a setting. The description on stdin says what to do and why.
- **PR review** (`pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`): section 6, step 3. Stdin holds optional extra context.
- Both commands refuse a request that names a PR and asks to review, merge or approve it without `--pr`.

**Never leave a request only in chat.** Raise a need first for each request that sprint work waits on. Examples: a decision on a sprint's scope or design, a PR review or merge, an action a task waits on.

- A Stop hook (`pm hook owner-request`) checks your final reply. It reads the open needs and actions that this session raised. Claude Haiku says if each request in the reply matches one of them.
- A request that matches none, or matches only another session's need, blocks the turn once.
- Clarifying questions pass. Offers of optional extra work that nothing waits on pass. Reports that only describe a request pass.
- An offer to do a step that the task or sprint needs before it can close is a request. Examples: merge its PR, choose a design. The wording does not change this.
- Asking leave to work the sprint, push its branch or open its PR blocks the turn, even with a need. Do it instead.
- When Beads or the judge fails, the hook says so on stderr and lets the turn end.
- A subagent's report to its parent is not checked.

**Owner replies.** The owner replies on the request's card. The site stores the reply as a Beads comment on the request, which stays open.

- The pm service pushes the reply at once into the inbox of the Claude Code session that raised the request. It pushes a reviewed PR's merge to main the same way.
- It arrives as a new turn, or between tool calls when you are busy. Do not poll, and do not start a waiter.
- Only the site's replies count, never comments by pm or by an agent.
- A reply that could not be pushed waits: the session had ended, or had no inbox, as in Codex. `pm show` flags it. `pm reply read <id>` prints it and marks it delivered.
- The owner can also answer in Beads (`bd human respond`) or in the conversation.

**Act on a reply at once.** Do its next step, then close the need.

- The answer sets a rule that later work must follow: record it with `pm decision add --need <id> --level …`. This cites the need and closes it. Use `--confirmed` instead for an answer that the owner gave in chat.
- The answer is a small input that sets no rule, such as a name or a port. Run `pm decision close <id> --reason "<why>"`. The answer goes on stdin. The answer and the reason stay in Beads, and the need gets the label `no-decision`.
- If you are not sure, record a decision. The record check fails on an answered decision need that no decision cites and that has no `no-decision` label.
- An action is done: check the evidence, then run `pm action done <id> --reason "<what showed it>"`. It needs no decision.
- Each of these closes refuses while the request holds a reply that has not reached a session. Read it with `pm reply read <id>` first.

# Part 3: writing to the owner

Obey these rules in chat replies, needs, actions and the records the owner reads:
- Put the conclusion first.
- Use 4 bullets or fewer.
- Do not use ids, hashes or file names unless the owner asks.
- Give the page link, not a file path.
- Tell what each number measures.
- Use ASD-STE100 approved words. Give each word one meaning.
- Use at most 20 words in an instruction sentence and 25 in a description sentence.
- Use the active voice and simple tenses.
- Write one instruction in each sentence.
