---
type: design
title: "Reply delivery: getting an owner reply into the session that asked"
project: pm-harness
covers:
  - "internal/service/deliver*.go"
  - "internal/service/codexqueue*.go"
  - "internal/hooks/store.go"
  - "internal/service/run*.go"
  - "internal/cli/needs.go"
  - "internal/cli/request.go"
  - "internal/cli/reply.go"
  - "internal/cli/review.go"
  - "internal/cli/decision.go"
  - "internal/cli/show.go"
---

## Problem

> What are we solving, and why now?

An owner reply posted on the site (or with `pm reply add` at a shell) must reach the agent session that raised the request, in Claude Code and in Codex. The earlier design, a background poller that a Claude Code hook started and that woke the session by exiting, lost replies in three ways:

| Failure | Cause |
|---|---|
| The raising command's output was piped through `grep`, so no waiter started | The hook learned which requests to watch from the command's printed text |
| Several requests raised in one command shared one waiter, which exited on the first reply | A waiter could wake the session only by exiting, and nothing restarted it |
| A second reply to one request had no waiter | Same: a waiter started only when a request was raised |

Root cause: the watched set was fixed at raise time, from printed text, and a waiter never came back after it exited. The design below moves delivery to the producer, the pm service, which knows the target the moment a reply is stored, and pushes it into the session; what the push cannot deliver stays flagged in the work store for the session to read.

### The pieces

| Term | Meaning |
|---|---|
| Request | A need (decision, action, PR review or question) in the work store. Its `raised_by` holds the raising session's id, its inbox and the inbox's host |
| Reply | A comment of kind reply on an open request, from the site's form or `pm reply add` |
| Delivered | The need's count of replies that reached a session. A request has an undelivered reply while its replies outnumber it |
| Inbox | Where the service hands a session its reply: a Claude Code session's messaging socket, `$CLAUDE_CODE_MESSAGING_SOCKET`; a Codex session's CODEX_HOME, stored as `codex-queue:<CODEX_HOME>`, under which the service runs `codex queue` |
| Merge | A reviewed PR's merge commit on the remote's main, which the service delivers like a reply |

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

