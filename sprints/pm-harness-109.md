---
type: sprint
title: Retire Python pm from Yeeef/pm
bead: yeeef-agents-9va.119
---

## Goal

> What should be true when this sprint ends, and why now?

Yeeef/pm holds one implementation of pm, Go, and no Python pm: the Python package, its launcher and the Python-vs-Go parity machinery are deleted, and the black-box harness suite keeps checking Go pm on every PR. Now, because yeeef-agents runs Go pm since 2026-10-09, the owner declared the soak over on 2026-10-10, and every change today pays for two implementations.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Delete `src/pm` (Python pm), `pyproject.toml`'s pm package and console script, the Python launcher and the Python bridge pieces of the release.
- Move the files Go embeds from `src/pm` (style.css, prompts) to a Go-owned path.
- Replace the parity corpus that Python generated (go_parity_corpus.py, test_go_parity.py, compare_transcripts.py, go-expected-failures.txt) with checks that need no Python pm: frozen expected outputs where they still say something, deletion where they only compared the two implementations.
- Make the shared harness suite (tests/, pytest) run Go pm by default, in `make test` and CI.
- Update AGENTS.md / CLAUDE.md / README in Yeeef/pm and the design pages that describe two implementations (pm-go, pm-product, work-store Storage, pm-versioning).

**Out:**
- Porting the pytest harness suite to Go: it is a black-box test of the pm binary, not pm itself.
- Deleting yeeef-agents' `pm/` copy, which is sprint 79's task.
- A new pm release: no behaviour change ships, so the pin stays.

## Done when

> What evidence will show the goal is met?

- `git ls-files src/pm` is empty on Yeeef/pm main, and no Go file embeds or reads a path under `src/`.
- `make test` and CI on main run the harness suite against Go pm and pass, with no expected-failure list.
- `grep -rni python` over Yeeef/pm's docs finds only the test harness's own tooling.
- The sprint's PR is on Yeeef/pm main.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
Also remove the launcher path that ran Python pins: a pin below 0.2.0 fails hard naming pm upgrade --to <X>; this ships as a breaking change in the changelog, not as a no-release change.
Every known repo pins 0.3.0 (all local .pm/config.toml checked), so the uv path only kept Python pm alive; end-to-end decommission asked for by the owner.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Python pm is gone from Yeeef/pm and yeeef-agents, and the harness suite checks only Go pm.

- Yeeef/pm PR #9, merged as 0097e78:
  - Deleted `src/pm`, the pm package and console script in pyproject, and the parity machinery: the corpus, `test_go_parity`, transcripts, the expected-failure list and the HTML normaliser.
  - Removed the `PM_IMPL` switch. The harness always runs `.go/pm`.
  - Removed the launcher's uv path for Python pins. A pin below 0.2.0 now fails hard. This is a breaking change, recorded in CHANGELOG's [Unreleased].
  - Moved `style.css` and `prompts/` to the module root, byte for byte.
  - The parity checks that still mean something are now Go golden tests: 16 site pages, 114 install piece cases, and the records table.
- yeeef-agents PR #102, merged as c597b69: deleted the `pm/` copy (193 files, −48,401 lines) and its Makefile targets.
- The design pages pm-go, pm-product, pm-versioning, work-store and pm-cli now describe one Go implementation.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `src/pm` is gone and no Go file reads under `src/`: met. `git ls-tree -r origin/main src` lists 0 files, and `git grep 'src/' -- '*.go'` finds nothing.
- The harness runs on Go with no expected-failure list: met.
  - Locally: `make test` 112 passed and 40 skipped (the live eval); `CI=1 make test-full` 151 passed and 41 skipped; `make test-go` ok in every package.
  - CI on main, 0097e78: all checks passed (pm tests, pm changelog, and pm go build-and-test on linux-amd64 and darwin-arm64).
  - A fresh-context review found no correctness defect. It also checked the goldens against main's pre-PR Go code, and they passed.
- `grep -i python` over the docs finds only harness tooling: partly met. AGENTS.md keeps 3 lines that name the retired Python releases (0.1.x) on purpose, for the launcher refusal, the release gate and the one-time legacy move. CHANGELOG keeps its history.
- PR on main: met. Yeeef/pm #9 is 0097e78, and yeeef-agents #102 is c597b69.
