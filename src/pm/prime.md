# pm rules

A project lives in two layers. Beads is the work layer: items, holders, status and dependencies. The Markdown records are the record layer: goals, sprint frames, decisions and findings. `pm` is the orchestration layer on top of both. It writes records, and it runs each project action that touches both layers or needs a check. The site that `pm serve` serves shows both layers to the owner. The owner reads the site; agents read `pm show`.

Each rule has a reason. When a rule and its reason disagree in a new case, follow the reason. `pm --help` lists the nouns. Run `pm <noun> --help` for its commands and flags.

## 1. The objects

| Object | In Beads | Record | What it is |
|---|---|---|---|
| Project | epic | `records/projects/<name>.md` | A standing goal that takes more than one sprint. It has no deadline. |
| Sprint | child epic of the project | `records/sprints/<project>-<n>.md` | One step toward the project's goal, framed by a goal, a scope and a done-when. It takes hours to days, never weeks. Its PR is where work goes back to the owner. |
| Task | task under the sprint, or a sub-task | none | One unit of work with one holder. |
| Need | issue labelled `human`; an action also `action` | none | What waits on the owner: a decision only they can make, or a step only they can do. |
| Decision | none | `::: decision` block in a project or sprint record | A ruling that changes what the team does, with its reason. Project level when later sprints must follow it; sprint level when it ends with the sprint. `until` names a known condition to revisit it. |
| Design page | none | `records/design/<name>.md` | The reference for one area of a design, in its final state. |
| Doc | none | `records/docs/<date>-<slug>.md` | A free-form result or explainer, tied to a bead or a project. |
| Postmortem | none | `records/postmortems/<date>-<slug>.md` | A costly incident: timeline, cost, root cause, what changed, what would have caught it earlier. |
| Day page | none | generated; nobody writes it | What moved on one date, across all projects. |

Records point at Beads ids and never copy a status. The site looks up the status when it builds a page. Two copies of one fact drift. There is no hand-kept status file.

**Using pm.**

| To do this | Use |
|---|---|
| See what is open, who holds what, and what waits on the owner | `pm show`. Session start injects it, stamped with its UTC time: orient from that copy. Other sessions change the state, so run it again before you tell the owner the project state. |
| See one sprint's frame, findings and tasks | `pm show --sprint ID` |
| Read one section of a record | `pm show --record <path, sprint id, project name or design slug> --section <name>`. Do not read the whole file for one part. |
| Find ready work | `bd ready --exclude-type=epic` |
| Add, claim, close or move a task | `pm task add --sprint ID --title "…"` (description on stdin), `pm task claim <id>`, `pm task close <id> --reason "…"`, `pm task move <id> --to SPRINT_ID` (reason on stdin, two lines or more) |
| Add a dependency or a sub-task | `bd dep add`, `bd create --parent <task>` |
| Open or close a project or sprint; write a record; raise or answer a need | `pm`; section 2 names the command for each record, section 3 for each need |
| Edit a record by hand | Edit it in `records/`, then `pm commit -m "…" <path>…` |
| Give the owner a link to a record | `pm record link <target>`; never a `records/…` path or a URL you built. When it fails, run the command that it names. |
| Keep a small operational fact (a command, a path, a gotcha) | `bd remember`; decisions go in records, not there |
| Report where pm got in your way | `pm feedback add --project NAME`, once, with what happened and what would have helped |

`pm` wraps an action only when it touches both Beads and a record, or needs a check beyond Beads. Other task work stays plain `bd`.

- Do not claim with `bd update --claim` or close with `bd close`. `pm task claim` records your session and refuses a task that another live session holds. `pm task close` names the commit in its reason.
- Before you start or delegate a task, read its holders and open needs in `pm show`. Do not take or brief work that another live session holds.
- Claim a task before you start it. Tell a subagent that it holds a task only after the claim succeeds.
- A subagent shares its session's id. It claims and closes its own task. It gets these rules and the Beads profile line, but no `pm show`.
- Examine a subagent's report against the files before you accept it.
- A scope change is a sprint decision. Do not rename a task or rewrite its description for it.
- Every change has a task in a sprint. Work outside any sprint becomes a small new sprint, so that it shows in Beads.

