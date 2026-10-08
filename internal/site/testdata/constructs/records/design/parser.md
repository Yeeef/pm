---
type: design
title: The parser
project: demo
---

## Problem

> What are we solving, and why now?

Records need a parser. See [sprint 1](../sprints/demo-1.md), its [goal](../sprints/demo-1.md#goal), the
[site](https://example.com/page.md), a [query](notes.md?x=1) and <https://example.com/auto>.
Line one\
line two, with a hard break, <kbd>inline HTML</kbd>, &copy; &#169; and an escaped \*star\*.

![a diagram](pic.png "Title")

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

1. One parser.
2. Two outputs:
   - HTML;
   - text.

     A loose item's paragraph.

* star list
* item

> A quote with **bold** and `code`.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

### Tables

| Left | Center | Right | None |
|:-----|:------:|------:|------|
| a | b | c | d |
| `x \| y` | **e** | [f](../sprints/demo-2.md) | |
| short |

| Only a header |
|---|

### The `parse()` *fast* path & [links](x.md)

Text.

### Notes

First.

### Notes

Second.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### 1. Steps

```mermaid
flowchart LR
  a["A & B"] --> b<c>
```
Reading: a then b.

```python
def f(x):
    return x < 1 and "q"
```

```
plain fence
```

    indented code

::: note
A note with a list:

- one
- two
:::

:::: note
Outer.

::: result {title="Inner & co"}
| k | v |
|---|---|
| 1 | 2 |
:::

::::

<details>
<summary>Raw HTML block</summary>

Markdown *inside*.

</details>

<div class="x">
  <p>raw</p>
</div>

---

Setext heading
--------------

#### Deep heading

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Why not |
|---|---|
| Regex | Lookarounds |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

markdown-it.

## Open questions

> What is still unresolved?

None yet.
