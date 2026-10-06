"""HTML rendering of records joined with live Beads status, as a static site."""

from __future__ import annotations

import html
import re
import shutil
import dataclasses
import time
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from markdown_it import MarkdownIt
from mdit_py_plugins.anchors import anchors_plugin
from mdit_py_plugins.container import container_plugin
from mdit_py_plugins.front_matter import front_matter_plugin

from .beads import (ACTION, HUMAN, NO_DECISION, REPLY_AUTHOR, ancestors, blockers, kind, owner_tasks, picked_up,
                    reply_body, state)
from .records import (BLOCK_ATTRS, BLOCKS, Record, RecordError, attrs, decisions, first_para, outcome,
                      project_of, section_text, validate_record)

STYLE = Path(__file__).resolve().parent.parent / "style.css"

PILL = {"done": ("done", "DONE"), "running": ("run", "RUNNING"),
        "blocked": ("blocked", "BLOCKED"), "ready": ("queued", "READY")}


def pill(st: str) -> str:
    cls, label = PILL[st]
    return f'<span class="pill {cls}">{label}</span>'


@dataclass
class Ctx:
    beads: dict[str, dict]
    rel: str = ""                      # path of the page being rendered, for errors
    mermaid: bool = False


def make_md(ctx: Ctx) -> MarkdownIt:
    md = MarkdownIt("commonmark", {"html": True}).enable("table")
    md.use(front_matter_plugin)
    md.use(anchors_plugin, min_level=2, max_level=3)

    def block(name: str):
        def render(self, tokens, idx, options, env):
            tok = tokens[idx]
            if tok.nesting == -1:
                return "</div>\n"
            a = attrs(tok.info)
            for req in BLOCK_ATTRS.get(name, []):
                if req not in a:
                    raise RecordError(f"{ctx.rel}: ::: {name} needs '{req}'")
            if name == "note":
                return '<div class="note">\n'
            if name == "result":
                return f'<div class="result"><p class="m-sec">{html.escape(a["title"])}</p>\n'
            if name == "decision":
                meta = f'<span>{html.escape(a["source"])}</span><span>{html.escape(a["date"])}</span>'
                if "until" in a:
                    meta += f'<span class="until">until {html.escape(a["until"])}</span>'
                return f'<div class="decision"><div class="meta">{meta}</div>\n'
        return render

    for name in BLOCKS:
        md.use(container_plugin, name=name, render=block(name))

    default_fence = md.renderer.rules["fence"]

    def fence(tokens, idx, options, env):
        tok = tokens[idx]
        if tok.info.strip() == "mermaid":
            ctx.mermaid = True
            return f'<pre class="mermaid">{html.escape(tok.content)}</pre>\n'
        return default_fence(tokens, idx, options, env)

    md.renderer.rules["fence"] = fence

    def table_open(tokens, idx, options, env):
        return '<div class="tbl"><table>\n'

    def table_close(tokens, idx, options, env):
        return "</table></div>\n"

    def link_open(tokens, idx, options, env):
        href = tokens[idx].attrs.get("href", "")
        if not re.match(r"^[a-z]+:", href):
            tokens[idx].attrs["href"] = re.sub(r"\.md(#|$)", r".html\1", href)
        return md.renderer.renderToken(tokens, idx, options, env)

    md.renderer.rules["link_open"] = link_open
    md.renderer.rules["table_open"] = table_open
    md.renderer.rules["table_close"] = table_close
    return md


def toc(tokens) -> str:
    items, open_sub = [], False
    for i, t in enumerate(tokens):
        if t.type != "heading_open" or t.tag not in ("h2", "h3"):
            continue
        text = html.escape(tokens[i + 1].content)
        href = t.attrs.get("id", "")
        if t.tag == "h2":
            items.append("</ul></li>" if open_sub else ("</li>" if items else ""))
            open_sub = False
            items.append(f'<li><a href="#{href}">{re.sub(r"^[0-9]+\. ", "", text)}</a>')
        else:
            if not open_sub:
                items.append("<ul>")
                open_sub = True
            items.append(f'<li><a href="#{href}">{text}</a></li>')
    if not items:
        return ""
    items.append("</ul></li>" if open_sub else "</li>")
    return '<nav class="toc"><p class="cap">CONTENTS</p><ol>' + "".join(items) + "</ol></nav>"


def link(frm: Record | None, to: str) -> str:
    depth = frm.rel.count("/") if frm else 0
    return "../" * depth + to


