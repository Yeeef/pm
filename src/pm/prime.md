# pm rules

A project lives in two layers. Beads is the work layer: items, holders, status and dependencies. The Markdown records are the record layer: goals, sprint frames, decisions and findings. `pm` is the orchestration layer on top of both. It writes records, and it runs each project action that touches both layers or needs a check. The site that `pm serve` serves shows both layers to the owner. The owner reads the site; agents read `pm show`.

Each rule has a reason. When a rule and its reason disagree in a new case, follow the reason. Run `pm <noun> --help` for its commands and flags.

## 1. The objects

| Object | In Beads | Record | What it is |
|---|---|---|---|
| Project | epic | `records/projects/<name>.md` | A standing goal that takes more than one sprint. It has no deadline. |
| Sprint | child epic of the project | `records/sprints/<project>-<n>.md` | A bet with a goal, a scope and a done-when. It takes hours to days, never weeks. It is the point where work goes back to the owner. |
| Task | task under the sprint, or a sub-task | none | One unit of work with one holder. |
| Need | issue labelled `human`; an action also `action` | none | What waits on the owner: a decision only they can make, or a step only they can do. |
| Decision | none | `::: decision` block in a project or sprint record | A ruling that changes what the team does. |
| Design page | none | `records/design/<name>.md` | The reference for one area of a design, in its final state. |
| Doc | none | `records/docs/<date>-<slug>.md` | A free-form result or explainer, tied to a bead or a project. |
| Postmortem | none | `records/postmortems/<date>-<slug>.md` | A costly incident: timeline, cost, root cause, what changed, what would have caught it earlier. |
| Day page | none | generated; nobody writes it | What moved on one date, across all projects. |

Records point at Beads ids and never copy a status. The site looks up the status when it builds a page. Two copies of one fact drift.

**Which tool.**

| To do this | Use |
|---|---|
| Find ready work | `bd ready --exclude-type=epic` |
| Add a dependency or a sub-task | `bd dep add`, `bd create --parent <task>` |
| Add, claim, close or move a task | `pm task add`, `pm task claim`, `pm task close`, `pm task move` |
| Read project state | `pm show`; one sprint with `pm show --sprint ID` |
| Open or close a project or sprint; add a finding, decision or doc; raise or answer a need | `pm` |
| Edit Goal, Scope, Done when, a delivery report, an Outcome, a design page or a postmortem | By hand in `records/`, then `pm commit -m "…" <path>…` |
| Give the owner a link to a record | `pm record link <target>` |
| Keep a small operational fact (a command, a path, a gotcha) | `bd remember`; decisions go in records, not there |

`pm` wraps an action only when it touches both Beads and a record, or needs a check beyond Beads. Other task work stays plain `bd`. Do not claim with `bd update --claim` or close with `bd close`. `pm task claim` records your session, and `pm task close` names the commit.

**Where the state lives.** There is no hand-kept status file.

| Question | Answered by |
|---|---|
| What is open across all projects? | The site's root page, or `pm show` |
| Where does a project or sprint stand? | Its generated Progress on its page, or `pm show --sprint ID` |
| What waits on the owner? | Open `human` issues: Decisions await you and Actions await you, in `pm show` and on the site |
| What constrains the work? | The project decisions, then the sprint decisions |
| What do I pick up next? | `bd ready --exclude-type=epic` |

**Session start.** At each session start, pm runs `pm setup`, then injects `pm where` and `pm show`. They come beside `bd prime`. The `pm show` part carries its UTC time. Orient from it; do not run `pm show` again for that. Other sessions change the state while you work. So run `pm show` again before you tell the owner the project state. Then read the record of the sprint you work on. To read one part of a record, run `pm show --record <path> --section <name>`. Do not read the whole file for one part. A subagent gets the Beads profile line and these rules, but no `pm show`.

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
- `pm` commits but never pushes. Sessions push neither Beads data nor the `records` branch (section 9).
- Many sessions write in one store. Commit only the paths that you edited.

**Format.** A record is Markdown with a small header and fixed `##` sections. The header holds ids only, never a status. Each section opens with a `>` prompt line that says what goes there. A record has all its sections from the start. An empty section holds "None yet.". A close-time section holds "Not closed yet.". A generated section holds only its prompt line.

| Record | Header | Sections |
|---|---|---|
| Project | `type: project`, `title`, `bead` | Goal; Progress (generated); Decisions; Design pages; Outcome (at close) |
| Sprint | `type: sprint`, `title`, `bead` | Goal; Scope; Done when; Design pages; Progress (generated); Decisions; Findings; Delivery report, with `### Outcome` and `### Against "Done when"` |
| Design page | `type: design`, `title`, `project` | Problem; Goals and non-goals; Constraints and key facts; Design; Alternatives considered; Prior art (optional); Open questions |
| Doc | `type: doc`, `title`, `date`, `bead` or `project` | Free |
| Postmortem | `type: postmortem`, `title`, `date`, `sprint` or `project` | Summary; Timeline; Cost; Root cause; What changed; What would have caught it earlier |

