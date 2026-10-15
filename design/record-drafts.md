---
type: design
title: Record drafts and publishing
project: pm-records
covers: []
---

## Problem

> What are we solving, and why now?

Every session writes records in one shared working tree, and the site renders that tree. Two open sprints edited the same 8 design pages in it, one starting two minutes after the other's commit; nothing was lost only because of that gap ([the starting analysis](../docs/2026-10-14-record-layer-concurrency.md)). Three problems follow from the one shared tree:

| Problem | Database name | Today |
|---|---|---|
| Two agents write one file | Dirty write, lost update | `pm commit` commits a named file whole, so it can carry another session's half-made edit to that file |
| The owner wants work in progress | Dirty read, wanted on purpose | The site renders uncommitted edits, unlabelled by writer and with no mark of done |
| Records describe code before it merges | Atomicity across two stores | A design page edit is published at `pm commit`; its code lands at the PR's merge. `pm check` lists covers patterns waiting on unmerged work (11 in this repo when this design began) |

This page is the record layer's model for drafts and publishing: each code worktree edits records in a private workspace on a shadow branch of `records`, journals publish at `pm commit` by a three-way merge, and design pages publish when their code merges.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals**

1. No commit carries another writer's edit; two writers' edits to one file merge by hunk, and an overlap is refused with the other commit named.
2. The owner sees published records by default and every draft beside them, labelled by worktree, branch, sprint and session, live.
3. A design page's edit is published when the code it describes reaches main, not before.
4. A draft that does not parse breaks only its own render.
5. The owner reads every version of a page on the site: the published one and each worktree's draft.
6. The Stop hook knows a session's uncommitted record edits by construction, not by matching paths in its transcript.

**Non-goals**

- Merging concurrent keystrokes (OT, CRDTs).
- Changes to the work store.
- Showing uncommitted edits made on another machine.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

**The store and its writers**

- One records store per clone: the `records` branch checked out at `<main checkout>/.pm/store/records`. Each worktree's `records/` is a git-ignored link to it, made by `pm init` ([Shared records store](records-store.md)).
- pm one-shot writes (`pm finding add`, `pm decision add`, …) validate against the store's HEAD plus their planned writes, refuse a target file with uncommitted changes, and commit exactly their paths, under an exclusive `flock` on the store directory.
- `pm commit` holds that lock from its start, validates HEAD plus the named files (not the working tree), and commits the named files whole. It never checks who edited a file or whether it moved since the committing session read it.
- Agents edit records with their own file tools; pm sees those edits only at `pm commit`.

**The site**

- The site renders the working tree, uncommitted edits included; the service looks for a change every second.
- One record that does not parse fails the whole load, and every page is served as a 500 error page until it is fixed.

**Hooks, sessions, sync**

- The Stop hook blocks when the session's transcript names an uncommitted store file in a tool call (substring match).
- `session.Uses` maps each worktree under `.claude/worktrees` to its latest live session; only `pm clean` uses it today.
- One session changes code only in its own worktree; `pm task claim` refuses the main checkout.
- The service pushes every 600 s. Records are not pulled when only behind. When ahead and behind, the store rebases onto the remote under the store lock, skipped while tracked records have uncommitted edits. The service fetches main only after it sees a review's PR merge.
- pm installs no git hook: session start's `pm init --session-start` sets up every worktree an agent works in as a code worktree. In a second worktree of `records` it fails ("this repo has no .pm/config.toml").

**Design pages and the code**

- `pm check` passes a `covers:` pattern that matches no tracked file, listing it as waiting on unmerged work. The gate (`pm action need --pr`, `pm sprint close`) passes a page covering a changed file when it was updated (a records commit since the sprint opened, plus a Design pages link) or waived. `pm sprint close` refuses while a pattern the sprint added matches no file on the remote's main ([Sprint lifecycle](sprint-lifecycle.md)).
- The service learns of a merge only for an open PR review: every 60 s, `gh pr view --json state,mergeCommit` per review. A PR merged without a review (`pm sprint close --merged`) is never seen by the service.

