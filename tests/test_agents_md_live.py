"""Live regression eval for the AGENTS.md evaluation (tests/agents_md/evaluate.py), on its labelled cases
(tests/agents_md/cases.py): each case is a copy of the checkout with its edits committed, graded RUNS times
(PM_LIVE_RUNS, default 3) by the real model on each of PM_AGENTS_MD_MODELS (default: the evaluation's model), and
every run must give the case's verdict: a planted case fails on its criterion with a finding that names the defect, a
clean case passes every criterion.

Skipped unless PM_LIVE_TESTS=1. Run with `make test-live-agents-md` after editing the rubric, the runner or AGENTS.md's
structure. Every call of every run goes through one pool (WORKERS at once). It prints each model's pass counts, the
per-call latency and the per-run latency (its slowest call: the calls of a run go at once), median, p90 and max, and
each run's cost. Claude runs as the user (their HOME and Claude config, where its login is); in GitHub Actions with
workload identity federation, as the evaluation runs there.
"""

from __future__ import annotations

import os
import statistics
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import pytest

from agents_md import cases, evaluate
from conftest import CHECKOUT, REAL

pytestmark = pytest.mark.skipif(os.environ.get("PM_LIVE_TESTS") != "1",
                                reason="live model tests: set PM_LIVE_TESTS=1 (make test-live-agents-md)")

RUNS = int(os.environ.get("PM_LIVE_RUNS", "3"))
WORKERS = int(os.environ.get("PM_AGENTS_MD_JOBS", "16"))
MODELS = os.environ.get("PM_AGENTS_MD_MODELS", evaluate.MODEL).split()
# PM_AGENTS_MD_CASES (names, space-separated) runs only those cases, while iterating on one; PM_AGENTS_MD_SKIP leaves
# those out (CI skips the clean case, which its check job's full-mode run repeats)
CASES = {n: c for n, c in cases.CASES.items() if n in os.environ.get("PM_AGENTS_MD_CASES", n).split()
         and n not in os.environ.get("PM_AGENTS_MD_SKIP", "").split()}


def stats(xs: list[float]) -> str:
    p90 = statistics.quantiles(xs, n=10, method="inclusive")[-1] if len(xs) > 1 else xs[0]
    return f"median {statistics.median(xs):.0f}s, p90 {p90:.0f}s, max {max(xs):.0f}s"


@pytest.fixture(scope="module", params=MODELS)
def outcomes(request, tmp_path_factory):
    """Every case's outcomes, RUNS each, on one model."""
    model = request.param
    tmp = tmp_path_factory.mktemp(f"agents-md-{model}")
    env = dict(os.environ, HOME=REAL["HOME"],
               CLAUDE_CONFIG_DIR=REAL["CLAUDE_CONFIG_DIR"] or str(Path(REAL["HOME"]) / ".claude"))
    roots = {name: tmp / name for name in CASES}
    bases = {name: cases.materialize(CASES[name], CHECKOUT, roots[name]) for name in CASES}
    jobs = [(name, run, call) for name in CASES for run in range(RUNS)
            for call in cases.calls(CASES[name], roots[name], bases[name])]
    texts = {name: (roots[name] / "AGENTS.md").read_text() for name in CASES}
    start = time.monotonic()
    with ThreadPoolExecutor(WORKERS) as pool:
        results = list(pool.map(lambda j: evaluate.run_call(j[2], model, roots[j[0]], texts[j[0]], env=env), jobs))
    took = time.monotonic() - start
    runs: dict[tuple[str, int], list[evaluate.Result]] = {}
    for (name, run, _), r in zip(jobs, results, strict=True):
        runs.setdefault((name, run), []).append(r)
    added = {name: cases.planted(roots[name], bases[name]) for name in CASES}
    out = {name: [cases.outcome(CASES[name], runs[name, n], added[name]) for n in range(RUNS)] for name in CASES}
    passed = sum(o == CASES[name].expect for name, os_ in out.items() for o in os_)
    run_secs = [max(r.seconds for r in rs) for rs in runs.values()]
    print(f"\n{model}: {passed}/{len(CASES) * RUNS} runs gave the case's verdict, {len(jobs)} calls in {took:.0f}s, "
          f"{WORKERS} at a time, ${sum(r.cost_usd for r in results):.2f} in all\n"
          f"  per call {stats([r.seconds for r in results])}; per run {stats(run_secs)}")
    for name, got in out.items():
        case, rs = CASES[name], [runs[name, n] for n in range(RUNS)]
        cost = statistics.median(sum(r.cost_usd for r in x) for x in rs)
        print(f"  {sum(o == case.expect for o in got)}/{RUNS}  {case.expect:4}  {name:26} {len(rs[0]):2} calls, "
              f"run {stats([max(r.seconds for r in x) for x in rs])}, ${cost:.3f}/run"
              + "".join(f"\n      got {o}" for o in sorted(set(got)) if o != case.expect))
    return out


@pytest.mark.parametrize("name", CASES)
def test_agents_md_evaluation(outcomes, name):
    want = CASES[name].expect
    assert outcomes[name] == [want] * RUNS, f"{name}: got {outcomes[name]}, want {want} in all {RUNS} runs"
