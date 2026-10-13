# Releasing pm

pm releases on every merge to main that leaves an entry file in `changelog.d/`, with no one cutting it (the
`release-policy` design page holds the policy). A release is a tag `pm-v<X>` on a main commit and its GitHub release.
No code file holds `X`: `build.sh` stamps it into the binary (`-ldflags -X …/buildinfo.Version=<X>`), and an untagged
build reports `dev`, which no repo pins.

| File | Does |
|---|---|
| `build.sh OUT_DIR [pm-v<X>]` | Builds this machine's binary with `X` from the tag (else the `pm-v*` tag on HEAD, else `dev`) and packs `pm-<X>-<os>-<arch>.tar.gz` |
| `../install.sh` | A release asset with `@VERSION@` filled in: installs that release's binary to `${PM_BIN_DIR:-$HOME/.local/bin}/pm` after checking it against `SHA256SUMS`, downloading as the launcher does (curl or wget; the token path needs curl) |
| `changelog.py` | `check`, `notes X`, `pr BASE`, `plan`, `next`, `release X`: the entry files' and `CHANGELOG.md`'s rules (its preamble), and the release section written from the entries |
| `main_ci.py` | The release's wait for main's CI on its commit |
| `merge_ready.py` | `make merge-ready PR=<n>`, the check before every merge |

## The workflow

`.github/workflows/pm-release.yml` runs on each push to main (one run at a time, none cut short; a pending run is
replaced by a newer one, which releases every entry; a hand-pushed tag's run is in a group of its own):

| Job | Does |
|---|---|
| `version` | `changelog.py plan`: `release X`, or `none: <why>` (no entry file; a `release/hold`; a `CHANGELOG.md` section written by hand whose tag is not pushed yet). No release either when main moved past the commit (the newer commit's run releases). It fails while the last release tag has no GitHub release (a failed publish, finished first). Then `main_ci.py` waits for every workflow run of that push to main and requires each to pass |
| `build` | `build.sh dist pm-v<X>` at the merge commit, natively on `macos-14` (darwin-arm64) and `ubuntu-22.04` (linux-amd64), cgo needing native runners; checks the tarball holds one `pm` whose `pm version` prints `X` |
| `tag` | `changelog.py release X` at the merge commit (`X`'s section from the entry files, which it deletes), committed by `github-actions[bot]` and checked to change only `CHANGELOG.md` and entry files, then `git push --atomic` of that commit to main and the tag `pm-v<X>`, with `GITHUB_TOKEN` (`contents: write`), judged by what origin then holds. Main moved: no release from this run |
| `publish` | `SHA256SUMS` of both tarballs and `install.sh` with `X` filled in; `gh release create pm-v<X> --verify-tag` with the four assets and notes `changelog.py notes X`: `X`'s section, its upgrade guide first |

A tag pushed with `GITHUB_TOKEN` starts no workflow, so one run builds, tags and publishes; no secret is needed. A tag
pushed by hand (a release candidate, a major) runs `version` from the tag (at least 0.2.0, a release on a main commit
whose `CHANGELOG.md` holds `X`'s notes), then `build` and `publish`.

The release commits on main (by `github-actions[bot]`: `CHANGELOG.md` changed and entry files deleted, nothing else)
ship nothing, so `merge_ready.py` lets a PR head lack them when the PR touches none of their files.

## The rules

- **The version** is the last release tag (`pm-vX.Y.Z`, pre-releases left out) bumped as the entries ask
  (`changelog.py next`). While pm is 0.x: the minor when an entry has its own `### Upgrade guide` or `### Removed`,
  since a repo that moves its pin must act; every other release is a patch. 1.0 and later majors only by the owner's
  decision: `changelog.py release X --summary "…"` in a PR, merged, then the tag pushed by hand.
- **The entry is the notes.** A PR that changes what a release ships (`SHIPPED` in `changelog.py`) adds one entry
  file, `changelog.d/<slug>.md`, in the form `CHANGELOG.md`'s preamble gives; never an edit to `CHANGELOG.md`. When
  users would not notice the change, label the PR `no-changelog` instead; its merge releases nothing.
  `pm-changelog.yml` checks both on every PR.
- **Main is releasable after every merge.** A feature that lands in parts lands as one PR, through a feature branch
  merged once, or in parts users cannot reach that add no entry. When parts users can reach must land first, the first
  adds `release/hold`, one line naming the feature and the PR that completes it, and that PR deletes it; while it
  exists nothing releases, fixes included, and `plan` warns once it is older than 3 days.
- **A failed release.** A failed build or a rejected push leaves no commit and no tag: the next merge releases
  everything. A failed publish after the push leaves the tag with no release: re-run the run's failed jobs
  (`gh run rerun <id> --failed`; no asset was ever downloaded, so this moves no tag); every later run fails until then.
- **Never move or recreate a release tag, and never rebuild a release's assets**: launchers keep each binary's sha256
  in `<data dir>/pm/pins/<X>/sha256` and fail hard when a later download differs.

## By hand

- Fix or backfill a published release's notes: change its section on main, then
  `gh workflow run pm-release-notes.yml -f version=<X>`, which replaces the release's notes alone (`gh release edit`).
- A release candidate is `pm-v<X>-rc.<n>` (a GitHub pre-release), on any commit, a PR's included, to check the release
  build before the PR merges; the launcher treats it as version `X`'s pre-release, so a repo can pin it to try it. Its
  notes are `X`'s section when the changelog has one, else the section the entry files give, as `release X` would
  write it with the rc's version in its first two steps; it fails when there is no entry file.
- Moving a repo's pin is a separate, ordinary PR, by hand, when the repo wants a release (`pm upgrade --to X`, merged
  any way), pm's own repo included; `pm doctor` names a newer release. A move across several releases follows each
  one's upgrade guide.
- Install a release on a machine:
  `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<X>/install.sh | sh`. The repo is public, so
  `install.sh` and the launchers download with no token; only when that fails, a token from `$GH_TOKEN` or
  `gh auth token` makes them try the GitHub API, as a private copy of the repo needs. `$PM_RELEASE_URL` names a mirror.

## Tests

`tests/test_changelog.py` (`changelog.py`), `tests/test_ci.py` (`pm-release.yml`'s jobs and tokens, `main_ci.py`, and
the `version` and `tag` steps against a local bare origin and a fake `gh`), `tests/test_merge_ready.py`, and
`tests/test_release.py` (`install.sh` against a local server; the build itself with `PM_RELEASE_BUILD=1`, as
`pm-release-build.yml` runs it on a change that can alter the release build and on every `pm-v*` tag).
