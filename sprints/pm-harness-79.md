---
type: sprint
title: "Go port: cut-over of yeeef-agents (P10)"
bead: yeeef-agents-9va.88
---

## Goal

> What should be true when this sprint ends, and why now?

This repo runs Go pm on its own Dolt work store in every clone, Beads is gone from its agent context and hooks, and, after a soak, Python pm is deleted. It is the last port step: Go pm reached parity in the install sprint, and only one work store is authoritative at a time.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The one-time migration in one clone: stop the service in every clone, `bd dolt push`, `bd export`, `pm init --import-bd`, push; the other clones (this Mac and the Linux server) attach with `pm init`.
- Moving the pin in `.pm/config.toml` to the first Go version.
- Switching `prime.md` from `bd` to pm commands; `pm upgrade` removing the Beads pieces (bd hook entries, the Beads block in `CLAUDE.md`, `.beads/hooks` from `core.hooksPath`).
- After a soak: deleting Python pm and updating `pm/AGENTS.md`, the pm-product page and the work store page's Storage section.
- `pm/AGENTS.md` "Releasing pm" rewritten to the tag-only procedure, retiring the merge-commit rule and the "pin must equal the pyproject version" note.

**Out:** Dolt history and audit events from Beads, which `bd export` does not carry; removing `.beads/` and the final bd export, which the owner does; other repos.

## Done when

> What evidence will show the goal is met?

- Every clone runs the Go version named by the pin; `pm doctor` is clean in each.
- `grep -rn '\bbd\b'` over the repo's agent config, hooks and docs finds no live use.
- The site is equal before and after the cut-over, after normalisation.
- `pm export` equals the final bd export on every check of the work store page's migration.
- After the soak, Python pm is deleted on main and the named docs describe Go pm.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Port order and coexistence
- [Work store: pm's own replacement for Beads](../design/work-store.md): Migration from bd

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-08}
Sprint 79 is rescoped to the Go port's cut-over of this repo: migrate the bd data with --import-bd, move the pin to the first Go version, remove the Beads pieces, and delete Python pm after a soak; it now depends on the install sprint (91) instead of sprint 78.
The owner chose embedded Dolt and a Go rewrite; the pm-go page's port order makes the cut-over the last step, after Go pm reaches parity.
:::

::: decision {source=agent date=2026-10-09}
Scope adds the tag-only Go release: a release is a tag on any main commit, its version taken from the tag name and stamped at build, none written in source; an untagged build reports dev; moving a pin is a separate ordinary PR.
The owner asked for it, relayed by the session that fixed Python releases in sprint 98: a version written in source forces a bump commit, a release PR and a merge-commit rule.
:::

::: decision {source=agent date=2026-10-09 until="the owner answers yeeef-agents-9va.88.2"}
The cut-over proceeds on decision need .88.2's default: Go pm, install.sh and a bridge 0.1.6 download releases through the GitHub API with a token from GH_TOKEN or gh auth token; the need stays open for the owner to override.
The owner authorised delivering the port end to end while asleep; the token path keeps the repo private, needs no hosting, and still works if the repo is later made public.
:::

::: decision {source=owner date=2026-10-09}
At the live switch, .beads is renamed to .beads.retired, so any bd command fails loudly; Beads history stays intact and one mv undoes it.
The owner chose retire: 30 sessions carry bd guidance, and a bd write after the switch would otherwise be lost silently.
Answers `yeeef-agents-9va.88.4`.
:::

::: decision {source=agent date=2026-10-09}
The live switch waits for sprint 102 (pm moves to Yeeef/pm), so pm-v0.2.0 is released from the public repo pm keeps.
The Go launcher builds the release URL in: a first Go release from yeeef-agents would leave every installed 0.2.0 launcher pointing at a repo pm leaves, needing another bridge.
:::

::: decision {source=agent date=2026-10-09}
The soak ends after a few days of normal use of Go pm on this repo (sessions, hooks, the service syncs, site replies) with no fix needed in Go pm; then pm/ is deleted here, and the owner decides on .beads.retired/.
The frame says Python pm is deleted after a soak but sets no length; a condition, not a date, keeps the cheap rollback until Go pm has run the real workload.
:::