def task_graph(sprints: list[dict], beads: dict[str, dict]) -> str:
    """Mermaid graph: one box per sprint holding its tasks, coloured by state, arrows for blockers."""
    nid = lambda i: re.sub(r"\W", "_", i)
    label = lambda s: s.replace('"', "#quot;")
    lines, edges = ["flowchart LR"], []
    for sp in sorted(sprints, key=lambda i: i["id"]):
        lines.append(f'  subgraph {nid(sp["id"])}["{label(sp["title"])}"]')
        lines.append("    direction TB")
        tasks = sorted((i for i in beads.values() if i.get("parent") == sp["id"]), key=lambda i: i["id"])
        for t in tasks:
            who = " · " + t["assignee"] if t.get("assignee") and state(t, beads) != "done" else ""
            lines.append(f'    {nid(t["id"])}["{label(t["title"])}{label(who)}"]:::{state(t, beads)}')
            edges += [f"  {nid(b)} --> {nid(t['id'])}" for b in blockers(t)]
        if not tasks:
            lines.append(f'    {nid(sp["id"])}_empty["no tasks yet"]')
        lines.append("  end")
    lines += edges
    lines += [
        "  classDef done fill:#e4f2e7,stroke:#2c7a3f,color:#1d2321",
        "  classDef running fill:#e3edf8,stroke:#1d5fa8,color:#1d2321",
        "  classDef ready fill:#f6efd9,stroke:#8a6a12,color:#1d2321",
        "  classDef blocked fill:#f7e3e1,stroke:#a8322d,color:#1d2321",
    ]
    legend = ('<p class="meta">' + " ".join(pill(s) for s in ("done", "running", "ready", "blocked")) +
              "<span>arrows: must finish first</span></p>\n")
    return '<pre class="mermaid">' + html.escape("\n".join(lines)) + "</pre>\n" + legend


def inline(text: str, frm: Record) -> str:
    """Render one line of Markdown, with .md links pointing at rendered pages relative to `frm`."""
    out = MarkdownIt("commonmark").renderInline(text)
    return re.sub(r'href="([^":]+?)\.md"', lambda m: f'href="{m.group(1)}.html"', out)


def progress(project: Record, recs: list[Record], beads: dict[str, dict], frm: Record) -> str:
    """Generated Progress section of a project: a graph of sprints and tasks, then a sprint table."""
    root = project.meta["bead"]
    graph = task_graph([i for i in beads.values() if i.get("parent") == root], beads)
    rows = []
    for r in sorted((r for r in recs if r.type == "sprint" and project_of(r, recs, beads) is project),
                    key=lambda r: r.meta["bead"]):
        b = beads[r.meta["bead"]]
        report = outcome(r)
        rows.append(f'<tr><td><a href="{link(frm, r.out)}">{html.escape(r.title)}</a></td>'
                    f'<td>{html.escape(b["created_at"][:10])}</td>'
                    f'<td>{inline(first_para(section_text(r.body, "Goal")), frm)}</td>'
                    f'<td>{inline(report, frm) if report else "<span class=empty>not closed</span>"}</td>'
                    f'<td>{pill(state(b, beads))}</td></tr>')
    table = ('<div class="tbl"><table><tr><th>Sprint</th><th>Opened</th><th>Goal</th>'
             '<th>Outcome</th><th>Status</th></tr>' + "".join(rows) + "</table></div>\n") if rows else ""
    return graph + table


def doc_list(docs: list[Record], frm: Record | None, heading: str = "h2", label: str = "Docs") -> str:
    """Generated list of dated records (docs or postmortems), newest first, linked relative to `frm`; empty when
    there are none."""
    if not docs:
        return ""
    lis = "".join(f'<li><a href="{link(frm, d.out)}">{html.escape(d.title)}</a>'
                  f'<span class="k">{html.escape(str(d.meta["date"]))}</span></li>'
                  for d in sorted(docs, key=lambda d: (str(d.meta["date"]), d.rel), reverse=True))
    return f'<{heading} id="{label.lower()}">{label}</{heading}><ul class="list">{lis}</ul>\n'


def sprint_docs(sprint: Record, recs: list[Record], beads: dict[str, dict]) -> list[Record]:
    """Docs whose bead is the sprint or one of its tasks."""
    sid = sprint.meta["bead"]
    return [r for r in recs if r.type == "doc" and "bead" in r.meta
            and (r.meta["bead"] == sid or sid in ancestors(beads, r.meta["bead"]))]


def project_docs(project: Record, recs: list[Record], beads: dict[str, dict], kind: str = "doc") -> list[Record]:
    """Records of `kind` (doc or postmortem) under the project, its sprints' included."""
    return [r for r in recs if r.type == kind and project_of(r, recs, beads) is project]


def sprint_postmortems(sprint: Record, recs: list[Record]) -> list[Record]:
    return [r for r in recs if r.type == "postmortem" and r.meta.get("sprint") == sprint.meta["bead"]]


def local_day(ts: str | None) -> str | None:
    """Local calendar date of a Beads UTC timestamp."""
    if not ts:
        return None
    return datetime.fromisoformat(ts.replace("Z", "+00:00")).astimezone().date().isoformat()


def sprint_title(sp: dict, recs: list[Record], frm: Record) -> str:
    """A sprint's title, linked to its record when it has one."""
    rec = next((r for r in recs if r.type == "sprint" and r.meta["bead"] == sp.get("id")), None)
    title = html.escape(sp.get("title", ""))
    return f'<a href="{link(frm, rec.out)}">{title}</a>' if rec else title


DAY_VERBS = (("closed_at", "closed", "done"), ("started_at", "started", "run"), ("created_at", "opened", "queued"))


