`pm upgrade` and `pm doctor` keep `design_sources` in `.pm/config.toml`, so a repo can set it.

### Fixed

- `pm upgrade` keeps `design_sources` in `.pm/config.toml`, and `pm doctor` no longer reports a config that sets it as
  drift. Before, pm's config writer dropped the key, so the next `pm upgrade` deleted it.
