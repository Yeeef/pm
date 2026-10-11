### Upgrade guide

1. In a repo whose `.pm/config.toml` sets `site_url`, put the site behind a Cloudflare Access application and add
   `access_team = "<team>.cloudflareaccess.com"` (the Access team domain) and `access_aud = "<AUD tag>"` (the
   application's AUD tag) to `.pm/config.toml`, commit them, then run `pm service restart` in each clone. Until then
   the site refuses every request through the tunnel; localhost is served as before. A repo without `site_url` needs
   no step.

### Breaking changes

- With `site_url` set, the pm service serves a request to its host, or any request Cloudflare's edge forwarded
  (it carries `Cf-Connecting-IP`, so a tunnel that rewrites `Host` to localhost is no way around it), only when its
  `Cf-Access-Jwt-Assertion` verifies: RS256 by a key from `https://<access_team>/cdn-cgi/access/certs`, `aud` holding
  `access_aud`, `iss` `https://<access_team>`, unexpired. It fails closed: without either key, or when the team's keys
  cannot be fetched, every such request gets a 403 saying why. The tunnel exposed the site, and a reply posted there
  becomes an instruction in an agent session; before this, anyone who reached the tunnel could read every page and post
  one (see step 1).

### Added

- `.pm/config.toml` takes `access_team` and `access_aud`, and `pm doctor` names the one a set `site_url` lacks.

### Security

- Every request now names this machine (127.0.0.1, localhost) or `site_url`'s host, or gets a 403: a page, and the
  reply token on it, is no longer served to a rebinding domain; only a reply was checked before. A 403 carries neither
  the store-path header nor the build header.