**Who writes what.**

- By hand, then `pm commit`: Goal, Scope, Done when, Design pages, the delivery report and a project's Outcome.
- By hand too: design pages, docs and postmortems.
- With `pm` commands: Decisions and Findings.
- Never: the generated sections. Progress keeps only its prompt line, and the site fills it from Beads.
- The site adds Decisions await you and Actions await you to a sprint page. It adds Docs and Postmortems to project and sprint pages. Never write a heading with one of these names.
- The record check (section 9) fails on hand-written text in a generated section.

**Blocks.** Use only these fenced blocks in a record.

- `::: decision {source=owner|agent date=YYYY-MM-DD until="…"}`. Give `until` only for a known condition to revisit.
- `::: result {title="…"}`: a titled table with a reading line under it.
- A `` ```mermaid `` diagram, with a one-line reading under it.

Use raw HTML only for what Markdown cannot show, such as a mock-up.

**What goes where.**

- Results and findings go in the sprint's Findings, with their numbers. A large result table is a `::: result` block.
- An explanation or a reference (an architecture, a data format, "explain step 1") is a design page or a doc.
- Anything the owner must decide or do is a need (section 3), not text in a record.
- A date or a fact that the records do not have is "not recorded". Do not guess it.
- When the repository's own docs are stale, say so on the page. Do not correct them silently.

**Writing a page.** The site gives every page one shared stylesheet; a record never carries its own CSS.

- Write an engineer's reference: dense, with no marketing and no decoration.
- Put facts in tables: dimensions, parameters, results. Write formulas out.
- Give only numbers that evidence backs, and state their scope.
- Draw real diagrams (Mermaid, or inline SVG), never ASCII art.
- Use one term for each concept, and define it once. Use plain table headers.
- Keep task ids and digests out of the prose. Put them in an evidence table when you need them.

**Design pages.** A design page holds the final state of one area of a design. It says what the design is, and the key facts, findings and constraints that led to it. It also says which alternatives were not chosen, and why.

- Start one with `pm design new <slug> --title "…" --project NAME`. It writes every section with its prompt line.
- The site requires every section except Prior art. Constraints and key facts, Alternatives considered and Open questions may hold "None yet.".
- Design has free `###` subsections. The page shows a table of contents of them.
- When the design changes, edit the page to the new state. Do not add a trail of findings; sprint records keep the findings.
- Put no date in the name or the header. Git keeps the history, and the site shows the created and updated dates.
- Decisions and plans stay in the project and sprint records, never on a design page.
- A page covers one area. When it grows to several areas, make each area a sub page. The main page keeps a short summary per area that links its sub page.
- A sprint's Design pages section links the sub pages that its work changed, not only the main page.

**Docs.** `pm doc new <slug> --title "…" --bead ID|--project NAME` creates `records/docs/<today>-<slug>.md`, with the body from stdin. A doc holds a free-form result or explainer.

**Postmortems.** An incident gets a postmortem when it cost more than a day. It also gets one when it broke other sessions or the owner's view. Write it once the incident is fixed. `pm postmortem new <slug> --title "…" --sprint ID|--project NAME` creates it with every section. Link it to the sprint that it hit.

**Day pages.** The site makes a day page for every date with activity, across all projects. Nobody writes a day record. Its Today summary comes from `pm day summarize`, which runs with the push (section 9). Older day records keep their hand-written paragraph as history.

## 3. Decisions and needs

**Level.** Record a decision where it governs. It stays there.

- Project: a later sprint must follow it, such as an architecture, a format, a convention or a scope boundary. Record it in the project record when you make it. Move it there as soon as it turns out to apply beyond one sprint.
- Sprint: it ends with the sprint, such as an approach, a scope cut or what to try first. It stays in the sprint record.
- Neither: it is cheap to reverse and nobody will ask. The commit message is enough.

**Body and source.** The body states the decision and its reason. Without the reason, nobody can tell later if it still holds. When the reason is too long, put it in a doc and link the doc. The source is `agent` for your own decisions. It is `owner` only when it answers a need (`--need`) or the owner confirmed it (`--confirmed`).

Record a decision with `pm decision add --level project|sprint --project NAME|--sprint ID`. The body comes on stdin: the decision on its first line, its reason on the next. `--level` has no default.

**Owner decisions are closed.** Read the Decisions before you ask the owner. Do not ask again about a decided question.