def day_moves(day: str, beads: dict[str, dict], under: str) -> list[tuple[dict, bool, list[tuple[str, str, dict]]]]:
    """Per sprint under the project epic `under` that moved on `day`: the sprint, whether it opened that day, and
    (verb, pill class, task) for each task opened, started or closed that day (its latest move only)."""
    out = []
    for sp in sorted((i for i in beads.values() if i.get("parent") == under and i.get("issue_type") == "epic"),
                     key=lambda i: i["id"]):
        rows = []
        for t in sorted((i for i in beads.values() if i.get("parent") == sp["id"]), key=lambda i: i["id"]):
            hit = next(((verb, cls) for f, verb, cls in DAY_VERBS if local_day(t.get(f)) == day), None)
            if hit:
                rows.append((*hit, t))
        opened = local_day(sp.get("created_at")) == day
        if rows or opened:
            out.append((sp, opened, rows))
    return out


def day_activity(day: str, beads: dict[str, dict], under: str, recs: list[Record], frm: Record) -> str:
    """Generated Sprints section of a day: per sprint, the tasks opened, started and closed that day."""
    out = []
    for sp, opened, moves in day_moves(day, beads, under):
        rows = ['<li><span class="pill queued">OPENED</span> this sprint</li>'] if opened else []
        rows += [f'<li><span class="pill {cls}">{verb.upper()}</span> {html.escape(t["title"])}'
                 f' <span class="k">{html.escape(t["id"])}</span></li>' for verb, cls, t in moves]
        out.append(f'<section class="card"><h4>{pill(state(sp, beads))}{sprint_title(sp, recs, frm)}</h4>'
                   f'<ul class="list">{"".join(rows)}</ul></section>')
    return "".join(out) or '<p class="empty">No sprint activity in Beads on this day.</p>'


def activity_days(recs: list[Record], beads: dict[str, dict]) -> set[str]:
    """Every date with activity a day page shows: a sprint opened or a task opened, started or closed in a project,
    or a doc dated that day."""
    days = {str(r.meta["date"]) for r in recs if r.type == "doc"}
    for p in (r for r in recs if r.type == "project"):
        for sp in (i for i in beads.values() if i.get("parent") == p.meta["bead"] and i.get("issue_type") == "epic"):
            days.add(local_day(sp.get("created_at")))
            for t in (i for i in beads.values() if i.get("parent") == sp["id"]):
                days.update(local_day(t.get(f)) for f, _, _ in DAY_VERBS)
    days.discard(None)
    return days


def with_days(recs: list[Record], beads: dict[str, dict], summaries: dict[str, dict]) -> list[Record]:
    """`recs` plus a generated day record, with no file, for each date with activity or a summary but no day
    record; every day carries its generated summary (days/<date>.summary.json, from pm day summarize) if it has one."""
    have = {str(r.meta["date"]) for r in recs if r.type == "day"}
    out = [dataclasses.replace(r, summary=summaries.get(str(r.meta["date"]))) if r.type == "day" else r
           for r in recs]
    for day in sorted((activity_days(recs, beads) | set(summaries)) - have):
        out.append(Record(Path("days") / f"{day}.md", f"days/{day}", {"type": "day", "date": day}, "## Today\n",
                          summary=summaries.get(day)))
    return out


def summary_md(rec: Record) -> str:
    """A day's generated summary, labelled with the time it was generated, as an HTML block for the page's Markdown;
    empty without one. The model's text is rendered with raw HTML escaped, and without blank lines, which would end
    the HTML block."""
    s = rec.summary
    if not s:
        return ""
    at = datetime.fromisoformat(s["generated_at"]).astimezone().strftime("%H:%M")
    text = re.sub(r"\n\s*\n", "\n", COMMENT_MD.render(s["text"].strip()))
    return f'<p class="meta"><span>generated at {at}</span></p>\n{text}'


def day_today(rec: Record) -> str:
    """A day's Today line for lists: its generated summary, else its hand-written paragraph."""
    return rec.summary["text"].strip() if rec.summary else first_para(section_text(rec.body, "Today"))


AWAITING = {
    "decision": ("decisions-await-you", "Decisions await you", "The owner chooses; the agent records the answer.",
                 "No decisions waiting on the owner."),
    "action": ("actions-await-you", "Actions await you", "The owner does something; the agent closes it once it "
               "sees it done.", "No actions waiting on the owner."),
}


def review_of(i: dict) -> dict | None:
    """The review context `pm action need --pr` stores in an issue's metadata, checked, or None for any other issue."""
    meta = i.get("metadata") or {}
    review = meta.get("review") if isinstance(meta, dict) else None
    if review is None:
        return None
    where = f"{i['id']} ({i['title']}): metadata.review"
    fix = (f"; every pm command refuses until it is fixed with bd update {i['id']} --metadata '{{...}}' or the "
           f"issue is closed with bd close {i['id']}")
    if not isinstance(review, dict):
        raise RecordError(f"{where} is not an object with pr, sprints and focus{fix}")
    for key, want in (("pr", str), ("focus", str), ("sprints", list), ("designs", list)):
        value = review.get(key, [] if key == "designs" else None)
        ok = (isinstance(value, str) if want is str
              else isinstance(value, list) and all(isinstance(x, str) for x in value))
        if not ok:
            raise RecordError(f"{where} needs {key!r} as {'a string' if want is str else 'a list of strings'}{fix}")
    return review


