---
type: sprint
title: "Serve the site on a network without Cloudflare: reach, address and gate as separate settings"
bead: pm-bfli.5
---

## Goal

> What should be true when this sprint ends, and why now?

An owner on a company network can let colleagues open the site at `http://<host>:<port>` with no Cloudflare, by setting how far the service listens, which URL pm prints and how a non-local request is let in as three separate settings. Today the three are one coupling: the service binds 127.0.0.1 only, and a `site_url` host always means a Cloudflare Access check. Now, because a first user runs pm on an internal network and wants exactly this.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Reach: a `listen` setting in `.pm/config.toml` for the HTTP server's bind address, 127.0.0.1 by default.
- Gate: an `access` setting, `cloudflare` (today's Access JWT check, with `access_team` and `access_aud`) or `none` (the network is the gate), required once the site is reachable beyond this machine; existing Cloudflare configs keep working.
- Local trust decided by the peer's address, not the `Host` header, so a LAN client cannot claim to be localhost.
- Who may post a reply from a non-local, unauthenticated viewer: one rule, the owner's decision.
- `pm init` flags, `pm doctor`, `pm where`, the service's probes and port checks following `listen`; the `site-replies` and `pm-product` design pages; tests; a changelog entry.

**Out:**
- pm running or configuring `cloudflared` or any tunnel.
- TLS, logins or identities of its own for the network mode.
- More than one `site_url` per clone.

## Done when

> What evidence will show the goal is met?

- In a scratch clone with `listen = "0.0.0.0"`, `access = "none"` and `site_url = "http://<machine's hostname>:<port>"`, a request from another address of the machine (its LAN IP, not loopback) gets the pages, and `pm record link` prints the hostname URL.
- The same request with `Host: localhost` from the LAN address is not treated as local (test).
- Replies from that viewer follow the owner's rule (test).
- This repo's Cloudflare config (`site_url`, `access_team`, `access_aud`) still serves through the tunnel after the change, and `access_test.go`'s cases still pass.
- A non-loopback `listen` with no `access` is refused at service start and named by `pm doctor`.

## Design pages

> Where is the detail?

- [Replies on the site](../design/site-replies.md): Security rewritten for `listen`, `site_url` and `access`; local trust by the peer and the `Host`; `access = "none"` read-only beyond this machine; the POST's order of checks.
- [pm as an installable product](../design/pm-product.md): `listen` and `access` in the `config.toml` schema.
- [pm in Go](../design/pm-go.md): the HTTP row binds `listen`, not 127.0.0.1 only.
- [Views and the site](../design/views-and-site.md): the reply POST's check names `access`.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-17}
Three independent settings: listen (bind address, default 127.0.0.1), site_url (the printed link and the one allowed non-local Host), access (cloudflare or none, required when listen is not loopback or site_url is set). An existing config with access_team set and no access is migrated to access = "cloudflare" by pm init.
Cloudflare is a gate, not a reach: its tunnel connects from loopback, so "expose beyond localhost" and "use Cloudflare" are not the two axes. Reach, address and gate are, as in Grafana (http_addr, root_url, auth) and Jupyter (ip, custom_display_url, token). A required access with no inferred default fails closed.
:::

::: decision {source=agent date=2026-10-17}
A request is local only when its TCP peer is loopback and it carries no Cf-Connecting-IP; the Host header no longer grants local trust.
Today local trust rests on Host, safe only because the socket is loopback-bound. With a LAN bind any colleague could send Host: localhost and skip every check.
:::

::: decision {source=owner date=2026-10-17}
With access = "none", non-local viewers only read: their pages hide the reply forms, and POST /reply from a non-loopback peer gets a 403 naming the rule.
Colleagues asked to look, not to steer; an unauthenticated write path into agent sessions must not open as a side effect of sharing pages.
Answers `pm-bfli.5.1`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check, network mode (scratch origin and clone under /tmp/pml.8mG9, HOME
  and CODEX_HOME there, fake supervisor as tests/conftest.py's; pm 0.9.0-dev
  from 3d54ff5 of this branch): pm init --listen 0.0.0.0 alone was refused
  before any write, naming --access; pm init --listen 0.0.0.0 --access none
  --site-url http://byeeefy:42075, then pm service restart: the log says
  'listening on 0.0.0.0, access none' and pm where 'site http://byeeefy:42075
  (served by the pm service, listening on 0.0.0.0, access none)'; pm record
  link printed http://byeeefy:42075/sprints/demo-1.html.

- Live check, requests from the machine's LAN address 192.168.178.115 to port
  42075: GET / with Host byeeefy:42075 got 200 with 0 reply tokens in the page
  and the read-only rule in place of the form; GET / with Host localhost:42075
  got 403 'refused: Host "localhost:42075" is neither this machine nor the
  configured site_url'; POST /reply with the page's valid token got 403
  'refused: replies are taken only from this machine: access = "none" …'. From
  127.0.0.1 (Host localhost): GET / 200 with 1 token, POST /reply 303 and pm
  reply read printed the reply. The [TEST] need is dismissed.

- Live check, Cloudflare mode and migration in the same scratch clone: with
  site_url https://pm.example.com and access_team/access_aud but no access, pm
  init --session-start wrote access = "cloudflare" ('as its access_team
  means'); after pm service restart a tunnel-like request (loopback peer,
  Cf-Connecting-IP, Host pm.example.com, no token) got 403 'carries no
  Cloudflare Access token', localhost 200, and the LAN address no longer
  connected (listen back to loopback). The scratch service was stopped;
  nothing listens on :42075. This repo's own tunnel was not exercised: its
  service runs the 0.15.5 pin.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
