Agents open the project, sprint or task for work the owner asked for without asking leave, as they already commit,
push and open PRs.

### Changed

- `pm prime`'s rules add the invariant that agents open the project, sprint or task for work the owner asked for
  without asking: the request is the leave, and a request that states the goal confirms it. When the goal is unclear,
  asking what it is is still a fair question; asking leave to open is not.
- The owner-request Stop hook blocks a reply that asks leave to open a project, sprint or task for work the owner
  asked for ("Shall I open a sprint for this?") as a needless ask, and tells the agent to open it. Asking whether to
  open one for work the owner did not ask for, or asking what the goal is, still has to be raised as a need.

### Fixed

- The owner-request judge on Codex matches a reply's request to the open need that asks the same thing when the reply
  writes another id for it, and passes a PR review the reply cites by id as open on the site, which it blocked now
  and then.