**Runtimes** ([Shared records store](records-store.md), Constraints)

- Claude Code writes through `records/` only with the store in `permissions.additionalDirectories`; a `claude -w` session is refused "a different worktree" for any other git worktree, the store included.
- Codex's sandbox keeps every git dir read-only unless that git dir is itself a writable root; pm lists the store and its git dir in `writable_roots`.

::: result {title="Measured on this clone"}
| Quantity | Value |
|---|---|
| Records store working tree | 2.6 MB, 231 files |
| A new worktree of `records` (`git worktree add`, hooks off) | 0.01 s; 2.6 MB tree + 56 KB admin dir |
| `git status` in the store; `git merge-tree --write-tree` on two `records` commits | 1 ms each (mean of 20) |
| `pm check` on the whole store (226 pages, work store read) | 0.20 s |
| Code worktrees of this clone | 10 |
:::

*Reading:* a records workspace costs about 2.6 MB and 10 ms per worktree; listing drafts costs about 1 ms per worktree; one version render costs at most one full record-set check.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

```mermaid
flowchart LR
  subgraph wtx["worktree on branch X"]
    wsx["records/: workspace<br/>branch drafts/x"]
  end
  subgraph wty["worktree on branch Y"]
    wsy["records/: workspace<br/>branch drafts/y"]
  end
  wsx -->|"pm commit: journals,<br/>3-way merge, overlap refused"| rec[("records branch,<br/>store in the main checkout")]
  wsy -->|"pm commit: journals"| rec
  wsx -.->|"design edits wait;<br/>published at X's PR merge"| rec
  rec -->|"follow"| wsx
  rec -->|"published view"| site["site"]
  wsx -->|"draft"| site
  wsy -->|"draft"| site
```

*Reading:* each worktree edits records in its own branch; journals reach `records` at `pm commit` by a three-way merge, design pages at the PR's merge.

### Terms

| Term | Meaning |
|---|---|
| Journal | Every record but a design page: project and sprint records, docs, postmortems, day summaries. Facts as they occur |
| Published | A record's text on the `records` branch, which the site renders by default |
| Draft | A change to a record that is not published: in a workspace's draft commit, or uncommitted in a working tree |
| Workspace | The private working tree in which one code worktree's sessions edit records |
| Version | One rendering of a record: the published one, or the published set with one draft laid over it |

### Published view and versions

A page's URL renders its published version: the record on `records`, never a working tree. A page with drafts carries a version bar:

| Entry | Shows |
|---|---|
| Published | The page's URL |
| One per draft | Worktree name, branch, sprints, session (when live), and its state: committed on the draft branch, uncommitted edits, or conflict (markers in the file, or a publish refused). A link to its full render and to its diff against published |

Worktree and branch come from `git worktree list`, the session from `session.Uses` widened to every worktree, the sprints from the open tasks that session holds. The main checkout's uncommitted edits are one more draft, named "main checkout".

**A version's URL** is `/drafts/<worktree>/<record path>.html`, for example `/drafts/pm-records-1/design/record-drafts.html`. A path rather than a query string (`?v=<worktree>`) because a record's relative links then stay in the same version: a link from a draft to another record renders that record in the same draft, which is the published text wherever the draft does not change it. The worktree's admin name is unique in the clone and stable for the worktree's life.

**A version's render**

- Its record set is the published set with the draft's files laid over it: the draft ref's tree (`drafts/<name>`) for committed edits, then the workspace's working tree for uncommitted ones.
- It is built on the first request and cached under the version's stamp: the draft ref's sha plus a digest of the workspace's record files, as the published render is cached under its own stamp. The service's look every second lists the drafts (`git status` in each workspace, about 1 ms); it renders none.
- A version that does not parse serves its own render as the error page and its entry says "does not render"; the published pages and every other version stay 200.

