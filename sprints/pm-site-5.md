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

None yet.

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

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
