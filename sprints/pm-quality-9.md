---
type: sprint
title: Rename a sprint, edit a need, stamp an agent's merge
bead: pm-d2k5.9
---

## Goal

> What should be true when this sprint ends, and why now?

Four write-path gaps close:
- A sprint whose goal changed can take a matching title. Today `task edit` refuses a sprint id and `sprint` has only open and close (pm 2026-10-10 02:14).
- An open need's body can be edited (formal-methods 2026-10-10 03:23).
- A sprint whose PR an agent merged closes with its "Merged as" stamp. Today the stamp comes only from a closed review need (formal-methods 2026-10-07 20:45). The owner decided that agents merge pm-quality's own PRs (project decision, 2026-10-10), so every pm-quality sprint now closes without the stamp.
- `pm commit` accepts a path relative to the store, such as `sprints/x.md` (pm-quality doc 2026-10-10 04:31).

Raised by the pm feedback triage of 2026-10-10 across pm, formal-methods, ai-safety and yeeef-agents.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm sprint edit ID --title "…"` with a reason body. It changes the work-store title and the record's `title:` header in one write, and records the reason as a sprint decision.
- `pm need edit ID --text-file -` for an open action or decision need. It refuses once the need holds a reply.
- `pm sprint close ID --merged SHA [--pr URL]`. The SHA must be on the remote's main. The stamp is the one a review close writes. It is refused when the sprint holds a review need.
- `pm commit` resolves `sprints/…`, `docs/…` and other store-relative paths as it resolves `records/…`.
- `--help`, prime.md's close line, and a CHANGELOG entry.

**Out:**
- An explicit order for needs. `pm dep add` already orders them, and S1 makes it visible.
- Editing closed items.

## Done when

> What evidence will show the goal is met?

- Harness tests:
  - `pm sprint edit` changes both titles and adds the decision;
  - `pm need edit` changes the body of an open need and refuses one that holds a reply;
  - `pm sprint close --merged <sha on main>` stamps "Merged as <sha>", a SHA not on main is refused with `repo.unchanged()`, and a sprint that holds a review need refuses `--merged`;
  - `pm commit -m … sprints/x.md` commits `records/sprints/x.md`.
- `make test` and the PR's CI pass.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
`pm need edit` takes an action's body with `--text`/`--text-file`, and a decision need's parts with the flags `pm decision need` takes (`--question`, `--fact`, `--option`, `--cost`, `--default`), not a free `--text-file` body.
A decision need has one checked layout (question, facts, options with costs, default, 25-word sentences); a free body would bypass those checks, so the edit rebuilds it through the same code.
:::

::: decision {source=agent date=2026-10-10}
The work store itself refuses a new description for a need that is closed or holds a reply (`Dolt.Edit`, inside the write transaction), besides the CLI refusal.
A reply that lands between the CLI check and the write would otherwise let the body change under an answer; the store check closes that window on one clone.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The store already kept a sprint title's 'Sprint <n>: ' prefix in step with
  its number (work/merge.go on a concurrent edit, work/move.go on a move), so
  pm sprint edit only had to write the prefix it found; the record header
  holds the bare title, as pm sprint open writes it.

- Before this sprint, pm commit joined any relative path other than
  records/... to the cwd, so sprints/x.md from the main checkout or a worktree
  resolved outside the store and was refused; the fallback now resolves a
  relative path to the store when the store holds a change or a file there,
  and leaves every other refusal as it was (e.g. .gitignore from the main
  checkout).

- A decision need's body is pm's own layout (Question, Facts, Options with
  costs, Default; 25-word sentences), built only by needMarkdown; the site
  does not parse it, so a free-text edit would have rendered but bypassed
  every check. pm need edit takes the parts flags instead (sprint decision).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the four write-path gaps are closed in PR #33, pending merge: a sprint can be renamed, an open need's body edited, an agent's merge stamped at close, and a store-relative path committed.

- `pm sprint edit ID --title "…"`: work-store title (its `Sprint <n>: ` kept), record `title:` header and a sprint decision with the reason, in one records commit; the old title is put back if the records step fails.
- `pm need edit ID`: an action's description, or a decision need's parts with `pm decision need`'s flags and checks; refused once the need holds a reply or is closed, and for a PR review. The work store's `Edit` refuses the same inside its write transaction.
- `pm sprint close ID --merged SHA [--pr URL]`: fetches the remote's main, requires the SHA on it, stamps `Merged as <sha> (PR #N).` as a review close does; refused when the sprint holds a review.
- `pm commit sprints/x.md` from anywhere commits `records/sprints/x.md`.
- `--help` texts, `prime.md`, a `changelog.d/` entry and the [pm CLI design page](../design/pm-cli.md) updated.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Harness tests: met.
  - `pm sprint edit` changes both titles and adds the decision: `test_sprint_edit_renames_the_sprint_in_both_stores_and_records_why` (work-store change `Sprint 1: Parse: tables too`, record header, decision block, and five refusals with `repo.unchanged()`).
  - `pm need edit` changes an open need's body and refuses one that holds a reply: `test_need_edit_rewrites_an_open_need_s_body_until_the_owner_replies` (action and decision edited; reply, closed, review, wrong body form and review-ask refused); Go `TestEditKeepsAnAnsweredNeedsDescription` for the store guard.
  - `pm sprint close --merged <sha on main>` stamps "Merged as <sha>": `test_sprint_close_merged_stamps_an_agent_s_merge_on_main` (integration, bare origin): stamp `Merged as <sha7> (PR #12).`, commit `[SPRINT] demo sprint 1: closed, merged as <sha7>`; a SHA not on main, a non-hex SHA and `--pr` alone refused with `repo.unchanged()`; a sprint holding a review refuses `--merged`.
  - `pm commit -m … sprints/x.md` commits `records/sprints/x.md`: `test_commit_takes_a_path_relative_to_the_store_from_anywhere` (from the main checkout and a worktree; `.gitignore` still refused).
- `make test` and the PR's CI pass: after the rebase on main da3423c, `make test` 145 passed, 63 skipped and the touched integration tests (`-k 'sprint_close or close_merged or commit or sprint_move or need_edit or sprint_edit'`) 17 passed; `make test-go` exit 0 before it; PR #33 CI green on head c672fcc (light, integration, changelog, build-vet-test, race and work on linux-amd64 and darwin-arm64), and `make merge-ready PR=33` says ready.
