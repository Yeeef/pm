# changelog.d

The changes merged since the last release, one file per change: `changelog.d/<slug>.md`, the slug in lowercase
letters, digits and `-`. A release writes its section of `CHANGELOG.md` from them and deletes them; this README stays.

An entry opens with a summary paragraph for pm's users, then the `###` categories it touches (`### Upgrade guide`
first, numbered from `1. `, naming no step by its number), each with its `- ` bullets; `CHANGELOG.md`'s preamble
holds the rules and `release/changelog.py check` checks them.
