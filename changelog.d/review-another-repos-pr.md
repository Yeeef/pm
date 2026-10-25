A pm repo can now review, close and stamp a pull request of another GitHub repo, so one pm project can track work
merged in the repos it manages, its own remote on GitHub or not.

### Added

- `pm action need --pr URL` takes another GitHub repo's PR: one whose OWNER/REPO the remote's URL does not name, or
  any GitHub PR when the remote is not on GitHub. The PR must target that repo's default branch, and the design-page
  check does not run on it, since the pages' covers are this repo's paths.
- `pm need close <review> --reason "merged as <sha>"`, at the close or at a restamp, checks another repo's PR's
  `<sha>` against the merge commit gh gives for it into that repo's default branch, instead of this repo's main.
- `pm sprint close` stamps another repo's PR as `OWNER/REPO#N` in the Outcome, and `--merged SHA --pr URL` takes
  another repo's PR the same way.
- The pm service sees another repo's PR merged into that repo's default branch and tells the session to close its
  review, without asking it to pull this repo's main.
