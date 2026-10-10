# pm

pm is project management for coding agents. It keeps a project in two layers:

- **Work**: what exists, who holds it, its status and what blocks what: projects, sprints, tasks, and the needs that
  wait on the owner.
- **Records**: Markdown under `records/`, on a `records` branch of the repo: goals, sprint frames, decisions,
  findings, design pages.

`pm` writes the records, runs each action that touches both layers and checks every write. A background service per
clone serves both layers as a site, where the owner reads the state and answers the agents' requests. Agents get
pm's rules and the project's state from session hooks (`pm prime`), so every session starts from the same context.

## Install

Go pm is released for macOS on Apple silicon (`darwin-arm64`) and Linux on x86-64 (`linux-amd64`). Install release
`<X>` (see [Releases](https://github.com/Yeeef/pm/releases); [`CHANGELOG.md`](CHANGELOG.md) lists what each
changed and how to upgrade) on a machine:

```sh
curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<X>/install.sh | sh
```

`install.sh` checks the binary against the release's `SHA256SUMS` and installs it to
`${PM_BIN_DIR:-$HOME/.local/bin}/pm`. Then, in each clone of a repo that uses pm:

```sh
pm init
```

`pm init` sets up the repo (its `.pm/config.toml`, which pins the pm version, hooks and the records branch) and the
clone (the records store, the work store, the pm service). It changes only what is missing; `pm doctor` reports
what differs and `pm upgrade` rewrites it. One `pm` on PATH serves every repo: in a repo that pins another version,
it downloads that release once and runs it.

Needs: `git`; `gh` for pull-request features; `claude` for the owner-request hook's judge; `uv` only for a repo
pinned to a Python release (0.1.x).

## Docs

- `pm --help` and `pm <noun> --help`: every command, its rules and its refusals.
- [`prime.md`](prime.md): the rules `pm prime` gives each agent session.
- [`AGENTS.md`](AGENTS.md): developing pm: layout, tests, releasing.