def sprint_reviews(beads: dict[str, dict], sprint_id: str) -> list[tuple[dict, dict]]:
    """(issue, review) of each PR review naming the sprint, open or closed (a review of a merged PR is often
    dismissed), by id: the PRs that deliver it."""
    reviews = ((i, review_of(i)) for i in sorted(beads.values(), key=lambda i: i["id"]))
    return [(i, r) for i, r in reviews if r and sprint_id in r["sprints"]]


def pr_label(url: str) -> str:
    n = re.search(r"/pull/(\d+)", url)
    return f"PR #{n.group(1)}" if n else "PR"


def review_designs(review: dict, recs: list[Record]) -> list[Record]:
    """The design pages behind a review: those named when it was raised, then those its sprints' records list."""
    sprints = [r for r in recs if r.type == "sprint" and r.meta["bead"] in review.get("sprints", [])]
    slugs = list(review.get("designs", []))
    for r in sprints:
        slugs += re.findall(r"\(\.\./design/([a-z0-9-]+)\.md[)#]", section_text(r.body, "Design pages"))
    by_slug = {r.name: r for r in recs if r.type == "design"}
    return [by_slug[s] for s in dict.fromkeys(slugs) if s in by_slug]


def review_context(review: dict, beads: dict[str, dict], recs: list[Record], frm: Record | None,
                   md: MarkdownIt) -> str:
    """What the owner reads before a PR's diff: the PR, the sprints it delivers, the design pages behind it, and
    what to focus on, each linked to its page."""
    pr = html.escape(review["pr"])
    sprints = ", ".join(sprint_title(beads.get(s, {"id": s, "title": s}), recs, frm) for s in review["sprints"])
    written = [r for r in recs if r.type == "sprint" and r.meta["bead"] in review["sprints"] and outcome(r)]
    if written:
        sprints += " · " + ", ".join(f'<a href="{link(frm, r.out)}#delivery-report">delivery report: '
                                     f'{html.escape(r.title)}</a>' for r in written)
    designs = ", ".join(f'<a href="{link(frm, r.out)}">{html.escape(r.title)}</a>'
                        for r in review_designs(review, recs)) or '<span class="empty">none listed</span>'
    return (f'<ul class="list review"><li>Pull request: <a href="{pr}">{pr}</a></li><li>Delivers: {sprints}</li>'
            f'<li>Design pages: {designs}</li></ul><p class="m-sec">Focus</p>{md.render(review["focus"])}')


def request_place(i: dict, beads: dict[str, dict], project_bead: str) -> tuple[str | None, str | None]:
    """(sprint id, task id) a request sits under in the project, from its parent chain: the sprint is the ancestor
    just below the project, the task the request's own parent when that is below the sprint; None where absent."""
    chain = ancestors(beads, i["id"])
    below = chain[:chain.index(project_bead)] if project_bead in chain else []
    return (below[-1] if below else None), (below[0] if len(below) > 1 else None)


def card_where(i: dict, proj: Record, beads: dict[str, dict], recs: list[Record], frm: Record | None) -> str:
    """Where a request sits: its project, then its sprint, then its task, each project and sprint linked to its
    page."""
    sid, tid = request_place(i, beads, proj.meta["bead"])
    parts = [f'<a href="{link(frm, proj.out)}">{html.escape(proj.title)}</a>']
    if sid:
        parts.append(sprint_title(beads.get(sid, {"id": sid, "title": sid}), recs, frm))
    if tid:
        parts.append(f'task {html.escape(beads.get(tid, {}).get("title", tid))}')
    return " · ".join(parts)


COMMENT_MD = MarkdownIt("commonmark", {"html": False})  # comments and model summaries: their HTML is escaped


def thread(i: dict) -> str:
    """An open request's comments (the owner's replies and any other), oldest first, each with its author, time and
    whether it reached the agent's session; a request read without its comments says only whether a reply waits."""
    comments, seen, n = i.get("comments") or [], picked_up(i), 0
    if not comments:
        return ""  # the first `seen` site replies reached the session; other comments are no replies
    out = []
    for c in comments:
        author = "you, on the site" if c.get("author") == REPLY_AUTHOR else c.get("author") or "unknown"
        when = (c.get("created_at") or "").replace("T", " ").removesuffix("Z")
        status = ""
        if c.get("author") == REPLY_AUTHOR:
            status = (' · <span class="picked">delivered to the agent&#x27;s session</span>' if n < seen else
                      f' · <span class="replied">{NOT_DELIVERED}</span>')
            n += 1
        out.append(f'<div class="reply-item"><p class="k">{html.escape(author)} · {html.escape(when)} UTC{status}</p>'
                   f'{COMMENT_MD.render(reply_body(c.get("text") or ""))}</div>')
    return f'<div class="replies"><p class="k">Replies</p>{"".join(out)}</div>'


NOT_DELIVERED = ("not delivered yet: the session that asked is not running, or the push failed and pm serve tries "
                 "again; the next session sees it")
DELIVERY = {"delivered": "delivered to the agent's session",
            "nothing to deliver": "the agent had read it already",
            "not running": "not delivered: the session that asked is not running, the next one will see it",
            "failed": "not delivered yet: the push failed; pm serve tries again every minute"}