**Diffs**

- Against published: by section. Each `##` or `###` section the draft changes is shown with a line diff of its Markdown; unchanged sections fold. `/drafts/<worktree>/<path>.diff.html`.
- Between two drafts: `/drafts/<a>/<path>.diff.html?vs=<b>`, the same view with draft B as the base. Both texts are already read for the version bar, so it costs one text diff of one file.

```mermaid
flowchart LR
  pub[("records branch")] -->|"page URL"| page["published version + version bar"]
  wa["workspace X: draft ref + working tree"] -->|"/drafts/x/…, rendered on request, cached by stamp"| va["version X"]
  wb["workspace Y"] -->|"/drafts/y/…"| vb["version Y"]
  page --> va
  page --> vb
  va -->|"diff by section"| page
  va -->|"diff ?vs=y"| vb
```

*Reading:* the page URL renders only the branch; each draft is a separate render at its own URL, so a broken draft fails only its own version.

- A published journal may link a design page that exists only in a draft (a new page in a sprint's Design pages); the site links the draft's version, and `pm check` accepts a link that resolves in a draft of this clone.
- The version bar applies to every record type: versions are built the same way whatever the record, and journals have drafts too (a delivery report in progress). Design pages are where several drafts of one page coexist, so they are where it matters most.

### Workspaces

- Each code worktree gets a workspace: a git worktree of the branch `drafts/<name>`, where `<name>` is the worktree's admin name (unique in the clone), checked out at `<worktree>/records` in place of today's link. `/records` is already git-ignored in code worktrees.
- The isolation unit is the code worktree: private to one session by the invariant, its branch is what merges, and `records/` already resolves per worktree. Alternatives considered says why not per session or per sprint.
- `drafts/<name>` is always a `records` commit (its base) plus at most one draft commit, which holds the worktree's committed, unpublished design-page edits.
- pm never runs `git rebase`, `git merge` or `git stash` in a workspace: it computes merges with `git merge-tree --write-tree` and `git merge-file`, writes commits with `git commit-tree`, moves refs with `git update-ref`, and writes files itself. No workspace is ever left mid-rebase or mid-merge: each operation runs under the store lock and either completes or changes nothing.
- **The main checkout** keeps the store as its records tree for journal hand edits, shared by its sessions as today; its uncommitted edits show as one draft labelled "main checkout". Design pages are edited only in a worktree: `pm commit` in the main checkout refuses a design page and names a worktree.
- **The Stop hook** checks the session's own workspace's uncommitted edits; in the main checkout it keeps the transcript match.

### Follow

Follow brings a workspace onto `records`. Every pm write from the worktree runs it first.

1. New draft tree: `git merge-tree --write-tree --merge-base <base> records <draft>`.
2. Each file that `records` changed since the base and that is uncommitted in the workspace: `git merge-file` of its base text, the working text and the `records` text. The merged text is written to the file, with conflict markers where hunks overlap.
3. A committed draft file that conflicts in step 1 leaves the draft commit: the file keeps the agent's text, merged with markers, as an uncommitted edit.
4. The ref moves to the `records` tip plus the new draft commit; files not uncommitted are refreshed from the new tree.

A file with conflict markers stays an ordinary uncommitted edit; `pm commit` refuses it, naming the `records` commits that touched it since the old base (`git log <base>..records -- <path>`). Step 2 rewrites a file the worktree's own sessions are editing, never another worktree's; a file tool that checks the file moved since it read it then rereads it.

### Writes and journal publish

Agents write `<worktree>/records/…` with file tools, inside their own worktree. Writes in the main checkout and the service's day summaries stay as today.

- **`pm commit` in a worktree**: follow; refuse a named file with markers; commit the named journal files on `records` in the store (the follow put them on its tip, so they apply cleanly); put named design pages into the draft commit; move the base to the new tip.
- **A one-shot write** commits on `records` in the store, computed on its newest HEAD as today, then follows. Each is a function of the current text, so two findings appended to one Findings section never conflict, while merged as text they would conflict on adjacent lines.
- A commit on `records` that would overwrite an uncommitted edit in the store (a main-checkout session's) is refused, naming the file.

### Publish at merge

| Sprint | Design pages published when | By |
|---|---|---|
| With a PR review | The service sees the PR merged (the existing 60 s look) and finds the worktree whose current branch is the PR's head | The service: follow's merge, then a commit on `records`. On a conflict it changes nothing and flags the draft on the site, in `pm show` and in the merge message it already delivers |
| Merged without a review, or no worktree on the head branch any more | `pm sprint close` | pm |
| No PR (a records-only sprint) | `pm sprint close`, or earlier by `pm design publish [<slug>…]` from the worktree | pm |

- `pm sprint close` refuses while a page the sprint's Design pages link has draft edits in any workspace, and publishes the closing worktree's. Those links are the sprint-to-draft map, so no commit carries a sprint id.
- **A conflict found after the code merged** leaves the page behind its code, never ahead. The session resolves it in its workspace and runs `pm design publish`; the sprint cannot close until it does.

### Covers

A published page changes only when its code is on main, so `pm check` fails on a published pattern that matches no file on the remote's main. The waiting list and the close's hold (`LiveOnMain`, `Own`, the author-date window) retire. In a workspace, `pm check` checks draft pages against that worktree's tracked files; the gate's "updated" reads the draft commit.

### Draft refs

The service pushes `records` as today, and each draft commit to `refs/pm/drafts/<clone>/<name>`, where `<clone>` is an id `pm init` generates once per clone: each clone is the only writer of its namespace. The draft commit is rewritten at every follow, so the push is forced with `--force-with-lease` against the sha this clone last pushed; a lease failure is flagged. The remote ref is deleted after the publish. Other clones fetch `refs/pm/drafts/*` and list them as committed drafts with no session.

### Setup

`pm init` replaces a worktree's link with `git worktree add <worktree>/records drafts/<name>`; no git hook of pm's runs there, and session start skips a `drafts/` checkout as it skips the store. Claude Code then writes inside its own worktree. Codex writes the files there too, but committing needs the workspace's git dir as one more writable root per worktree, unless pm runs outside the sandbox (`.codex/rules/pm.rules`).

### Failure modes and cost

- A code worktree removed by `git worktree remove`, by Claude Code at session end or by `git clean -ffdx` loses its uncommitted record edits; the draft commit survives on its branch and remote ref. `pm clean` refuses a worktree whose workspace has uncommitted edits, and `pm where` lists orphaned draft branches.
- A draft never published, or a publish conflict, is flagged and holds the sprint's close.
- About 2.6 MB per workspace (26 MB for this clone's 10 code worktrees), made in about 10 ms; a follow is a merge-tree and a few file writes under the store lock.

### The starting analysis's open questions

| Question | Answer |
|---|---|
| Per-session or per-sprint workspaces | Per code worktree (Workspaces) |
| The Stop hook's check | The own workspace's uncommitted edits; the main checkout keeps the transcript match |
| Generated records and sync | The service commits day summaries on `records`; Progress is rendered; workspaces follow at each pm write; the sync pushes draft refs |
| A draft's render without breaking the published page | Each version renders the published set plus one draft, alone, at its own URL |
| Cost of drafts and stale workspaces | About 1 ms per worktree per look; a version's render at most 0.20 s, on request, then cached by stamp; a workspace lives with its worktree until `pm clean` |

### Increments

| Step | Builds | Check |
|---|---|---|
| 1 | Workspaces, follow, journal publish at `pm commit`; the published view, the version bar, per-version renders and diffs | Scratch clone with two worktrees: each edits one design page differently; the page shows three versions, each renders at its URL, each diff against published and the draft-to-draft diff show exactly the edited lines; a malformed draft fails only its own version, every published page 200. Disjoint edits to one sprint record from both reach `records`; an overlap is refused naming the first commit, markers only in the second workspace |
| 2 | Stop hook on the own workspace; `pm clean` and `pm where` guards; the main checkout's design-page refusal | The Stop hook blocks only the editing session; `pm clean` refuses a worktree whose workspace has uncommitted edits |
| 3 | Publish at merge, `pm design publish`, the close's draft check; covers waiting and hold retired | Fake `gh` reports a merge: the page reaches `records` within one 60 s look, also for a branch cut inside the worktree; a conflict is flagged and the close refuses until `pm design publish` |
| 4 | Per-clone draft refs | A second clone lists the first's draft as a version after two follows and one sync each |

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Why not |
|---|---|
| A per-file version check in `pm commit`: refuse a file that moved since this session's last write | Rejected by the owner: it refuses the second writer without separating the two sessions' edits, works per file not per hunk, and pm cannot see edits made with file tools without a new hook |
| Per-session workspaces | `records/` is one path for every session in a worktree, the main checkout's included: file tools cannot see the session id. A session is not a PR: it runs several sprints, and its subagents share its id |
| Per-sprint records branches, as the starting analysis proposed | A sprint spans worktrees and PRs, so its sessions would share one tree again; file tools cannot choose a sprint's tree |
| Following `records` with `git rebase --autostash` | Rebase refuses a dirty tree, the stash pop can conflict, and a stopped rebase outlives the command's lock |
| OT or CRDTs (Google Docs, Automerge, Yjs) | They merge keystrokes; agents make discrete edits to sectioned Markdown, which git's three-way merge handles |
| Dolt for records | Loses hand-editable Markdown and git tooling ([Shared records store](records-store.md)) |
| Jujutsu (jj): conflicts as first-class objects | A second VCS on every machine and runtime; its gain, never refusing a writer, is what the overlap refusal gives up on purpose |
| Design pages in the code tree (`design/*.md` on main), journals staying in the shared store | Not chosen by the owner. Its real advantage: a page travels in its code PR, so conflicts surface on GitHub before the merge and the merge publishes it by construction. But journals keep the shared tree, so goals 1, 2 and 6 go unmet for them; it reverses the rule that no code branch tracks records; and every repo using pm needs a migration of its pages and their history |
| The same, with journals in per-worktree workspaces | Meets every goal, but needs this design's workspaces and follow anyway, plus the migration |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

| Source | Idea | Taken by |
|---|---|---|
| MVCC, snapshot isolation | Each transaction writes privately and publishes at commit | Workspaces |
| Optimistic concurrency control (Kung and Robinson, 1981); HTTP `If-Match` (RFC 9110) | A write carries its base; a stale base is refused | The overlap refusal, by hunk |
| Preview deployments (Vercel, Netlify); CMS draft and publish (WordPress revisions, Contentful environments) | Readers see published work; each branch or draft gets a marked preview at its own URL | Versions beside the published page |
| Architecture Decision Records (Nygard, "Documenting Architecture Decisions"), kept in the code repository; the Linux kernel's and Git's in-tree `Documentation/` | Design text changes in the same commit and review as the code | The alternative not chosen: design pages in the code tree |
| Python PEPs, Kubernetes KEPs, Go proposals, Rust RFCs, each in a repository of its own | A design document is reviewed and merged in its own repository, apart from the code that implements it | The separate records store; this design adds the publish at the code's merge, which these processes do not have |
| Git's three-way merge, `git merge-tree`, `git merge-file` | Disjoint hunks merge; overlaps conflict | Follow and journal publish |

## Open questions

> What is still unresolved?

- Whether `claude -w` isolation refuses writes to a git worktree nested inside the session's own worktree; a live check decides whether workspaces work there.
- Whether pm commits a workspace from inside Codex's sandbox (one writable root per workspace) or only through `.codex/rules/pm.rules`.
- Whether the site should show other machines' committed drafts, which carry no live session.
