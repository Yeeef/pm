---
type: sprint
title: Published view and page versions on the site
bead: pm-c72a.2
---

## Goal

> What should be true when this sprint ends, and why now?

The owner reads each record's published version on the site, and every draft of it beside it, so live edits are visible and labelled instead of mixed into the page. Today the site renders the store's working tree, so a half-made edit shows as if published and one malformed record turns every page into a 500. This is the first increment of the [record-drafts](../design/record-drafts.md) design (Model A, the owner's choice), split so the site side ships before the per-worktree workspaces: the version machinery takes drafts from a source that the workspaces plug into next.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The published view renders the records at the `records` ref, not the working tree.
- Drafts as a source the site lists: for now the shared store's uncommitted edits, as one draft named for the store.
- The version bar on a page with drafts; each version's full render at `/drafts/<name>/<record path>.html`; its diff against published, section by section, and draft against draft.
- A malformed draft fails only its own version.
- A live check: whether a `claude -w` session can write a git worktree nested inside its own worktree, and whether Codex can; the answer fixes where workspaces live, recorded in the design page.

**Out:**
- Per-worktree workspaces, follow, journal publish (the next sprint).
- The Stop hook, publish at merge, draft refs.

## Done when

> What evidence will show the goal is met?

- In a scratch clone, a hand edit to a design page leaves its published page unchanged and adds a version; the version renders at its URL and its diff shows exactly the edited lines (a test, and a live check on the scratch service).
- A malformed draft serves an error only at its own version URL; every published page answers 200 (a test).
- The site's integration tests pass in CI on the sprint's PR.
- The `claude -w` and Codex live check's commands and results are in the sprint's Findings, and the design page's Open questions and Workspaces say where workspaces live.

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): Published view and versions; Workspaces (where they live)
- [Views and the site](../design/views-and-site.md): Views (versions), Implementation (design page dates), Read path (published records), Alternatives considered
- [Shared records store](../design/records-store.md): Reading (the site renders the records branch)

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-15}
The design page increment 1 is split: this sprint ships the published view and versions over the shared store; workspaces, follow and journal publish follow in the next sprint.
The site side is independent of the workspaces and gives the owner the version view first; the claude -w live check that fixes where workspaces live runs here, before workspaces are built.
:::

::: decision {source=agent date=2026-10-15}
The design page site-replies stays as it is for this sprint's code changes.
Replies, the token and the access refusal are unchanged; the versions under /drafts/ pass the same refusal as every other path, before routing.
:::

::: decision {source=agent date=2026-10-15}
The design page reply-delivery stays as it is for this sprint's code changes.
The service's reply spool, write thread and push are unchanged; only its page routing gained the versions' ?vs= key and their assets.
:::

::: decision {source=agent date=2026-10-15}
The design page pm-go stays as it is for this sprint's code changes.
The package layout and embedded files are unchanged; versions.go and diff.go are new files of internal/site, drafts.go of internal/store.
:::

::: decision {source=agent date=2026-10-15}
The design page record-layer stays as it is for this sprint's code changes.
Record format and checks are unchanged; ParseSummary splits ReadSummary so a summary read from git is checked the same way.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Nested-workspace live check (scratch repo: main + `records` branch, store
  worktree at `.pm/store/records`, each code worktree with `git worktree add
  <wt>/records drafts/<name>`, `/records` git-ignored). Claude Code 2.1.300,
  `-p --model haiku --permission-mode acceptEdits`, judged from disk and the
  transcripts' tool results. (1) `claude -w t1`: Write into
  `<t1>/records/design/x.md` (a nested worktree of another branch) succeeded;
  Write into the store was refused: "This session is isolated in the worktree
  <t1>, so its writes are limited to that folder. This path is in a different
  worktree (<main>/.pm/store/records). Edit the copy of this file under <t1>
  instead." So the -w isolation follows the path prefix, not git worktree
  boundaries: a worktree nested inside its own is allowed. (2) Plain `claude`
  in a normal worktree: Write and Edit in the nested `records/` succeeded; the
  store write was refused without `additionalDirectories` ("Claude requested
  permissions to write to <store path>, but you haven't granted it yet.") and
  succeeded with `--settings
  '{"permissions":{"additionalDirectories":["<store>"]}}'`. (3) Codex 0.162.1
  `codex exec --sandbox workspace-write`: not measurable on this host: every
  sandboxed command fails before running ("bwrap: loopback: Failed
  RTM_NEWADDR: Operation not permitted"; with network_access=true "bwrap:
  setting up uid map: Permission denied"; legacy Landlock panics
  "filesystem-restricted execution requires bubblewrap"), since
  kernel.apparmor_restrict_unprivileged_userns=1 and bwrap has no AppArmor
  profile. A command matched by a `.codex/rules` `prefix_rule(...
  decision="allow")` ran outside the sandbox and committed in the nested
  workspace (3 of 3 runs, with and without extra writable_roots), so pm-run
  commits work; whether the sandboxed agent itself can write or `git commit`
  there is unverified here (the Constraints' 0.131.0 finding still stands: git
  dirs stay read-only unless listed, and a commit also writes the common
  `.git` objects and refs, not only `.git/worktrees/<name>`).

- Published view and versions, live check on a scratch clone (pm init with the
  fixtures' fake supervisor, HOME and CODEX_HOME under the temp dir, service
  from the built binary): a hand edit to design/parser.md showed as the store
  version within 0.20 s; the published page stayed byte-equal to the page
  before the edit plus the version bar; /drafts/store/design/parser.diff.html
  marked exactly the 3 edited lines (- None yet., + Records need a parser., +
  It streams.) in 1 changed section, 7 folded; a malformed header made only
  /drafts/store/design/parser.html 500 (its diff 200; /, index, project and
  design pages 200); refresh 4-6 ms, page GETs 0-1 ms. On the real store one
  git archive of refs/heads/records (2.5 MB tar) takes 0.01-0.02 s and the
  drafts' git status under 0.01 s.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