def owner_card(i: dict, where: str, beads: dict[str, dict], md: MarkdownIt, recs: list[Record],
               frm: Record | None) -> str:
    """One open `human` issue as a card: its description (for a PR review, the review context), where it sits,
    and how it gets closed."""
    review = review_of(i)
    desc = (review_context(review, beads, recs, frm, md) if review else
            md.render(i.get("description") or "") or '<p class="empty">No details given.</p>')
    how = (f'Reply below, or answer with <code>bd human respond {html.escape(i["id"])}</code>; the agent that '
           "asked records your answer." if kind(i) == "decision"
           else "Reply below with the evidence once done; the agent checks it and closes this.")
    return (f'<div class="card need" id="need-{html.escape(i["id"])}"><h4>{pill(state(i, beads))}'
            f'{html.escape(i["title"])}</h4><p class="k">{where} · {html.escape(i["id"])}</p>{desc}'
            f'<p class="k">{how}</p>{thread(i)}{REPLY_SLOT.format(id=html.escape(i["id"]), kind=kind(i))}</div>')


# The slot a card leaves for its reply form. pm serve fills it with a form carrying the server's token on every page
# it serves; the static site keeps the comment, so it shows no form that could not work.
REPLY_SLOT = "<!--pm-reply {id} {kind}-->"
# Each submit carries a reply id, new unless the text is the one the id was made for (a double click, or a failed
# reply sent again as it came back), so pm serve stores a resent reply once.
REPLY_FORM = ('<form class="reply" method="post" action="/reply" data-text="{text}" onsubmit="'
              'const t = this.elements.text.value, r = this.elements.rid; '
              'if (!r.value || this.dataset.text !== t) {{ r.value = crypto.randomUUID(); this.dataset.text = t; }}">'
              '<input type="hidden" name="token" value="{token}"><input type="hidden" name="id" value="{id}">'
              '<input type="hidden" name="rid" value="{rid}"><textarea name="text" rows="3" required '
              'placeholder="{hint}">{text}</textarea><button type="submit">Send reply</button></form>')


# A reply on its way shows with its text until a snapshot read after it was stored lists it among the card's replies.
REPLY_NOTE = {"saving": '<div class="reply-item"><p class="k replied">Saving your reply…</p>{text}</div>',
              "sent": '<div class="reply-item"><p class="k replied">Reply saved; {delivery}.</p>{text}</div>',
              "failed": '<p class="k failed">Your reply was not stored: {error} Its text is back in the box; send it '
                        'again.</p>'}


def fill_replies(page: str, token: str, replies: dict[str, dict] | None = None) -> str:
    """`page` with each card's reply slot replaced by its form, carrying `token` for the POST. `replies` holds the
    server's replies still on their way to Beads, by issue id: {"state": "saving" | "sent" | "failed", "error",
    "text", "rid", "delivery" (for a sent one: a DELIVERY key)}; each shows with its text above its card's form, and a failed one puts its text back in the box with
    its reply id, so sending it again unchanged stores it once."""
    hints = {"decision": "Your answer: the option you choose, or what you want instead",
             "action": "The evidence that it is done: a command's output, a link"}

    def form(m: re.Match) -> str:
        r = (replies or {}).get(html.unescape(m.group(1))) or {}
        delivery = DELIVERY.get(r.get("delivery", ""), NOT_DELIVERED)
        note = (REPLY_NOTE[r["state"]].format(error=html.escape(r.get("error", "")), delivery=delivery,
                                              text=COMMENT_MD.render(r.get("text", ""))) if r else "")
        failed = r.get("state") == "failed"
        text, rid = (html.escape(r["text"]), html.escape(r["rid"])) if failed else ("", "")
        return note + REPLY_FORM.format(token=html.escape(token), id=m.group(1), hint=hints[m.group(2)], text=text,
                                        rid=rid)
    return re.sub(r"<!--pm-reply (\S+) (decision|action)-->", form, page)


def awaiting(cards: list[tuple[dict, str]], prompt: bool) -> str:
    """The 'Decisions await you' and 'Actions await you' sections from (issue, card) pairs; with `prompt`, each
    opens with the generated-section prompt the day page shows."""
    out = []
    for k, (anchor, heading, what, empty) in AWAITING.items():
        body = "".join(c for i, c in cards if kind(i) == k) or f'<p class="empty">{empty}</p>'
        quote = (f"<blockquote><p>{what} Generated from Beads: open issues labelled <code>{HUMAN}</code>"
                 + (f" and <code>{ACTION}</code>" if k == "action" else f", without <code>{ACTION}</code>")
                 + ".</p></blockquote>\n") if prompt else ""
        out.append(f'<h2 id="{anchor}">{heading}</h2>\n{quote}{body}')
    return "\n".join(out)


# The slot for the line stating the age of a page's data. pm serve fills it on every page it serves; the static site
# keeps the comment, since a file in site/ is as old as its make render.
STATUS_SLOT = "<!--pm-status-->"

PAGE = """<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{title}</title>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&family=IBM+Plex+Mono:wght@400;500&family=Fraunces:opsz,wght@9..144,600&display=swap">
<link rel="stylesheet" href="{css}">
</head><body><div class="wrap">{status}
<nav class="topbar"><a href="{home}">Home</a>{crumbs}</nav>
<div class="eyebrow">{kind}</div>
<h1>{title}</h1>
{body}
</div>{mermaid}</body></html>
"""

