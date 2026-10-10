---
type: doc
title: pm feedback triage
date: 2026-10-10
project: pm-quality
---

# pm feedback triage, 2026-10-10

Read-only triage of every `### ` entry in the five feedback docs against Yeeef/pm `origin/main` at b13e5bf (release 0.4.0 plus `[Unreleased]`). Code references are `origin/main` paths. "In flight" means the pm-quality sprints 1-6, pm-site sprint 1, pm-codex sprint 1 and pm-harness sprint 68 (the open sprints `pm show --project` lists).

Notes before the table:

- The yeeef-agents doc (18 entries) is a copy of the first 18 entries of the pm-harness doc in this repo; the bodies match after the session ids are removed (`diff`). Its rows give the same verdict as the pm-harness row they copy. Counted once each: 39 distinct entries.
- formal-methods, ai-safety, yeeef-agents and this repo all still pin 0.3.0 (`.pm/config.toml`). So 0.4.0's fixes (the request rule, the judge prompt, its cases) are released but do not run in any of these repos yet.
- Judge verdicts were not run live. A FIXED verdict for a judge entry rests on a labelled case in `tests/owner_request_cases.json`, which `make test-live` checks. An OPEN verdict means that the reported sentence is in no case, so nobody has measured how the current judge handles it.

## 1. Verdicts

