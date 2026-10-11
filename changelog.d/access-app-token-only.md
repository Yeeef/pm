### Security

- The Cloudflare Access check accepts only an application token (`type` `app`). The login redirect hands anyone a
  `meta` token signed by the team's key for the site's `aud`, which 0.7.0 refused only because it carries no `iss`;
  it is now refused by rule, as is the team's global session token (`org`).
