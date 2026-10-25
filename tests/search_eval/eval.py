#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""The offline retrieval eval of pm search: the gate of every ranking change (the Search index design page, Evaluation).

It asks: for a sprint about to be framed, do the earlier records it cites come up when an agent searches its topic?
Every input is frozen beside this file, so runs compare:

| File | Holds |
|---|---|
| `corpus.json` | the records commit the corpus is read from, the items' cut-off time, how labels.json was built |
| `items.jsonl.gz` | the work store's items created before the cut-off (`pm export` lines, comments and holders dropped) |
| `labels.json` | per sprint: when it opened, its strict and extended targets; per record: when it was created |
| `queries.json` | per sprint: the 3 queries a model wrote from its title and its Goal's first sentence (`eval.py queries`) |

Labels are the records' own citations. A sprint's strict targets are the records its Goal and Scope cite: a Markdown
link to a record, a record name (`pm-harness-12`), an item id (a task's id cites its sprint), or `sprint N` (in the
citing sprint's project unless a project name precedes it). Its extended targets add the design pages its Design pages
section lists. A target created at or after the sprint opened, and the sprint itself, are dropped. A sprint or project
was created when its item was; a doc on its header date; a postmortem on its name's date; a design page has no
creation date the store keeps (the records branch starts with a bulk move), so it counts as created when the first
sprint that links it opened, and a page no sprint links as always there.

Each sprint is searched against its as-of corpus: the records created before it opened, without its own record, and
the items created before it opened. Systems:

| System | What it ranks |
|---|---|
| `pm:<label>` | pm search's output lines, from internal/search built at a git revision (`searchrun/main.go`) |
| `rg-and-count` | files holding every term (`rg -F -i`), most matches first |
| `rg-or-count` | files holding any term, most matches first |
| `rg-and-newest` | files holding every term, newest record first |
| `bm25-or` | whole records, BM25 over pm's tokens (lower case, S-stemmed), any term |

A query's terms are its words, as an agent types them. Relevance is per record: an output line counts at its rank, and
a record counts once, at its first line; an item line takes a rank and is never relevant. Metrics: recall@5 and @10,
MRR, nDCG@10 (binary gains), the zero-result rate, and the tokens an agent reads up to the first relevant record
(characters / 4: pm search's lines from its header; rg's file list, and `rg -n` lines of each file up to that one).
Single query is the mean of a sprint's 3 queries; best of 3 the best query per sprint and metric; union the targets
in any of the 3 top k. Differences carry paired bootstrap 95% CIs over sprints (10,000 resamples, seed 0).

    eval.py run [--pm LABEL=REV ...] [--sets strict,extended] [--json OUT]
    eval.py freeze --export FILE      rebuild labels.json and items.jsonl.gz (changes the eval: commit the result)
    eval.py queries                   rewrite queries.json with claude -p (changes the eval: commit the result)

REV is a commit, or `worktree` for the checkout as it is; `make search-eval` runs `run` with stage 0 and the
worktree. Run it on demand, never in CI. Needs git, go, rg on PATH (or RG=<path>) and the corpus commit in this repo (it is on the
records branch: `git fetch origin records`).
"""
from __future__ import annotations

import argparse
import concurrent.futures as cf
import gzip
import json
import math
import os
import random
import re
import shlex
import shutil
import statistics
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = Path(subprocess.run(["git", "rev-parse", "--show-toplevel"], cwd=HERE, capture_output=True, text=True,
                           check=True).stdout.strip())
CORPUS, ITEMS, LABELS, QUERIES = (HERE / n for n in ("corpus.json", "items.jsonl.gz", "labels.json", "queries.json"))
KINDS = {"sprint", "project", "design", "doc", "postmortem"}  # the record types pm search indexes
RG = ("rg-and-count", "rg-or-count", "rg-and-newest")
WORKERS = os.cpu_count() or 4
RG_BIN = os.environ.get("RG", "rg")  # the ripgrep binary; RG=PATH picks one off PATH


# ---------------------------------------------------------------- the corpus


def corpus_tree(dest: Path) -> Path:
    """The records tree of the frozen commit, extracted under dest."""
    commit = json.loads(CORPUS.read_text())["records_commit"]
    if subprocess.run(["git", "cat-file", "-e", commit + "^{commit}"], cwd=ROOT, check=False).returncode:
        sys.exit(f"records commit {commit} is not in this repo: git fetch origin records")
    tree = dest / "records"
    tree.mkdir(parents=True)
    archive = subprocess.run(["git", "archive", commit], cwd=ROOT, capture_output=True, check=True).stdout
    subprocess.run(["tar", "-x", "-C", str(tree)], input=archive, check=True)
    return tree


def meta(text: str) -> dict[str, str]:
    """A record's header fields, as plain strings."""
    out = {}
    if text.startswith("---\n"):
        for line in text[4:text.index("\n---", 4)].splitlines():
            key, sep, value = line.partition(":")
            if sep and not line.startswith(" "):
                out[key.strip()] = value.strip().strip('"')
    return out


def read_records(tree: Path) -> dict[str, tuple[dict[str, str], str]]:
    """Every record of a kind pm search indexes, by rel (`sprints/pm-harness-12`): its header and its text."""
    out = {}
    for p in sorted(tree.rglob("*.md")):
        text = p.read_text()
        m = meta(text)
        if m.get("type") in KINDS:
            out[p.relative_to(tree).with_suffix("").as_posix()] = (m, text)
    return out


def section(text: str, name: str) -> str:
    m = re.search(r"^## " + re.escape(name) + r"[ \t]*$(.*?)(?=^## |\Z)", text, re.M | re.S)
    return m.group(1) if m else ""


LINK = re.compile(r"\]\(([^)#\s]+\.md)(?:#[^)]*)?\)")


def links(rel: str, text: str, recs: dict) -> list[str]:
    out = []
    for m in LINK.finditer(text):
        target = os.path.normpath(os.path.join(os.path.dirname(rel), m.group(1)))[:-3]
        if target in recs and target not in out:
            out.append(target)
    return out


def project_of(rel: str) -> str:
    return rel.split("/")[-1].rsplit("-", 1)[0]


def cites(rel: str, text: str, recs: dict, by_name: dict, by_bead: dict, projects: set[str]) -> list[str]:
    """The records text cites, in order of first citation."""
    out = links(rel, text, recs)

    def add(target):
        if target and target not in out:
            out.append(target)

    for m in re.finditer(r"\b(pm-[a-z]+-\d+)\b", text):
        add(by_name.get(m.group(1)))
    for m in re.finditer(r"\b([a-z][a-z0-9]*(?:-[a-z0-9]+)*-[a-z0-9]{2,4}(?:\.\d+)+)\b", text):
        parts = m.group(1).split(".")
        for k in range(len(parts), 1, -1):  # a sprint's id, or a task's or need's id under it, cites the sprint
            target = by_bead.get(".".join(parts[:k]))
            if target and recs[target][0]["type"] == "sprint":
                add(target)
                break
    for m in re.finditer(r"(?:\b([a-z]+(?:-[a-z]+)*) )?\b[Ss]prints? (\d+)\b", text):
        proj = m.group(1) if m.group(1) in projects else project_of(rel)
        add(by_name.get(f"{proj}-{m.group(2)}"))
    return out


def freeze(export: Path) -> None:
    """labels.json and items.jsonl.gz from the corpus commit and a pm export file."""
    cfg = json.loads(CORPUS.read_text())
    cutoff = cfg["items_cutoff"]
    items = {}
    lines = []
    for line in export.read_text().splitlines():
        it = json.loads(line)
        if it["created_at"] >= cutoff:
            continue
        for key in ("comments", "holder", "closed_by"):
            it.pop(key, None)
        items[it["id"]] = it
        lines.append(json.dumps(it, sort_keys=True))
    with gzip.GzipFile(ITEMS, "wb", mtime=0) as f:
        f.write(("\n".join(lines) + "\n").encode())
    with tempfile.TemporaryDirectory() as tmp:
        recs = read_records(corpus_tree(Path(tmp)))
    by_name = {r.split("/")[-1]: r for r, (m, _) in recs.items() if m["type"] in ("sprint", "project", "design")}
    by_bead = {m["bead"]: r for r, (m, _) in recs.items() if m.get("bead")}
    projects = {r.split("/")[-1] for r, (m, _) in recs.items() if m["type"] == "project"}
    opened = {r: items[m["bead"]]["created_at"] for r, (m, _) in recs.items()
              if m["type"] in ("sprint", "project") and m.get("bead") in items}
    created: dict[str, str] = {}
    for r, (m, _) in recs.items():
        kind = m["type"]
        if kind in ("sprint", "project"):
            if r in opened:
                created[r] = opened[r]
        elif kind == "doc":
            created[r] = m.get("date", "")
        elif kind == "postmortem":
            created[r] = r.split("/")[-1][:10]
    for r, (m, text) in recs.items():  # a design page: when the first sprint that links it opened
        if m["type"] == "sprint" and r in opened:
            for t in links(r, text, recs):
                if recs[t][0]["type"] == "design":
                    created[t] = min(created.get(t, opened[r]), opened[r])
    for r in recs:
        created.setdefault(r, "")  # a design page no sprint links, a record with no date: always there
    sprints = {}
    for r, (m, text) in sorted(recs.items()):
        if m["type"] != "sprint" or r not in opened:
            continue
        at = opened[r]

        def older(t, at=at, r=r):
            return t != r and created[t] < at

        strict = [t for t in cites(r, section(text, "Goal") + section(text, "Scope"), recs, by_name, by_bead,
                                   projects) if older(t)]
        listed = [t for t in links(r, section(text, "Design pages"), recs)
                  if older(t) and recs[t][0]["type"] == "design" and t not in strict]
        if strict or listed:
            goal = section(text, "Goal")
            sprints[r] = {"opened": at, "title": m.get("title", ""), "goal": first_sentence(goal),
                          "strict": strict, "extended": strict + listed}
    LABELS.write_text(json.dumps({"records": dict(sorted(created.items())), "sprints": sprints}, indent=1) + "\n")
    for name in ("strict", "extended"):
        s = [v[name] for v in sprints.values() if v[name]]
        print(f"{name}: {len(s)} sprints, {sum(map(len, s))} targets")


def first_sentence(goal: str) -> str:
    """The Goal's first sentence, its prompt lines dropped and each link reduced to its text."""
    text = " ".join(line for line in goal.strip().splitlines() if not line.startswith(">")).strip()
    text = re.sub(r"!?\[([^\]]*)\]\([^)]*\)", r"\1", text)
    text = " ".join(text.replace("`", "").replace("**", "").split())
    m = re.search(r"(?<=[.!?])\s+(?=[A-Z`])", text)
    return text[:m.start()] if m else text


# ---------------------------------------------------------------- the queries

PROMPT = """An agent is about to frame this sprint, and first searches the project's records (earlier sprints, \
decisions, design pages, docs) for prior work on its topic. Write 3 different keyword search queries it would type, \
each 1 to 4 words, no quotes and no operators.

Sprint title: {title}
Goal: {goal}

Answer with a JSON array of 3 strings and nothing else."""

STOP = set("""a an the and or of to in on for with by from at as is are be it its this that these those into over
under than then via per not no all any each every one two three can how what when why who which where""".split())  # noqa: SIM905


def queries() -> None:
    labels = json.loads(LABELS.read_text())["sprints"]

    def ask(rel):
        s = labels[rel]
        prompt = PROMPT.format(title=s["title"], goal=s["goal"])
        for _ in range(3):
            out = subprocess.run(["claude", "-p", "--model", "haiku", "--setting-sources", "", "--strict-mcp-config",
                                  "--tools", "", "--no-session-persistence", "--output-format", "text"],
                                 input=prompt, capture_output=True, text=True, cwd=tempfile.gettempdir(), check=False)
            m = re.search(r"\[.*\]", out.stdout, re.S)
            try:
                qs = json.loads(m.group(0)) if m else None
            except json.JSONDecodeError:
                qs = None
            if isinstance(qs, list) and len(qs) >= 3 and all(isinstance(q, str) and 1 <= len(q.split()) <= 4
                                                              for q in qs[:3]):
                return rel, qs[:3]  # a fourth query, which the model sometimes adds, is dropped
        raise SystemExit(f"{rel}: no 3 queries of 1-4 words after 3 tries: {out.stdout[:300]} {out.stderr[:300]}")

    with cf.ThreadPoolExecutor(16) as pool:
        model = dict(pool.map(ask, sorted(labels)))
    QUERIES.write_text(json.dumps({"writer": "claude -p --model haiku", "prompt": PROMPT,
                                   "sprints": {rel: model[rel] for rel in sorted(labels)}}, indent=1) + "\n")


def title_words(title: str) -> str:
    """A title's content words: lower case, possessives and punctuation dropped, no stop word or single rune."""
    words = (w.removesuffix("'s").strip(".,:;()'\"") for w in title.lower().split())
    return " ".join(w for w in words if len(w) > 1 and w not in STOP)


# ---------------------------------------------------------------- the systems


def tokens(text: str) -> list[str]:
    """pm search's tokens (internal/search Tokens): lower case, split at each rune not a letter or digit, S-stemmed."""
    words = re.split(r"[^\w]|_", text.lower())
    return [stem(w) for w in words if w]


def stem(t: str) -> str:
    if len(t) < 4:
        return t
    if t.endswith("ies") and not t.endswith(("eies", "aies")):
        return t[:-3] + "y"
    if t.endswith("es") and not t.endswith(("aes", "ees", "oes")):
        return t[:-1]
    if t.endswith("s") and not t.endswith(("us", "ss")):
        return t[:-1]
    return t


def build_searchrun(rev: str, dest: Path) -> Path:
    """searchrun built from internal/search at rev ("worktree": this checkout as it is)."""
    out = dest / f"searchrun-{rev}"
    src = ROOT
    wt = None
    if rev != "worktree":
        wt = dest / f"src-{rev}"
        subprocess.run(["git", "worktree", "add", "--detach", "-q", str(wt), rev], cwd=ROOT, check=True)
        shutil.copytree(HERE / "searchrun", wt / "tests/search_eval/searchrun", dirs_exist_ok=True)
        src = wt
    try:
        subprocess.run(["go", "build", "-tags", "gms_pure_go", "-o", str(out), "./tests/search_eval/searchrun"],
                       cwd=src, check=True)
    finally:
        if wt:
            subprocess.run(["git", "worktree", "remove", "--force", str(wt)], cwd=ROOT, check=True)
    return out


def as_of(rel: str, labels: dict, recs: dict) -> list[str]:
    at = labels["sprints"][rel]["opened"]
    return [r for r in recs if r != rel and labels["records"].get(r, "") < at]


def run_pm(binary: Path, tree: Path, labels: dict, recs: dict, qsets: dict) -> dict:
    """{(sprint, set, i): (ranked records or None per line, output text)} from pm search's lines."""
    cases = [{"id": rel, "keep": as_of(rel, labels, recs), "n": 100000,
              "items_before": labels["sprints"][rel]["opened"],
              "queries": [q.split() for q in qsets[rel]]} for rel in qsets]
    proc = subprocess.run([str(binary), "-records", str(tree), "-items", str(ITEMS)], input=json.dumps(cases),
                          capture_output=True, text=True, check=False)
    if proc.returncode:
        sys.exit(f"searchrun failed: {proc.stderr}")
    by_target = {}
    for r, (m, _) in recs.items():
        by_target[r] = r
        if m["type"] in ("sprint", "project", "design"):
            by_target.setdefault(r.split("/")[-1], r)
    out = {}
    for res in json.loads(proc.stdout):
        for i, text in enumerate(res["outputs"]):
            out[res["id"], i] = (pm_ranked(text, by_target), text)
    return out


def pm_ranked(text: str, by_target: dict) -> list[str | None]:
    """The record of each hit line of pm search's output; None for an item."""
    lines = text.splitlines()
    if len(lines) < 2 or not lines[-1].startswith("read one:"):
        return []
    ranked = []
    for line in lines[1:-1]:
        cols = re.split(r" {2,}", line.strip())
        address = cols[3]
        ranked.append(by_target.get(shlex.split(address)[0]))
    return ranked


def rg_counts(term: str, files: list[str], tree: Path) -> dict[str, int]:
    proc = subprocess.run([RG_BIN, "--count-matches", "-F", "-i", "--no-messages", "--with-filename", "--", term,
                           *[f + ".md" for f in files]], cwd=tree, capture_output=True, text=True, check=False)
    if proc.returncode > 1:  # 1 is no match
        sys.exit(f"rg failed: {proc.stderr}")
    out = {}
    for line in proc.stdout.splitlines():
        path, _, n = line.rpartition(":")
        out[path[:-3]] = int(n)
    return out


def run_rg(tree: Path, labels: dict, recs: dict, qsets: dict) -> dict:
    """{system: {(sprint, i): ranked records}} for the rg variants."""
    jobs = [(rel, i, q) for rel in qsets for i, q in enumerate(qsets[rel])]

    def one(job):
        rel, i, q = job
        files = as_of(rel, labels, recs)
        per = [rg_counts(t, files, tree) for t in q.split()]
        total: dict[str, int] = {}
        for counts in per:
            for f, n in counts.items():
                total[f] = total.get(f, 0) + n
        every = [f for f in total if all(f in c for c in per)]

        def by_count(f):
            return -total[f], f

        def newest(f):
            return labels["records"].get(f, ""), total[f], f

        return (rel, i), {"rg-and-count": sorted(every, key=by_count), "rg-or-count": sorted(total, key=by_count),
                          "rg-and-newest": sorted(every, key=newest, reverse=True)}

    out = {s: {} for s in RG}
    with cf.ThreadPoolExecutor(WORKERS) as pool:
        for key, ranks in pool.map(one, jobs):
            for s in RG:
                out[s][key] = ranks[s]
    return out


def run_bm25(labels: dict, recs: dict, qsets: dict) -> dict:
    """{(sprint, i): ranked records}: BM25 (k1 1.2, b 0.75) over whole records, any term, as-of statistics."""
    toks = {r: tokens(text) for r, (_, text) in recs.items()}
    tf = {r: {} for r in recs}
    for r, ts in toks.items():
        for t in ts:
            tf[r][t] = tf[r].get(t, 0) + 1
    out = {}
    for rel in qsets:
        docs = as_of(rel, labels, recs)
        n = len(docs)
        avg = sum(len(toks[d]) for d in docs) / max(n, 1)
        for i, q in enumerate(qsets[rel]):
            qt = list(dict.fromkeys(t for w in q.split() for t in tokens(w)))
            df = {t: sum(1 for d in docs if t in tf[d]) for t in qt}
            scored = []
            for d in docs:
                s = 0.0
                for t in qt:
                    f = tf[d].get(t, 0)
                    if f:
                        idf = math.log(1 + (n - df[t] + 0.5) / (df[t] + 0.5))
                        s += idf * f * 2.2 / (f + 1.2 * (0.25 + 0.75 * len(toks[d]) / avg))
                if s > 0:
                    scored.append((-s, d))
            out[rel, i] = [d for _, d in sorted(scored)]
    return out


# ---------------------------------------------------------------- the metrics


def firsts(ranked: list) -> dict[str, int]:
    """Each record's first rank, 1-based."""
    out = {}
    for k, r in enumerate(ranked, 1):
        if r is not None and r not in out:
            out[r] = k
    return out


def metrics(ranked: list, targets: list[str]) -> dict[str, float]:
    first = firsts(ranked)
    ranks = sorted(first[t] for t in targets if t in first)
    dcg = sum(1 / math.log2(k + 1) for k in ranks if k <= 10)
    idcg = sum(1 / math.log2(k + 1) for k in range(1, min(len(targets), 10) + 1))
    return {"R@5": sum(k <= 5 for k in ranks) / len(targets), "R@10": sum(k <= 10 for k in ranks) / len(targets),
            "MRR": 1 / ranks[0] if ranks else 0.0, "nDCG@10": dcg / idcg, "zero": float(not ranked)}


METRICS = ("R@5", "R@10", "MRR", "nDCG@10")


def per_sprint(ranks: dict, sprints: dict, name: str) -> dict[str, dict[str, dict[str, float]]]:
    """{mode: {sprint: {metric: value}}} for single (mean of the queries), best (of 3) and union."""
    out = {"single": {}, "best": {}, "union": {}}
    for rel, s in sprints.items():
        targets = s[name]
        if not targets:
            continue
        qs = [metrics(ranks[rel, i], targets) for i in range(3)]
        out["single"][rel] = {m: statistics.mean(q[m] for q in qs) for m in (*METRICS, "zero")}
        out["best"][rel] = {m: max(q[m] for q in qs) for m in METRICS}
        union = {}
        for k in (5, 10):
            got = set()
            for i in range(3):
                got |= {t for t, r in firsts(ranks[rel, i]).items() if r <= k}
            union[f"R@{k}"] = len(got & set(targets)) / len(targets)
        out["union"][rel] = union
    return out


def bootstrap(a: dict[str, float], b: dict[str, float] | None = None, reps: int = 10000) -> tuple[float, float, float]:
    """The mean difference a - b over the sprints (a's mean with no b) and its paired bootstrap 95% CI."""
    keys = sorted(a)
    d = [a[k] - (b[k] if b else 0) for k in keys]
    rng = random.Random(0)
    means = sorted(statistics.fmean(rng.choices(d, k=len(d))) for _ in range(reps))
    return statistics.fmean(d), means[int(0.025 * reps)], means[int(0.975 * reps) - 1]


def chars_to_tokens(text: str) -> int:
    return math.ceil(len(text) / 4)


def token_costs(system: str, ranks: dict, texts: dict | None, sprints: dict, tree: Path, qsets: dict) -> dict:
    """{"sprint#i": tokens to the first strict target} for the queries that reach one."""
    out = {}
    for rel, s in sprints.items():
        if not s["strict"]:
            continue
        for i in range(3):
            ranked = ranks[rel, i]
            hit = next((k for k, r in enumerate(ranked) if r in s["strict"]), None)
            if hit is None:
                continue
            if texts is not None:
                lines = texts[rel, i].splitlines()
                out[f"{rel}#{i}"] = {"lines": chars_to_tokens("\n".join(lines[:hit + 2]))}
                continue
            files = ranked[:hit + 1]
            listing = "\n".join(f + ".md" for f in files)
            terms = [t.lower() for t in qsets[rel][i].split()]
            grep = []
            for f in files:
                for n, line in enumerate((tree / (f + ".md")).read_text().splitlines(), 1):
                    if any(t in line.lower() for t in terms):
                        grep.append(f"{f}.md:{n}:{line}")
            out[f"{rel}#{i}"] = {"files": chars_to_tokens(listing), "lines": chars_to_tokens("\n".join(grep))}
    return out


# ---------------------------------------------------------------- the run


def run(args) -> None:
    labels = json.loads(LABELS.read_text())
    qdata = json.loads(QUERIES.read_text())["sprints"]
    sprints = labels["sprints"]
    sets = args.sets.split(",")
    pms = dict(p.split("=", 1) for p in args.pm)
    rgv = subprocess.run([RG_BIN, "--version"], capture_output=True, text=True, check=True).stdout.splitlines()[0]
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        tree = corpus_tree(tmp)
        recs = read_records(tree)
        model = {rel: qdata[rel] for rel in sprints}
        title = {rel: [title_words(sprints[rel]["title"])] * 3 for rel in sprints}
        with cf.ThreadPoolExecutor(len(pms)) as pool:
            bins = dict(zip(pms, pool.map(lambda rev: build_searchrun(rev, tmp), pms.values()), strict=True))
        results = {}
        for qname, qsets in (("model", model), ("title", title)):
            ranks, texts = {}, {}
            for label, binary in bins.items():
                got = run_pm(binary, tree, labels, recs, qsets)
                ranks[f"pm:{label}"] = {k: v[0] for k, v in got.items()}
                texts[f"pm:{label}"] = {k: v[1] for k, v in got.items()}
            ranks.update(run_rg(tree, labels, recs, qsets))
            ranks["bm25-or"] = run_bm25(labels, recs, qsets)
            results[qname] = (ranks, texts)
        report = {"rg": rgv, "pm": pms, "records_commit": json.loads(CORPUS.read_text())["records_commit"],
                  "sets": {}, "tokens": {}}
        for name in sets:
            n = [s for s in sprints.values() if s[name]]
            report["sets"][name] = {"sprints": len(n), "targets": sum(len(s[name]) for s in n), "queries": {}}
            for qname, (ranks, _) in results.items():
                report["sets"][name]["queries"][qname] = {sys_: per_sprint(r, sprints, name)
                                                          for sys_, r in ranks.items()}
        ranks, texts = results["model"]
        for sys_ in ranks:
            report["tokens"][sys_] = token_costs(sys_, ranks[sys_], texts.get(sys_), sprints, tree, model)
    print_report(report, list(bins), args)
    if args.json:
        Path(args.json).write_text(json.dumps(report, default=list, indent=1))


def print_report(report: dict, pm_labels: list[str], args) -> None:
    systems = [f"pm:{p}" for p in pm_labels] + list(RG) + ["bm25-or"]
    print(f"records {report['records_commit'][:8]}, {report['rg']}, pm builds {report['pm']}")
    for name, s in report["sets"].items():
        print(f"\n## {name}: {s['sprints']} sprints, {s['targets']} targets")
        for qname, by_sys in s["queries"].items():
            for mode in ("single", "best", "union"):
                ms = METRICS if mode != "union" else ("R@5", "R@10")
                if qname == "title" and mode != "single":
                    continue
                print(f"\n{qname} queries, {mode}" + (" (one query per sprint)" if qname == "title" else ""))
                print(f"{'system':16}" + "".join(f"{m:>9}" for m in ms) + ("     zero" if mode == "single" else "")
                      + "   R@10 95% CI")
                for sys_ in systems:
                    vals = by_sys[sys_][mode]
                    row = f"{sys_:16}" + "".join(f"{statistics.fmean(v[m] for v in vals.values()):9.3f}" for m in ms)
                    if mode == "single":
                        row += f"{statistics.fmean(v['zero'] for v in vals.values()):9.3f}"
                    _, lo, hi = bootstrap({k: v["R@10"] for k, v in vals.items()})
                    print(row + f"   [{lo:.3f}, {hi:.3f}]")
                best_rg = max(RG, key=lambda r: statistics.fmean(v["R@10"] for v in by_sys[r][mode].values()))
                pairs = [(p, best_rg) for p in systems[:len(pm_labels)]]
                if len(pm_labels) > 1:
                    pairs += [(f"pm:{pm_labels[-1]}", f"pm:{pm_labels[0]}")]
                pairs += [(p, "bm25-or") for p in systems[:len(pm_labels)]]
                for a, b in pairs:
                    cis = []
                    for m in ms[1:3] if mode != "union" else ms:
                        d, lo, hi = bootstrap({k: v[m] for k, v in by_sys[a][mode].items()},
                                              {k: v[m] for k, v in by_sys[b][mode].items()})
                        cis.append(f"{m} {d:+.3f} [{lo:+.3f}, {hi:+.3f}]")
                    print(f"  {a} - {b}: " + "; ".join(cis))
    print("\ntokens to the first strict target, model queries (median over the queries that reach one)")
    for sys_, per in report["tokens"].items():
        if not per:
            continue
        cols = sorted({c for v in per.values() for c in v})
        print(f"  {sys_:16} n={len(per):3} " + "  ".join(
            f"{c} {statistics.median(v[c] for v in per.values()):.0f}" for c in cols))
    last = report["tokens"][f"pm:{pm_labels[-1]}"]
    print(f"paired with pm:{pm_labels[-1]} (median over the queries both reach)")
    for sys_, per in report["tokens"].items():
        both = sorted(set(per) & set(last))
        if sys_ == f"pm:{pm_labels[-1]}" or not both:
            continue
        print(f"  {sys_:16} n={len(both):3} pm {statistics.median(last[k]['lines'] for k in both):.0f}  " + "  ".join(
            f"{c} {statistics.median(per[k][c] for k in both):.0f}" for c in sorted(per[both[0]])))


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("--pm", action="append", required=True,
                   help="LABEL=REV: a pm search build to evaluate; the last one is compared with the first")
    r.add_argument("--sets", default="strict,extended")
    r.add_argument("--json", help="write the per-sprint results here")
    f = sub.add_parser("freeze")
    f.add_argument("--export", type=Path, required=True, help="a pm export file of this repo's work store")
    sub.add_parser("queries")
    args = ap.parse_args()
    {"run": run, "freeze": lambda a: freeze(a.export), "queries": lambda a: queries()}[args.cmd](args)


if __name__ == "__main__":
    main()
