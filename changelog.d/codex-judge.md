### Upgrade guide

1. `pm upgrade --to X` also rewrites the owner-request hook entries in `.claude/settings.json`
   (`pm hook owner-request --runtime claude`) and `.codex/hooks.json` (`--runtime codex`, with a 45 s timeout).
   Commit them with the pin move.
2. Where Codex works in the repo: run `/hooks` in Codex, opened in the main checkout, and trust pm's changed Stop hook;
   until then Codex skips it. `pm doctor` names it.
3. In a repo whose machines have Codex and not Claude Code, set `summary_runtime = "codex"` in `.pm/config.toml` and
   commit it.

### Added

- `summary_runtime` in `.pm/config.toml`, `claude` (the default) or `codex`: the runtime `pm day summarize`, which the
  pm service runs every 10 minutes, asks. On `codex` it runs `codex exec` on `gpt-5.6-luna`. It never falls back to
  the other runtime; any other value fails the config check.
- `pm doctor` fails when the CLI `summary_runtime` names is not on PATH, since the pm service summarizes with it. It
  checks no judge's CLI: each judge runs only in its own runtime's sessions.
- A pm command in a Codex session whose rollout holds no pm session-start context, because Codex skipped pm's hooks
  (an untrusted hook is skipped silently), prints one warning on stderr, once per session, naming the fix: trust
  pm's hooks in `/hooks` and start a new session.
- The request rule names each runtime's ask tool: AskUserQuestion in Claude Code; `request_user_input_async` in Codex
  (Default mode), `request_user_input` in its Plan mode. The owner-request hook reads the stopping turn's ask tool
  calls from the session's transcript and gives their questions to the judge as asked, so a question put with the
  tool, or its repeat in the final reply (Codex's async question can surface there), is not taken for an ask left
  only in chat; any other chat ask in the same reply still blocks. A block reason names the session's own ask tool.
- A Codex session's start context opens with one line naming the pm hooks Codex skips (untrusted, changed since
  trusted, or disabled) and the fix, as after a `pm upgrade` rewrote an entry the user had trusted. pm asks Codex
  (`codex app-server`, `hooks/list`) only when `.codex/hooks.json` or the trust in the user's Codex config changed
  since the last answer, which `.pm/run/codex-trust.json` keeps.

### Changed

- The owner-request Stop hook's judge runs on the session's own runtime, named by each hook entry
  (`pm hook owner-request --runtime claude|codex`): Claude Haiku through `claude -p` in Claude Code, `gpt-5.6-luna`
  through `codex exec` in Codex. A Codex session no longer needs `claude`; without it the judge failed, and Codex
  took the failure as a silent pass. The Codex judge runs from an empty temp dir with Codex's hooks, rules and your
  `config.toml` off and saves no session, so it cannot run the hook again. An entry without `--runtime` runs the
  Claude judge, as before.
- A timed-out judge or summary call now kills its whole process group, not only the `claude` or `codex` launcher, and
  a hook stopped by its runtime (SIGTERM, SIGINT or SIGHUP, as at the entry's timeout) kills its judge too.
- The day summary's prompt asks for at least one bullet, so a quiet day gets one on either runtime.
