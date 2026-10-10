---
type: design
title: Decision need layout
project: pm-harness
---

## Problem

> What are we solving, and why now?

An agent writes a decision need's description as free text. pm only checks that the text has an Options line and a Default line. So the card layout depends on each agent. Need yeeef-agents-9va.38.18 rendered on the site as one long paragraph, and the owner could not read it. Its single newlines collapse in Markdown.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

Goals:

- Every new decision need card has the same layout: the question, the facts, each option with its cost, and the default.
- pm writes the Markdown. The agent gives only the parts.
- pm refuses an option without a cost, a default that names no option, and a sentence of more than 25 words.

Non-goals:

- Action needs keep their free-text description. The owner is satisfied with them.
- pm does not rewrite old needs, open or closed.
- The owner-request Stop hook does not change.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- The site renders a need's description with the shared markdown-it renderer (CommonMark, `site.py` `owner_card`). A single newline does not make a new line. A blank line and a `- ` list item do.
- A free-text pm body (a frame, a doc, an action) comes on stdin, sent as a quoted heredoc (`<<'EOF'`). A decision need is not free text: each part is a flag, so its form comes from `--help` and argparse, not from rules the agent must remember.
- In a double-quoted shell argument, a backtick runs a command. Need texts often hold code spans, so each flag value goes in single quotes.
- Need texts often hold inline code: commands, flags, paths. In a double-quoted shell argument, a backtick starts command substitution. In a quoted heredoc (`<<'EOF'`), it does not.
- ASD-STE100 sets 25 words as the maximum for a descriptive sentence.
- Open decision needs today: yeeef-agents-9va.38.16, .38.17 and .38.18 already use this layout (set by hand). Only .38.19 is free text.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### Input form

`pm decision need --title "…" --parent ID` takes each part as a flag, and reads no stdin. Each value is one line:

| Flag | Count | Meaning |
|---|---|---|
| `--question TEXT` | exactly 1 | The question the owner answers. |
| `--fact TEXT` | 1 or more | One fact the owner needs to decide. One bullet each. |
| `--option LABEL TEXT` | 2 or more | One option. The label is letters or digits, for example `a`. |
| `--cost LABEL TEXT` | exactly 1 per option | The cost of the option with that label. |
| `--default LABEL REASON` | exactly 1 | The label of the option taken if the owner does not answer, and why. |

argparse refuses a missing flag and a flag with too few values. pm keeps each flag's order of input. Each part is one line, so an option cannot hold a nested list; put shared detail in a `--fact`. A cost names its option by label, not by position, so a cost cannot land on the wrong option.

Example call:

```
pm decision need --title 'Where the public site URL is stored' --parent yeeef-agents-9va.38 \
  --question 'Where does pm keep the public site URL?' \
  --fact 'Today, each clone keeps the URL in its git config.' \
  --fact 'You asked why the URL is not in `.pm/config.toml`.' \
  --option a 'In `.pm/config.toml`. A pm command still writes it.' --cost a 'the repo has one URL.' \
  --option b 'In each clone, as today.' --cost b 'you must give the URL to each new clone.' \
  --default a 'All clones then give the same link.'
```

### Markdown that pm writes

pm writes this description into Beads, always in this order and form:

```
**Question:** Where does pm keep the public site URL?

**Facts:**

- Today, each clone keeps the URL in its git config.
- You asked why the URL is not in `.pm/config.toml`.

**Options:**

- **(a) In `.pm/config.toml`.** A pm command still writes it. *Cost:* the repo has one URL.
- **(b) In each clone, as today.** *Cost:* you must give the URL to each new clone.

**Default:** (a). All clones then give the same link.
```

Rules for the layout:

- Blank lines separate the four blocks, so each block is its own paragraph or list.
- An option bullet is the label in parentheses and the option's first sentence in bold, then the other sentences, then `*Cost:*` and the cost.
- The Default line is the label in parentheses and a period, then the reason.
- pm copies the text as given. Inline Markdown, such as code spans, stays.
- pm escapes a fact's leading block marker (`#`, `>`, `-`, `+`, `*` or a number before `.` or `)`), for example `2026\.`, so each fact stays one plain list item.

