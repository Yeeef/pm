---
type: doc
title: "pm: your agents' project manager"
date: 2026-10-07
project: pm-harness
---

Nine slides, about a minute each, for people who run AI coding agents. Each slide shows a headline, four or five short points and one visual; the sources, the fine print and the jokes sit in the speaker notes under it. Every number is traced to a record, a command or a file in this repo.

<section style="border:1px solid var(--line);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem">
  <div style="font:.85rem var(--mono);color:var(--muted)">1/9</div>
  <div style="display:flex;flex-wrap:wrap;gap:2rem;align-items:center;flex:1 1 auto">
    <div style="flex:1 1 20rem;min-width:0">
      <h2 style="margin:0 0 1.2rem;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">Day 3 of an agent project</h2>
      <div style="font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
        <div style="border-left:5px solid var(--block);padding-left:.8rem">Status is prose that drifts; the reasons are gone</div>
        <div style="border-left:5px solid var(--queue);padding-left:.8rem">Two sessions pick one task; a third redoes a done one</div>
        <div style="border-left:5px solid var(--run);padding-left:.8rem">The question for you is three scrolls up, in a dead session</div>
        <div style="border-left:5px solid var(--muted);padding-left:.8rem">Every session rereads the plan to find what is blocked</div>
      </div>
    </div>
    <figure style="flex:1 1 16rem;min-width:0;margin:0;display:flex;justify-content:center">
      <svg viewBox="0 0 320 220" role="img" aria-label="A dog in a burning room says: this is fine" style="width:100%;max-width:24rem;height:auto">
        <rect x="0" y="0" width="320" height="220" rx="10" fill="var(--code-bg)" stroke="var(--line)"/>
        <g opacity=".85">
          <path d="M14 206 Q24 130 40 165 Q50 110 64 150 Q74 125 84 170 Q94 160 96 206Z" fill="#f08a24"/>
          <path d="M30 206 Q40 165 50 180 Q58 150 66 185 Q76 175 82 206Z" fill="#ffd166"/>
          <path d="M224 206 Q234 120 248 160 Q256 100 272 150 Q286 125 294 175 Q302 160 306 206Z" fill="#f08a24"/>
          <path d="M244 206 Q254 160 264 180 Q272 150 282 185 Q292 175 296 206Z" fill="#ffd166"/>
        </g>
        <rect x="110" y="130" width="100" height="10" rx="3" fill="var(--muted)"/>
        <rect x="118" y="140" width="8" height="66" fill="var(--muted)"/>
        <rect x="194" y="140" width="8" height="66" fill="var(--muted)"/>
        <text x="160" y="122" font-size="40" text-anchor="middle">🐕</text>
        <text x="198" y="126" font-size="17" text-anchor="middle">☕</text>
        <rect x="46" y="14" width="228" height="40" rx="20" fill="var(--panel)" stroke="var(--line)"/>
        <path d="M150 53 L158 74 L168 53Z" fill="var(--panel)" stroke="var(--line)"/><rect x="151" y="50" width="16" height="5" fill="var(--panel)"/>
        <text x="160" y="41" font-size="19" font-family="var(--mono)" text-anchor="middle" fill="var(--fg)">session 14: this is fine</text>
      </svg>
    </figure>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Your agents ship fast on day 1. By day 3, three questions have no good answer: where is the work and why that choice, who holds what, what do they need from me.</li>
      <li>The meme: nobody knows what is blocked. The agent rewrites the status page. Again.</li>
      <li>pm answers all three. One problem per slide, then how it fits, then proof, then one command.</li>
    </ul>
  </details>
</section>