::: decision {source=owner date=2026-10-10}
The done-when item "the named docs describe Go pm" (pm-product page, work store page Storage section) moves to Sprint 109 task .119.2; Sprint 79 closes once task .12 is on main.
The owner chose handoff: those pages are already Sprint 109 scope, held by its session, and one item has one owner.
Answers `yeeef-agents-9va.88.13`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Bridge release 0.1.6 shipped by the Python procedure (pm/AGENTS.md,
  Releasing pm): launcher change 4fd0090 (the Python launcher, the Go launcher
  and install.sh find a release's assets through api.github.com with a token
  from GH_TOKEN, else gh auth token; no token a hard error naming both; the
  token goes to the API alone, never to the storage host an asset redirects
  to, which refuses a second auth mechanism; PM_RELEASE_URL still a tokenless
  mirror). Commit A 32a34d5 tagged pm-v0.1.6 and pushed before commit B
  2bcd9d4 moved the pin (its pre-commit hook ran the tag's build). PR #96: CI
  green on 2bcd9d4 (5 jobs), merged by the sprint's agent with a merge commit,
  de7550b; main holds 32a34d5. Evidence: make test 135 passed; 6 new shared
  launcher tests against a GitHub API stand-in pass on Python and Go, 4 new
  install.sh tests pass. Live against GitHub with gh auth token: Python
  launch.binary('0.2.0-rc.3') kept the 111,380,098-byte binary in 6.0 s; the
  Go launcher in a repo pinned to 0.2.0-rc.3 downloaded the same binary (cmp
  equal) and pm version printed 0.2.0-rc.3; install.sh for rc.3 installed it
  from api.github.com/.../releases/assets/624150605.

- Cut-over preparation PR #97 (go-port-cutover-prep), inert until the pin
  moves to a Go release: Go pm prints its own pm/prime.md (work store; pm task
  ready, pm show <id>, pm dep add, pm task add --parent; bd remember gone),
  with pm show ID and pm task add --parent TASK as new Go-only forms; pm prime
  --subagent prints pm's commit-and-push rule instead of reading the Beads
  agent profile; Go pm init/upgrade/doctor keep pm's hook scripts in tracked
  .pm/hooks and remove the bd hook entries (.claude/settings.json -9 lines,
  .codex/hooks.json -45) and the Beads CLAUDE.md block (-55 lines), measured
  on a scratch clone of main; decision close's undo names pm decision add
  --need, which marks a no-decision need answered; help, site and refusal
  texts name pm. Parity holds every by-design difference in a checked list
  (GO_HELP 31 phrases, GO_WORDING and REWORDED 4 refusals, 1 parity-allow
  entry, 5 transcript normalisations). Locally: make test 135 passed; make
  test-go every package ok, test_go_parity 117 passed; make test-go-suite 134
  passed, 134 transcripts equal. Fresh-context review: 1 correctness finding
  fixed in a2a2a22 (a pin moved in a worktree pointed the clone's hooksPath at
  a main checkout without .pm/hooks, so no hook ran; Beads' path now stays
  until main has pm's hooks), 1 minor fixed (no-decision is a resolution, not
  a label), 1 minor left (compare_transcripts skips Go-only transcripts). The
  push-state key needed no code: Go's steps are work, summary and records and
  Go ignores Python's beads entry, but a clone's first Go run flags the work
  push overdue from installed_at until its first push, so the runbook runs pm
  push once after the switch.

- Rehearsal of the cut-over on scratch clones of a local mirror of origin
  (HOME, CODEX_HOME, CLAUDE_CONFIG_DIR, XDG dirs and the uv tool dir under a
  scratch dir; the test suite's fake launchctl; GH_TOKEN from gh auth token),
  release pm-v0.2.0-rc.5 of PR head a2a2a22. Clone A set up by the bridge
  0.1.6 (uvx pm init: bd bootstrap, pm doctor clean); baseline: 149 site pages
  crawled from the Python service, bd export of 568 issues. Switch: pm upgrade
  --to 0.2.0-rc.5 through the bridge (6.8 s, download included) moved the pin,
  removed the bd entries from both hook files and the Beads CLAUDE.md block,
  wrote .pm/hooks and moved the hooks path; pm init --import-bd imported 568
  items and 286 comments in 0.8 s; pm sync before pm init fails (the store has
  no remote until pm init attaches it), so the order is import, pm init
  (pushed refs/pm/work, replaced the uv tool's link with the Go binary,
  installed the service), pm push once. After: pm doctor clean; pm where shows
  work ok; session context (2 rule chunks of 9,762 and 6,244 characters, state
  2,929, subagent line) holds no bd or Beads; site 149 pages, 145 equal after
  normalisation and 4 task graphs equal up to edge order (the known blocked_by
  order); pm export equals the bd export on all 568 items on every migration
  check (type, status, parent, blockers, title, description, comment count 286
  in all, raised_by and delivered of 192 needs) and on every field against
  work_items.py; grep for bd in agent config and hooks finds only CLAUDE.md's
  setup paragraph, fixed on branch go-port-cutover-switch (bb0ae30). Clone B
  on a machine with only the bridge uv tool: pm init (9.0 s) cloned the work
  store from refs/pm/work, pm doctor clean, pm export byte-equal to A's (568
  items). bd dolt push was not rehearsed: the bootstrapped database's remote
  is GitHub. bd refuses a .beads under /tmp (unsafe BEADS_DIR), so a rehearsal
  must live elsewhere.

- Hook skip on PR 97: commit baf9d05 (the Go hook move and Beads removal,
  written by a subagent) was committed with the verify skip; the subagent
  reported no refusal, it skipped the hooks to keep the live bd pre-commit
  section from running from its worktree. It touches no records/ path, the one
  thing pm's pre-commit section refuses on a code branch. Proof on the final
  tree: on a scratch branch off origin/main, a no-commit merge of a2a2a22 (29
  files, 0 under records/) committed through the real pre-commit hook, the
  trace showing .beads/hooks/pre-commit then prepare-commit-msg run, exit 0
  (baa4877, branch deleted, never pushed). The same hook refused an earlier
  attempt that staged a2a2a22's whole tree over a newer main, since that
  reverted main's newer records/ copies (records/days/2026-10-09.summary.json,
  sprints/pm-harness-79.md and pm-harness-91.md): an artefact of that method,
  not of the PR, whose diff touches no records/. The merge c283775 that
  brought baf9d05 into the branch also ran the pre-commit hook with its
  changes staged. No change to the PR was needed.

