---
type: project
title: Project management harness
bead: yeeef-agents-9va
---

## Goal

> Why do we do it? What is it? What outcome do we expect?

Agents and the owner share one project record. Beads holds the work (items,
owners, status, dependencies); Markdown records hold the context (goals,
sprint frames, decisions, daily log); a CLI writes both and checks them; one
renderer turns them into an HTML site for the owner and compact text for
agents.

Success: an agent resumes a project from `pm show` alone, and the owner reads
status on the site without asking.

## Progress

> Where are we now, and what's next? Generated from Beads and the sprint
> records when the page is rendered. Do not write here.

## Decisions

> What constrains every future sprint? Sprint-only choices live in the sprint
> record.

::: decision {source=owner date=2026-10-02}
Decisions are recorded at the level they govern. A project decision
constrains every future sprint (architecture, formats, conventions, scope
boundaries). A sprint decision is a choice about that sprint's own work
(approach, scope cut, what to try first). Both stay in their record
permanently. A decision later sprints must follow is a project decision,
recorded there when it is made or as soon as it turns out to apply more
widely.
Cheap, reversible choices are not recorded; their commit message is enough.

A decision block has `source` and `date`, and `until` only when there is a
known condition to revisit it. Its body states the decision and its reason,
because without the reason nobody can tell later whether it still holds.
:::

::: decision {source=owner date=2026-09-30 until="Beads fails the sprint 1 mapping check"}
Beads is the work layer. We build only the record layer, the CLI and the
views on top of it, because Beads already handles items, claims and
dependencies well and none of the existing tools covers the context layer.
:::

::: decision {source=owner date=2026-09-30}
Records are Markdown with a small header and fenced-div blocks for
structured parts. Free Markdown, tables and diagrams are allowed inside
blocks. Markdown stays readable raw and cheap for agents; the blocks give
tools something to check.
:::

::: decision {source=owner date=2026-09-30}
Only record sources are committed; rendered HTML is regenerated, so the
source is the one thing reviewed and the two cannot drift.
:::

::: decision {source=owner date=2026-10-01}
Beads data syncs to the git remote under `refs/dolt/data`. The JSONL export
is not committed, because it would only add churn to git history.
:::

::: decision {source=owner date=2026-10-02}
Design pages live in `records/design/` as Markdown records (`type: design`)
beside the project they describe, so `make docs` renders them with every
other record. Raw HTML is allowed inside a page only for parts Markdown
cannot express, such as a mock-up.
:::

::: decision {source=owner date=2026-10-02}
Every page links one shared stylesheet,
`skills/project-management/harness/style.css`, and carries no CSS of its
own, because per-page CSS is how poker-ai ended up with 36 different looks.
:::

::: decision {source=owner date=2026-10-03}
Mermaid diagrams render in the browser from a CDN script, because it needs
no Node toolchain to view the site locally. Answered in
`yeeef-agents-9va.1.6`; build-time rendering was not chosen.
:::

::: decision {source=owner date=2026-10-03}
The site is rendered by hand with `make docs`, not by the CLI after each
write or by a file watcher, because rendering when someone wants to look is
enough and adds no process to run. Answered in `yeeef-agents-9va.1.6`.
:::

::: decision {source=owner date=2026-10-03}
The site keeps its current look (the shared stylesheet as it is), with no
redesign planned. Answered in `yeeef-agents-9va.1.6`.
:::

::: decision {source=owner date=2026-10-03}
A repo holds many projects, so every `pm` write names its target: `--sprint`
for sprint-level writes (the project comes from Beads) and `--project` for
project-level ones. Nothing is inferred from "the only project" or "the open
sprint", because poker-ai will be cut into many projects.
:::

::: decision {source=owner date=2026-10-03}
Day records are repo-wide: one per day, covering every project, with Needs
you and Sprints generated for all projects, because the owner reads one
daily page, not one per project.
:::

::: decision {source=owner date=2026-10-03}
Closing a sprint or a project is tied to a commit: the close happens after
the delivery report or Outcome is committed, and the Beads epic's close
reason names that commit, so every milestone points at the exact state it
delivered. Closing an ordinary task only names its commit when there is one.
:::

::: decision {source=owner date=2026-10-03}
The pm CLI v1 design (design page section 5) is approved with these
choices: pm wraps only actions that touch both Beads and records or need a
check, and single-step task work stays plain bd (task add, close and move
wait for v2); pm only creates files and inserts entries, while Goal, Scope
and the delivery report are edited by hand and checked by the renderer;
pm decision add requires --level with no default; pm sprint close does not
check decisions, because sprint decisions stay in the sprint record; the
code is a small harness package shared by pm.py and render.py, run through
a bin/pm wrapper; pm never renders, make docs stays by hand. Answers
`yeeef-agents-9va.6.4`.
:::

::: decision {source=owner date=2026-10-03}
The project-management skill keeps the owner-communication rules
(plain-language, bullet-first messages, answer-by times, owner decisions are
not re-asked) and drops poker-ai's operating style: the hourly status
cadence, chat channels and threads, and owner notifications. Status lives in
Beads and the rendered site.
:::

