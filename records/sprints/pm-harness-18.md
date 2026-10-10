---
type: sprint
title: Progressive disclosure in the pm CLI
bead: yeeef-agents-9va.22
---

## Goal

> What should be true when this sprint ends, and why now?

An agent learns the `pm` CLI the way it learns a skill: `pm --help` names only the top-level nouns with one line each, `pm <noun> --help` names that noun's verbs, and a verb's help holds the flags, so finding the right command costs a few hundred tokens instead of the whole surface.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a top-level help that lists nouns only, one line each; per-noun help that lists its verbs; placing the commands that are not yet under a noun (`show`, `where`, `setup`, `serve`, `render`, `commit`, and the single-verb nouns `record link`, `doc new`, `design new`, `postmortem new`, `day new`, `finding add`) as the owner's decision on regrouping says; folding `pm action review` into `pm action need` as flags (`--pr URL` then requires `--sprint`, repeatable, and `--focus`; `--design` stays optional; no alias), with RULES.md, the skill and tests updated; the RULES.md command list, reduced to the nouns plus a pointer to `pm <noun> --help` if that loses no rule.

**Out:** renaming commands without a reason; changing what any command does, beyond the `action review` fold.

## Done when

> What evidence will show the goal is met?

- The sizes of `pm --help`, the noun-level helps, and the RULES.md command list are measured before and after, and recorded as a finding.
- A test pins `pm --help` to the nouns only, one line each, and one test checks that `pm action need --pr` without `--sprint` or `--focus` fails.
- A fresh agent given only `pm --help` and the help it chooses to read finds the right command for a few set tasks (for example: raise a PR review, record a finding, close a task), and the tokens it read are recorded.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-04}
pm commands are not regrouped and the help output stays as it is; task 9va.22.1 (nouns-only top-level help) is dropped. The fold of pm action review into pm action need stands.
The owner reviewed the current pm --help and per-noun help and found them already good: argparse already lists one line per noun and each noun lists its verbs.
Answers `yeeef-agents-9va.22.4`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Baseline at 81f0bd7: `pm --help` is 2164 bytes, 41 lines (~540 tokens), with
  17 entries (11 nouns, 6 standalone commands) and the choice list printed
  twice; the 17 first-level helps total 4505 bytes, 147 lines (~1.1k tokens);
  the 19 verb helps total 7869 bytes, 210 lines (~2.0k tokens); learning the
  whole CLI from help costs 14538 bytes (~3.6k tokens). 25 leaf commands; 24
  once `action review` folds into `action need`. RULES.md is 10726 bytes, 73
  lines (~2.7k tokens, loaded every session); its "Write records through pm"
  section is 4295 bytes (~1.1k tokens), 19 command lines.

- Prior art: gh groups by noun (`gh pr`, `gh issue`) with verbs under each and
  top-level help in titled groups (core, additional); kubectl groups top-level
  verbs by section (Basic, Deploy, Cluster Management); git's `--help` shows
  ~20 common commands by task and leaves the rest to `git help -a`. Agent
  skills disclose in three levels: name and description always, SKILL.md on
  trigger, references on demand, as `skills/project-management/SKILL.md` does
  with `references/owner-communication.md`.

- RULES.md trim (9va.22.3, acbeed0): RULES.md went from 12216 bytes, 77 lines
  (~3.1k tokens) to 9007 bytes, 71 lines (~2.3k), -26%; its Write records
  section went from 4663 to 1682 bytes, with 19 per-command lines replaced by
  one line per noun and a pointer to pm <noun> --help. The rules that lived
  only in command lines moved to the rule sections. The help surface after the
  change is 15054 bytes (~3.8k tokens): top-level 2164 bytes, 17 first-level
  helps 4393 bytes, 18 verb helps 8497 bytes. Only decision close's help
  changed, to say a failed label is safe to retry. A fresh Sonnet agent given
  only RULES.md and pm --help got all five set tasks right: raise a decision
  need, record the owner's answer with decision add --need, request a PR
  review with action need --pr after a draft report, add a finding, and the
  sprint close sequence. It read about 8.3 KB (~2.1k tokens) of help across 12
  calls. Another 2.1 KB were argparse errors from its own shell quoting. It
  never ran the top-level pm --help.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: an agent learns the pm CLI from `RULES.md`'s list of nouns and
`pm <noun> --help`, and PR reviews are raised through `pm action need --pr`.

- `pm action review` folded into `pm action need --pr` (owner question on PR #14).
- `RULES.md` lists the nouns and points to their help: 12,216 to 9,007 bytes.
- The help output stays as it was (owner decision on 9va.22.4).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Help and RULES.md sizes measured before and after: met (finding on this
  sprint; RULES.md about 3.1k to 2.3k tokens).
- A test pins `pm --help` to nouns only: dropped; the owner kept the current
  help (9va.22.4), and task 9va.22.1 was closed as not done.
- `--pr` without `--sprint` or `--focus` fails: met,
  `test_action_need_review_flags_refuse` (8ab72bd).
- A fresh agent finds the right command from help alone: met, 5 of 5 tasks
  right with about 2.1k tokens of help read (acbeed0).