- Cut-over preparation merged as d8ba52d (PR #97; CI green on all 9 checks of
  a2a2a22; inert until the pin moves: it changes no Python prime.md,
  .pm/config.toml, CLAUDE.md or agent hook file). Bridge 0.1.6 (PR #96, tag
  pm-v0.1.6) downloads Go releases through the GitHub API with a token. The
  live switch waits on decision need yeeef-agents-9va.88.4, because 30
  sessions on this Mac carry bd guidance and a bd write after the switch would
  be lost silently.

- Live cut-over of this Mac's clone: final bd export of 580 issues (297
  comments); pm-v0.2.0 installed from Yeeef/pm with install.sh; pin PR #98
  merged as 945d025; pm init --import-bd imported 580 items, pm init pushed
  the store to refs/pm/work and reinstalled the service; pm export equals the
  bd export on every field of 580/580 items; pm doctor clean.

- Site before and after the switch, 152 pages: 125 equal after normalisation;
  of the 27 that differ, every difference is records changed between the
  crawls or ids in natural order instead of text order (day pages, sprint
  lists, diagram nodes); no Go rendering difference. bd is retired: a .beads
  file makes bd list and bd init fail with not a directory.

- One bd write fell after the final export: task yeeef-agents-9va.88.9, raised
  by this session through Python pm while the pin was still 0.1.6 (bd in
  .beads.retired holds 581 issues against the export's 580, no other item
  changed). It was recreated in the work store through Go pm. The
  export-to-switch window is the cut-over's one loss risk; .beads.retired
  keeps bd's copy.

- Go pm's Beads and uv tool texts: Yeeef/pm PR #3 rewords 30 user-facing texts
  in 9 Go files to name the work store, the installed pm or install.sh. They
  are .pm/README.md, the Codex SubagentStart status message, 6 help texts (pm
  service, service run, upgrade --to, uninstall), 4 site texts, 2 pm check
  errors, 2 Progress prompts, 13 refusals and the store's undo hint. Texts
  about the bd import, the Beads cleanup, the legacy harness and Python pins
  stay as they are. Python pm is unchanged. Local runs: make test 135 passed;
  make test-go all Go packages ok and parity 117 passed; make test-go-suite
  132 passed, 134 transcripts, 0 differ. One reviewer pass found no
  correctness issues. Repos pick it up after a release and pm upgrade.