::: decision {source=owner date=2026-10-03}
When an agent needs the owner's judgement and the owner is away, it raises a
need (an issue labelled `human` with options and a default) and continues
with other ready work, rather than stopping or deciding silently.
:::

::: decision {source=agent date=2026-10-03}
Decisions stay `::: decision` blocks in project and sprint records, with
their `date` attribute; Beads' `decision` issue type is not used. Its
issues have no level, source or until fields, open ones appear in `bd ready`
as work, and git blame dates differ from the decision date for 4 of 16
project decisions, so neither can carry what the blocks carry. See
[the evaluation](../docs/2026-10-03-beads-decision-type.md)
(`yeeef-agents-9va.6.1`).
:::

::: decision {source=owner date=2026-10-03}
Decisions stay as `::: decision` blocks in records, not Beads decision issues, as the agent recommended in [the evaluation](../docs/2026-10-03-beads-decision-type.md).

A decision states its reasoning in its own text when that fits; when the reasoning is too long, it goes in a separate doc that the decision links. Which to use is up to the agent.
Answers `yeeef-agents-9va.6.7`.
:::

::: decision {source=owner date=2026-10-03}
Design pages follow the template: Problem, Goals and non-goals, Constraints and key facts, Design (free subsections), Alternatives considered, Prior art (optional), Open questions; the renderer checks only that the required headings exist.

The owner approved it as proposed, because it keeps design pages comparable while leaving their content free-form.
Answers `yeeef-agents-9va.5.5`.
:::

::: decision {source=owner date=2026-10-03}
A sprint record has a hand-written Design pages section after Done when, like the project record; a page may be listed in both. Day pages link each sprint to its record.
The design page behind a sprint is the detail its Scope points to, so the sprint should lead to it directly.
:::

::: decision {source=owner date=2026-10-03}
Design pages live with the other records in the shared records store, not on code branches.
One store keeps every page visible from every worktree; if pages drift from the code they describe, revisit.
Answers `yeeef-agents-9va.9.7`.
:::

::: decision {source=owner date=2026-10-03}
Records live in one store per clone: a `records` branch checked out at `.records` in the main checkout, read through a `records` symlink in each worktree, written by `pm` as commits under a lock, and copied onto main by CI, as described on the Shared records store design page.
Records on code branches diverge per branch while Beads is per clone, so findings and decisions went missing across worktrees.
:::

::: decision {source=owner date=2026-10-04}
Main's records/ copy keeps per-file history through git blame only; the full per-file commit list is read on the records branch (git log records -- <file>).
Rebuilding the branch with a records/ prefix or dropping main's copy costs more than browsing history on the records branch.
Answers `yeeef-agents-9va.9.8`.
:::

::: decision {source=owner date=2026-10-04}
The local site stays current without anyone running make render by hand; this replaces the 2026-10-03 decision that the site is rendered by hand with make docs.
Rendering by hand left the owner reading stale pages (sprint 9 showed open after it closed), and friction in reading status defeats the site.
:::

::: decision {source=owner date=2026-10-04}
Main's records/ copy refreshes only on pushes to main (or a manual run of the copy Action); no scheduled run.
The owner declined a daily run: a copy that lags between pushes to main is acceptable, since the live records are on the records branch.
:::

::: decision {source=owner date=2026-10-04}
A design page covers one area. When a page grows to several areas, each becomes a sub design page and the main page keeps a short summary per area linking to it; a sprint's Design pages section links the specific sub pages its work changed, not the whole main page.
Linking a 600-line page leaves the reader to hunt for the relevant section; a focused page is what a reviewer or an agent actually needs to read.
:::

::: decision {source=owner date=2026-10-04}
A sprint closes only when its PR is merged to main. Its review is part of the sprint: the review action sits under the sprint and blocks its close. Before the PR is reviewed, the agent writes the delivery report as a draft, so the owner reads the outcome on the sprint page while reviewing the PR; after the merge the agent finalises the report and closes the sprint, tied to the merged state.
Done should mean delivered to main; a report written before review can describe a state main never got, and a draft report gives the reviewer the outcome next to the diff.
Answers `yeeef-agents-9va.22.5`.
:::

::: decision {source=owner date=2026-10-04}
Sprint 4 (work trunks and the SDLC) is dropped from pm-harness and becomes its own project, agent-sdlc (epic yeeef-agents-9ac), opened but not started; its candidates move to that project's Goal, and the close-on-merge work done under sprint 4 (yeeef-agents-9va.4.2) moves to a new pm-harness sprint.
The implementation cycle is worth a project of its own rather than one sprint of pm-harness, and another agent will run it.
Answers `yeeef-agents-9va.4.1`.
:::

::: decision {source=owner date=2026-10-04}
`pm sprint close` does not check PRs on GitHub: it requires a committed, non-draft delivery report and no open tasks, and the PR review action under the sprint is one of those tasks, closed by the agent only once the PR is on main.
The open review already blocks the close; a GitHub check added a gh dependency, edge cases and a fake gh for a mistake the agent made, not a missing check.
Answers `yeeef-agents-9va.23.2`.
:::

