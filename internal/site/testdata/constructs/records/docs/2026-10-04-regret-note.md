---
type: doc
title: Regret minimization note
date: 2026-10-04
project: demo
---

## Math

On each iteration $t$ you sample $a_t \sim \sigma_t$ and track regret, with $\pi_{-i}(h)$ the reach of the
others and an $\varepsilon$-Nash at the end:

$$
R_T(a) \;=\; \sum_{t=1}^{T} \bigl[\, u_t(a) - u_t(a_t)\,\bigr]
$$

$$\sigma_{T+1}(I, a) \;\propto\; \max\bigl(R_T(I, a),\, 0\bigr)$$

Inline display: $$\bar{\sigma}_T \to \text{Nash}$$, and escapes kept: $\{x\_1, x\_2\} \subset X$.

Not math: a price of $5 and $10, a code span `$a_t$`, an escaped \$x\$, and `echo $HOME`:

```sh
echo "$a_t $b_t"
```

## Diagram

<div class="research-viz">
  <p class="viz-kicker">Information sets</p>
  <svg class="viz-scene" viewBox="0 0 820 360" role="img" aria-label="Two histories collapse into one information set">
    <defs>
      <linearGradient id="infosetFill" x1="0" x2="0" y1="0" y2="1">
        <stop offset="0%" stop-color="#e8f5f2" />
      </linearGradient>
    </defs>
    <!-- history 1 -->
    <rect x="210" y="40" width="180" height="80" rx="10" fill="#fff" stroke="#46635f" stroke-width="1.5" />
    <text x="300" y="68" text-anchor="middle" font-weight="700">History h₁</text>
    <g stroke="#46635f" stroke-width="2" fill="none">
      <path d="M150 170 C180 150, 195 110, 210 90" />
    </g>
  </svg>
  <p class="viz-note">Several histories collapse into one info set.</p>
</div>

<details>
<summary>A plain HTML block</summary>

Still Markdown, after the blank line.

</details>

## Links

- [the notes, by slug](notes.md) and [by ./slug](./notes.md#notes)
- [the plan, by file name](2026-10-03-plan.md)
- [the parser design](../design/parser.md)
- [the outage, by slug](../postmortems/outage.md)
- [a code link](../../README.md) leaves the store