| Repo | Entry | Gist | Verdict | Evidence |
|---|---|---|---|---|
| pm | 2026-10-06 23:10 (a) | A long-lived feedback doc must carry a date in its name | COVERED | pm-quality sprint 5: one feedback doc per repo; its Scope settles how a repo-level doc validates |
| pm | 2026-10-06 23:10 (b) | `pm task claim --help` says `--session` applies only with no env id; the code takes the flag first | OPEN → S1 | `internal/cli/commands.go:328` help text, against `cmdTaskClaim` in `writes.go:880`, which uses the flag first |
| pm | 2026-10-07 03:43 | Judge blocked a stated plan ("When they report I'll merge pm-service into pm-init") | OPEN → S4 | The prompt has "not asked … the agent's own plan", but "offers to merge … is never authorized" can catch the word "merge". No case holds this sentence |
| pm | 2026-10-07 13:25 | `pm task add` hung reading stdin from a socket | FIXED | pm reads stdin only for `--text-file -`, and refuses stdin that is not a pipe or a file (`internal/cli/store_commands.go:244`; CLAUDE.md "pm reads stdin only for `--text-file -`") |
| pm | 2026-10-07 19:50 | `pm init` crashed on a busy port; every repo defaulted to 8000 | FIXED | The default is the first free port that no pm unit names (`service.FreePort`, `install.go:270`). A held port refuses before any write: "pm init wrote nothing. Run PORT=<free> pm init" (`service/lifecycle.go:156`) |
| pm | 2026-10-08 00:01 | Judge blocked how-to instructions that answered the owner's own question | OPEN → S4 | No case. Under 0.4.0's stricter rule, the label of an instruction that answers the owner's question is undecided |
| pm | 2026-10-08 02:45 | Judge flagged "Still waiting on the map…", a status line about the agent's subagent | OPEN → S4 | No case. "A statement that something waits on the owner" in Step 1 can catch it |
| pm | 2026-10-08 04:17 | A release bump commit fails, because the new pin's tag does not exist yet | FIXED | Go releases have no bump commit: the version comes from the tag (`release/build.sh`, `-ldflags …buildinfo.Version`), and a repo moves its pin in a later PR once the release exists (CLAUDE.md, Releasing pm) |
| pm | 2026-10-08 04:51 | Worktree-isolated subagents cannot raise a PR review before the report, and cannot write records | INVALID | The order is deliberate: the review is raised once the report is written, so the owner reads it beside the diff (`needs.go:477-509`). pm's rule is that a subagent works in its parent's worktree, and the parent session holds the sprint, writes the report and raises the review |
| pm | 2026-10-08 20:41 | Judge flagged a reply that pointed, by id, at needs this session had raised | FIXED | Cases "describes an open need without asking" (pass) and "matched decision, cites its id" (pass) in `tests/owner_request_cases.json`. Those are the session's own needs, which `openRequests` passes to the judge (`internal/cli/ownerrequest.go`) |
| pm | 2026-10-08 21:47 | Judge flagged conditional facts ("takes effect once your installed pm includes…") | OPEN → S4 | No case. Since 0.4.0, a statement that makes the next step depend on the owner counts as an "action", so these sentences need labels |
| pm | 2026-10-08 22:09 | `pm sprint open` crashed on project children with flat ids | FIXED | Go numbering checks for a dot before it parses a suffix (`writes.go:452-458`) and uses typed sprint numbers. Items directly under a project show in the site's "Not in a sprint" section (`site/site.go:240`) and last in `pm task ready` |
| pm | 2026-10-09 04:27 | `pm sprint open` ignored a frame on stdin | FIXED | Bodies are `--text-file - <<'EOF'` or `--text=` everywhere. prime.md "Bodies" says so |
| pm | 2026-10-09 16:06 | `pm task move` refuses a task directly under a project | OPEN → S2 | `cmdTaskMove` refuses: "is not in a sprint with a record" (`writes.go:977-982`) |
| pm | 2026-10-09 19:23 | `pm task close --commit` resolves commits in this repo only | OPEN → S2 | `rev-parse --verify` runs in `r.root` only (`writes.go:829-834`) |
| pm | 2026-10-10 01:00 | Judge blocks asks in a chat with no sprint | FIXED | 0.4.0 settled the rule: ask with AskUserQuestion when the owner is in the chat (prime.md:97; `prompts/owner_request_reason.txt`). The three sentences are labelled `block` cases ("2026-10-10 chat: …"), and "reports a question it asked with AskUserQuestion" passes |
| pm | 2026-10-10 01:35 | No mid-sprint PR review for a prerequisite PR | INVALID | Each sprint delivers one PR, reviewed with its report. A prerequisite PR is a small sprint of its own ("work outside any sprint becomes a small new sprint"), and that sprint raises its review once its report is written |
| pm | 2026-10-10 02:14 | No way to rename a sprint whose goal changed | OPEN → S3 | The `sprint` noun has only `open` and `close` (`commands.go:271-296`). `task edit` refuses a sprint id (`c.item(id, work.Task)`, `store_commands.go:475`) |
| pm | 2026-10-10 03:56 | Judge blocked a reply that only reported a need held in another clone | OPEN → S4 | `openRequests` reads only this session's needs from the clone of the cwd (`ownerrequest.go:11-33`). The sentence should be "not asked" whatever the list holds, but no case covers it |
| pm | 2026-10-10 03:59 | Four snags: `need` missing from help; `decision close` stores the body as the owner's answer; `--commit` still warns about a dirty tree; `decision add` refuses an uncommitted edit | OPEN → S1, S2 | (1) `need`, `dep`, `task ready/edit/release`, `comment`, `reply add` and `sync` are dispatched outside the command tree (`work.go:goOnly`), so `pm --help` omits them and `pm need --help` errors. (2) `pm need dismiss` is the moot close, but nothing points to it. (3) The dirty-tree warning runs even with `--commit` (`writes.go:852-858`). (4) INVALID: the refusal is deliberate and names the fix (`agent.go:185`) |
| pm | 2026-10-10 04:00 | `pm task close` closes tasks held by live sessions; no project move-out | OPEN → S2 | `cmdTaskClose` has no holder check, while claim has one (`writes.go:916`). Move-out is partly done: 0.4.0 `pm init --import` loads into an empty store, and pm-quality sprint 4 adds `pm sprint move` |
| pm | 2026-10-10 13:04 | Stop hooks and pre-commit refuse in a worktree whose branch lacks `.pm/config.toml` | FIXED | `.pm/config.toml` is tracked on main since b4f96a6 (merged in #11, 04:36 UTC), so a branch cut from main has it. Failing open on the hook's own error is deliberate (CLAUDE.md "Hooks fail open") |
| pm | 2026-10-10 04:31 (pm-quality doc) | No `pm sprint move`; `pm commit` refuses a store-relative path | COVERED (+ OPEN part → S3) | pm-quality sprint 4. Separately, `pm commit sprints/x.md` resolves against the cwd, not the store (`writes.go:1060-1064`) |
| pm | 2026-10-10 04:39 (pm-quality doc) | Sprint move took six steps, and a mid-move service restart left it half done | COVERED | pm-quality sprint 4 (all or nothing, and the old id resolves) |
| formal-methods | 2026-10-07 20:24 | No need can confirm a goal before a project exists; `site_url` is hard to find | FIXED | 0.4.0: ask with AskUserQuestion when the owner is in the chat, and prime's "open a project" step waits for the owner's confirmation in their own words. prime.md:136 names `pm init --site-url URL` |
| formal-methods | 2026-10-07 20:33 | Agent asked design questions with AskUserQuestion and not as needs | FIXED | 0.4.0 made AskUserQuestion the sanctioned chat path and defines "clarification" as a request (prompt Step 2). Its wish for a need under the project epic is INVALID: the block reason says to open a task or sprint for it |
| formal-methods | 2026-10-07 20:34 | No "dropped" close for a task; no hidden records; homework as actions | OPEN (part) → S2 | No not-done close: `cmdTaskClose` always closes as `work.Done` (`writes.go:868`). Hidden records: INVALID, because the site is the owner's one interface. Homework as actions: INVALID, because the action need fits it as written |
| formal-methods | 2026-10-07 20:45 (a) | Projects where agents merge lose the "Merged as" stamp; the merge notice pulls into a non-main checkout | OPEN (part) → S3 | The stamp comes only from closed review needs (`writes.go:618-650`). The owner chose agent merges for pm-quality itself. The pull part is INVALID: since Go pm, the main checkout stays on main, because claim refuses there (`writes.go:893`) |
| formal-methods | 2026-10-07 20:45 (b) | `pm hook stop` blocked the parent over a record a running subagent was writing | OPEN → S4 | `Touched` scans the main transcript, where the Agent call's prompt names the path (`hooks/stop.go:121`). It blocks once per stop |
| formal-methods | 2026-10-07 20:51 | No `pm record preview` for light, dark and phone checks | INVALID | Go prime.md no longer asks for a visual check, so the rule this served is gone. The page has a viewport meta and responsive rules (`site.go:704`, `style.css`). The clipping at `--window-size=390` matches headless Chrome's minimum window width, not a site bug |
| formal-methods | 2026-10-07 20:58 | `pm record link` gives the Access-protected public URL; no local link | OPEN → S5 | `cmdRecordLink` prints `site_url` whenever one is set and has no local form (`show.go:115-150`) |
| formal-methods | 2026-10-09 15:43 | Go cut-over notes: README and codex labels name Beads; no `pm service stop`; runbook gaps | OPEN (part) → S5 | (1)(2) FIXED in 0.2.1 (a32ce1b): `.pm/README.md` installs with `install.sh`, and the codex `statusMessage`s name pm. (3) OPEN: `service` has only install, status, restart, logs and run. (4) Demanding a need for the owner's step is the rule. (5)-(7) concern the one-time Beads runbook, which no longer applies |
| formal-methods | 2026-10-10 00:24 | `pm doctor` errors on a `.beads` file, and exits 0 | FIXED | 0.2.2 (a5d7580): a path under a file reads as absent (`install/pieces.go:882`). Errors now print `error:` and exit 1 (`cli.go:455`) |
| formal-methods | 2026-10-10 00:29 | 0.3.0's release notes said nothing | FIXED (+ OPEN part → S5) | Each release's notes are now its CHANGELOG section, and the release workflow refuses a tag without one. `gh release view pm-v0.3.0` now shows the backfilled notes. "doctor says a newer release exists" is not built |
| formal-methods | 2026-10-10 00:54 | `pm uninstall` refuses with the service down; no way to keep a service off | OPEN → S5 | `WorkUnsynced` dials the service (`install/workstore.go:202`). Session start reinstalls a missing service and starts an installed one that is down (CLAUDE.md) |
| formal-methods | 2026-10-10 03:23 | `pm dep` is "not a command"; needs cannot be edited or ordered | OPEN → S1, S3 | `pm dep` prints "invalid choice: 'dep'" (run on 0.3.0, and main's tree is the same), yet `pm dep add` works and prime names it. No command edits a need's body: `task edit` takes tasks only |
| formal-methods | 2026-10-10 03:31 | Judge flagged a status line restating open needs raised by an earlier session | OPEN → S4 | The list holds only this session's needs (`ownerrequest.go`). A restatement should be "not asked", but no case covers another session's need |
| ai-safety | 2026-10-07 20:41 | Mermaid shrinks wide diagrams, uses an unpinned `mermaid@11` with no fallback, and reads the theme only once | OPEN → S6 | `site/site.go:716-720`: `mermaid@11`, default `useMaxWidth`, and the theme set once in `initialize` |
| ai-safety | 2026-10-07 20:58 | The site serves no image files, and `style.css` has no img or figure rule | OPEN → S6 | No image content type or img rule in `internal/site/serve.go`, `internal/cli/serve.go` or `style.css` |
| yeeef-agents | 2026-10-06 23:10 (a) | Same entry as the pm doc | COVERED | As the pm row |
| yeeef-agents | 2026-10-06 23:10 (b) | Same | OPEN → S1 | As the pm row |
| yeeef-agents | 2026-10-07 03:43 | Same | OPEN → S4 | As the pm row |
| yeeef-agents | 2026-10-07 13:25 | Same | FIXED | As the pm row |
| yeeef-agents | 2026-10-07 19:50 | Same | FIXED | As the pm row |
| yeeef-agents | 2026-10-08 00:01 | Same | OPEN → S4 | As the pm row |
| yeeef-agents | 2026-10-08 02:45 | Same | OPEN → S4 | As the pm row |
| yeeef-agents | 2026-10-08 04:17 | Same | FIXED | As the pm row |
| yeeef-agents | 2026-10-08 04:51 | Same | INVALID | As the pm row |
| yeeef-agents | 2026-10-08 20:41 | Same | FIXED | As the pm row |
| yeeef-agents | 2026-10-08 21:47 | Same | OPEN → S4 | As the pm row |
| yeeef-agents | 2026-10-08 22:09 | Same | FIXED | As the pm row |
| yeeef-agents | 2026-10-09 04:27 | Same | FIXED | As the pm row |
| yeeef-agents | 2026-10-09 16:06 | Same | OPEN → S2 | As the pm row |
| yeeef-agents | 2026-10-09 19:23 | Same | OPEN → S2 | As the pm row |
| yeeef-agents | 2026-10-10 01:00 | Same | FIXED | As the pm row |
| yeeef-agents | 2026-10-10 01:35 | Same | INVALID | As the pm row |
| yeeef-agents | 2026-10-10 02:14 | Same | OPEN → S3 | As the pm row |

::: result {title="Verdict counts"}
| Verdict | Distinct entries (39) | All rows (57, with the yeeef-agents copies) |
|---|---|---|
| FIXED | 12 | 19 |
| COVERED | 3 | 4 |
| INVALID | 3 | 5 |
| OPEN | 21 | 29 |
:::
Rows marked "part" count under their main verdict. Three FIXED or COVERED entries also carry an OPEN part, which goes into S3 or S5.

## 2. Proposed pm-quality sprints

Each sprint stands alone. S1 makes `pm need` and `pm dep` real nouns, so S3's `pm need edit` is easier after S1, but it does not depend on it.

### S1. Every pm command is listed in `pm --help`

## Goal

`pm --help`, each `pm <noun> --help` and `pm prime`'s noun list name every command pm runs, and each help text says what the code does.

Today `goOnly` (`internal/cli/work.go`) dispatches `task ready/edit/release`, `dep add/rm`, `comment add`, `need dismiss`, `reply add`, `sync`, `show ID`, `version` and `export` outside the command tree. So `pm dep` answers "invalid choice: 'dep'", although prime.md says to run `pm dep add` (formal-methods 2026-10-10 03:23). `pm need dismiss`, the close for a replaced review or a moot need, was found only by grepping the source (pm 2026-10-10 03:59). `pm task claim --help` says `--session` applies only when the environment has no session id, but the code uses the flag first (pm 2026-10-06 23:10).

## Scope

**In:**
- Move the store commands into `commands.go`'s tree with unchanged flags and behaviour, so they show in `pm --help`, in `pm <noun> --help` and in `hooks.Commands`' noun list.
- `pm dep` and `pm need` with no subcommand print their noun's help.
- `pm decision close --help`, and its refusal on a need the owner never answered, name `pm need dismiss` for a need that became moot.
- Fix the `--session` help of `pm task claim`.
- A test that fails when a command the dispatcher runs is missing from the `pm --help` tree.
- A CHANGELOG entry.

**Out:**
- New commands, and renamed ones.
- Hand edits to prime.md beyond what the generated noun list adds. If the longer list outgrows a chunk, move a heading in `hooks.Starts`.

## Done when

- `pm dep --help` and `pm need --help` exit 0 and list their subcommands. `pm --help` lists `dep` and `need`, and `pm task --help` lists `ready`, `edit` and `release`.
- The new test walks `storeCommands` and `goOnly`'s names and passes. Removing one command from the tree makes it fail.
- `pm prime` lists `dep` and `need`, and `test_rules_chunks_fit_the_cap_and_add_up_to_the_rules` passes.
- A harness test shows that `pm task claim --session X` records X even with `$CLAUDE_CODE_SESSION_ID` set, and the help text says so.
- `make test`, `make test-go` and the PR's CI pass.

Tasks:
- Move the store commands into the command tree
- Test: every command pm dispatches is in pm --help
- Point pm decision close at pm need dismiss for a moot need; fix pm task claim's --session help

### S2. pm task close and move cover dropped, held, foreign-commit and orphan tasks

## Goal

Closing or moving a task records what happened, with no stand-in commits and no silent override of another session.

Today:
- `pm task close` records every close as done and asks for a commit, so a dropped task is closed with an unrelated records commit (formal-methods 2026-10-07 20:34; pm 2026-10-10 03:59).
- It warns about a dirty tree even when `--commit` names the work (pm 2026-10-10 03:59).
- It resolves `--commit` only in this repo (pm 2026-10-09 19:23).
- It closes a task that another live session holds, which `pm task claim` refuses (pm 2026-10-10 04:00).
- `pm task move` refuses a task directly under a project. The site lists such tasks under "Not in a sprint", and the rules say to bring them into a sprint (pm 2026-10-09 16:06).

## Scope

**In:**
- A not-done close: `pm task close ID --dropped` with a required reason and no commit, shown as dropped in `pm show` and on the site. Choose before any code whether this is a new resolution, which every clone's store check (`work/check.go:85`) must accept so an upgrade step follows, or the existing `dismissed`. Record the choice as a sprint decision.
- Warn about the dirty tree only when `--commit` is not given.
- `--commit OWNER/REPO@SHA` and a PR URL, resolved with `gh`. One that does not resolve is refused.
- `pm task close` refuses a task that another live session holds, with the same liveness check and wording as claim, and names `pm task release`.
- `pm task move` from directly under a project into one of that project's open sprints, recording the scope added as a decision in the sprint it joins.
- `--help` texts, prime.md's close and move lines, and a CHANGELOG entry.

**Out:**
- Moving a whole project to another repo (0.4.0 `pm init --import` and pm-quality sprint 4).
- A backlog object.

## Done when

- Harness tests:
  - a `--dropped` close writes its resolution and reason, warns nothing and needs no commit;
  - a close with `--commit` in a dirty tree prints no warning;
  - `--commit Yeeef/pm@<sha>`, answered by `fake_gh`, puts that commit in the reason, and an unknown one is refused with `repo.unchanged()`;
  - a close of a task that another live session holds is refused with `repo.unchanged()`;
  - a project-level task moves into a sprint, and that sprint's record gains the decision.
- `make test` and the PR's CI pass.

Tasks:
- Decide and build the dropped close of a task
- pm task close: --commit in another repo or a PR, and no dirty-tree warning when --commit is given
- pm task close refuses a task another live session holds
- pm task move takes a task from directly under a project into a sprint

### S3. Rename a sprint, edit a need, stamp an agent's merge

## Goal

Four write-path gaps close:
- A sprint whose goal changed can take a matching title. Today `task edit` refuses a sprint id and `sprint` has only open and close (pm 2026-10-10 02:14).
- An open need's body can be edited (formal-methods 2026-10-10 03:23).
- A sprint whose PR an agent merged closes with its "Merged as" stamp. Today the stamp comes only from a closed review need (formal-methods 2026-10-07 20:45). The owner decided that agents merge pm-quality's own PRs (project decision, 2026-10-10), so every pm-quality sprint now closes without the stamp.
- `pm commit` accepts a path relative to the store, such as `sprints/x.md` (pm-quality doc 2026-10-10 04:31).

## Scope

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

- Harness tests:
  - `pm sprint edit` changes both titles and adds the decision;
  - `pm need edit` changes the body of an open need and refuses one that holds a reply;
  - `pm sprint close --merged <sha on main>` stamps "Merged as <sha>", a SHA not on main is refused with `repo.unchanged()`, and a sprint that holds a review need refuses `--merged`;
  - `pm commit -m … sprints/x.md` commits `records/sprints/x.md`.
- `make test` and the PR's CI pass.

Tasks:
- pm sprint edit --title, with the reason as a sprint decision
- pm need edit for an open need's body
- pm sprint close --merged for a PR an agent merged
- pm commit accepts store-relative paths

### S4. The Stop hooks block only what is the agent's to fix

## Goal

The owner-request judge passes the non-requests agents reported, and does so on labelled cases that `make test-live` measures. `pm hook stop` no longer blocks a parent over a record that its running subagent is writing.

None of the reported sentences is in `tests/owner_request_cases.json` (40 cases), so the current judge's verdict on them is unmeasured:
- a stated plan (pm 2026-10-07 03:43);
- how-to instructions that answer the owner's own question (pm 2026-10-08 00:01);
- a "still waiting on" status line about the agent's own subagent (pm 2026-10-08 02:45);
- conditional facts (pm 2026-10-08 21:47);
- a line reporting a need held in another clone (pm 2026-10-10 03:56);
- a summary restating open needs raised by an earlier session (formal-methods 2026-10-10 03:31).

The stop-hook case is formal-methods 2026-10-07 20:45.

## Scope

**In:**
- Each reported sentence becomes a labelled case, its name citing the entry. Its label comes from the request rule in prime.md, never from the judge's answer.
- Where the rule does not settle a label, raise a decision need before labelling. Two cases need one: instructions that answer the owner's own question, and a restated need that is not in the judge's list.
- Decide whether the judge's OPEN REQUESTS list also carries the clone's other open needs, marked as another session's, so a restatement can be checked against them. Today the list holds only this session's needs (`internal/cli/ownerrequest.go`).
- Change the prompt and the block texts until every case passes. One rule lives in three texts, so change them in one commit.
- `pm hook stop` leaves out a path named only in the prompt of a subagent call that has not returned.

**Out:**
- Reading needs from other clones on the machine.
- Codex (pm-codex sprint 1).
- Checking AskUserQuestion calls.

## Done when

- `tests/owner_request_cases.json` holds at least 7 new cases, one per reported sentence.
- `make test-live` with `PM_LIVE_RUNS=3` passes every case 3/3, the new cases and all 40 existing ones, the chat-request block cases included. Pass counts and latency go in Findings.
- A harness test of `pm hook stop` shows no block for a path named only by a subagent call that has not returned, and a block for the same path edited by the session's own tool call.
- `make test` and the PR's CI pass.

Tasks:
- Label the reported judge false positives as cases, raising a need where the rule is silent
- Change the judge prompt and texts until make test-live passes every case
- pm hook stop: skip a path named only by a running subagent's call

### S5. A clone's pm service can be stopped, uninstalled and linked locally

## Goal

The owner or an agent can:
- stop a clone's service and keep it stopped;
- uninstall pm while the service is down;
- get a record's localhost link;
- learn that a newer release exists.

Today:
- Stopping the service needs `launchctl` or `systemctl` by hand, which agents may not run, and the owner's own `!` runs did not take (formal-methods 2026-10-09 15:43).
- Session start reinstalls or restarts a stopped service, and `pm uninstall` refuses until the service answers (formal-methods 2026-10-10 00:54).
- `pm record link` prints the public URL behind Cloudflare Access, which a headless check cannot open (formal-methods 2026-10-07 20:58).
- Nothing says that a release newer than the pin exists (formal-methods 2026-10-10 00:29).

## Scope

**In:**
- `pm service stop`: stops and disables the unit, then checks that the socket no longer answers and says so.
- Session start leaves a stopped service alone, and the state it injects names `pm service restart`. A typed `pm init` or `pm service restart` starts it again.
- `pm uninstall` with the service down checks for unsynced work without breaking the one access path (`internal/work/access_test.go`), for example by starting the service for the check. Record the choice in a design page section.
- `pm record link --local` prints `http://127.0.0.1:<port>/…` from the clone's unit port.
- `pm doctor` names a release newer than the pin, with its notes link. When the release list cannot be read, it says so; it never guesses.
- `pm service --help` and a CHANGELOG entry.

**Out:**
- Cloudflare Access (pm-harness sprint 68).
- Moving a clone between machines.

## Done when

- Harness tests with `fake_sched`:
  - stop disables the unit and the socket stops answering;
  - `pm init --session-start` then leaves the service stopped, and `pm service restart` brings it back;
  - `pm uninstall` with the service stopped succeeds on a synced store and refuses with an unsynced one;
  - `pm record link --local` prints the localhost URL while `site_url` is set;
  - `pm doctor` against a local release server that holds a newer release prints the line.
- These integration tests pass under `make test-full ARGS="-k …"` and in CI.
- A live check in a scratch clone under a temp `HOME` runs stop, then session start, then restart, and its output goes in Findings.

Tasks:
- pm service stop, kept stopped by session start
- pm uninstall with the service down
- pm record link --local
- pm doctor names a newer release

### S6. Site diagrams and images read on a phone

This sprint could equally go to the pm-site project.

## Goal

A wide Mermaid diagram stays legible at phone width and survives a CDN blip and a theme switch, and a record can show an image file kept next to it.

Today the site loads `mermaid@11` with no exact version and no fallback, lets `useMaxWidth` shrink wide flowcharts to 3-6 px text at 390 px, and sets the theme once at load (`internal/site/site.go:716-720`) (ai-safety 2026-10-07 20:41). The site serves no image files from the store and `style.css` has no img or figure rule, so figures are inlined as base64 (ai-safety 2026-10-07 20:58).

## Scope

**In:**
- Pin an exact Mermaid version.
- `flowchart.useMaxWidth=false`, with `pre.mermaid` scrolling sideways.
- Diagrams re-render when the colour scheme changes.
- The service serves `.svg`, `.png`, `.jpg` and `.webp` files from the records store with their content types, confined to the store.
- `pm commit` and `pm check` accept such files.
- An `img`, `figure` and `figcaption` rule in `style.css`.
- Golden pages updated.

**Out:**
- Serving Mermaid from the binary, unless the pin alone does not answer the blip. Record that as a decision.
- Image processing.

## Done when

- A golden page with a diagram loads the pinned version with `useMaxWidth` off.
- A serve test gets `200 image/svg+xml` for `records/docs/x.svg` and `404` for a path escaping the store.
- A live check at a 390 px device-emulated viewport, in light and then dark, shows the wide diagram scrolling and the figure inside the column (screenshots in Findings).
- `make test`, `make test-go` and the PR's CI pass.

Tasks:
- Mermaid: pinned version, no max-width shrink, re-render on theme switch
- Serve image files from the records store, with an img and figure style

## 3. Unsure

- The INVALID verdicts on pm 2026-10-08 04:51 and 2026-10-10 01:35 rest on one reading: one PR per sprint, and subagents work in their parent's worktree. If the owner wants mid-sprint reviews, they become an S3 item.
- The judge entries are OPEN because no case covers them, not because a run showed them failing. 0.4.0's prompt may already pass some of them, and S4's first task measures that.
- In S2, a new task resolution changes what every clone's store check accepts, so it needs an upgrade step for every clone of a repo. Reusing `dismissed` avoids that.
- Every repo checked still pins 0.3.0, so 0.4.0's fixes (FIXED rows 2026-10-10 01:00, formal-methods 20:24 and 20:33) are not running in them yet.