::: decision {source=owner date=2026-10-05}
Build the hooks the survey recommends: a SessionStart hook that runs `pm show` beside `bd prime` (Claude Code and Codex), a Stop hook that blocks once while `.records` has uncommitted changes, and a `make render` check that fails on hand-written text in generated sections; skip the PreToolUse guard on records/, PreCompact, the unclosed-task Stop check and the UserPromptSubmit needs hook.
The owner read the survey and accepted its recommendation (option 1 of the need), whose costs were measured: +0.81 s and about 610 tokens per session start, under 0.01 s per stop.
Answers `yeeef-agents-9va.14.2`.
:::

::: decision {source=owner date=2026-10-05}
Each pm command reads and checks only what it touches: reads load only the project or sprint shown, writes validate only the records they write plus the issues those reference; whole-store invariants (an uncited answered need, a draft report on a closed sprint) are checked by make render, the records pre-commit hook and the site, not by every command.
The full bd read and whole-store render on every command grow with the store and fill agents' context while serving no command's own need; commit and render still guard the whole store before anything lands.
Answers `yeeef-agents-9va.25.2`.
:::

::: decision {source=owner date=2026-10-05}
Until bd offers a single-call subtree query, each pm command keeps one full bd read and scopes only its validation to the records it touches; reads switch to a subtree query once one exists.
With today's bd every call costs about 0.48 s and a subtree needs one call per level, so exact scoped reads would make each command 0.4–1.5 s slower, while the full read stays in pm's process and never reaches an agent's context.
Answers `yeeef-agents-9va.25.3`.
:::

::: decision {source=owner date=2026-10-05}
Every pm write and pm commit pushes the records branch right after committing it, so a commit and its push are one step; nobody pushes records by hand.
The records-store design never decided who pushes; agents forgot or handed it to the owner, and the copy to main only sees origin/records. The owner chose bundling (option 4).
Answers `yeeef-agents-9va.30.2`.
:::

::: decision {source=owner date=2026-10-05}
This repo uses the Beads agent profile team-maintainer (agent.profile in .beads/config.yaml): agents commit, push and sync Beads as routine work; an explicit no-commit/no-push instruction still wins.
The default conservative profile forbade every git commit and push repo-wide, overriding the harness's own records workflow; the owner chose to switch.
:::

::: decision {source=owner date=2026-10-05}
Beads data is pushed by one scheduled job per machine (launchd agent on macOS, systemd user timer or cron on Linux, about every 10 minutes, under a lock and a timeout), installed by pm setup and shown by pm where; sessions never push Beads themselves, and a failed or overdue push shows on the site and in pm show.
Dolt pushes to a git remote must not run concurrently; one pusher per machine rules that out locally, and unlike a SessionEnd hook it also covers killed and long-running sessions.
Answers `yeeef-agents-9va.30.7`.
:::

::: decision {source=owner date=2026-10-05}
pm stays local: it commits records but never pushes. The per-machine scheduled job pushes both Beads data and the records branch (fetch, rebase onto origin/records, push when ahead); a failed or overdue push of either shows on the site and in pm show. This replaces the decision on need yeeef-agents-9va.30.2 (bundle commit and push in pm).
The owner chose to keep pm light and local; one job per machine covers both stores, at the cost of up to one job interval before a record reaches origin and main's copy.
Answers `yeeef-agents-9va.30.8.1`.
:::

::: decision {source=agent date=2026-10-05}
Site read path: serve the last good page at once and refresh it in the background, with the page stating how old its data is and refreshing itself when newer data is ready (B); store a reply's Beads write in the background, showing saving and then the result (D); a page may be up to 10 s behind and says so, replacing sprint 11's "a reload always shows the current state" (E). Committed-snapshot reads (A) or a background Beads thread (C) are added only if the stated ages run too high.
Measured on 2026-10-05: renders take 3-7 ms but a page waited up to 6.5 s and a reply up to 9.5 s behind the store lock and bd; B+D+E takes every wait off the request path with the least new machinery.
:::

::: decision {source=owner date=2026-10-05}
Clones that run the scheduled push job live outside the macOS-protected folders (~/Desktop, ~/Documents, ~/Downloads); this Mac's clone moves out of ~/Desktop, in its own sprint.
Background launchd jobs cannot read those folders and cannot get the access prompt; moving the clone needs no privacy grant at all.
Answers `yeeef-agents-9va.30.10`.
:::

::: decision {source=owner date=2026-10-05}
A SubagentStart hook, in both .claude/settings.json and .codex/hooks.json, injects one line naming the active Beads profile into every subagent; nothing else is injected. The team-maintainer close protocol from bd prime and hand-offs to the owner stay as they are (no RULES.md precedence line, no close-protocol override).
Subagents already load CLAUDE.md with the harness rules but not SessionStart output, so the profile is the one missing piece; held work is enforced by pm task claim. The owner judged the other two conflicts fine as they are.
Answers `yeeef-agents-9va.30.9`.
:::