- **Goal:** both ways the owner answers keep working: a reply on the site, and an answer given in chat that the agent records and closes (see The two workflows).
- **Goal:** every reply reaches the session that asked at least once while that session runs, by push, in Claude Code and in Codex, with no long-lived per-session process. A Claude Code question is the one exception: its answer goes through the hook that waits beside its dialog ([Owner questions](owner-questions.md)). What the push cannot deliver (a session with no inbox, or on another machine) `pm show` flags.
- **Goal:** a reply whose session is gone is never silently dropped. The next session sees it.
- **Goal:** the core (what is undelivered, what marks it delivered) is runtime-agnostic. The push and `pm reply read` are thin adapters on it.
- **Goal:** an idle session, Claude Code's or Codex's, starts a turn on the reply with nothing typed.
- **Non-goal:** exactly once. A reply may reach a session twice; it may never be lost.
- **Non-goal:** delivering into a session on another machine. Such a session finds the reply at its next start.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- **The producer knows the target.** The service stores a site reply on a need, and the need stores the raising session (`raised_by`). So the place where the event happens can address the session.
- **Claude Code has a per-session inbox.** Since v2.1.224 every session binds a Unix-socket inbox, named in `$CLAUDE_CODE_MESSAGING_SOCKET`, exported to hooks and Bash commands (code.claude.com/docs/en/cross-session-messaging names "a script or hook to post into a session" as a use). An idle session starts a new turn with a message; a busy one reads it between tool calls.
  - `encoding/json` encodes a typed `type`/`message` object as one newline-terminated JSON message. JSON escaping and whitespace do not carry meaning; the receiver reads the decoded message fields.
  - The message line `{"type":"user","message":{"role":"user","content":"…"}}` is observed (a live test with 2.1.290, and the binary's own log hint), not documented. The socket's path is read from the variable, never built.
  - The socket closes a connection that sends no complete line within 30 s, so a writer connects only once its line is ready.
  - A session in bypassPermissions mode holds a message from an unverified sender until someone approves it, unless `crossSessionInbound: "accept"` is set or the first line is `{"type":"auth","token":$CLAUDE_CODE_MESSAGING_TOKEN}`. The token never goes into the work store, which syncs to the remote.
  - The session shows the message as coming from "another Claude session", so the text says it is the owner's reply relayed from the site.
- **Codex queues a message into a session from outside.** `codex queue --thread <UUID|name> --message <TEXT>` ("Queue a message for an existing session"; shipped in codex-cli 0.149.0, 2026-08-20, whose release notes also say "Queued messages now wake idle sessions reliably", github.com/openai/codex releases `rust-v0.149.0`) sends `thread/queue/add` to its own embedded app-server, which writes a durable item to `$CODEX_HOME`'s SQLite thread queue (codex-rs `tui/src/session_queue_commands.rs:27-78`). Every app-server, a TUI's embedded one included, polls that queue every 10 s and dispatches with `start_turn_if_idle`, also when a running turn ends and when the thread is next loaded (`ext/queue/src/service.rs:88-246`).
  - It prints `Queued message <id> for thread <UUID>.`; the service checks that the thread is the one asked for.
  - The thread is always the root session's UUID: a spawned subagent's thread is refused while it is not loaded, and a name resolves to the most recent session that has it.
  - Text only, not empty.
  - When a local app-server daemon answers on `$CODEX_HOME/app-server-control/app-server-control.sock`, a TUI attaches to it and an embedded queue call is refused (`session_queue_commands.rs:36-45`); the call then goes to the daemon with `--remote unix://<socket>`, decided by whether the socket answers before the call.
  - A review on openai/codex PR #39034 flagged a possible cross-process race that consumes one queue item twice: at worst a duplicate turn.
- **`codex exec resume` fails while the thread has a writer**, so the service cannot run a turn of its own on a live thread.
- **Codex inherits its launcher's environment**, `CLAUDE_CODE_MESSAGING_SOCKET` included: a Codex run started from a Claude Code session sees that session's socket.
- **A Stop hook block ends in `stop_hook_active`**: the continuation it forces stops with `stop_hook_active` set, and every Stop check lets that stop through ([Owner-request Stop hook](../design/owner-request-hook.md)).
- **A PR merge has no producer in pm.** GitHub is the source, so something must look; the service already runs for the owner to reply at all.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### The two workflows

**1. The owner replies on the site.** The agent raised a need and carried on, or went idle. The owner answers on the request's card. The reply travels to the session by the routes below.

**2. The owner answers in chat.** The agent already has the answer: it records it (`pm decision add --need`, `pm decision close`, `pm action done`) and the close stamps pm's own answer comment at the close. That comment is no reply to deliver, so nothing is undelivered and the close is never refused.

A question need (an ask-tool question, which `pm hook ask` raises) is answered either way too, but pm, not the agent, closes it with the first answer ([Owner questions](owner-questions.md)).

"Undelivered" counts only reply comments made before the need closed, against the need's delivered count; pm's own and agents' comments never count.

### Routes by request kind and runtime

How a site reply reaches the session that asked depends on the request's kind and the session's runtime:

| Request | Claude Code | Codex |
|---|---|---|
| Decision, action, PR review (and a reviewed PR's merge) | the service pushes the reply into the session's messaging socket the moment it is stored, then on every sweep; an idle session starts a turn on it | the service queues it with `codex queue` under the session's CODEX_HOME, the moment it is stored, then on every sweep; an idle TUI starts a turn on it, and a thread not loaded takes it in when it next loads |
| Question | no push: `pm hook ask`, the PermissionRequest hook waiting beside the question's dialog, reads the reply and answers the tool with it | `codex queue` with the answer's text alone, only after the session's rollout shows no message the owner typed since the question; a typed message closes the question instead |
| Any kind, what the push did not deliver | `pm show` flags it and `pm reply read` prints it: a session with no inbox, a push that failed, the next session if this one ended | the same |

A question's whole flow, its two surfaces and how the first answer wins: [Owner questions](owner-questions.md). The rest of this page is the route for decisions, actions and reviews, with where a question differs.

```mermaid
sequenceDiagram
  participant A as Agent session
  participant S as pm service
  participant W as Work store
  participant O as Owner
  A->>W: pm decision need (raised_by: session, inbox: socket or CODEX_HOME)
  O->>S: reply on the card
  S->>W: store the reply
  alt Claude Code, inbox on this machine
    S->>A: push the reply and the next step into the socket
    S->>W: delivered = reply count
  else Codex, session live on this machine
    S->>A: codex queue --thread <session> --message <the same text>
    S->>W: delivered = reply count
  else the push failed, the session ended, or it has no inbox
    A->>W: pm show flags the reply (session start, or any later look)
    A->>W: pm reply read: delivered = reply count, once printed
  end
  A->>W: pm decision add --need, decision close, or action done
```

Reading (a decision, action or review): the push is the one route into a running session; the flag and `pm reply read` read the same undelivered set and write the same delivered count, for whatever the push did not deliver.

The raising command's output names the session's route: with a socket, that the service pushes the reply; with a CODEX_HOME, that it queues the reply into the Codex session, which starts a turn with it when idle; either way, that if the session has ended by then `pm show` flags the reply for the next one, which reads it with `pm reply read <id>`. Without an inbox, that the session gets no push, that `pm show` flags the reply once it is posted, and that `pm reply read <id>` prints it.

### Raising: which session, which inbox

`pm decision need`, `pm action need` (a PR review too) and `pm hook ask` (a question) store `raised_by`: the session ([Session claims](../design/session-claims.md), Session resolution), its inbox by its runtime, and the machine's hostname. A Claude Code session's inbox is `$CLAUDE_CODE_MESSAGING_SOCKET` (never the token); a Codex session's is its CODEX_HOME (`$CODEX_HOME`, else `~/.codex`, made absolute), stored as `codex-queue:<CODEX_HOME>` in the same column, so the work store's schema is unchanged and an older pm reads it as a socket that is not there.

- Every kind stores the same inbox; what differs is its use. A decision's, action's or review's inbox takes the push. A Claude Code question's socket takes no push: the service only reads whether it is still there, and dismisses the question once it is gone. A Codex question's CODEX_HOME takes the queued answer, and names the rollouts the service reads for an answer typed in the chat.
- A Codex session never stores the Claude Code socket it inherits: a push there would land in the Claude Code session that started it.
- A resumed Claude Code session keeps its id but binds a new socket. The session-start hook runs `pm show --refresh-inbox`, which points the session's open needs at its current inbox and host; run from a Codex hook, it points them at the session's CODEX_HOME, replacing a socket an earlier pm stored. A session with no inbox clears the one its needs carry.

### Push

The service's writer makes every write one at a time. After it stores a reply, it pushes the need's undelivered replies (and an unreported merge) into `raised_by.inbox`; a question takes the route in its row above instead:

| Step | Rule |
|---|---|
| Host | Push only when `raised_by.host` is this machine; otherwise "not running: the session's inbox is on <host>" |
| Connect | `PushTimeout` 5 s; a missing socket or a refused connection means the session is not running |
| Write | One line, as above, with the reply text, its UTC stamps and the next command (`pm decision add --need …` or `pm decision close …`; for an action, `pm action done …`) |
| Mark | Delivered is set only once the line is written. The store is not held across the push |

For a Codex inbox the push is a `codex queue` call:

| Step | Rule |
|---|---|
| Host | As for a socket: only on the machine that stored the inbox |
| Live | The session's root rollout under the stored CODEX_HOME changed within 30 minutes; else "not running: the session that asked has ended", left undelivered as a dead socket is |
| codex | `codex` on the service's PATH, at 0.149.0 or later (`codex --version`, read once per service run); else "no codex: …", undelivered, and `pm doctor` names it for a repo with Codex hooks |
| Call | `codex queue --thread <root session UUID> --message <the same text a socket push writes>`, with `CODEX_HOME` the stored one, in that directory, `--remote unix://<socket>` when the home's control socket answers; bounded at 10 s, the process group killed at the timeout |
| Mark | Delivered is set once codex exits 0 saying it queued the message for that thread |

Measured (`make test-live-parity`, an idle interactive session in a pty, nothing typed): a Codex TUI started a turn on the reply 6.1 s after `pm reply add`; a Claude Code TUI 1.0 s after it, by push.

A reply any command stored (`pm reply add` at a shell) is pushed as soon as the work store moves: the service looks at the store's change mark every `ServeCheck` (1 s), and a moved mark queues a sweep.

### Sweep

A sweep pushes again what an open need's session has not received: a failed push, or a crash between storing a reply and pushing it. It runs at start, every `MergePoll` (60 s) after the merge watch, and whenever the store moves. It also settles open questions: a Codex question whose rollout shows an answer typed after the call closes with it, and a Claude Code question whose socket is gone is dismissed ([Owner questions](owner-questions.md), Closing). A need whose inbox socket is missing costs one `stat`; one whose push found the session not running is skipped until its socket path, inode or mtime changes, or, for a Codex inbox, until the session's rollouts change. It logs only a change.

### Safety net: `pm show` and the next session

Session start injects `pm show`, which flags every open request holding an undelivered reply or merge (`[undelivered reply: pm reply read ID]`), so a reply whose session ended reaches the next session working on that project, and a session the service cannot push into sees it at its next `pm show`. `pm reply read [ID…]` prints undelivered replies and merges (without ids, this session's open requests; with ids, those, closed ones too) and marks them delivered; it does not wait.

Closing a request through pm (`pm decision add --need`, `pm decision close`, `pm action done`) refuses while it holds an undelivered reply, so pm never buries one under a close.

### Merges of reviewed PRs

At start and every `MergePoll` (60 s) the service checks each open review's PR: `gh pr view`, then `git fetch` and whether the merge commit is an ancestor of the remote's main (a PR merged into a stacked base is not). Each call has a 60 s timeout; a failing look is logged and retried at the next tick. A merge on main is stored on the review and delivered through the same routes as a reply, naming `pm action done` and the `git pull --ff-only` that updates the main checkout.

### Questions

A question need differs from the other kinds in three ways ([Owner questions](owner-questions.md) holds the detail):

- It is answered once: the first answer, in the session's chat or on the site, closes it, and a close of a closed need fails and changes nothing. A site reply that lost stays on the need, counted delivered, with a note that the chat answered first.
- Its site answer reaches the session one way per runtime, never by two: through its hook in Claude Code, through `codex queue` after the rollout check in Codex (Routes by request kind and runtime).
- `pm reply read` words a question's reply as an answer to go on with, naming no `pm decision` or `pm action` command.

### Duplicates and loss

| Case | Outcome |
|---|---|
| Push accepted, mark write fails | Pushed again by the next sweep: a duplicate |
| Codex consumes one queued item twice (a race a codex review flagged) | A duplicate turn |
| Session ended | Stays undelivered; `pm show` flags it for the next session |
| Codex question answered in chat between the rollout check and `codex queue` | Both answers reach the session: a duplicate turn |
| Claude Code question answered in its dialog between the hook's last transcript read and its close | The need records the site's answer, counted delivered, while the session took the dialog's; Claude Code ignores the hook's late answer |

Every failure leaves the reply undelivered or delivers it twice; none marks a reply delivered that no session saw, but for the last row, a window of one store write that pm accepts ([Owner questions](owner-questions.md), First answer wins).

## Alternatives considered

> What else was considered and not adopted, and why not?

| Option | Why not |
|---|---|
| Per-request poller, ids parsed from the raising command's output | Lost replies three ways (Problem) |
| Per-session poller restarted on every Bash call and at turn end (PostToolUse and Stop `asyncRewake` hooks) | Works, but ties delivery to Claude Code's hook lifecycle, relies on undocumented `asyncRewake` on Stop, and starts background processes after every Bash call |
| Per-request poller started by `pm`, pushing into the inbox until the request closes | One process per request with lifecycle rules (stop with the session, restart after a reboot, never two per request), for no guarantee push lacks |
| Claude Code channels (an MCP server pushes `notifications/claude/channel`) | Research preview; a hidden dev flag with an interactive confirmation in every session; ignored under `-p` |
| Explicit acknowledgement as the only mark (`pm reply read`) | The agent must remember a command; "accepted into the inbox" or "printed into the prompt's context" is close enough, and `pm show` covers the rest |
| `codex exec resume <thread>` from the service | Fails while the thread has a writer, and starts agent runs nobody asked for |
| A shared Codex app-server (`turn/start`) | Reaches only TUIs started after it on the default socket; pm would run Codex's server |
| Async hooks that rewake the session | Measured in Codex: an async hook cannot start a turn |
| A blocking Stop hook that waits for the reply | Holds the turn open (the session never goes idle) for as long as the owner takes, within the hook's timeout, and keeps the stop checks waiting behind it |
| An MCP tool that long-polls for the reply | A blocking call within a turn, plus an MCP server per session; it cannot wake an idle session |
| An in-turn wait command the agent blocks on after raising a need, and a UserPromptSubmit hook that hands undelivered replies over as context at the session's next prompt (both shipped, then removed) | In about 180 Claude Code and 1,760 Codex transcripts on the owner's machine neither delivered a reply the push had not; push delivered every one. They date from before Codex had push, and each added a route to every raise's output and to this page. The hook also could not run at Stop: a block there ends the turn that acts on the reply with `stop_hook_active`, which lets the stop checks through. The `pm show` flag and `pm reply read` cover what the push misses |
| Push into the inbox a Codex session inherits | Lands in the Claude Code session that started Codex |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

Webhooks and push notifications: the producer delivers, and the consumer keeps a durable record of what it has seen. We took push on the event, plus a durable "undelivered" count in the work store as the catch-up path for every other route.

## Open questions

> What is still unresolved?

- The inbox message line is observed, not documented. A Claude Code change could break it; a push failure then shows on the card and as the `pm show` flag, which sends the session to `pm reply read`, but no idle session wakes on the reply.
- `codex queue` is observed against codex-cli 0.154.0 and its source, and documented only by its help and release notes. A Codex change could break it; the outcome then shows on the card and as the `pm show` flag, which sends the session to `pm reply read`, but no idle session wakes on the reply.
- Not recorded: a push into a busy interactive session, and into a remote or bridge session.