### Sentence and word rules

pm checks each text: the question, each fact, each option text, each cost and the default reason.

- Before the split, pm replaces each code span (`` `…` ``) with one token. A code span is one word, and a period in it ends no sentence.
- A sentence ends at `.`, `!` or `?` followed by white space, or at the end of the text. So an id such as `yeeef-agents-9va.38.18`, a number such as `1.7`, or a path ends no sentence.
- A word is a run of characters between white space that holds at least one letter or digit. So ids, commands, paths, URLs and numbers count as one word. A lone dash counts as none.
- The split can only make sentences shorter (for example after "e.g."). So it can let a long sentence pass, but it does not refuse a correct one.

### Refusals

pm refuses the call, writes nothing, and prints one message. `<shape>` is the example call from the Input form section above.

| Condition | Message |
|---|---|
| A flag is missing, or has too few values | argparse's usage error, for example `the following arguments are required: --question` |
| `--question` or `--default` given twice | `give exactly one <flag>; <shape>` |
| A value is empty | `<flag> is empty; <shape>` |
| A value has more than one line | `<flag> has more than one line; give each part one line` |
| A label is not letters and digits | `the option label '<label>' is not letters and digits only, e.g. a, b or keep` |
| Two options have the same label | `two options have the label <label>; give each option its own label` |
| A cost names no option | `--cost <label> names no option; the labels are <a, b, …>` |
| An option has two costs | `option <label> has two --cost flags; give each option one cost` |
| An option has no cost | `option <label> has no cost; add --cost <label> '<what it costs>'` |
| Fewer than 2 options | `give at least two --option flags; a decision needs a choice; <shape>` |
| The default names no option | `--default <label> names no option; the labels are <a, b, …>` |
| A sentence has more than 25 words | `the sentence "<first 6 words> …" in <part> has <n> words; the limit is 25 (ASD-STE100); split it` |

`<part>` names where the sentence is: `the question`, `fact <n>`, `option <label>`, `the cost of option <label>` or `the default`. The existing refusal for a request that asks to review or merge a PR stays, and runs first.

### Site card

The site needs no change. `owner_card` renders the description with the shared Markdown renderer, and the layout above renders as paragraphs and lists (checked with markdown-it CommonMark). An old need with a free-text description renders the same as before. Nothing reads the layout back, so nothing has to detect it.

### Help and guidance text

`NEED_SHAPE` is the example call above. `pm decision need --help` and `prime.md` describe only the flag form.

## Alternatives considered

> What else was considered and not adopted, and why not?

- Key-per-line stdin (`Question:`, `Fact:`, `Option <label>:`, `Cost:`, `Default:`), the form before flags. Replaced on the owner's request: only `pm prime` stated the layout in full, so the form depended on the agent having read it. Flags put the form in `--help` and let argparse refuse a missing part. Its one gain, that a heredoc keeps backticks, is kept by single quotes.
- `--option LABEL TEXT COST`, the cost as the third value. Not adopted: three positional values are easy to put in the wrong order, and a swapped text and cost pass every check. `--cost LABEL TEXT` names its option.
- Sections in the body (`## Question`, `## Options`, as `pm sprint open` reads its frame). Not adopted: an option and its cost then need a second inner syntax, and the form again depends on rules outside `--help`.
- Keep free text and only check it more strictly. Not adopted: a check cannot make a free text render well. Need .38.18 passed the check and was unreadable.
- Enable soft line breaks (`breaks`) when the site renders a description. Not adopted: it only helps text with single newlines. It does not give one layout, and it changes how the other records render.
- Detect the new layout and render it with special HTML. Not adopted: Markdown already renders it correctly, so detection adds code for no gain.

## Prior art

> Optional. What existing work did we learn from, and what did we take?

- ASD-STE100 Simplified Technical English: the 25-word limit for a descriptive sentence.
- Needs yeeef-agents-9va.38.16 to .38.18, which were set by hand in this layout and which the owner could read.

## Open questions

> What is still unresolved?

- Need yeeef-agents-9va.38.19 is free text. It keeps its current rendering. The agent that raised it can raise it again in the new form, but this sprint does not rewrite it.