::: decision {source=owner date=2026-10-05}
Record pages reach the owner on other devices through the owner's Cloudflare tunnel to pm serve at https://pm.yeeefs.com/ (option C, private hosting by tunnel); links given to the owner use that base, not localhost.
The owner follows sessions through Remote Control from other devices, where localhost links do not open, and has already exposed the site at that address.
Answers `yeeef-agents-9va.15.3`.
:::

::: decision {source=owner date=2026-10-06}
The site's public URL is configured with a pm command, `pm setup --site-url URL`, not by hand-editing config; pm stores it for the clone and uses it for printed links and for accepting site replies from that host.
The owner wants the one machine-specific fact set through pm's own setup command rather than a raw git config or shell variable.
Answers `yeeef-agents-9va.15.5.2`.
:::

::: decision {source=owner date=2026-10-06}
Site pages never reload themselves: pm serve keeps refreshing its data, and an open page only shows that newer data is available (with a reload link); the owner loads it on demand.
Automatic reloads flash the page every few seconds and move the reader; the owner wants to choose when the page changes.
:::

::: decision {source=owner date=2026-10-06}
pm's first installable version sets up a brand-new repo from nothing (creates the records branch, runs bd init), supports GitHub remotes only, and keeps the site and scheduled push as they are apart from moving their settings into pm's config; the public site URL is optional, with localhost the default and a site URL opted into.
The owner chose option (a) and added that the site URL is opt-in.
Answers `yeeef-agents-9va.38.8`.
:::

::: decision {source=owner date=2026-10-06}
pm ships as a Python package installed with uv (one line: uvx --from git+… pm init); the code lives in the installed package and no repo holds a copy.
The owner chose option (a), a Python package.
Answers `yeeef-agents-9va.38.5`.
:::

::: decision {source=owner date=2026-10-06}
pm's context sits beside bd's, not over it: pm's section and hooks are added alongside bd's block and bd prime, and pm does not rewrite or pin bd's context; pm builds on bd as the work layer, and changing what bd says is left to sprint 36.
The owner chose option (b) as simpler, judging that pm's context does not conflict with bd's and that altering the work layer by hand is premature until sprint 36 decides fork or replace.
Answers `yeeef-agents-9va.38.7`.
:::

::: decision {source=owner date=2026-10-06}
pm's agent context is hook-only: pm writes nothing into AGENTS.md or CLAUDE.md; all of it (rules and project state) comes from pm prime at SessionStart (including after compaction and clear) and SubagentStart, and the hook fails loudly when pm is missing.
The owner chose option (d); pm targets only hosts with hooks (Claude Code, Codex), the hook carries live state and stays current with the installed pm, and the .pm/ directory (9va.38.10) is the visible sign pm is in use.
Answers `yeeef-agents-9va.38.6`.
:::

::: decision {source=owner date=2026-10-06}
Each repo pins its pm version in .pm/config.toml; pm commands and hooks fail hard when the installed pm differs, and pm upgrade moves the pin and rewrites pm's managed files.
The owner chose option (a), so every session on a repo follows the same rules and an upgrade is a deliberate step.
Answers `yeeef-agents-9va.38.9`.
:::

