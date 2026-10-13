pm releases on every merge to main that carries a changelog entry, with no one cutting the release, and the version
says whether moving a pin needs action: a minor release asks for a step beyond installing it and `pm upgrade`, a
patch release asks for none.

### Changed

- A release is published by CI on every merge to main that leaves an entry in `changelog.d/`, once every check on
  the merge commit passed. A merge with no entry releases nothing, and a `release/hold` file on main, naming the
  feature it waits for, holds every release until the PR that completes the feature deletes it.
- While pm is 0.x, a release is a minor one only when it asks a repo that moves its pin for a step beyond installing
  the release and `pm upgrade --to X` (its upgrade guide is longer than those two steps) or removes something; every
  other release, new features included, is a patch. 1.0 and later majors are cut by hand.
- Each repo still moves its pin when it chooses, by its own PR; `pm doctor` names a newer release.