MERMAID = """
<script type="module">
import mermaid from "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs";
const dark = matchMedia("(prefers-color-scheme: dark)").matches;
mermaid.initialize({ startOnLoad: true, theme: dark ? "dark" : "default" });
</script>
"""


def page(rec: Record | None, kind: str, title: str, body: str, ctx: Ctx, crumbs: str = "") -> str:
    return PAGE.format(status=STATUS_SLOT, title=html.escape(title), css=link(rec, "style.css"),
                       home=link(rec, "index.html"), crumbs=crumbs, kind=html.escape(kind), body=body,
                       mermaid=MERMAID if ctx.mermaid else "")


def with_docs(body_md: str, before: str, docs: list[Record], postmortems: list[Record], frm: Record) -> str:
    """Insert the generated doc and postmortem lists before the `before` heading line."""
    listing = doc_list(docs, frm) + doc_list(postmortems, frm, label="Postmortems")
    if not listing:
        return body_md
    return re.sub(rf"(?m)^{re.escape(before)}$", lambda mm: listing + "\n" + mm.group(0), body_md, count=1)


Dates = dict[str, tuple[str, str]] | None  # (created, last updated) by design record rel, from store.design_dates


def design_dates_meta(rec: Record, dates: Dates) -> str:
    """A design page's created and last-updated dates as meta spans; empty for a render that only validates."""
    if dates is None:
        return ""
    created, updated = dates[rec.rel]
    return f'<span>created {html.escape(created)}</span><span>updated {html.escape(updated)}</span>'


def render_record(rec: Record, recs: list[Record], beads: dict[str, dict], dates: Dates = None) -> str:
    ctx = Ctx(beads, rec.rel)
    md = make_md(ctx)
    m = rec.meta
    validate_record(rec, recs, beads)
    proj = project_of(rec, recs, beads) if rec.type != "day" else None  # days are repo-wide
    crumbs = f' / <a href="{link(rec, proj.out)}">{html.escape(proj.title)}</a>' if proj and rec.type != "project" else ""

    pre = ""
    body_md = rec.body
    if rec.type == "project":
        ctx.mermaid = True
        generated = progress(rec, recs, beads, rec)
        body_md = re.sub(r"(## Progress\n(?:\n?>.*\n)*)", lambda mm: mm.group(1) + "\n" + generated + "\n", body_md, count=1)
        body_md = with_docs(body_md, "## Outcome", project_docs(rec, recs, beads),
                            project_docs(rec, recs, beads, "postmortem"), rec)
    elif rec.type == "sprint":
        b = beads[m["bead"]]
        pre = (f'<p class="meta">{pill(state(b, beads))}<span>{html.escape(m["bead"])}</span>'
               f'<span>opened {html.escape(b["created_at"][:10])}</span></p>')
        ctx.mermaid = True
        generated = task_graph([b], beads)
        body_md = re.sub(r"(## Progress\n(?:\n?>.*\n)*)", lambda mm: mm.group(1) + "\n" + generated + "\n", body_md, count=1)
        # The sprint's own requests, its PR review included, between Progress and Decisions.
        needs = awaiting([(i, owner_card(i, card_where(i, proj, beads, recs, rec), beads, md, recs, rec))
                          for i in owner_tasks(beads, m["bead"])], prompt=True)
        body_md = re.sub(r"(?m)^## Decisions$", lambda mm: needs + "\n\n" + mm.group(0), body_md, count=1)
        body_md = with_docs(body_md, "## Delivery report", sprint_docs(rec, recs, beads),
                            sprint_postmortems(rec, recs), rec)
    elif rec.type == "day":
        projects = [r for r in recs if r.type == "project"]
        generated = awaiting([(i, owner_card(i, card_where(i, p, beads, recs, rec), beads, md, recs, rec))
                              for p in projects for i in owner_tasks(beads, p.meta["bead"])], prompt=True)
        generated += ('\n<h2 id="sprints">Sprints</h2>\n<blockquote><p>Which sprints moved today, and what changed? '
                      'Generated from Beads: tasks opened, started or closed on this date.</p></blockquote>\n'
                      + "".join(f'<h3>{html.escape(p.title)}</h3>' + day_activity(str(m["date"]), beads, p.meta["bead"], recs, rec)
                                for p in projects))
        generated += doc_list([r for r in recs if r.type == "doc" and str(r.meta["date"]) == str(m["date"])], rec)
        if rec.summary:  # the generated summary opens Today, above an older day's hand-written paragraph
            body_md = re.sub(r"(## Today\n(?:\n?>.*\n)*)", lambda mm: mm.group(1) + "\n" + summary_md(rec) + "\n",
                             body_md, count=1)
        elif not rec.text:  # a generated day without a summary
            body_md += "\nNo summary has been generated for this day.\n"
        body_md = body_md.rstrip("\n") + "\n\n" + generated + "\n"
    elif rec.type == "doc":
        where = (f'<span>{html.escape(m["bead"])}</span>' if "bead" in m else "")
        pre = f'<p class="meta"><span>{html.escape(str(m["date"]))}</span>{where}</p>'
    elif rec.type == "postmortem":
        where = (f'<span>{sprint_title(beads.get(m["sprint"], {"id": m["sprint"], "title": m["sprint"]}), recs, rec)}'
                 '</span>' if "sprint" in m else "")
        pre = f'<p class="meta"><span>{html.escape(str(m["date"]))}</span>{where}</p>'
    elif rec.type == "design" and dates is not None:
        pre = f'<p class="meta">{design_dates_meta(rec, dates)}</p>'

    tokens = md.parse(body_md)
    content = md.renderer.render(tokens, md.options, {})
    nav = toc(tokens) if rec.type == "design" else ""
    return page(rec, rec.type, rec.title, pre + nav + content, ctx, crumbs)