::: decision {source=owner date=2026-10-06}
pm keeps content and machinery apart: records stay at records/ (the copy on main and each worktree's link, as today); everything else pm owns lives in .pm/ (tracked config.toml, README.md, hooks/; git-ignored runtime state; the store checkout moves from <main>/.records to <main>/.pm/store).
The owner chose option (d): records stay visible at the repo root and on GitHub and no links change, while pm's machinery has one home.
Answers `yeeef-agents-9va.38.10`.
:::

::: decision {source=owner date=2026-10-06}
A sprint's delivery report has no draft state: the agent writes it in full before raising the PR review (`pm action need --pr` requires the Outcome and "Against Done when"), the sprint page shows that sprint's own open decisions and actions (the review card included) in generated sections instead of a draft banner, and `pm sprint close` requires every task closed with the review closed as merged, appends "Merged as <sha>." to the Outcome, commits the record and closes the epic. This replaces the draft-and-finalise part of the decision answering `yeeef-agents-9va.22.5` and the "non-draft" clause of the one answering `yeeef-agents-9va.23.2`; a sprint still closes only once its PR is on main.
The open review action already says the report awaits a merge, so the draft paragraph only duplicated it and cost two hand steps per sprint (delete the line, commit) that pm can do; the agent still edits and commits the report by hand whenever review changes the work.
:::

::: decision {source=owner date=2026-10-06}
The pm package stays in yeeef-agents, in a subdirectory, and installs with uvx --from "git+https://github.com/Yeeef/yeeef-agents@<tag>#subdirectory=<dir>"; the command stays `pm`, and a distribution name is chosen only if publishing to PyPI is wanted.
The owner chose option (b), keeping pm in this repo rather than splitting it into a new one.
Answers `yeeef-agents-9va.38.11`.
:::

::: decision {source=owner date=2026-10-06}
pm's git hook code is a marked section (# --- BEGIN PM v<X> --- … # --- END PM ---) holding one line, `pm hook git-<name>`, in Beads' hook files in .beads/hooks; core.hooksPath stays .beads/hooks, and .pm/ has no hooks/ directory. This replaces the hooks/ part of the answer to 9va.38.10.
The owner chose option (a): Beads preserves content outside its markers, git runs one hook file per event, so sharing Beads' file is simpler than taking over core.hooksPath.
Answers `yeeef-agents-9va.38.13`.
:::

::: decision {source=owner date=2026-10-06}
Owner replies and PR merges reach a Claude Code session by push: `pm serve` writes them into the inbox socket the raising command recorded (`CLAUDE_CODE_MESSAGING_SOCKET`, stored as `metadata.inbox`), marks them delivered only once the socket accepts them, and watches open reviews' merges itself; undelivered replies are read with `pm reply read` and flagged by `pm show` and session start. The reply-wait hooks and the per-session poller are not shipped; Codex is out of scope (design page reply-delivery).
Delivery is triggered by the event, structurally, instead of depending on Claude Code's hook lifecycle or the agent remembering to wait; a push failure is visible on the card and in `pm show`. Answers `yeeef-agents-9va.44.2`.
:::

::: decision {source=owner date=2026-10-06}
Day pages are a fully rendered view with no hand-written day record: their Today summary is generated periodically by an agent from the day's activity and kept current through the day, not written once in the morning.
A one-off morning paragraph goes stale and mostly restates the sprints; a periodic summary reflects what actually happened.
:::

::: decision {source=owner date=2026-10-06}
Agents write everything addressed to the owner (chat replies, needs, actions, records the owner reads) in ASD-STE100 Simplified Technical English: approved words with one meaning each, short sentences (at most 20 words for an instruction, 25 for a description), active voice, simple tenses, one instruction per sentence; technical names (commands, files, ids) are allowed as technical names. pm prime carries this rule with the other owner-writing essentials.
The owner asked for it on 2026-10-06 to make agent messages short and unambiguous.
:::

::: decision {source=owner date=2026-10-07}
pm prime prints one merged list of owner-writing rules: put the conclusion first; use 4 bullets or fewer; do not use ids, hashes or file names unless the owner asks; give the page link, not a file path; tell what each number measures; use ASD-STE100 approved words, each with one meaning; at most 20 words per instruction sentence and 25 per description sentence; active voice and simple tenses; one instruction per sentence. The rule that each message must stand alone for a reader returning after days is dropped.
The owner merged the structure rules and the ASD-STE100 rules on 2026-10-07 and judged the returning-reader rule too specific.
:::

::: decision {source=owner date=2026-10-07}
pm ships one guidance channel: pm prime prints the core rules plus the essential judgment rules (the sprint frame, sprint size, the smallest test that can change a decision, the merged owner-writing rules, a design page holds the final state of one area), about 900 tokens, then pm show; there is no pm guide; SKILL.md, the reference files and the site files are deleted, and a rule found missing later goes into code or into pm prime.
The owner took the recommended option (d): text that most sessions never load is not read, so the essentials must be in every session.
Answers `yeeef-agents-9va.38.18`.
:::

::: decision {source=owner date=2026-10-07}
The public site URL is a repo setting in the tracked .pm/config.toml (site_url), written by a pm command (pm init --site-url URL), not by hand; every clone prints the same links. This replaces the per-clone storage of 9va.15.5.2 and keeps its rule that a pm command sets it.
The owner took the recommended option (a): the owner reads one site, so every clone and session should link to it.
Answers `yeeef-agents-9va.38.16`.
:::

::: decision {source=owner date=2026-10-07}
The site and the push run as one supervised pm service per clone (pm service install|status|restart|logs; a systemd user service on Linux, a launchd agent with KeepAlive on macOS) that serves the site live and pushes Beads and records every 10 minutes; pm prime reports its health each session, and when it is down the agent runs pm service restart and, if that fails, raises an action to the owner and files a bug task. pm render goes away; pm check validates records.
The owner took the recommended option (a): one thing to install and watch, and the site is no longer down because nobody started it.
Answers `yeeef-agents-9va.38.17`.
:::

::: decision {source=owner date=2026-10-07}
The owner-request Stop check in Claude Code is a command hook: it reads the open needs and actions this session raised from Beads and calls Haiku through `claude -p` with thinking off (MAX_THINKING_TOKENS=0, CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1); a reply needs no id. The judge prompt is tuned until a test set of replies gets the right verdict, and those tests guard against regressions.
Reason: an agent hook adds 2.9 to 10.1 s per stop, which the owner found not acceptable; the command hook measured about 2.3 s against 1.0 to 1.5 s today, and the id check conflicts with the no-ids rule for owner messages.
Answers `yeeef-agents-9va.38.19`.
:::

::: decision {source=owner date=2026-10-07}
Working a sprint, pushing its branch and opening its PR never need the owner's approval: agents do them as part of the sprint, without asking. The owner's part starts at the PR review action (review and merge).
Reason: the owner stated it as a standing rule on 2026-10-07; asking for these steps only adds a wait.
Answers `yeeef-agents-9va.56.3`.
:::

::: decision {source=owner date=2026-10-07}
The owner-writing list in pm prime stays as merged on 2026-10-07; the three old rules (answer or answer-by time in the first reply, do a repeated request in full, tell the mechanism) are not added.
The owner chose option (c), keeping the list short.
Answers `yeeef-agents-9va.50.7`.
:::

::: decision {source=owner date=2026-10-07}
pm's guidance keeps a layer of redundancy: the rules pm prime prints also state the key rules that code, hooks or CI enforce, so agents stay clear of the bad behaviour, and the guards act as the safety net; only rules that are both enforced and rarely relevant are left to the guards and to --help.
The owner said on 2026-10-07 that structural guards are good but the agent's behaviour should also be shaped so it does not trigger them.
:::

::: decision {source=owner date=2026-10-07}
At session start pm prints only top-level project state, not all of pm show; pm show is restructured for progressive disclosure, so an agent reads more levels (a project, a sprint, a record section) only when its work needs them.
The owner chose a short top level at session start and asked for pm show to allow progressive disclosure.
Answers `yeeef-agents-9va.50.8`.
:::

::: decision {source=owner date=2026-10-07}
No structural guard is added for task claims: no PreToolUse hook refuses raw `bd update --claim` or `bd close`, and no Stop check requires a claimed task for commits on a code branch; the rules text that pm prime prints and `pm task claim`'s refusal of tasks held by live sessions stay the only defences.
The owner declined both gap fixes on 2026-10-07; do not propose them again unless a new incident shows the rules text is not enough.
:::

::: decision {source=owner date=2026-10-07}
The rule that a goal is written as a mechanism with a distance is not part of pm's guidance (pm prime and --help); do not add it back.
The owner said on 2026-10-07 that it need not be kept.
:::

::: decision {source=owner date=2026-10-06}
The harness tests are split into a fast set, run during development, and a slow set, run with the fast set once a PR is ready for review.
The full run takes about 1.5 min, which is too slow for every change in the development cycle.
:::

::: decision {source=agent date=2026-10-07}
pm never reads stdin: every command takes its text from flags (--text for free text), in harness/pm.py and in the pm/ package alike.
A command that reads stdin to EOF hangs forever when stdin stays open, as interactive Claude Code's Bash stdin socket does; a timeout would only shorten the hang and change what a slow pipe means.
:::

::: decision {source=owner date=2026-10-07}
pm prime has a "what" part (the layers, the objects and how they relate with their invariants, the interfaces — the site for the owner, the pm CLI and the records for agents — and how to work with them), then "how" parts as ordered procedures with a balanced amount of command reference (reading state; project, sprint and task operation; records operation; needs and actions), then the owner-writing list. A subsystem's detailed context, such as the pm service's, lives in its command's --help, and pm prime only names it.
The owner took the agent's outline on 2026-10-07, added the interface part, and asked that pm be self-contained: pm explains itself through its own commands.
Answers `yeeef-agents-9va.50.10.3`.
:::

::: decision {source=owner date=2026-10-07}
pm is self-contained: it orchestrates the work and gives the context about itself. With no skill installed, the pm CLI does the progressive disclosure of context: pm prime gives the model and the procedures at session start, pm show gives state level by level, and each command's --help holds that command's and subsystem's detail.
The owner stated it on 2026-10-07 as a core philosophy of pm's design: context served by the CLI sits beside the code it describes, so the two change together and do not drift.
:::

::: decision {source=owner date=2026-10-07}
Guidance for developing pm itself (its tests and live checks, [TEST] needs, how the package, hooks and site are built and checked) lives in pm/AGENTS.md (with a pm/CLAUDE.md link) in this repo, not in pm prime or any text the package ships; pm prime and --help carry only what a user's agents need.
The owner asked on 2026-10-07 to factor the pm-development context out of the shipped context; a nested AGENTS.md and CLAUDE.md load when an agent works under pm/, and the package ships only pm/src/pm.
:::

::: decision {source=owner date=2026-10-07}
Heavy integration tests (those that start real processes, ports, temporary homes, clones, remotes, the site server or the service) run only in CI on each PR, not on agents' machines; agents run only the light test set while they work, and a PR review is raised once CI passes.
The owner asked for it on 2026-10-07: integration tests are slow and flaky under the load of parallel sessions, and local development needs small fast tests.
:::

::: decision {source=agent date=2026-10-07}
pm reads stdin only for --text-file -, and only when stdin is a pipe or a regular file (a heredoc); any other stdin is refused at once, and every other body comes from --text (one plain line) or --text-file PATH. This replaces "pm never reads stdin" of 2026-10-07.
The review of PRs #59 and #60 found that --text="$(cat <<'EOF' …)" breaks on macOS bash 3.2 with an apostrophe or a lone ")", and that double-quoted --text runs backtick code spans; the type check keeps the open socket stdin of interactive Claude Code from ever being read.
:::

::: decision {source=owner date=2026-10-07}
pm prime prints all of its text (Part 1 what, Part 2 how, Part 3 writing to the owner) at every session start and after each compaction, split over as many hook entries as Claude Code's 10,000-character per-hook cap needs, each part under its own heading because hooks arrive in any order; this replaces the one-channel, about 900 tokens size of 2026-10-07.
The owner chose option A: every agent gets the whole model and every procedure, and repeated tokens are cheap with prompt caching.
Answers `yeeef-agents-9va.50.10.1`.
:::

::: decision {source=agent date=2026-10-07}
pm has one install command, pm init: its repo step (writing pm's pieces, bd init, creating the records branch) runs only when .pm/config.toml is absent; its clone and worktree step runs every time and is what session start runs; repairing a managed piece is pm doctor then pm upgrade; pm setup is removed.
The owner asked on 2026-10-07 why pm needs two commands, and the agent found none: with the repo step limited to a first install, running pm init at every session start cannot overwrite a branch's deliberate change, so a second command adds nothing.
:::

::: decision {source=owner date=2026-10-07}
The installed `pm` is a small launcher: it reads the repo's pin in .pm/config.toml and runs that exact pm version through uvx, cached per version, so repos on different pins share one machine and each upgrades when it chooses; the exact pin and the hard failure on a version the launcher cannot run stay.
The owner chose option B: with three repos on one machine, one tool and an exact pin would force an upgrade commit in every repo at each release.
Answers `yeeef-agents-9va.73.4`.
:::

::: decision {source=owner date=2026-10-07}
pm prime no longer prints a separate owner-writing list; its writing rules are the style guide under Records (page links, no ids or digests in prose, bullets over long paragraphs); this replaces the 2026-10-07 decision that pm prime carries the merged owner-writing list.
The owner removed the Writing to the owner section in their prime.md rewrite, merged as PR #68 on 2026-10-07.
:::

::: decision {source=owner date=2026-10-07}
Drop main's `records/` copy and keep each worktree's `records/` link: delete the copy Action, the PR guard, the pre-commit guard and the sparse checkout; records are browsed on the `records` branch or the site.
No machine reads main's copy, and it alone causes those four mechanisms; the link keeps the `records/` path agents and docs already use. Answers `yeeef-agents-9va.65.3`.
:::

::: decision {source=owner date=2026-10-07}
pm replaces Beads with its own work store: it borrows bd's ideas (ids, dependencies, ready work, claims, Dolt sync) and rewrites them for pm, so pm is one clean, self-contained product; the store may still use Dolt.
The owner chose option C over staying layered or forking: pm uses 14 of bd's about 120 commands, works around 10 bd constraints, and bd's 405 k lines of Go break in most minor releases. Answers `yeeef-agents-9va.41.2`.
:::

::: decision {source=owner date=2026-10-07}
pm's work store keeps its items in Dolt, embedded in pm, not as files on a git branch.
The owner chose Dolt for its abstractions: an enforced schema, SQL, transactions, row-level history and built-in cell-level merge. A Go pm embeds Dolt in process, so no server or separate binary is needed. Answers `yeeef-agents-9va.86.4`.
:::

::: decision {source=owner date=2026-10-07}
pm is rewritten from Python into Go.
The owner chose it in chat with the Dolt decision: Dolt embeds only into Go programs, and a Go pm ships as one binary that starts faster than Python.
:::

::: decision {source=agent date=2026-10-08 until="the work store core sprint chooses how Go pm takes test seeds"}
The pm tests read work data as work-store items, but write seeds as bd issues through `repo.set_issue` and `repo.add_issue`, not as items.
A bd issue is the form the work store imports (`pm init --import-bd`), so Go can take the same seeds through its importer, and a second, item-to-bd mapper in the tests would be a lossy mapping of its own to keep true; the frame's "seed and read as items" is met for reads, and seeding is one seam for the Go side to fill.
:::

::: decision {source=owner date=2026-10-08 until="concurrent pm commands measurably wait on the store's lock in practice"}
Each pm command opens the embedded Dolt store itself, once, and closes it at exit; the pm service does not hold the store, and every store access goes through work.Store so a later move to a service-held store changes only the transport.
The owner agreed with the agent's recommendation: per command is simpler, needs no running service, and loads every item in 64 ms against the 300 ms goal; a service-held store waits on nothing but makes every command depend on the service. Answers `yeeef-agents-9va.87.3`.
:::

::: decision {source=owner date=2026-10-09 until="the owner is back and reviews again"}
For the Go port sprints, the agent session merges each PR itself once CI is green on its final commit and an independent fresh-context review finds no open correctness issue, instead of raising a review need for the owner.
The owner gave the agent session agency in chat to merge all the PRs and deliver the port with high quality end to end while they sleep.
:::

::: decision {source=owner date=2026-10-09}
Go pm releases come from a new public GitHub repo for pm, so anonymous downloads work; the token path built for the cut-over stays as a fallback until the public repo serves releases.
The owner chose a new public repo for pm over a token on each machine or a hosted mirror.
Answers `yeeef-agents-9va.88.2`.
:::

::: decision {source=owner date=2026-10-09}
pm moves, with its history, from pm/ in yeeef-agents to its own public GitHub repo, which holds its source, CI and releases; yeeef-agents consumes released pm like any other repo.
The owner chose source over a releases-only public repo: pm becomes its own product, and anonymous downloads of its releases work.
Answers `yeeef-agents-9va.88.5`.
:::

::: decision {source=owner date=2026-10-09}
pm keeps per-repo exact version pins and the launcher: one pm on PATH reads the repo pin and runs that version, downloading it once per machine.
The owner chose per-repo versions with plain pm, which needs a machine-level dispatcher; in the public Yeeef/pm repo with tag-only releases its remaining cost is the launcher code and one download per pin.
Answers `yeeef-agents-9va.112.4`.
:::

::: decision {source=owner date=2026-10-09 until="sprint 102 closes"}
For sprint 102 (pm moves to Yeeef/pm), the agent session merges its PRs, in Yeeef/pm and in yeeef-agents, once CI is green on the final commit and a fresh-context review finds no open correctness issue.
The owner gave the agent session agency in chat to merge the Yeeef/pm PR as well.
:::

::: decision {source=owner date=2026-10-09}
Yeeef/pm is licensed under MIT.
The owner chose MIT: short and permissive, the license of Beads, which pm borrows from, so other users may use, change and redistribute pm with the notice kept.
Answers `yeeef-agents-9va.112.6`.
:::

::: decision {source=owner date=2026-10-09}
The pm service holds the Dolt work store and serves it to pm commands over a Unix socket (a Dolt SQL server in process); commands no longer open the embedded store, replacing the per-command store of decision need yeeef-agents-9va.87.3.
The owner asked in chat for a sprint that converts embedded Dolt to a Dolt server: no lock waits between commands, a warm store (7.8 ms load against 64 ms), and no rule against holding the store across slow work.
:::

::: decision {source=owner date=2026-10-10}
pm-harness is tracked in Yeeef/pm itself: pm runs on its own repo, with its work store and records branch public on GitHub
pm development moved to Yeeef/pm; keeping tracking with the code puts sprint PRs and records in one repo and dogfoods pm; the owner accepted public records, so moved records are scrubbed of hostnames, emails and machine paths
:::

::: decision {source=owner date=2026-10-10}
All pm-harness records move to Yeeef/pm: the project record, every sprint record, design pages, docs and postmortems
one complete history next to the code; yeeef-agents keeps only its own projects
:::

::: decision {source=owner date=2026-10-10}
Each session runs in autopilot by default, where every request to the owner left only in chat is blocked, with no exemption for requests outside a sprint (such a request is a project-level need); the owner can switch a session to interactive, where the owner-request hook does not run
a request is a request whether or not a sprint holds it, and the sprint-work condition was never checkable; live conversation, the case that made judging every request too rigid, is what interactive mode covers
:::

::: decision {source=owner date=2026-10-10}
In autopilot, a request to the owner that no sprint or task holds gets one first (a new task or a small new sprint), then its need; never a need filed under the project itself. When the agent has a clear reason not to raise a need, such as the owner plainly being in the chat, it asks with AskUserQuestion, never as a plain-text question
every request is tracked work, so it belongs to a sprint; AskUserQuestion keeps a question the owner answers live out of the reply the hook judges, where a plain-text question is lost when nobody reads the chat
:::

::: decision {source=owner date=2026-10-10}
No per-session interactive mode: every session is judged by the owner-request hook, and a question for an owner who is in the chat goes through AskUserQuestion. This replaces the autopilot and interactive part of the earlier decision of today; its no-sprint-exemption part stands
the mode is complex (a transcript reader for two runtimes, open questions on Codex and compaction) for what AskUserQuestion already gives: the hook never sees a question asked with it
:::

::: decision {source=owner date=2026-10-10}
The Go cut-over soak is over: delete Python pm from yeeef-agents (pm/) and from Yeeef/pm; Go is the only implementation.
Owner, 2026-10-10: ready to decommission the Python code for pm; Go pm has run this repo since 2026-10-09.
:::

::: decision {source=owner date=2026-10-10}
pm.yeeefs.com serves the Yeeef/pm pm site (localhost:8001); agents.yeeefs.com serves the yeeef-agents pm site (localhost:8000)
pm-harness, the project the owner follows most, now lives in Yeeef/pm; the owner set the Cloudflare routes on 2026-10-10
Answers `yeeef-agents-9va.116.9`.
:::

::: decision {source=owner date=2026-10-10}
Quality work (code, architecture, tests, feedback) lives in project pm-quality and Codex integration in project pm-codex; pm-harness sprints 34, 56, 105 and 110 moved there on 2026-10-10
the owner split pm's work by kind so each project holds one kind of sprint
:::

## Design pages

> Where is the detail?

- [Project management harness design](../design/pm-harness.md): the problem, goals and layers, with a summary of each area
- [Work layer: Beads](../design/work-layer.md): how our concepts map to Beads and which tool agents use
- [Record layer](../design/record-layer.md): record types, their templates, decision levels and the block vocabulary
- [pm CLI](../design/pm-cli.md): the commands and how they write records
- [Views and the site](../design/views-and-site.md): the views, the renderer and how the site is served
- [Shared records store](../design/records-store.md): where records live across branches and worktrees
- [Owner-request Stop hook](../design/owner-request-hook.md): how an unraised owner request is caught at turn end

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
