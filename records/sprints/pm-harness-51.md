---
type: sprint
title: Decision needs have one fixed, readable layout
bead: yeeef-agents-9va.60
---

## Goal

> What should be true when this sprint ends, and why now?

Every decision need card the owner reads has the same layout: a question, facts, each option with its cost, and a default. pm builds that layout from structured input, so no agent can produce an unreadable card. Why now: the owner could not read need yeeef-agents-9va.38.18, because its free-text description rendered as one long paragraph.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm decision need` takes structured input (for example flags `--question`, repeatable `--fact`, repeatable `--option` with a label, text and cost, and `--default`; or one structured stdin format; the sprint's design fixes the exact form) and writes the Markdown itself in one fixed layout.
- Refusals: an option without a cost, a default that names no option, and a sentence over 25 words (ASD-STE100 limit for a description).
- The site card renders that layout.
- `pm decision need --help`, RULES.md and SKILL.md updated.
- Existing open decision needs keep rendering.

**Out:**
- Action needs: the owner said they are fine as they are.
- The owner-request Stop hook (sprint 49).
- Rewriting old closed needs.

## Done when

> What evidence will show the goal is met?

- Tests show each refusal and the fixed layout.
- A decision need raised with the new form renders on the site as the layout, checked on the served page at phone width.
- RULES.md, SKILL.md and `pm decision need --help` describe only the new form.

## Design pages

> Where is the detail?

- [Decision need layout](../design/decision-need-layout.md): the stdin input form, the Markdown pm writes, the refusals and the site card

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Trigger: need yeeef-agents-9va.38.18 rendered on the site as one paragraph,
  because its single newlines collapse in Markdown (site.py owner_card renders
  the description with md.render, lines 349-355). The only check today is the
  presence of an Options and a Default line (pm.py NEED_SHAPE lines 108-112,
  check lines 466-470).

- Live check (yeeef-agents-9va.60.5): need yeeef-agents-9va.60.6, raised with
  the stdin form from this branch, rendered on the local `pm serve`
  (127.0.0.1:8000, main checkout) sprint page at 390x844 as Question
  paragraph, Facts list of 2, Options list of 2 each with "Cost:", Default
  paragraph, and `pm serve` as `<code>`; scrollWidth 390 = innerWidth 390,
  card 356 px wide (16 to 374), 0 overflowing descendants. A Default naming no
  option printed "Default: c names no option; the labels are a, b", exit 1,
  and created nothing. The public URL redirects to Cloudflare Access, so the
  check used the local server.

- Served-site timing tests are flaky in full runs: each of 4 full runs on this
  branch failed one different pm serve test (e.g.
  test_serve_reuses_a_page_only_while_records_and_beads_are_unchanged), each
  passed alone; none uses needs; not checked on main.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `pm decision need` builds every decision need card from structured parts in one fixed layout, and refuses input that would make an unreadable card.

Merged as fb6a82e (PR #54).

- An agent gives one part per line on stdin: `Question:`, `Fact:`, `Option <label>:` with a `Cost:` line under it, and `Default: <label>`.
- pm writes the Markdown itself: a bold Question, a Facts list, an Options list with each cost, and the Default.
- pm refuses a missing or extra part, an option without a cost, a Default that names no option, and a sentence over 25 words.
- The site needed no change; old free-text needs still render.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Tests show each refusal and the fixed layout: met. `uv run skills/project-management/harness/tests/run.py -k "need or block_marker or body_format"` gives 96 passed: one case per refusal, an exact-Markdown test, a test that facts starting with `2026.`, `#`, `>` or `-` stay one plain item, and a site test for the new and the old card.
- A need raised with the new form renders on the site as the layout at phone width: met. The live check finding above gives the DOM checks at 390x844 (no overflow). It used the local `pm serve`, because the public URL asks for a Cloudflare Access login.
- RULES.md, SKILL.md and `pm decision need --help` describe only the new form: met. The owner-request Stop hook texts were also updated, because they described the old form.

Full suite: 417 passed, 7 skipped, 1 failed. The failure is a served-site timing test that passed when run alone (see Findings).