**Needs.** When work waits on the owner, raise a need under the sprint or task. Then continue other ready work. Do not stop, and do not decide silently. The owner reads the need days later, so its description stands alone. A need is one of two kinds, and the site shows each under its own heading.

- **Decision** (`pm decision need --title "…" --parent ID`, under "Decisions await you"): the owner chooses. Stdin gives one part per line:
  - one `Question:`;
  - one or more `Fact:`;
  - two or more `Option <label>:`, each with a `Cost:` line under it;
  - one `Default: <label>`, then its reason.
- pm writes the card in one layout. It refuses an option without a cost, a default that names no option, and a sentence over 25 words. Send stdin with a quoted heredoc (`<<'EOF'`), so code spans stay.
- **Action** (`pm action need --title "…" --parent ID`, under "Actions await you"): the owner does a step that only they can do. Examples: run a command, apply a setting. The description says what to do and why.
- **PR review** (`pm action need --pr URL --sprint ID --focus "…" [--design SLUG]`): an action of its own form. See section 6.
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

## 4. The `pm` CLI

These rules hold for every `pm` write.

- It names its target with `--sprint ID` or `--project NAME`. A repo holds many projects, so pm never infers "the only project" or "the open sprint".
- It checks what it touches: the records that it writes, and the records of the issues that it changes. It checks them on the `records` branch as its commit will leave them.
- Other sessions' uncommitted files are not checked and not relied on.
- It refuses a record with uncommitted changes, so it never carries an edit in progress. Commit that record with `pm commit -m "…" <path>`, or revert it. Then run the write again.
- It commits what it wrote on the `records` branch. If the commit fails, it puts its files back and says so.
- `pm commit` checks the whole store as its commit will leave it.

A Stop hook (`pm hook stop`) blocks your turn once while records that your tool calls named are uncommitted. Commit them with `pm commit -m "…" <path>`, or revert them, before you hand back. Leave a file that another session is writing.

| Command | Does |
|---|---|
| `pm show [--json]` | Projects, open sprints, tasks and their holders, needs, today and recent decisions, in a few hundred tokens. |
| `pm show --sprint ID` | One sprint's frame, findings and tasks. |
| `pm show --record <path> --section <name>` | One section of a record. The record is a path, a sprint id, a project name or a design slug. An unknown section is refused with the record's section names. |
| `pm record link <target>` | The URL of a record's page on the site. Use only this URL for a record; never a `records/…` path or a URL you built. When it fails, run the command that it names. |
| `pm setup [--site-url URL]` | Makes a clone ready; see below. |
| `pm where [records]` | Every location and its state: the store, this checkout, Beads, the hooks, the Codex roots, the push and the site. With `records`, only the store's path. |
| `pm commit -m "…" <path>…` | Checks and commits only the named hand edits. With no path, it lists what is uncommitted and commits nothing. |
| `pm render`, `pm serve` | See section 9. |
| `pm finding add "<text>" --sprint ID` | Adds a bullet to the sprint's Findings. |
| `pm decision add`, `need`, `close` | See section 3. |
| `pm action need`, `done` | See sections 3 and 6. |
| `pm reply read [ID…]` | Prints the replies and merges that did not reach a session, and marks them delivered. It does not wait. |
| `pm doc new`, `pm design new`, `pm postmortem new` | See section 2. |
| `pm project open`, `close` | See section 8. |
| `pm sprint open`, `close` | See section 6. |
| `pm task add --sprint ID --title "…"` | Creates a task in an open sprint. The description comes on stdin. |
| `pm task claim <id>` | Claims a task and records your session. It refuses a task that another live session holds. |
| `pm task close <id> [--reason "…"] [--commit REF]` | Closes a task. The reason names the commit: HEAD if it is newer than the task, or `--commit`. It refuses epics and needs. |
| `pm task move <id> --to SPRINT_ID` | Moves a task to another open sprint. It writes the scope change as a decision in the sprint it leaves. The reason comes on stdin, on at least two lines. |
| `pm day summarize` | Writes today's Today summary; the push runs it (section 9). |
| `pm feedback add --project NAME [--sprint ID] [--task ID]` | Adds an entry to the project's pm feedback doc. |

**`pm setup`.** It makes a clone ready in one command, and does each step only if it is missing:

- It connects Beads with `bd bootstrap` from the remote's `refs/dolt/data`. Never run `bd init`, which makes a new database.
- It sets the Beads agent profile to `team-maintainer`, so agents may commit.
- It installs the git hooks, checks out the store, and links this worktree's `records/` to it.
- It adds the store and the git dirs to the Codex sandbox's writable roots, when Codex is used.
- It adds the store to Claude Code's `permissions.additionalDirectories`, so writes through `records/` need no prompt.
- It installs the scheduled push (section 9).
- `--site-url URL` writes the site's public base URL to `.pm/config.toml`. pm's printed links then use it.

