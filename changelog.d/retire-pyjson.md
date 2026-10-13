pm reads and writes JSON with Go's own encoder now that Python pm is retired, and quotes a value in its messages the
Go way; the files pm writes in a repo stay byte for byte the same when their text is ASCII.

### Changed

- A message that quotes a value quotes it in double quotes, Go's way, instead of Python's single quotes: for example
  `no open project "old"; open ones: demo` (was `'old'`), `--merged "HEAD" is not a commit sha`, and the records
  commit `pm: renamed sprint <id> to "Sprint 1: …"`. The wording around the value is unchanged. A record header's list
  value shows its strings the same way (`covers is a list of path patterns …; got "internal/**"`), and a hook input of
  the wrong type is named by its JSON type (`last_assistant_message is a number`, was `a int`).
- `pm show --json` writes non-ASCII text as it is (UTF-8) instead of `\uXXXX` escapes; its layout is unchanged.
- The hooks' JSON output (`pm prime --hook-json`, `pm hook inbox`, `pm hook stop`, `pm hook owner-request`) is compact
  (`{"decision":"block","reason":"…"}`) and keeps non-ASCII text as it is. Claude Code and Codex read it as before.
- The site's `/version` answer is compact JSON, and its time drops a trailing `.0`.
- `.pm/config.toml` keeps a non-ASCII value (a remote or branch name, say) as it is instead of a `\uXXXX` escape; a
  character outside the Basic Multilingual Plane was written as a pair of escapes TOML does not read, and is now
  written as it is. With a non-ASCII value, `pm doctor` names the file as differing until `pm upgrade` rewrites it.
- `.claude/settings.json`, `.codex/hooks.json` and `.claude/settings.local.json` keep a number as the file writes it
  (`1.50` stays `1.50`) where pm rewrote it the way Python writes numbers, and `pm init` no longer refuses a file
  only for such a number. A string holding U+2028 or U+2029
  is now written with that character escaped.