<section style="border:1px solid var(--line);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem">
  <div style="font:.85rem var(--mono);color:var(--muted)">2/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">Track work and context, structurally</h2>
  <div style="display:flex;flex-wrap:wrap;gap:2rem;align-items:center;flex:1 1 auto">
    <div style="flex:1 1 18rem;min-width:0;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
      <div style="border-left:5px solid var(--done);padding-left:.8rem">Work in a tracker: who, what, status, what blocks what</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">Context in records: goals, decisions with reasons, findings</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">Every session starts from both, in a few hundred tokens</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">Each fact has one home, so nothing drifts</div>
    </div>
    <div style="flex:1 1 18rem;min-width:0;display:flex;flex-direction:column;gap:.8rem">
      <div style="border:1px solid var(--line);border-left:6px solid var(--block);border-radius:.8rem;padding:1rem 1.2rem;background:var(--block-bg)">
        <div style="font-size:.8rem;letter-spacing:.08em;text-transform:uppercase;color:var(--block);font-weight:700">Today</div>
        <div style="font:600 clamp(1.6rem,4vw,2.4rem)/1.15 var(--display);color:var(--block);margin-top:.2rem">52 status pages,<br>7.2k tokens each</div>
      </div>
      <div style="border:1px solid var(--line);border-left:6px solid var(--done);border-radius:.8rem;padding:1rem 1.2rem;background:var(--done-bg)">
        <div style="font-size:.8rem;letter-spacing:.08em;text-transform:uppercase;color:var(--done);font-weight:700">With pm</div>
        <div style="font:600 clamp(1.6rem,4vw,2.4rem)/1.15 var(--display);color:var(--done);margin-top:.2rem">One home per fact</div>
      </div>
    </div>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Today: every status page invents its own words, so neither people nor tools can rely on them, and each session may get a different answer about what is blocked.</li>
      <li>The numbers: 52 hand-written status pages on one project, 7.2k tokens per page, read again every session. Measured on a real agent-run project (poker-ai, commit 197516a); the Problem table of the pm-harness design page.</li>
      <li>The tracker's fields are fixed, so there is nothing to reword. The records are written once and kept for good.</li>
      <li>"My agent forgot what it decided yesterday" stops being a sentence you say.</li>
    </ul>
  </details>
</section>

<section style="border:1px solid var(--line);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem">
  <div style="font:.85rem var(--mono);color:var(--muted)">3/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">Agents collaborate, with structure</h2>
  <div style="display:flex;flex-wrap:wrap;gap:2rem;align-items:center;flex:1 1 auto">
    <div style="flex:1 1 18rem;min-width:0;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
      <div style="border-left:5px solid var(--done);padding-left:.8rem">One holder per task: claim first, and a held task is refused</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">Dependencies say what is ready; blocked work waits</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">One shared store for every session, branch and worktree</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">Subagents get the same rules and hold what their session holds</div>
    </div>
    <div style="flex:1 1 20rem;min-width:0;border:1px solid var(--line);border-radius:.8rem;overflow:hidden;background:var(--code-bg);font-size:clamp(1rem,2.2vw,1.3rem)">
      <div style="display:flex;font-size:.8rem;letter-spacing:.08em;text-transform:uppercase;color:var(--muted);padding:.5rem 1rem;border-bottom:1px solid var(--line);gap:.6rem"><span style="flex:3 1 0;min-width:0">Task</span><span style="flex:2 1 0;min-width:0">Holder</span><span style="flex:2 1 0;min-width:0">State</span></div>
      <div style="display:flex;padding:.7rem 1rem;border-bottom:1px solid var(--line);align-items:center;gap:.6rem"><span style="flex:3 1 0;min-width:0">Login form</span><span style="flex:2 1 0;min-width:0;font:.9em var(--mono)">session A</span><span style="flex:2 1 0;min-width:0"><span style="background:var(--run-bg);color:var(--run);border-radius:1rem;padding:.15rem .7rem;font-weight:700">in progress</span></span></div>
      <div style="display:flex;padding:.7rem 1rem;border-bottom:1px solid var(--line);align-items:center;gap:.6rem"><span style="flex:3 1 0;min-width:0">Login tests</span><span style="flex:2 1 0;min-width:0;color:var(--muted)">—</span><span style="flex:2 1 0;min-width:0"><span style="background:var(--block-bg);color:var(--block);border-radius:1rem;padding:.15rem .7rem;font-weight:700">blocked</span></span></div>
      <div style="display:flex;padding:.7rem 1rem;align-items:center;gap:.6rem"><span style="flex:3 1 0;min-width:0">Mail sender</span><span style="flex:2 1 0;min-width:0;color:var(--muted)">—</span><span style="flex:2 1 0;min-width:0"><span style="background:var(--queue-bg);color:var(--queue);border-radius:1rem;padding:.15rem .7rem;font-weight:700">ready</span></span></div>
    </div>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Today: several sessions run at once and step on each other. Each subagent gets whatever rules its parent remembered to paste. Two agents, one task, zero chance it merges.</li>
      <li>"Refused": pm declines a claim on a task that another live session holds, and says who holds it.</li>
      <li>The board is a mock-up of what every session sees. Session C starts, sees one ready task (the mail sender), takes it. Nobody touches the form; the tests wait on it.</li>
    </ul>
  </details>
