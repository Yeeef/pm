An owner's reply reaches the session that asked by the pm service's push alone, with `pm show`'s flag and
`pm reply read` for a session it did not reach; `pm reply wait` and the UserPromptSubmit hook `pm hook inbox` are gone.

### Removed

- `pm reply wait <id>`. In the transcripts on the owner's machine it never delivered a reply that the push had not
  delivered already. A raising command now names the push, or, for a session with no inbox, the `pm show` flag and
  `pm reply read <id>`.
- `pm hook inbox`, the UserPromptSubmit hook in Claude Code and Codex. `pm upgrade` to this version takes it out of
  `.claude/settings.json` and `.codex/hooks.json`, and so does `pm init` in a repo without `.pm/config.toml`; `pm init`
  in an installed repo, session start's included, leaves those files alone. Your own UserPromptSubmit hooks stay.
  `pm doctor` reports a file that still holds it.
