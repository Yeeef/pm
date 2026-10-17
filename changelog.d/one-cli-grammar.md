Every command names its scope with `--sprint`, `--task` or `--project`, and one verb, `pm need close`, closes a need
that sets no rule.

### Breaking changes

- A need names its sprint or task with `--sprint ID` or `--task ID`, in place of `--parent`, and a decision's
  level is the scope flag alone. Migration:
  `pm decision need --parent ID …` → `pm decision need --sprint ID …` (or `--task ID`);
  `pm action need --parent ID …` → `pm action need --sprint ID …` (or `--task ID`);
  `pm decision add --level project --project NAME …` → `pm decision add --project NAME …`;
  `pm decision add --level sprint --sprint ID …` → `pm decision add --sprint ID …`;
  `pm task add --parent TASK …` → `pm task add --task TASK …`.
- `pm need close ID --reason "…"` closes a decision need whose answer sets no rule (with the answer as its body), an
  action, a PR review and a question; `--dropped` closes any open need as dismissed. Migration:
  `pm decision close ID --reason "…" --text-file -` → `pm need close ID --reason "…" --text-file -`;
  `pm action done ID --reason "merged as <sha>"` → `pm need close ID --reason "merged as <sha>"`;
  `pm need dismiss ID --reason "…"` → `pm need close ID --dropped --reason "…"`.
- Each removed flag and verb is refused, naming the form that replaced it; none is kept as an alias.

### Changed

- The positional argument of `pm project open` and `pm project close` is named `project` in their help, as the
  naming grammar names it.