</section>

<section style="border:1px solid var(--line);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem">
  <div style="font:.85rem var(--mono);color:var(--muted)">4/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">Stop reading chat. Read the site.</h2>
  <div style="display:flex;flex-wrap:wrap;gap:2rem;align-items:center;flex:1 1 auto">
    <div style="flex:1 1 18rem;min-width:0;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
      <div style="border-left:5px solid var(--block);padding-left:.8rem">Chat is one session's stream: the wrong place to check outcomes</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">The site shows every project, sprint, design page and doc, live</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">A sprint page: goal, progress, decisions, findings, report</div>
      <div style="border-left:5px solid var(--done);padding-left:.8rem">Current on any device; nobody writes a status page</div>
    </div>
    <div style="flex:1 1 18rem;min-width:0;border:1px solid var(--line);border-radius:.8rem;overflow:hidden;background:var(--bg);font-size:clamp(1rem,2.2vw,1.25rem)">
      <div style="display:flex;gap:.4rem;padding:.6rem .9rem;background:var(--code-bg);border-bottom:1px solid var(--line)"><span style="width:.7rem;height:.7rem;border-radius:50%;background:var(--block)"></span><span style="width:.7rem;height:.7rem;border-radius:50%;background:var(--queue)"></span><span style="width:.7rem;height:.7rem;border-radius:50%;background:var(--done)"></span></div>
      <div style="padding:1rem 1.2rem;display:flex;flex-direction:column;gap:.8rem">
        <div style="font:600 clamp(1.3rem,3vw,1.8rem)/1.2 var(--display)">Sprint 72</div>
        <div style="display:flex;align-items:center;gap:.8rem"><span style="flex:0 0 auto;color:var(--muted)">Progress</span><span style="flex:1 1 auto;display:flex;gap:.25rem"><span style="flex:1;height:.9rem;border-radius:.3rem;background:var(--done)"></span><span style="flex:1;height:.9rem;border-radius:.3rem;background:var(--done)"></span><span style="flex:1;height:.9rem;border-radius:.3rem;background:var(--done)"></span><span style="flex:1;height:.9rem;border-radius:.3rem;background:var(--done)"></span><span style="flex:1;height:.9rem;border-radius:.3rem;background:var(--run)"></span><span style="flex:1;height:.9rem;border-radius:.3rem;background:var(--line)"></span><span style="flex:1;height:.9rem;border-radius:.3rem;background:var(--line)"></span></span></div>
        <div style="display:flex;align-items:center;gap:.8rem;flex-wrap:wrap"><span style="color:var(--muted)">Decisions</span><span style="font-weight:700">3</span><span style="color:var(--muted);margin-left:.6rem">Findings</span><span style="font-weight:700">5</span></div>
        <div style="display:flex;align-items:center;gap:.8rem"><span style="color:var(--muted)">Report</span><span style="background:var(--done-bg);color:var(--done);border-radius:1rem;padding:.15rem .7rem;font-weight:700">done</span></div>
      </div>
    </div>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Today: you find out what is happening by asking, or by digging through chat, which is in the agent's order and gone when the session ends. The agent asks permission for things you already allowed, and skips the one thing you wanted to see.</li>
      <li>The design pages and docs live on the site too, so you read the why and the results, not just the status. The mock-up: a sprint page with its progress, its decision and finding counts and its delivery report.</li>
    </ul>
  </details>
</section>

