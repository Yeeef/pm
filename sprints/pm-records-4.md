---
type: sprint
title: Per-worktree records workspaces and journal publish
bead: pm-c72a.4
---

## Goal

> What should be true when this sprint ends, and why now?

Each code worktree edits records in its own workspace, so no session's commit can carry another's edit and the owner sees each worktree's edits as its own labelled version. Today every session edits the one shared store and `pm commit` commits whole files, which is how two sprints edited the same 8 design pages on 2026-10-14. This is step 2 of [record-drafts](../design/record-drafts.md): workspaces at `<worktree>/records`, follow, and journals published at `pm commit` with overlaps refused; the site's versions (sprint 2) gain one version per workspace.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A workspace per code worktree: a git worktree of `drafts/<name>` at `<worktree>/records`, in place of the link; `pm init` makes it, and turns an existing worktree's link into one at that worktree's own session start.
- Follow, by `git merge-tree` and `git merge-file`, never rebase, merge or stash; no workspace left mid-operation.
- `pm commit` and one-shot writes from a worktree: follow, commit journals on `records`, refuse a file with conflict markers naming the commits that touched it.
- Each workspace as a draft on the site, labelled with worktree, branch, sprint and live session.
- Design pages committed from a workspace go to its draft commit tagged with their sprint. Until step 4 builds publish at sprint close, they publish at `pm commit` as journals do.

**Out:**
- Publish at sprint close and `--discard` (step 4); the Stop hook and `pm clean` guards (step 3); draft refs across clones (step 5).
- Codex's sandboxed agent writing a workspace (untestable on this host).

## Done when

> What evidence will show the goal is met?

- In a scratch clone with two worktrees: disjoint edits to one sprint record from both reach `records`; an overlapping edit is refused at the second `pm commit`, naming the first commit, with markers only in the second workspace (a test).
- A one-shot write merges into another workspace's uncommitted edit of the same file and leaves no workspace mid-operation (a test).
- The site lists each workspace as a version of the pages it changes, labelled with its worktree (a test and a live check on a scratch clone).
- A `claude -w` session writes and commits a record from its own worktree with no extra settings (a live check in a scratch worktree).
- CI green on the sprint's PR.

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): Workspaces, Follow, Writes and journal publish, Published view and versions
- [Shared records store](../design/records-store.md): the main checkout's link and each worktree's workspace; Reading, Writing, Setup
- [pm CLI](../design/pm-cli.md): pm commit and pm where
- [Agent lifecycle](../design/agent-lifecycle.md): where hand edits happen
- [pm as an installable product](../design/pm-product.md): what pm init sets up in a worktree
- [Session claims](../design/session-claims.md): pm clean and a worktree's workspace
- [Views and the site](../design/views-and-site.md): each workspace a version

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-15}
Until publish at sprint close is built, a design page committed from a workspace is tagged with its sprint and published at pm commit, as journals are.
Held drafts with no publish step would strand design edits; tagging now means step 4 only changes when the draft commit publishes, not what it holds.
:::

::: decision {source=agent date=2026-10-15}
The design page pm-go stays as it is for this sprint's code changes.
serve.go and session.Uses gained the workspaces' drafts and labels; the package map and the service's shape the page describes did not change
:::

::: decision {source=agent date=2026-10-15}
The design page site-replies stays as it is for this sprint's code changes.
serve.go's change is the drafts source and its stamp; who may reach the site and how replies are served are unchanged
:::

::: decision {source=agent date=2026-10-15}
The Stop hook check of the session own workspace moves from step 3 into this sprint.
Without it a worktree session edits records the Stop hook no longer sees, so this PR would drop a check that exists today; in a workspace the check is simpler than the transcript match it replaces there.
:::