Session start runs `pm setup` each time, so a worktree that an agent works in gets set up. When it fails or times out, session start says so. Then run `pm setup` by hand. A fresh clone's first Beads bootstrap needs `pm setup` by hand. A worktree used without an agent session needs it too.

**pm feedback.** When pm gets in your way, run `pm feedback add` once. Examples: a confusing refusal, a missing command, a rule that cost time. Say what happened and what would have helped. Do not use it for routine use.

## 5. Breaking the work down

1. List what must become true for the goal to be met. An item that needs more than one sprint is a project. An item that fits in a sprint is a sprint.
2. Split the work along independence, so that projects and sprints run in parallel. Where one feeds another, record the dependency in Beads.
3. Test each item against the goal. If finishing it would not move the work toward the goal, it is harness: checkers, tools, audits. Build harness only when a named sprint is blocked without it.
4. Plan one sprint ahead. Choose the next sprint from the last result.
5. Run the smallest test that can change the decision. If each result leads to the same next step, do not run it.

Every change has a task in a sprint. Work outside any sprint becomes a small new sprint, so that it shows in Beads.

## 6. Sprint lifecycle

**Open** with `pm sprint open <project> --title "…"`. The frame comes on stdin as `## Goal`, `## Scope` and `## Done when`. Write it before anything runs.

- Goal: what is true when the sprint ends, and why now.
- Scope: an **In:** list and an **Out:** list, at a high level. Put the detail in a design page.
- Done when: checkable evidence that the goal is met, or the finding that voids it. For a one-shot or costly run, write the expected result before the run. You cannot choose it again after the numbers show.

**Run.**

- Create tasks under the sprint with `pm task add`.
- Before you start or delegate a task, read its holders and open needs in `pm show`. Do not take or brief work that another live session holds.
- Claim a task with `pm task claim` before you start it. Tell a subagent that it holds a task only after the claim succeeds.
- A subagent shares its session's id. It claims and closes its own task.
- Examine a subagent's report against the files before you accept it.
- Add findings to the sprint's Findings as they occur, with `pm finding add`. Give each result its numbers.
- A scope change is a sprint decision. Do not rename a task or rewrite its description for it.
- Sprint decisions go in the sprint's Decisions. A decision that later sprints must follow goes in the project record.

**Close.** A sprint closes only when its PR is on main, so "done" means delivered to main. `pm sprint close` does not ask GitHub. The open review blocks the close, and you close the review only once the PR is on main.

1. Write the full delivery report, then commit it with `pm commit`.
   - Outcome: start with `done`, `partial` or `voided`, plus one sentence. A bullet list of what shipped may follow. Only the first paragraph becomes the Beads close reason.
   - A sprint that a finding voided is voided, not failed.
   - Against "Done when": give each item as met or not, with its evidence: a page, a command, a number.
2. Run the review loop on the whole sprint (section 7). Push the branch and open the PR without asking.
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

## 7. The implementation cycle

Scale the cycle to the cost of being wrong. Restartable work gets one review pass. One-shot, costly or irreversible work gets adversarial review and recorded evidence.

- Make small commits. Git history is documentation. Do not leave everything untracked and commit one giant change.
- Smoke tests between commits check wiring only. They are not completion evidence.
- A work trunk is a part of a sprint that you can test and review alone, in one pass.
- Close a work trunk with these steps:
  - the project's real tests, and the meaningful run that answers the sprint's question;
  - cleanup of dead code and results;
  - a review by a fresh-context agent, which sees the change, the intent and the evidence, but not your reasoning;
  - repeat the review until it finds no correctness problem;
  - then close the trunk's tasks, each naming the commit.

Commit prefixes: `[SPEC]` specifications, `[FEATURE]` new feature, `[FIX]` bug fix, `[PERF]` performance, `[SPRINT]` sprint boundary.

## 8. Project lifecycle

**Open** with `pm project open <name> --title "…"`, with a one-paragraph Goal on stdin. The owner confirms the goal in their own words before you open the project. Then open the first sprint with `pm sprint open`.

**Pause** when the next sprint depends on something outside the team: an owner decision, hardware, data. Raise the need, and record what would resume the project. Then move to work that does not depend on it.

**Close.**

1. Close every sprint first.
2. Write the project record's `## Outcome` by hand. Give the results against the goal in numbers, what was learned, and what was retired. Link the sprints' delivery reports.
3. Commit the record with `pm commit -m "…" <path>`.
4. Run `pm project close <name>`. It ties the close to that commit.

Nothing is deleted. Closed projects stay on the site.

## 9. The site, the push and the checks

This section holds pm's background machinery: the site, the push and the record check.

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

## 10. The owner's interface

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