<section style="border:1px solid var(--line);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem">
  <div style="font:.85rem var(--mono);color:var(--muted)">5/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">What waits on you is a card</h2>
  <div style="display:flex;flex-wrap:wrap;gap:2rem;align-items:center;flex:1 1 auto">
    <div style="flex:1 1 18rem;min-width:0;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
      <div style="border-left:5px solid var(--queue);padding-left:.8rem">Decisions, actions, PR reviews: each with a reply box</div>
      <div style="border-left:5px solid var(--queue);padding-left:.8rem">Every option names its cost; a default is proposed</div>
      <div style="border-left:5px solid var(--queue);padding-left:.8rem">Your reply goes back into the session that asked</div>
      <div style="border-left:5px solid var(--block);padding-left:.8rem">A guard stops requests left only in chat</div>
    </div>
    <div style="flex:1 1 18rem;min-width:0;border:1px solid var(--line);border-left:6px solid var(--queue);border-radius:.8rem;padding:1rem 1.2rem;background:var(--queue-bg);font-size:clamp(1rem,2.2vw,1.25rem);display:flex;flex-direction:column;gap:.6rem">
      <div style="font:600 clamp(1.2rem,2.8vw,1.6rem)/1.2 var(--display)">Which port?</div>
      <div><span style="color:var(--muted)">Fact</span> 8000 is taken</div>
      <div><span style="color:var(--muted)">A</span> 8080 <span style="color:var(--muted)">·</span> one config line</div>
      <div><span style="color:var(--muted)">B</span> 8000, stop the other <span style="color:var(--muted)">·</span> an afternoon</div>
      <div><span style="color:var(--muted)">Default</span> A</div>
      <div style="border:1px solid var(--line);border-radius:.4rem;padding:.4rem .7rem;background:var(--panel);color:var(--muted)">Reply… <span style="float:right;color:var(--accent);font-weight:700">↵</span></div>
    </div>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Today: requests to you get buried in chat; the agent waits, or guesses.</li>
      <li>Every decision card has this one layout, and pm refuses an option without a cost. The reply arrives in the asking session as its next turn.</li>
      <li>The skit: "May I push my branch and open the PR?" You don't need to ask; the owner allowed that for every sprint. "Then I'll merge it too?" No, ask for a review; that click is the owner's. "Dear owner, whenever you have a sec, could you maybe…" That is a request, and it is only in chat. Put it on the site first.</li>
    </ul>
  </details>
</section>

<section style="border:1px solid var(--line);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem">
  <div style="font:.85rem var(--mono);color:var(--muted)">6/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">How it fits together</h2>
  <div style="display:flex;flex-wrap:wrap;gap:2rem;align-items:center;flex:1 1 auto">
    <div style="flex:1 1 16rem;min-width:0;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
      <div style="border-left:5px solid var(--accent);padding-left:.8rem">Agents talk to pm: claim, record, ask</div>
      <div style="border-left:5px solid var(--run);padding-left:.8rem">pm writes the tracker and the records, and checks every write</div>
      <div style="border-left:5px solid var(--queue);padding-left:.8rem">The site shows both to you; replies go back through pm</div>
      <div style="border-left:5px solid var(--block);padding-left:.8rem">pm refuses what would break a rule, and says why</div>
    </div>
    <svg viewBox="0 0 760 340" role="img" aria-label="Diagram: the agents talk to pm; pm writes the tracker and the records; the site reads both for you, and replies go back through pm" style="flex:2 1 22rem;min-width:0;width:100%;max-width:44rem;height:auto">
      <defs><marker id="arr" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" fill="var(--muted)"/></marker></defs>
      <rect x="60" y="210" width="280" height="110" rx="12" fill="var(--run-bg)" stroke="var(--run)" stroke-width="2"/>
      <text x="200" y="258" font-size="30" font-weight="700" text-anchor="middle" fill="var(--run)">the work</text>
      <text x="200" y="294" font-size="18" text-anchor="middle" fill="var(--fg)">tracker</text>
      <rect x="420" y="210" width="280" height="110" rx="12" fill="var(--done-bg)" stroke="var(--done)" stroke-width="2"/>
      <text x="560" y="258" font-size="30" font-weight="700" text-anchor="middle" fill="var(--done)">the context</text>
      <text x="560" y="294" font-size="18" text-anchor="middle" fill="var(--fg)">records</text>
      <rect x="250" y="60" width="260" height="90" rx="12" fill="var(--accent-soft)" stroke="var(--accent)" stroke-width="2"/>
      <text x="380" y="118" font-size="40" font-weight="700" text-anchor="middle" fill="var(--accent)">pm</text>
      <line x1="330" y1="150" x2="230" y2="208" stroke="var(--muted)" stroke-width="3" marker-end="url(#arr)"/>
      <line x1="430" y1="150" x2="530" y2="208" stroke="var(--muted)" stroke-width="3" marker-end="url(#arr)"/>
      <text x="80" y="100" font-size="44" text-anchor="middle">🤖</text>
      <text x="80" y="134" font-size="18" text-anchor="middle" fill="var(--muted)">agents</text>
      <line x1="118" y1="105" x2="248" y2="105" stroke="var(--muted)" stroke-width="3" marker-end="url(#arr)"/>
      <text x="690" y="46" font-size="40" text-anchor="middle">🧑‍💻</text>
      <text x="690" y="76" font-size="18" text-anchor="middle" fill="var(--muted)">you</text>
      <rect x="560" y="90" width="150" height="60" rx="10" fill="var(--queue-bg)" stroke="var(--queue)" stroke-width="2"/>
      <text x="635" y="129" font-size="24" font-weight="700" text-anchor="middle" fill="var(--queue)">the site</text>
      <line x1="690" y1="84" x2="665" y2="88" stroke="var(--queue)" stroke-width="3" marker-end="url(#arr)"/>
      <line x1="600" y1="152" x2="330" y2="210" stroke="var(--queue)" stroke-width="3" stroke-dasharray="6 4" marker-end="url(#arr)"/>
      <line x1="630" y1="152" x2="590" y2="208" stroke="var(--queue)" stroke-width="3" stroke-dasharray="6 4" marker-end="url(#arr)"/>
      <line x1="560" y1="112" x2="512" y2="106" stroke="var(--queue)" stroke-width="3" marker-end="url(#arr)"/>
    </svg>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Four pieces: the work in a tracker, the context in records, pm in the middle, a site for you. The dashed arrows are the site reading both layers.</li>
      <li>Problem one: tracker and records, one home per fact. Problem two: one holder per task, one store for all sessions. Problem three: the site, the cards and the guard.</li>
      <li>Rules that hold because a tool checks them, not because an agent remembered them.</li>
    </ul>
  </details>