- Regression from retiring bd with a .beads file (PR #99): Go pm 0.2.0 and
  0.2.1 fail pm doctor and pm upgrade with 'open .beads/hooks/post-checkout:
  not a directory'; session start still passes. pm-v0.2.1 (texts naming the
  work store, Yeeef/pm#3) is released and verified, but the pin stays at 0.2.0
  until a 0.2.2 treats a .beads file like a missing .beads/.

- Go pm 0.2.x failed pm doctor and pm upgrade in a repo whose .beads is a file
  (open .beads/hooks/post-checkout: not a directory): the only read under
  .beads/ is LegacyRepo's look at .beads/hooks/*, through install.Read, which
  took only ENOENT as absent. Yeeef/pm PR #4 makes Read take ENOTDIR as absent
  too, as Python's Path.exists() does; permission errors still fail. New tests
  TestABeadsFileIsNoBeads and
  test_a_beads_file_is_no_beads_for_doctor_and_upgrade failed before the fix
  with that error; after it make test 135 passed, make test-go all Go packages
  ok and parity 117 passed, make test-go-suite 132 passed with 0 transcript
  diffs; CI green on a5d7580.

- Linux server clone (beeefy, ~/Desktop/workspace/yeeef-agents) moved onto Go
  pm 0.3.0 over ssh by the runbook: old push timer
  local.pm-push.yeeef-agents.c9b30263 stopped and its units kept in
  ~/pm-old-units; .beads/ kept as .beads.retired; .records moved to
  .pm/store/records (its 2 unpushed 2026-10-07 commits were superseded on
  origin, kept as tag server-records-before-go); pm init cloned the work
  store; pm doctor clean; pm export 595 items on the server and on this Mac;
  site 200; pm push ok.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: both clones, this Mac's and the Linux server's, run Go pm from the public Yeeef/pm on the Dolt work store, with bd retired; deleting Python pm after the soak remains.

- Cut-over preparation ([#97](https://github.com/Yeeef/yeeef-agents/pull/97)) and the bridge release 0.1.6; the rehearsal on scratch clones.
- The live switch on this Mac: final bd export (580 issues), Go pm installed from Yeeef/pm, pin 0.2.0 ([#98](https://github.com/Yeeef/yeeef-agents/pull/98)), import, `pm init`, the work store pushed to `refs/pm/work`, the service on Go pm.
- bd retired: a `.beads` file blocks every bd command, `bd init` included ([#99](https://github.com/Yeeef/yeeef-agents/pull/99)); Beads history kept in `.beads.retired/`.
- The [cut-over runbook](../docs/2026-10-09-go-pm-cut-over-runbook.md).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** both clones run the pinned Go pm (0.3.0 since another sprint's release) with `pm doctor` reporting every managed piece matching: this Mac's, and the Linux server's, moved by the [Linux server runbook](../docs/2026-10-09-linux-server-runbook.md); both export 595 items.
- **Met:** `grep -w bd` over `CLAUDE.md`, `.claude/settings.json`, `.codex/hooks.json`, `.pm/hooks`, `commands/` and `skills/` finds no use.
- **Met:** the site before and after the switch: 152 pages, 125 equal after normalisation; every difference on the other 27 is records changed between the crawls or ids in natural order instead of text order; no Go rendering difference.
- **Met:** `pm export` equals the final bd export on every field of 580 of 580 items, 297 comments on each side.
- **Not yet:** deleting Python pm after the soak, and the named docs describing Go pm.
- **Met so far:** the sprint's PRs are on main: [#97](https://github.com/Yeeef/yeeef-agents/pull/97) as `d8ba52d`, [#98](https://github.com/Yeeef/yeeef-agents/pull/98) as `945d025`, [#99](https://github.com/Yeeef/yeeef-agents/pull/99) as `eb97cac`; the PR deleting Python pm comes after the soak.
