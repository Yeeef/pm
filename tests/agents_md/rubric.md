# The AGENTS.md rubric

AGENTS.md (CLAUDE.md is a link to it) is the file every coding-agent session that develops pm loads whole at its start,
Claude Code and Codex alike. Its reader is an agent changing pm's own code, tests, hooks and site. That agent can read
the checkout itself: the code and its doc comments, the test modules' docstrings, the Makefile and its comments, the
workflows, `release/README.md`, and every `pm <noun> --help`. Sessions in this repo also load `prime.md`, which
`pm prime` prints at session start, so whatever `prime.md` says reaches them already. AGENTS.md is for what that agent
cannot infer from those and would get wrong untold, and for pointers to where the rest lives.

Each criterion below is judged pass or fail on its own.

- A fail names a concrete defect. Each finding quotes the AGENTS.md text it is about verbatim, copied character for
  character (a short quote, one sentence or one table row at most), says what is wrong, and gives the evidence: a
  `file:line` in the checkout, or the passage elsewhere that the text copies or contradicts. A rule that AGENTS.md
  lacks (the complete criterion) is a finding with an empty quote.
- No evidence, no fail. When in doubt about a claim, look it up with the tools (Read, Grep, Glob) before you judge.
  A claim you checked and could not refute passes. Style preferences, wording you would choose differently and
  improvements that fix no defect are not findings.
- Report only what clearly breaks the criterion, a defect an editor of AGENTS.md would fix once shown it. A passage
  that could be read either way passes.
- A criterion with no finding passes.
- The rules the complete criterion lists belong in AGENTS.md even where the code also states them: a rule of that
  list, stated in a sentence or two (what triggers it and what to do), is never a non_inferable, map or no_copies
  defect.

## accurate

Every claim AGENTS.md makes about pm holds against the checkout: the code, the commands and Make targets, the files and
paths, the flags and environment variables, the tests and test markers, the CI workflows, and every number (a size, a
count, a timeout, a version).

- Fail: a Make target, flag, file, function, test or environment variable that does not exist; a behaviour the code
  does not have (a target said to run another that it does not run, a check said to refuse what it accepts); a number
  that differs from the code's.
- Check each claim in the part you are given against the code: open the Makefile for a target, grep for a flag, a
  function or an environment variable, read the workflow for a CI claim.
- Pass: every claim you checked holds. A rule about how agents must work (a policy, not a fact about the code) is not
  an accuracy claim.

## non_inferable

AGENTS.md holds only what an agent cannot infer from the checkout: no passage restates what the code, its doc comments
or docstrings, the Makefile and its comments, or a `--help` text already says.

- Fail: a file-by-file or function-by-function tour of a package; a narration of what a test, a fixture or a fake
  does step by step; a description of a target's internals that its Makefile comment already gives; a list that copies
  a `--help` text.
- Pass: commands, rules and conventions that the code does not state, the reasons behind them, and short pointers
  ("the hook entries are `claudeHooks` and `codexHooks` in `internal/install/pieces.go`"). A table of the Make
  targets an agent runs, each with a line on what it is for and when to run it, passes.
- Pass: a Map row's one-line summary of an area that points at where its detail lives (a package doc comment, a
  docstring, a file) is what `map` asks for, never a non_inferable defect. A tour of an area's files or functions
  still fails.
- Exempt: a passage stating one of the must-know rules is never a defect under this criterion. They are: merging
  through `make merge-ready`; not running the whole integration set locally; a `changelog.d/` entry or the
  `no-changelog` label; never moving a release tag; golden files rewritten with `-update`; `make go-test-work
  UPDATE=1` for a new or renamed `internal/work` test; when a test is marked `integration`; `PM_LAUNCHED`; the
  owner-request rule's three texts and its case labels; a hook change editing both `claudeHooks` and `codexHooks`;
  `[TEST]` needs; live checks in a scratch clone.

## map

AGENTS.md is a map: it points to where the detail lives (a file, a package, a doc, a `--help`, a design page) instead of
holding that detail.

- Fail: a passage that holds detail of one area at the depth of a package's documentation: the internals of a
  subsystem, its protocol or schema, every parameter of a mechanism, a long paragraph or table row that walks through
  one component. Several sentences of such detail where one sentence and a pointer would do.