**Reading pm show.**

| Text | Meaning | Do |
|---|---|---|
| `held by <session>, <age>, live` | A session works on it. | Leave it. |
| `held by <session>, <age>, idle` | Its session wrote nothing for 30 minutes. | `pm task claim` can take it. |
| `held by <name> without a session` | Someone claimed it outside `pm`. | Leave it, unless the owner tells you to take it. |
| `[undelivered reply: pm reply read <id>]` | An owner reply did not get to its session. | Run `pm reply read <id>`. Then do its next step. |
| `warning: the scheduled push needs attention` | The push failed or is late. | Read `pm where` and `.git/pm-push.log`. If you cannot fix it, raise an action and add a bug task. |

## 2. Records

**Where records live.** Each clone has one store: the `records` branch, checked out at `<main checkout>/.records`. Each worktree's `records/` is a git-ignored link to it, made by `pm setup`. So a write from any branch or worktree shows everywhere at once, as Beads does. `pm where records` prints the store's path.

- Code branches never commit `records/`. The pre-commit hook and the PR guard refuse it.
- A GitHub Action copies the store into `main`'s `records/` on each push to `main`.
- `pm` commits but never pushes. Sessions push neither Beads data nor the `records` branch (section 5).
- Many sessions write in one store. Commit only the paths that you edited.

**Format.** A record is Markdown with a small header and fixed `##` sections. The header holds ids only, never a status. Each section opens with a `>` prompt line that says what goes there. A record has all its sections from the start. An empty section holds "None yet.". A close-time section holds "Not closed yet.". A generated section holds only its prompt line: the site fills Progress from Beads. The site also adds Decisions await you, Actions await you, Docs and Postmortems to pages. Never write a heading with one of these names. The record check (section 5) fails on hand-written text in a generated section.

| Record | Sections | Write | Read |
|---|---|---|---|
| Project | Goal; Progress (generated); Decisions; Design pages; Outcome (at close) | Open: `pm project open <name> --title "…"`, a one-paragraph Goal on stdin, once the owner confirmed the goal in their own words. Decisions: `pm decision add --level project --project NAME`. Close: close every sprint, then write Outcome by hand, `pm commit` it, and run `pm project close <name>`. Outcome gives the results against the goal in numbers, what was learned and what was retired. It links the sprints' delivery reports. Nothing is deleted: closed projects stay on the site. | `pm show`; `pm show --record NAME --section Decisions` |
| Sprint | Goal; Scope; Done when; Design pages; Progress (generated); Decisions; Findings; Delivery report, with `### Outcome` and `### Against "Done when"` | Open: `pm sprint open <project> --title "…"`, the frame on stdin as `## Goal`, `## Scope` and `## Done when`, before anything runs. Goal: what is true when the sprint ends, and why now. Scope: an **In:** and an **Out:** list, at a high level; detail goes in a design page. Done when: checkable evidence that the goal is met, or the finding that voids it. For a one-shot or costly run, write the expected result before the run. Findings: `pm finding add "<text>" --sprint ID`, as they occur, with their numbers; a large result table is a `::: result` block. Decisions: `pm decision add --level sprint --sprint ID`. Design pages and the delivery report: by hand, then `pm commit`. Close: section 3, PR review and sprint close. | `pm show --sprint ID`; `pm show --record ID --section Findings` |
| Design page | Problem; Goals and non-goals; Constraints and key facts; Design (free `###` subsections); Alternatives considered; Prior art (optional); Open questions | `pm design new <slug> --title "…" --project NAME` writes every section with its prompt line; then edit by hand and `pm commit`. Constraints, Alternatives and Open questions may hold "None yet.". An explanation or a reference (an architecture, a data format, "explain step 1") goes here or in a doc. | `pm show --record SLUG --section Design` |
| Doc | Free | `pm doc new <slug> --title "…" --bead ID\|--project NAME`, the body on stdin; later edits by hand and `pm commit`. | `pm show --record records/docs/<date>-<slug>.md --section <name>` |
| Postmortem | Summary; Timeline; Cost; Root cause; What changed; What would have caught it earlier | `pm postmortem new <slug> --title "…" --sprint ID\|--project NAME` writes every section; then by hand and `pm commit`. Due when an incident cost more than a day, or broke other sessions or the owner's view. Write it once the incident is fixed, under the sprint it hit. | the same |
| Day page | all generated | Nobody. The push runs `pm day summarize` (section 5); older day records keep their paragraph as history. | the site |