</section>

<section style="border:1px solid var(--line);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem">
  <div style="font:.85rem var(--mono);color:var(--muted)">7/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">Proof: pm runs itself</h2>
  <div style="display:flex;flex-wrap:wrap;gap:2rem;align-items:center;flex:1 1 auto">
    <div style="flex:1 1 18rem;min-width:0;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
      <div style="border-left:5px solid var(--accent);padding-left:.8rem">Built with pm from its first sprint</div>
      <div style="border-left:5px solid var(--accent);padding-left:.8rem">This deck is a pm record, reworked in a tracked task</div>
      <div style="border-left:5px solid var(--accent);padding-left:.8rem">Two test jobs on every PR; the install is tested on a fresh repo</div>
      <div style="border-left:5px solid var(--accent);padding-left:.8rem">Every active day gets a generated day page; nobody writes one</div>
    </div>
    <div style="flex:1 1 18rem;min-width:0;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:.8rem">
      <div style="background:var(--accent-soft);border-radius:.8rem;padding:1rem;text-align:center"><div style="font:600 clamp(2.4rem,6vw,3.6rem)/1 var(--display);color:var(--accent)">69</div><div style="font-size:clamp(1rem,2vw,1.25rem);margin-top:.3rem">sprints</div></div>
      <div style="background:var(--accent-soft);border-radius:.8rem;padding:1rem;text-align:center"><div style="font:600 clamp(2.4rem,6vw,3.6rem)/1 var(--display);color:var(--accent)">65</div><div style="font-size:clamp(1rem,2vw,1.25rem);margin-top:.3rem">PRs merged</div></div>
      <div style="background:var(--accent-soft);border-radius:.8rem;padding:1rem;text-align:center"><div style="font:600 clamp(2.4rem,6vw,3.6rem)/1 var(--display);color:var(--accent)">2</div><div style="font-size:clamp(1rem,2vw,1.25rem);margin-top:.3rem">postmortems</div></div>
      <div style="background:var(--code-bg);border-radius:.8rem;padding:1rem;text-align:center"><div style="font-size:clamp(2.2rem,5vw,3rem);line-height:1">🐶🍽️</div><div style="font-size:clamp(1rem,2vw,1.25rem);margin-top:.3rem;font-weight:700">Dogfooded daily</div></div>
    </div>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>The tracked task: this version is task 83.1 in sprint 74 of the pm-harness project; the first rework was in sprint 72.</li>
      <li>69 sprints opened on the pm-harness project: the highest sprint number in the records store on 2026-10-07.</li>
      <li>65 pull requests merged into main: <code>gh pr list --state merged</code> on 2026-10-07.</li>
      <li>2 postmortems, each linked to the sprint it hit: the records' postmortems folder.</li>
      <li>The two test jobs are unit then integration, in the repo's test workflow; the install test sets up a brand-new repo in the suite.</li>
      <li>When the guard blocked its own author's turn, the guard was right.</li>
    </ul>
  </details>