- Pass: each area gets a line or a short paragraph and a pointer to where its detail lives.
- Exempt: a passage stating one of the must-know rules is never a defect under this criterion. They are: merging
  through `make merge-ready`; not running the whole integration set locally; a `changelog.d/` entry or the
  `no-changelog` label; never moving a release tag; golden files rewritten with `-update`; `make go-test-work
  UPDATE=1` for a new or renamed `internal/work` test; when a test is marked `integration`; `PM_LAUNCHED`; the
  owner-request rule's three texts and its case labels; a hook change editing both `claudeHooks` and `codexHooks`;
  `[TEST]` needs; live checks in a scratch clone.

## no_copies

Nothing in AGENTS.md copies `prime.md` (every session in this repo loads it already) or the preamble of `CHANGELOG.md`
(its rules for entries and releases, which `release/changelog.py check` enforces). Both are given below.

- Fail: a sentence, rule or list that restates a passage of either, verbatim or nearly so (the same rule in the same
  words, or reworded with nothing added).
- Pass: AGENTS.md names `prime.md` or the changelog rules and points to them, or states a rule for pm's developers
  that neither holds.
- Exempt: a passage stating one of the must-know rules is never a defect under this criterion. They are: merging
  through `make merge-ready`; not running the whole integration set locally; a `changelog.d/` entry or the
  `no-changelog` label; never moving a release tag; golden files rewritten with `-update`; `make go-test-work
  UPDATE=1` for a new or renamed `internal/work` test; when a test is marked `integration`; `PM_LAUNCHED`; the
  owner-request rule's three texts and its case labels; a hook change editing both `claudeHooks` and `codexHooks`;
  `[TEST]` needs; live checks in a scratch clone.

## actionable

Every instruction is something an agent can do: exact commands, paths and flags.

- Fail: a prohibition with no instruction for what to do instead ("don't break X" with no command, check or step that
  keeps X working); a vague directive ("be careful with", "make sure it works") with no command or check.
- Pass: each "never" or "do not" comes with the command, check or alternative to use, in the same sentence or the
  same paragraph.

## no_history

AGENTS.md states the current rules, not their history.

- Fail: a dated measurement or run result ("on 2026-10-12: 189/189, median 2.0 s"); a record of past runs, past
  sizes or past counts; a decision trail ("we used to", "was changed in sprint N", "after the incident of").
- Pass: a rule may name the reason it exists. A configured value that the code holds (a timeout, a cap) is not
  history. A pointer to where results are recorded is not history.

## consistent

Nothing in AGENTS.md contradicts another passage of AGENTS.md, `prime.md`, or the checks the code enforces (a Make
target's refusal, a test's assertion, a workflow's condition).

- Fail: two passages that cannot both be followed; a rule that a check in the checkout rejects; a procedure that
  `prime.md` states differently.
- Pass: no such pair. Two passages that cover the same topic at different depths, without conflict, pass.

## complete

Each rule an agent developing pm must know and would get wrong untold is present in AGENTS.md, stated or pointed to
with the rule itself named:

1. A PR merges only through `make merge-ready PR=<n>`, run right before the merge.
2. Never run the whole integration test set locally: CI runs it on every PR; locally run only the tests a change
   touches (`make test-full ARGS="-k …"`).
3. A change to what a release ships adds one entry file under `changelog.d/`, or the PR gets the `no-changelog`
   label; never an edit to `CHANGELOG.md` for it.
4. Never move or recreate a release tag (or rebuild a release's assets).
5. Golden files are rewritten with `-run Golden -update` and the diff is reviewed in the PR.
6. A new or renamed `internal/work` test needs `make go-test-work UPDATE=1`.
7. When a test is marked `integration` (what it starts that only an integration test may).
8. A test that pins another pm version and does not test the launch sets `PM_LAUNCHED=<pin>`.
9. The owner-request rule is one rule in three texts (the rule in `prime.md`, the judge prompt, the block reasons),
   changed in one commit with a labelled case for each case it moves.
10. A labelled case's label comes from the rule, never from what the judge answers.
11. A hook change edits both `claudeHooks` and `codexHooks`.
12. A need raised during a test or live check starts its title with `[TEST]` and is dismissed once the check is done.
13. A live check of `pm init`, `pm push` or the service runs in a scratch clone of a scratch origin, with `HOME` and
    `CODEX_HOME` in a temp directory, never on a real clone.

- Fail: a rule of the list that AGENTS.md does not state, or states only in a form that loses its point. One finding
  per missing rule: an empty quote, and the rule's number and words as the problem.
- Pass: every rule of the list is present.
