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

::: decision {source=agent date=2026-10-10}
The Access token check ships as a Go port in Yeeef/pm, not as yeeef-agents PR #72; the review of #72 is dismissed, and the request for the team domain and AUD tag stays open.
PR #72 changes the Python pm in yeeef-agents, which Go pm replaced; pm-harness moved to Yeeef/pm on 2026-10-10.
:::

::: decision {source=owner date=2026-10-11}
The Access check also requires type "app"; release it as 0.7.1, then move the pin straight to 0.7.1 with access_team and access_aud in one PR
The meta token anyone can fetch from the login redirect is signed by the team key with the right aud and was refused only for lacking iss; the keys are public identifiers, so they go in the tracked config
:::

::: decision {source=owner date=2026-10-11}
Deliver sprint 68 end to end in this session: merge PR #49, the 0.7.1 release notes PR and the pin move to 0.7.1 with the Access keys, tag pm-v0.7.1, restart the service and run the live check
Owner said "deliver it end to end" in chat on 2026-10-11 after reviewing the plan
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

- Go port (PR #40): make test-go ok, make test 145 passed/63 skipped,
  service+doctor integration 15 passed, CI green on all 9 checks; two fresh
  reviews found no bypass, and their shared low finding (an unknown kid
  refetched the certs on every request under the cache lock) is fixed with a
  once-a-minute refetch limit.

- Live check of PR #40's build (0.9.0-dev at 4551ad4, scratch repo, pm service
  run on :18765) with the real access_team yeeef.cloudflareaccess.com and
  access_aud 7bf3…fb13: localhost 200; Host pm.yeeefs.com without a token 403;
  Cf-Connecting-IP with Host localhost 403; the real Cloudflare-signed meta
  token from the login redirect (right aud, valid signature by the team's
  keys, no iss) 403 on the issuer, so the real certs are fetched and parsed;
  made-up kid 403; flipped signature 403; Host evil.example 403; access_aud
  removed: 403 naming it, and pm doctor names it. No 403 carries X-PM-Store or
  X-PM-Version. A valid login token through the tunnel is not yet checked:
  0.6.0 refuses the two keys, so they go in with the pin move to the release
  that carries PR #40.

- Live check on this clone after the pin move to 0.7.1 (PR #51, 8a188fd) and
  pm service restart, service reporting X-PM-Version 0.7.1 on :8001: localhost
  200; Host pm.yeeefs.com without a token 403; Cf-Connecting-IP with Host
  localhost 403; the real Cloudflare-signed meta token from the login redirect
  403 'it is not an Access application token (type app)'; Host evil.example
  403; POST /reply on the public host without a token 403; no 403 carries
  X-PM-Store or X-PM-Version. https://pm.yeeefs.com unauthenticated is 302 to
  yeeef.cloudflareaccess.com's login.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: since pm 0.7.1, this clone's pm service serves a request through pm.yeeefs.com only with a valid Cloudflare Access application token, and it fails closed on a missing key or an unfetchable key set; the owner's browser login through the tunnel is the one check left, open as an owner request.

- The Access check in Go (PR #40): RS256 against the team's certs, `aud`, `iss`, `exp`, `nbf`, `iat`; requests carrying `Cf-Connecting-IP` need the token whatever their Host; any other Host is refused on every request; an unknown `kid` refetches the keys at most once a minute.
- Only `type` `app` tokens pass (PR #49), so the public `meta` token from the login redirect is refused by rule.
- `access_team` and `access_aud` config keys; `pm doctor` names a missing one.
- Released as pm 0.7.0 and 0.7.1; this repo pins 0.7.1 with both keys (PR #51).
- The site-replies design page describes the checks as the code does them.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| Tests show each refusal and the valid case, localhost unchanged | Met | Go tests in `internal/service/access_test.go` (each refusal, the valid token, localhost, edge requests), with mutation checks; CI green, 12 checks, on [PR #40](https://github.com/Yeeef/pm/pull/40), [PR #49](https://github.com/Yeeef/pm/pull/49) and [PR #51](https://github.com/Yeeef/pm/pull/51) |
| Live check on this machine: 403 without a token, normal load after the Access login | Met for the refusals; the login load waits on the owner | On :8001 with 0.7.1: Host pm.yeeefs.com with no token 403; the real meta token 403 on `type`; Host evil.example 403; localhost 200 (Findings). The browser login is an open owner request |
| The site-replies Security section matches the code | Met | [site-replies](../design/site-replies.html) Security section |