def render_index(recs: list[Record], beads: dict[str, dict], site_name: str, dates: Dates = None) -> str:
    """Root page: the overview of everything open, across all projects. Design pages are listed by last update,
    newest first (by title within a day), with both dates; without `dates`, by title."""
    ctx = Ctx(beads)
    md = MarkdownIt("commonmark", {"html": True})
    projects = [r for r in recs if r.type == "project"]

    cards = [(i, owner_card(i, card_where(i, p, beads, recs, None), beads, md, recs, None))
             for p in projects for i in owner_tasks(beads, p.meta["bead"])]
    out = [awaiting(cards, prompt=False)]

    days = sorted((r for r in recs if r.type == "day"), key=lambda r: str(r.meta["date"]), reverse=True)[:7]
    if days:
        lis = "".join(f'<li><a href="{r.out}">{html.escape(r.title)}</a>'
                      f'<span class="k">{COMMENT_MD.renderInline(day_today(r)) if r.summary else inline(day_today(r), r)}</span></li>'
                      for r in days)
        out.append(f'<h2 id="days">Days</h2><ul class="list">{lis}</ul>')

    for p in projects:
        out.append(f'<h2><a href="{p.out}">{html.escape(p.title)}</a></h2>')
        sprint_recs = {r.meta["bead"]: r for r in recs if r.type == "sprint"}
        rows = []
        for sp in sorted((i for i in beads.values() if i.get("parent") == p.meta["bead"] and i.get("issue_type") == "epic"),
                        key=lambda i: i["id"]):
            tasks = [t for t in beads.values() if t.get("parent") == sp["id"]]
            done = sum(t["status"] == "closed" for t in tasks)
            rec = sprint_recs.get(sp["id"])
            title = (f'<a href="{rec.out}">{html.escape(sp["title"])}</a>' if rec else html.escape(sp["title"]))
            rows.append(f'<li>{pill(state(sp, beads))} {title}'
                        f'<span class="k">{done} of {len(tasks)} tasks done</span></li>')
        out.append(f'<h3>Sprints</h3><ul class="list">{"".join(rows)}</ul>' if rows else "")
        designs = sorted((r for r in recs if r.type == "design" and project_of(r, recs, beads) is p),
                         key=lambda r: r.title)
        if dates is not None:
            designs.sort(key=lambda r: dates[r.rel][1], reverse=True)  # stable, so titles stay ordered within a day
        if designs:
            lis = "".join(f'<li><a href="{r.out}">{html.escape(r.title)}</a>'
                          f'<span class="k">{design_dates_meta(r, dates)}</span></li>' for r in designs)
            out.append(f'<h3>Design pages</h3><ul class="list">{lis}</ul>')
        out.append(doc_list(project_docs(p, recs, beads), None, "h3"))
        out.append(doc_list(project_docs(p, recs, beads, "postmortem"), None, "h3", "Postmortems"))
    return page(None, "overview", site_name, "".join(out), ctx)




def dismissed(issue: dict) -> bool:
    """Closed with bd human dismiss, which sets the reason to "Dismissed" or, with a note, "Dismissed: <note>"."""
    return (issue.get("close_reason") or "").startswith("Dismissed")


def cites(text: str, issue_id: str) -> bool:
    """Whether `text` names `issue_id` as a whole id (not a prefix of a longer one)."""
    return re.search(rf"(?<![\w.-]){re.escape(issue_id)}(?![\w-]|\.\w)", text) is not None


def check_needs_answered(recs: list[Record], beads: dict[str, dict], ids: set[str] | None = None) -> None:
    """Every answered decision need (a closed `human` issue inside a project, not an action, not dismissed) is
    either cited by the body of some decision in a project or sprint record, so a rule the owner set lives in the
    records, or labelled `no-decision`, the explicit mark that its answer sets no rule. `ids` limits the check to
    those issues."""
    epics = {r.meta["bead"] for r in recs if r.type == "project"}
    bodies = [body for r in recs if r.type in ("project", "sprint") for _, body in decisions(r.text)]
    for i in sorted(beads.values(), key=lambda i: i["id"]):
        labels = i.get("labels") or []
        if ids is not None and i["id"] not in ids:
            continue
        if (i["status"] == "closed" and HUMAN in labels and kind(i) == "decision" and NO_DECISION not in labels
                and not dismissed(i) and epics & set(ancestors(beads, i["id"]))
                and not any(cites(b, i["id"]) for b in bodies)):
            raise RecordError(f"need {i['id']} ({i['title']}) is closed but no decision cites it; record the "
                              f"owner's answer with pm decision add --need {i['id']}, or, if the answer sets no "
                              f"rule, close it with pm decision close {i['id']} --reason \"<why>\"")