</section>

<section style="border:2px solid var(--accent);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem;justify-content:center">
  <div style="font:.85rem var(--mono);color:var(--muted)">8/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">One command</h2>
  <div style="font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35">In a clone of your GitHub repo, with git, <a href="https://docs.astral.sh/uv/">uv</a> and <a href="https://github.com/gastownhall/beads">bd</a> installed:</div>
  <pre style="font-size:clamp(1rem,2.4vw,1.5rem);line-height:1.5;padding:1.2rem 1.4rem;border:2px solid var(--accent);background:var(--accent-soft);border-radius:.8rem;white-space:pre-wrap;word-break:break-all;user-select:all;-webkit-user-select:all;margin:0;overflow:hidden">uvx --from "git+https://github.com/Yeeef/yeeef-agents@pm-v0.1.2#subdirectory=pm" pm init</pre>
  <div style="font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
    <div style="border-left:5px solid var(--accent);padding-left:.8rem">It writes pm's files and hooks into the repo</div>
    <div style="border-left:5px solid var(--accent);padding-left:.8rem">It creates the records branch and starts the site</div>
    <div style="border-left:5px solid var(--accent);padding-left:.8rem">Run it again and it changes nothing</div>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Click the box to select the whole command.</li>
      <li>bd is <a href="https://github.com/gastownhall/beads">Beads</a>, the tracker. The repo needs an <code>origin</code> remote on GitHub.</li>
      <li>Options: <code>--site-url URL</code> makes links public; <code>PORT=&lt;n&gt; pm init</code> when port 8000 is taken.</li>
    </ul>
  </details>
</section>

<section style="border:2px solid var(--accent);border-radius:1.2rem;padding:clamp(1.2rem,4vw,3rem);margin:1.5rem 0;background:var(--panel);max-width:100%;min-height:70vh;display:flex;flex-direction:column;gap:1.5rem;justify-content:center">
  <div style="font:.85rem var(--mono);color:var(--muted)">9/9</div>
  <h2 style="margin:0;font-size:clamp(1.9rem,5vw,2.8rem);line-height:1.1">Then three steps</h2>
  <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr));gap:1rem">
    <div style="background:var(--accent-soft);border-radius:.8rem;padding:1.2rem;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35"><div style="font:600 clamp(2rem,5vw,3rem)/1 var(--display);color:var(--accent);margin-bottom:.6rem">1</div>Commit and push the files it lists</div>
    <div style="background:var(--accent-soft);border-radius:.8rem;padding:1.2rem;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35"><div style="font:600 clamp(2rem,5vw,3rem)/1 var(--display);color:var(--accent);margin-bottom:.6rem">2</div>Start a session: it gets pm's rules and the live state</div>
    <div style="background:var(--accent-soft);border-radius:.8rem;padding:1.2rem;font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35"><div style="font:600 clamp(2rem,5vw,3rem)/1 var(--display);color:var(--accent);margin-bottom:.6rem">3</div>Open a project and a sprint; your agents take it from there</div>
  </div>
  <div style="font-size:clamp(1.1rem,2.2vw,1.3rem);line-height:1.35;display:flex;flex-direction:column;gap:.6rem">
    <div style="border-left:5px solid var(--accent);padding-left:.8rem">Works with Claude Code and Codex</div>
    <div style="border-left:5px solid var(--accent);padding-left:.8rem">Your agents will still argue with you. Now on a card, with the reason on record.</div>
  </div>
  <details style="font-size:.9rem;color:var(--muted);line-height:1.5"><summary style="cursor:pointer;font-weight:600">Speaker notes</summary>
    <ul style="margin:.5rem 0 0;padding-left:1.2rem">
      <li>Step 3 in commands: <code>pm project open &lt;name&gt; --title "…"</code>, then <code>pm sprint open &lt;name&gt; --title "…"</code>.</li>
      <li>Session start runs the hooks the command installed: they print pm's rules and the live state in a few hundred tokens.</li>
    </ul>
  </details>
</section>