Anything the owner must decide or do is a need (section 3), never text in a record.

**Every `pm` write.**

- It names its target with `--sprint ID` or `--project NAME`. A repo holds many projects, so pm never infers "the only project" or "the open sprint".
- It checks the records it writes and those of the issues it changes, as its commit will leave them. Other sessions' uncommitted files are not checked and not relied on.
- It refuses a record with uncommitted changes, so it never carries an edit in progress. Commit that record with `pm commit -m "…" <path>`, or revert it. Then run the write again.
- It commits what it wrote on the `records` branch. If the commit fails, it puts its files back and says so.
- `pm commit` checks the whole store. With no path, it lists what is uncommitted and commits nothing.
- A Stop hook (`pm hook stop`) blocks your turn once while records that your tool calls named are uncommitted. Commit or revert them before you hand back. Leave a file that another session is writing.

**Blocks.** Use only these fenced blocks in a record.

- `::: decision {source=owner|agent date=YYYY-MM-DD until="…"}`, which `pm decision add` writes.
- `::: result {title="…"}`: a titled table with a reading line under it.
- A `` ```mermaid `` diagram, with a one-line reading under it.

Use raw HTML only for what Markdown cannot show, such as a mock-up.

**Writing style guide.** The site gives every page one shared stylesheet; a record never carries its own CSS.

- Write an engineer's reference: dense, with no marketing and no decoration.
- Put facts in tables: dimensions, parameters, results. Write formulas out.
- Give only numbers that evidence backs, and state their scope.
- A date or a fact that the records do not have is "not recorded". Do not guess it.
- When the repository's own docs are stale, say so on the page. Do not correct them silently.
- Draw real diagrams (Mermaid, or inline SVG), never ASCII art.
- Use one term for each concept, and define it once. Use plain table headers.
- Keep task ids and digests out of the prose. Put them in an evidence table when you need them.

**Design pages.** A design page says what the design is, and which facts, findings and constraints led to it. It also says which alternatives were not chosen, and why.

- When the design changes, edit the page to the new state. Do not add a trail of findings; sprint records keep the findings.
- Decisions and plans stay in the project and sprint records, never on a design page.
- Put no date in the name or the header. Git keeps the history, and the site shows the created and updated dates.
- A page covers one area. When it grows to several areas, make each area a sub page. The main page keeps a short summary per area that links its sub page.
- A sprint's Design pages section links the sub pages that its work changed, not only the main page.

## 3. Decisions and needs

**Level.** Record a decision where it governs, and it stays there. A decision that turns out to apply beyond one sprint moves to the project record as soon as that shows. A choice that is cheap to reverse, and that nobody will ask about, needs no record. The commit message is enough.

**Body and source.** The body comes on stdin: the decision on its first line, its reason on the next. Without the reason, nobody can tell later if it still holds. When the reason is too long, put it in a doc and link the doc. `--level` has no default. The source is `agent` for your own decisions. It is `owner` only when it answers a need (`--need`) or the owner confirmed it (`--confirmed`).

**Owner decisions are closed.** Read the Decisions, project then sprint, before you ask the owner. Do not ask again about a decided question.

**Needs.** When work waits on the owner, raise a need under the sprint or task. Then continue other ready work. Do not stop, and do not decide silently. The owner reads the need days later, so its description stands alone. A need is one of two kinds, and the site shows each under its own heading.

- **Decision** (`pm decision need --title "…" --parent ID`, under "Decisions await you"): the owner chooses. Stdin gives one part per line:
  - one `Question:`;
  - one or more `Fact:`;
  - two or more `Option <label>:`, each with a `Cost:` line under it;
  - one `Default: <label>`, then its reason.
- pm writes the card in one layout. It refuses an option without a cost, a default that names no option, and a sentence over 25 words. Send stdin with a quoted heredoc (`<<'EOF'`), so code spans stay.
- **Action** (`pm action need --title "…" --parent ID`, under "Actions await you"): the owner does a step that only they can do. Examples: run a command, apply a setting. The description on stdin says what to do and why.
- **PR review** (`pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`): an action of its own form. See PR review and sprint close below.
- Both commands refuse a request that names a PR and asks to review, merge or approve it without `--pr`.
- A need that a test or a live check raises starts its title with "[TEST]". Close it with `bd human dismiss <id>` when the check ends.

**Never leave a request only in chat.** Raise a need first for each request that sprint work waits on. Examples: a decision on a sprint's scope or design, a PR review or merge, an action a task waits on. Then the reply asks for it in plain words, with no id.

- A Stop hook (`pm hook owner-request`) checks your final reply. It reads the open needs and actions that this session raised. Claude Haiku says if each request in the reply matches one of them.
- A request that matches none, or matches only another session's need, blocks the turn once.
- Clarifying questions pass. Offers of optional extra work that nothing waits on pass. Reports that only describe a request pass.
- An offer to do a step that the task or sprint needs before it can close is a request. Examples: merge its PR, choose a design. The wording does not change this.
- Work the sprint, push its branch and open its PR without asking. The owner authorizes these steps for every sprint. Asking leave for them blocks the turn, even when a need matches.
- When Beads or the judge fails, the hook says so on stderr and lets the turn end.
- A subagent's report to its parent is not checked.

**Owner replies.** The owner replies on the request's card on the site. The site stores the reply as a Beads comment on the request, which stays open.

- `pm serve` pushes the reply at once into the inbox of the Claude Code session that raised the request. It pushes a reviewed PR's merge to main the same way.
- It arrives as a new turn, or between tool calls when you are busy. Do not poll, and do not start a waiter.
- A push that fails is tried again every minute while `pm serve` runs. Session start points this session's open requests at its current inbox.
- Only the site's replies count, never comments by pm or by an agent. The `picked_up` count on the request marks how many reached the session.
- A reply that could not be pushed waits: the session had ended, or had no inbox, as in Codex. `pm show` flags it. `pm reply read <id>` prints it and marks it delivered.
- The owner can also answer in Beads (`bd human respond`) or in the conversation.

**Act on a reply at once.** Do its next step, then close the need.

- The answer sets a rule that later work must follow: record it with `pm decision add --need <id> --level …`. This cites the need and closes it. Use `--confirmed` instead for an answer that the owner gave in chat.
- The answer is a small input that sets no rule, such as a name or a port. Run `pm decision close <id> --reason "<why>"`. The answer goes on stdin. The answer and the reason stay in Beads, and the need gets the label `no-decision`.
- If you are not sure, record a decision. The record check fails on an answered decision need that no decision cites and that has no `no-decision` label. So a missed rule is caught.
- An action is done: check the evidence, then run `pm action done <id> --reason "<what showed it>"`. It needs no decision.
- Each of these closes refuses while the request holds a reply that has not reached a session. Read it with `pm reply read <id>` first.

**PR review and sprint close.** A sprint closes only when its PR is on main. So "done" means delivered to main. `pm sprint close` does not ask GitHub. The open review blocks the close, and you close the review only once the PR is on main.

1. Write the full delivery report, then commit it with `pm commit`.
   - Outcome: start with `done`, `partial` or `voided`, plus one sentence. A bullet list of what shipped may follow. Only the first paragraph becomes the Beads close reason.
   - A sprint that a finding voided is voided, not failed.
   - Against "Done when": give each item as met or not, with its evidence: a page, a command, a number.
2. Push the branch and open the PR without asking.
3. Raise the review with `pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`.
   - Every sprint named must be open, with its delivery report written and committed.
   - The review sits under the first sprint named, shows in its Actions await you, and blocks its close.
   - Its card links the PR, the sprints and their reports, and the design pages behind them. It shows the focus. So the owner reads the goal, outcome and design before the diff.
4. Close each other open task with `pm task close`, or move it to another sprint with `pm task move`.
5. Once the PR is on main, close the review: `pm action done <review> --reason "merged as <sha>"`. A PR merged into a stacked base is not on main yet.
6. Fast-forward the main checkout: `git -C <main checkout> pull --ff-only origin main`. Hooks, rules and links read it.
7. Run `pm sprint close <id>`.
   - It refuses an unwritten report, any open task or review, and a review closed without `merged as <sha>`. It skips a dismissed review, such as a replaced PR's.
   - It stamps `Merged as <sha> (PR #N).` into the Outcome after the verdict. It commits that on the `records` branch.
   - The epic's close reason names that commit, so the milestone points at the exact state it delivered.
   - A voided sprint with no PR closes with no review and no stamp.

## 4. Breaking the work down

1. List what must become true for the goal to be met. An item that needs more than one sprint is a project. An item that fits in a sprint is a sprint.
2. Split the work along independence, so that projects and sprints run in parallel. Where one feeds another, record the dependency in Beads.
3. Test each item against the goal. If finishing it would not move the work toward the goal, it is harness: checkers, tools, audits. Build harness only when a named sprint is blocked without it.
4. Plan one sprint ahead. Choose the next sprint from the last result.
5. Run the smallest test that can change the decision. If each result leads to the same next step, do not run it.

## 5. The site, the push and the checks

This section holds pm's background machinery: setup, the site, the push and the record check.

**Setup.** Session start runs `pm setup`, then injects `pm where` and `pm show` beside `bd prime`. `pm setup` makes a clone ready, and does each step only if it is missing. It connects Beads with `bd bootstrap` and sets the Beads profile to `team-maintainer`. It installs the git hooks, checks out the store and links `records/` to it. It adds the store to the sandbox roots of Codex and Claude Code, and installs the push. Never run `bd init`, which makes a new database. When session start says setup failed or timed out, run `pm setup` by hand. A fresh clone's first Beads bootstrap needs that too, as does a worktree used without an agent session. `pm where` shows every location and its state.

**The site.** `pm serve` serves the site on localhost, from the records and Beads.

- A page is at most 10 s behind the records and Beads, and states its data's age.
- An open page never reloads itself. Within about 10 s of a change it shows that newer data exists. A reload loads it.
- Nobody renders a page to see a change.
- When `.pm/config.toml` sets `site_url` (a tunnel to `pm serve`), pm's links use it. `pm setup --site-url URL` sets it.
- `pm record link` fails, naming the fixing command, when nothing serves the site or another store answers.
- Nothing on the site is edited by hand. Only record sources are committed.

**The push.** `pm setup` installs one scheduled job per clone: launchd on macOS, a systemd user timer or cron on Linux.

- Every 10 minutes it runs `pm push` from the main checkout. It pushes Beads data with `bd dolt push`.
- It runs `pm day summarize` when today's activity changed.
- It pushes the `records` branch, rebased onto `origin/records` when the remote moved.
- Its log is `.git/pm-push.log`. `pm show`, `pm where` and the site flag a failed or late push.
- Sessions never run `pm push`.

**The record check.** `pm render` checks every record and writes the site into `site/`. Run it to check hand edits without a commit.

- `pm commit`, `pm render` and `pm serve` check the whole store.
- When a record fails the check, the site shows that error in place of its pages. Fix the record, not the site.

## 6. The owner's interface

The owner reads status on the site and answers there. Each decision and action card has a reply box (section 3). The owner also does the actions.

- Do not post status outside the site: no status cadence, no channel posts. Anything that the owner must decide or do is a need.
- To send the owner a page, get its URL with `pm record link`. Open the page and look at it before you send it, in light and dark mode and at phone width. A page that nobody looked at is not done.
- Make one accuracy pass on the page. No edit for looks may change a number.

**Writing to the owner.** Obey these rules in chat replies, needs, actions and the records the owner reads:
- Put the conclusion first.
- Use 4 bullets or fewer.
- Do not use ids, hashes or file names unless the owner asks.
- Give the page link, not a file path.
- Tell what each number measures.
- Use ASD-STE100 approved words. Give each word one meaning.
- Use at most 20 words in an instruction sentence and 25 in a description sentence.
- Use the active voice and simple tenses.
- Write one instruction in each sentence.