::: decision {source=agent date=2026-10-15}
The design page pm-in-codex stays as it is for this sprint's code changes.
pieces.go changed only its package doc comment to name the workspaces; the Codex hooks, rules and writable_roots pm init writes are unchanged
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check of workspaces (this branch's .go/pm 0.9.0-dev) in a scratch clone
  of a scratch bare origin under /tmp/pmlive.HXHQ, HOME, CODEX_HOME and
  CLAUDE_CONFIG_DIR there, the supervisor the harness's fake_sched (the
  service really served on :47889). `pm init` in the main checkout, then `git
  worktree add .claude/worktrees/{one,two}` and `pm init --session-start` in
  each: "made …/one/records a records workspace on branch drafts/one", 0.12 s
  and 0.11 s. Disjoint edits to records/sprints/demo-1.md (Goal paragraph a in
  one, b in two): `pm commit` 0.06 s and 0.07 s, records log `dabc932 two:
  goal b`, `f81837e one: goal a`, both paragraphs on records. Each worktree
  edited design/parser.md's Problem differently, uncommitted:
  design/parser.html 200 without either edit, its version bar listing
  published, one ("worktree one on one, sprint repo-i7v0.1, session sess-one
  live; uncommitted edits", with a fake transcript for sess-one) and two
  ("worktree two on two, no live session; uncommitted edits"); drafts/one/…,
  drafts/two/…, drafts/one/….diff.html and ….diff.html?vs=two all 200 with the
  right edits. `pm commit` of the page from one: records commit d3e247f with
  "Sprint: design/parser.md repo-i7v0.1"; the same from two refused:
  "records/design/parser.md has conflict markers, which records changed under
  your edit in d3e247f one: parser problem", markers in two only (0 in one and
  in the store), no MERGE/REBASE state in either workspace, two's version then
  "uncommitted edits, conflict", pm where "1 uncommitted files (conflict
  markers in records/design/parser.md), on the records tip". A finding added
  from one (0.05 s with the follow) and one from two merged into two's
  uncommitted Done when edit. `pm commit` of a design page in the main
  checkout refused ("is a design page: design pages are edited and committed
  in a code worktree's records workspace"). Scratch dir removed after.

- Live check of PR #101 at a5c3c06 (pm 0.9.0-dev from make go-build, git
  2.43.0, Claude Code 2.1.300, haiku): scratch origin + clone set up by
  tests/agents.py Clone (fake supervisor runs the service; HOME and
  CLAUDE_CONFIG_DIR scratch, login copied mode 600; no
  permissions.additionalDirectories anywhere). (1) claude -w t1 -p
  --permission-mode acceptEdits: SessionStart made <worktree>/records a
  records workspace (git worktree of the store on drafts/t1, not a link); Read
  and Edit of records/sprints/demo-1.md succeeded with no denial; Bash (pm
  commit) needed approval, as any Bash does in acceptEdits -p. The Stop hook
  (pm hook stop) blocked naming records/sprints/demo-1.md as uncommitted in
  the t1 workspace; the next stop passed on stop_hook_active. (2) claude -w
  t2, same plus --allowedTools Bash: Edit, then pm commit gave 'committed
  records/sprints/demo-1.md as 694039d'; the store's records branch head is
  694039d and its file holds the line; workspace clean; stop_hook_summary
  hookErrors []. Scratch dir held the clone at /tmp/claude-1000/wcXXXX: the
  scratchpad path made work.sock 127 bytes, over the 103 limit.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: each code worktree edits records in its own workspace, `pm commit` publishes journals and refuses an overlapping edit, and the site shows each workspace as a labelled version.

Merged as 7dfd28f (PR #101).

- Workspaces at `<worktree>/records`, made at that worktree's own setup; follow by `git merge-tree` and `git merge-file`, never rebase, merge or stash, with a base ref pm keeps itself.
- `pm commit` and one-shot writes through the workspace; conflict markers name the commits; a figure changed on both sides is refused once, naming them.
- The Stop hook checks the session's own workspace (moved here from step 3); `pm clean` and `pm uninstall` refuse a workspace with uncommitted edits or drafts.
- git older than 2.40 fails only the workspace step, with a warning at session start and in `pm doctor`; that worktree keeps the shared store.
- Design pages are tagged with their sprint and, until publish at sprint close is built, published at `pm commit`.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Disjoint edits to one sprint record from two worktrees both reach `records`; an overlap is refused at the second `pm commit`, naming the first commit, markers only in the second workspace: met. Go tests in `internal/store` and `internal/cli`, `tests/test_workspaces.py`; live check on a scratch clone: overlap refused naming d3e247f, no merge or rebase state left (Findings).
- A one-shot write merges into another workspace's uncommitted edit and leaves no workspace mid-operation: met. Go test; live check (Findings).
- The site lists each workspace as a version labelled with its worktree: met. Go tests; live check: three entries on the version bar, every version and diff URL 200.
- A `claude -w` session writes and commits a record from its own worktree with no extra settings: met. Live check with Claude Code 2.1.300: workspace made at session start, file-tool edit, `pm commit` landed as the records head; the Stop hook blocked on an uncommitted edit and passed when clean (Findings). `pm commit` itself needs Bash permission, as any pm command does.
- CI green on the sprint's PR: met. PR #101 at a6ebcb4, every check green; two fresh-context reviews, all their findings fixed or documented in the design page's Failure modes.
