The site can serve a company network without Cloudflare: where it listens (`listen`), the link pm prints
(`site_url`) and how a request from beyond this machine gets in (`access`) are now three settings.

### Upgrade guide

1. Commit `.pm/config.toml` once `pm upgrade` (or `pm init`) has added `access = "cloudflare"` to a config that sets
   `access_team`. A config with `site_url` and no `access_team` gets no access: set one with
   `pm init --access cloudflare` or `pm init --access none`, or the pm service does not start.

### Added

- `pm init --listen ADDR` sets `listen`, the IP the site binds (127.0.0.1 when unset; `0.0.0.0` for every
  interface), and `pm init --access cloudflare|none` sets `access`. With `access = "none"` the network is the gate:
  colleagues read the site at `site_url` with no login, but replies are taken only from this machine; their pages
  show the rule in place of each reply form. `pm where` and the service log state a non-loopback `listen`.

### Changed

- `access` is required once `listen` is not loopback or `site_url` is set: the pm service does not start without
  it, and `pm doctor` names it.

### Security

- A request is local only when its TCP peer is loopback, its `Host` names this machine and Cloudflare's edge did not
  forward it; the `Host` header alone no longer grants local trust, so a client on the network sending
  `Host: localhost` is refused.