def render_pages(recs: list[Record], beads: dict[str, dict], site_name: str, dates: Dates = None,
                 summaries: dict[str, dict] | None = None) -> dict[str, str]:
    """Every page of the site, keyed by its path under the site; raises RecordError on an invalid record.
    `dates` are the design pages' dates; a render that only validates leaves them out, and its pages show none.
    A day page is rendered for every date with activity, with its generated summary from `summaries`."""
    check_needs_answered(recs, beads)
    recs = with_days(recs, beads, summaries or {})
    pages = {r.out: render_record(r, recs, beads, dates) for r in recs}
    pages["index.html"] = render_index(recs, beads, site_name, dates)
    return pages



def render_page(path: str, recs: list[Record], beads: dict[str, dict], site_name: str, dates: Dates,
                summaries: dict[str, dict] | None = None) -> str | None:
    """The page at `path`, rendered as render_pages renders it, or None when the site has no such page; the caller has
    already run check_needs_answered on the same records and Beads. A record that fails to render fails only its own
    page here, where render_pages fails the whole site."""
    recs = with_days(recs, beads, summaries or {})
    if path == "index.html":
        return render_index(recs, beads, site_name, dates)
    rec = next((r for r in recs if r.out == path), None)
    return render_record(rec, recs, beads, dates) if rec else None

def write_site(pages: dict[str, str], site: Path) -> None:
    if site.exists():
        shutil.rmtree(site)
    for rel, text in pages.items():
        dest = site / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(text)
    shutil.copy(STYLE, site / "style.css")


SERVE_BEHIND = 10  # seconds a page from pm serve may be behind the records and Beads before it says it is behind
STATUS = ('<p class="asof{behind}" data-asof="{asof:.3f}" data-page="{digest}">{text}</p>\n<script>{script}</script>')
# Keeps the stated age current and polls /version: an unchanged page takes the newer age; a changed one never
# reloads by itself (owner decision: auto reloads flash the page) but shows a sticky "Newer data: reload" banner the
# owner clicks. That reload keeps the reader where they were: it saves the scroll position and drops the URL's
# #fragment (a reply's redirect to its card), which would otherwise re-jump on every reload. It asks first when a reply
# box holds unsent text, which a reload would discard.
STATUS_SCRIPT = """(() => {
const el = document.currentScript.previousElementSibling, behind = %d, key = "pm-scroll:" + location.pathname;
let newer = false, gone = false;
function reload() {
  const draft = [...document.querySelectorAll("textarea")].some(t => t.value.trim());
  if (draft && !confirm("Reload and discard the reply you have not sent?")) return;
  try { sessionStorage.setItem(key, String(scrollY)); } catch (e) {}
  if (location.hash) history.replaceState(null, "", location.pathname + location.search);
  location.reload();
}
let saved = null;
try { saved = sessionStorage.getItem(key); sessionStorage.removeItem(key); } catch (e) {}
if (saved !== null) addEventListener("load", () => scrollTo(0, +saved));
function show() {
  const asof = +el.dataset.asof, age = Math.max(0, Math.round(Date.now() / 1000 - asof));
  const at = new Date(asof * 1000).toLocaleTimeString([], {hour12: false});
  el.classList.toggle("behind", !newer && age > behind);
  el.textContent = `Data as of ${at}` + (newer ? "; newer data exists, reload to see it" : ` (${age} s ago)`) +
    (gone ? "; the server is not answering" : !newer && age > behind ? "; other sessions are writing, so it is behind" : "");
  if (newer && !document.querySelector(".newer")) {
    document.body.insertAdjacentHTML("afterbegin", '<a class="newer" href="">Newer data: reload</a>');
    document.querySelector(".newer").onclick = e => { e.preventDefault(); reload(); };
  }
}
async function poll() {
  try {
    const r = await fetch("/version?page=" + encodeURIComponent(location.pathname), {cache: "no-store"});
    const v = await r.json();
    gone = false;
    if (v.page === el.dataset.page) el.dataset.asof = v.asof;
    else newer = true;
  } catch (e) { gone = true; }
  show();
  setTimeout(poll, 2000);
}
show(); setInterval(show, 1000); setTimeout(poll, 2000);
})();""" % SERVE_BEHIND


def fill_status(page: str, as_of: float, digest: str, now: float) -> str:
    """`page` with its status slot replaced by the line stating its data's age: as of `as_of` (epoch seconds), the
    time from which the records and Beads it shows are known current, and `digest`, which /version compares."""
    age = max(0, round(now - as_of))
    text = (f"Data as of {time.strftime('%H:%M:%S', time.localtime(as_of))} ({age} s ago)"
            + ("; other sessions are writing, so it is behind" if age > SERVE_BEHIND else ""))
    return page.replace(STATUS_SLOT, STATUS.format(behind=" behind" if age > SERVE_BEHIND else "", asof=as_of,
                                                   digest=digest, text=text, script=STATUS_SCRIPT), 1)
