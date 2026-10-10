---
type: sprint
title: The pm site's public host requires a valid Cloudflare Access token
bead: yeeef-agents-9va.77
---

## Goal

> What should be true when this sprint ends, and why now?

The pm service accepts a request on its public host (`site_url`) only when the request carries a valid Cloudflare Access token for this site's Access app. Today the code never checks Access: if the Access policy is missing, misconfigured or added after the tunnel route, anyone who reaches the site can post a reply, and pm pushes that reply into an agent session as the owner's instruction. After this sprint, a missing or wrong Access setup fails closed, and the design page states the threat model the code enforces.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Verify `Cf-Access-Jwt-Assertion` on every request (GET and POST) whose Host is the `site_url` host: RS256 signature against the team's published certs, `aud` equal to the configured AUD tag, `iss` equal to the team domain, and not expired.
- Repo config for the team domain and the AUD tag. With `site_url` set and either one missing, every public-host request is refused with a message that names the missing key.
- `pm doctor` reports the missing Access config.
- Requests to `127.0.0.1` or `localhost` keep today's checks.
- Update the Security section and the stale POST check order in the site-replies design page.
- Harness tests for the token checks.

**Out:**
- Cloudflare dashboard settings: the Access app, its policy, and the tunnel's "Protect with Access" option. These are the owner's.
- Public pages that skip Access, such as sharing one doc.
- Size and rate limits on replies.
- Replies forged by someone with a shell on the machine through `bd comments add`. That threat model is unchanged.

## Done when

> What evidence will show the goal is met?

- Harness tests pass and show that a public-host request is refused (403) when it has no token, a bad signature, a wrong `aud`, a wrong `iss` or an expired token; that it is accepted with a valid token; and that localhost requests are unchanged.
- On this Mac, after the owner sets the AUD tag: `curl -H 'Host: pm.yeeefs.com' http://127.0.0.1:8000/` returns 403, and https://pm.yeeefs.com loads normally after the Access login.
- The site-replies design page's Security section describes the localhost and public-host checks as the code does them.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
The Access token is required on every public-host request, page reads included, not only on POST /reply.
Pages hold project state and the reply token, and fail-closed reads are the safer default; it means a single doc cannot be shared through a Bypass path on pm.yeeefs.com, so public sharing uses a static copy hosted elsewhere.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Before this sprint the site's only owner gate on pm.yeeefs.com was the
  Cloudflare Access policy: check_reply accepted any client that loaded a page
  (the reply token is in every page) and set an allowed Host. After it, make
  test-full gives 106 passed, 35 skipped, 0 failed, and a fresh review found
  no correctness issue; its two hardening points (a tunnel rewriting Host to
  localhost, store path in 403 headers) are fixed in db9a2ea.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: the code verifies the Cloudflare Access token on every public-host and edge-forwarded request and fails closed; the live check against real Cloudflare keys waits on the owner's two Access values.

- `check_access` in the pm service: RS256 against the team's certs, `aud`, `iss`, `exp`; unknown Hosts refused; `Cf-Connecting-IP` requests need the token whatever their Host.
- `access_team` and `access_aud` config keys; `pm doctor` reports them missing.
- The site-replies design page describes the localhost and public-host checks.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| Harness tests show each refusal and the valid case | Met | `make test-full`: 106 passed, 35 skipped (live eval), 0 failed; CI green on [PR #72](https://github.com/Yeeef/yeeef-agents/pull/72) |
| Live check on this Mac: 403 without a token, normal load after the Access login | Not yet | Waits on the owner's team domain and AUD tag (request 77.3); run after the merge and a service restart |
| The site-replies Security section matches the code | Met | [site-replies](../design/site-replies.html) Security section and check order |
