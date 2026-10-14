UV ?= uv
# The harness (tests/, pytest) runs in the environment uv keeps at .venv, from pyproject.toml's dev group.
PYRUN := $(UV) run --quiet

.PHONY: test test-full test-live test-live-parity test-live-agents-md agents-md-check test-go go-build go-fmt go-vet go-test go-test-but-work go-test-work \
	go-os-check go-test-work-macos go-test-but-work-os go-test-work-os go-test-race-os \
	go-build-pm go-test-race go-test-race-service go-test-race-work merge-ready lint lint-go lint-deadcode lint-py

# ARGS goes to pytest: make test-full ARGS="-k serve" runs only the integration tests a change touches.
ARGS ?=

# Run the light harness tests (pm's commands, records, hooks) in parallel against temp repos, on the pm go-build
# builds; run while working.
test: go-build
	$(PYRUN) python tests/run.py -n auto -m "not integration" $(ARGS)

# Run the harness tests ARGS selects, the integration ones too (the pm service, pm init, push, the session-start
# hook). CI runs the whole integration set on every PR, so locally it refuses to run without ARGS; CI=1 forces it.
test-full: go-build
	@if [ -z "$(ARGS)" ] && [ -z "$(CI)" ]; then \
	  echo 'make test-full: CI runs every integration test on each PR; do not run the whole set locally.' >&2; \
	  echo '  Run the tests your change touches: make test-full ARGS="-k <name>"' >&2; \
	  echo '  Watch CI instead: gh pr checks <n> --watch. Force the whole set: CI=1 make test-full' >&2; \
	  exit 2; fi
	$(PYRUN) python tests/run.py -n auto $(ARGS)

# Check the owner-request Stop hook's judge on its labelled cases with live model calls, on each runtime (Claude Haiku
# through claude -p, a small Codex model through codex exec); run after editing its prompt. PM_LIVE_RUNS sets the runs
# per case (default 3); PM_LIVE_RUNTIMES=codex runs one runtime.
test-live: go-build
	PM_LIVE_TESTS=1 $(PYRUN) python tests/run.py -s -k owner_request_prompt_live

# Check that a real Claude Code session and a real Codex session get the same pm guarantees, and work one sprint
# together, in scratch clones (tests/test_agent_parity_live.py); needs claude and codex on PATH, both logged in. Its
# scenarios run at once, one worker each; -rA prints each one's latencies and transcript paths.
# PM_PARITY_CODEX_SANDBOX sets Codex's mode: approve-for-me (what a Codex user runs, like Claude Code's auto mode) or
# a sandbox mode with no approvals (default workspace-write; danger-full-access where bubblewrap cannot start).
test-live-parity: go-build
	PM_LIVE_TESTS=1 $(PYRUN) python tests/run.py -n 15 -rA -k agent_parity_live $(ARGS)

# Grade AGENTS.md against tests/agents_md/rubric.md with claude -p reading the checkout (tests/agents_md/evaluate.py):
# MODE=full checks every criterion on the whole file, MODE=diff checks that the diff since BASE makes no AGENTS.md
# claim false and adds no rule it lacks, MODE=auto (the default) is full when that diff changes AGENTS.md. Each call
# runs with the user's claude login (in CI: workload identity federation, agents-md.yml); the answers go to .go/.
BASE ?= origin/main
MODE ?= auto
agents-md-check:
	python3 tests/agents_md/evaluate.py --mode $(MODE) --base $(BASE) --out .go/agents-md.json

# Check the AGENTS.md evaluation on its labelled cases (tests/agents_md/cases.py) with live model calls; run after
# editing the rubric or the runner. PM_LIVE_RUNS sets the runs per case (default 3); PM_AGENTS_MD_MODELS="haiku sonnet"
# compares models.
test-live-agents-md: go-build
	PM_LIVE_TESTS=1 $(PYRUN) python tests/run.py -s -k agents_md_live

# Check that PR N may be merged: its head contains origin's base branch and every check on that head passed, so its
# CI tested what the merge lands (release/merge_ready.py). Run it right before merging: make merge-ready PR=N
merge-ready:
	@test -n "$(PR)" || { echo 'make merge-ready: give the PR number, make merge-ready PR=<n>' >&2; exit 2; }
	python3 release/merge_ready.py $(PR)

# The linters, each at one exact version; pm-lint.yml runs make lint on every PR and push to main. golangci-lint
# (.golangci.yml) runs twice: its gating linters on every line, then the ratcheted ones (RATCHET), whose backlog is not
# cleared yet, on the lines changed since the merge base with origin/main. deadcode fails on a function nothing reaches,
# from pm's binaries or from the tests. ruff (pyproject.toml) checks the harness. golangci-lint loads with -trimpath, as
# go-build (and pm-go.yml's go vet and go test) compile, so it reuses their compiled packages instead of building the
# Dolt tree again.
GOLANGCI_LINT_VERSION := 2.14.0
DEADCODE := golang.org/x/tools/cmd/deadcode@v0.52.0
RATCHET := errcheck,gocognit,funlen,revive,reassign
lint: lint-go lint-deadcode lint-py
lint-go:
	@v=$$(golangci-lint version --short 2>/dev/null); [ "$$v" = "$(GOLANGCI_LINT_VERSION)" ] || { \
	  echo "make lint: needs golangci-lint $(GOLANGCI_LINT_VERSION) on PATH, found '$$v'; see https://golangci-lint.run/docs/welcome/install/" >&2; exit 2; }
	@rc=0; export CGO_ENABLED=1 GOFLAGS="$$GOFLAGS -trimpath"; set -x; \
	  golangci-lint run --disable $(RATCHET) ./... || rc=1; \
	  golangci-lint run --enable-only $(RATCHET) --new-from-merge-base origin/main ./... || rc=1; exit $$rc
lint-deadcode:
	@out=$$(go run $(DEADCODE) -tags gms_pure_go ./cmd/pm ./tests/render-pages && \
	  go run $(DEADCODE) -tags gms_pure_go -test ./...) || exit 1; \
	  if [ -n "$$out" ]; then printf '%s\n' "$$out"; echo 'make lint-deadcode: nothing reaches the functions above; remove them' >&2; exit 1; fi
lint-py:
	$(PYRUN) ruff check tests release

# Build pm as the release builds it (cgo, -tags gms_pure_go, stripped) into .go/pm, and the harness's page renderer
# (tests/render-pages) into .go/render-pages. A release takes its version from its tag (release/build.sh); this build
# reports VERSION, which no release has: the harness's test repos pin it (the renderer's work-store handshake needs the
# same), and its tests place it above the older Go pins they use (0.2.0, 0.3.0) and below the newer one (9.0.0). The
# same pm stamped VERSION_BEFORE, .go/pm-before, is the pm a branch from before a pin move to VERSION runs, for the
# harness's pin-move tests (only the link step differs, so it adds seconds).
VERSION := 0.9.0-dev
VERSION_BEFORE := 0.8.0-dev
GO_FLAGS = -trimpath -tags gms_pure_go -ldflags "-s -w -X github.com/Yeeef/pm/internal/buildinfo.Version=$(1)"
go-build: go-build-pm
	CGO_ENABLED=1 go build $(call GO_FLAGS,$(VERSION)) -o .go/render-pages ./tests/render-pages
	CGO_ENABLED=1 go build $(call GO_FLAGS,$(VERSION_BEFORE)) -o .go/pm-before ./cmd/pm
# pm alone, as go-build builds it: pm-go.yml's macOS job, where no harness runs.
go-build-pm:
	CGO_ENABLED=1 go build $(call GO_FLAGS,$(VERSION)) -o .go/pm ./cmd/pm

# The work store's concurrency tests, which make test-go runs again under the race detector (about a minute; the whole
# internal/work package under -race would take several): the write lock, the slot, gc and merges racing writers.
RACE_TESTS := Racing|Migrating|OutsideTheLock|Outlasting|WriteLock|PmLock|ASecondLock
# Build, the gofmt check, go vet and the Go tests (with the same tag: the embedded Dolt needs it), and the pm service
# and the work store's concurrency tests (RACE_TESTS) again under -race, all at once: no part reads another's output,
# and one after another they left most cores waiting (35 s against 20 s on 32 cores). pm-go.yml runs the parts as
# parallel jobs: go-build, go-fmt, go-vet and go-test-but-work; go-test-work; go-test-race. Measure WORK_SECONDS with
# make go-test-work UPDATE=1 alone, not through test-go, whose other parts would slow the tests it times.
test-go:
	$(MAKE) -j go-build go-fmt go-vet go-os-check go-test-but-work go-test-work go-test-race

# Fail when gofmt would change a Go file of the module, naming each one. The files are those of go list's packages,
# every build tag's and the tests' included, so the module's own: no testdata, vendor or other worktree under .claude/
# (gofmt given a directory would walk all of them). The listing is taken before gofmt runs, so a go list that fails
# fails the target instead of checking nothing. Fix with gofmt -w on the files it names.
GO_FILES = {{$$d := .Dir}}{{range .GoFiles}}{{$$d}}/{{.}} {{end}}{{range .CgoFiles}}{{$$d}}/{{.}} {{end}}\
{{range .TestGoFiles}}{{$$d}}/{{.}} {{end}}{{range .XTestGoFiles}}{{$$d}}/{{.}} {{end}}\
{{range .IgnoredGoFiles}}{{$$d}}/{{.}} {{end}}{{range .InvalidGoFiles}}{{$$d}}/{{.}} {{end}}
go-fmt:
	@files=$$(go list -tags gms_pure_go -f '$(GO_FILES)' ./...) && files=$$(gofmt -l $$files) && \
	  if [ -n "$$files" ]; then \
	    printf '%s\n' "$$files" | sed 's|^$(CURDIR)/||'; \
	    echo 'make go-fmt: gofmt would change the files above; run gofmt -w on them' >&2; exit 1; fi

# The flags of go vet and go test but the race tests: the tag the embedded Dolt needs, and -trimpath, as go-build and
# the release build compile, so all of them share one compile of each package in the Go cache (without it vet and test
# compile every package a second time, doubling the cache CI saves and restores).
GO_TEST_FLAGS = -trimpath -tags gms_pure_go
# Test binaries link without DWARF (-w), which only a debugger reads: a panic's or the race detector's stack names
# files and lines from the binary's own tables. The flag is the link's alone, so the compiles above stay shared.
GO_TEST_LINK = -ldflags=-w
# GO_PKGS narrows go-test: make go-test GO_PKGS=./internal/service.
GO_PKGS ?= ./...
# SELECTED names a file of the packages whose tests run, one per line as go test takes them (., ./internal/site): on a
# pull request pm-go.yml sets it to what .github/scripts/go_changes.py select wrote, the packages the PR's changes can
# affect. Unset, every package's tests run. Each target below runs only its selected packages' tests, and one with none
# selected says so and passes; a SELECTED file that is missing fails it.
SELECTED ?=
OS_TESTS = python3 .github/scripts/go_os_tests.py
# $(call selected,./internal/work): a shell condition, true when that package is selected.
selected = $(if $(SELECTED),{ test -f '$(SELECTED)' || { echo 'make: no file $(SELECTED)' >&2; exit 2; }; \
  grep -qx '$(1)' '$(SELECTED)'; },true)
go-vet:
	CGO_ENABLED=1 go vet $(GO_TEST_FLAGS) ./...
go-test:
	CGO_ENABLED=1 go test $(GO_TEST_FLAGS) $(GO_TEST_LINK) $(GO_PKGS)
# go-test on every package but internal/work, which go-test-work runs. The package list is taken before the test runs,
# so a go list that fails fails the target instead of testing nothing.
go-test-but-work:
ifeq ($(SELECTED),)
	pkgs=$$(go list -tags gms_pure_go ./...) && \
	  CGO_ENABLED=1 go test $(GO_TEST_FLAGS) $(GO_TEST_LINK) $$(printf '%s\n' $$pkgs | grep -v '/internal/work$$')
else
	pkgs=$$($(OS_TESTS) packages --but internal/work --in '$(SELECTED)') && if [ -z "$$pkgs" ]; then \
	  echo '$@: no package selected'; else CGO_ENABLED=1 go test $(GO_TEST_FLAGS) $(GO_TEST_LINK) $$pkgs; fi
endif

# internal/work's tests, the longest package's (about 100 s in one process, mostly waiting on git and Dolt), in
# WORK_SHARDS processes at once, balanced by the seconds each test took (WORK_SECONDS, which must name every test:
# UPDATE=1 measures and rewrites it). .github/scripts/go_shards.py fails unless every test ran and passed. WORK_RUN
# narrows it to test names joined by |, as go_shards.py's --run takes them.
WORK_SHARDS ?= 8
WORK_SECONDS := .github/scripts/work-test-seconds.txt
WORK_RUN ?= .
go-test-work:
	if $(call selected,./internal/work); then \
	  CGO_ENABLED=1 go test -c $(GO_TEST_FLAGS) $(GO_TEST_LINK) -o .go/work.test ./internal/work && \
	  python3 .github/scripts/go_shards.py --shards $(WORK_SHARDS) $(if $(UPDATE),--update,--run '$(WORK_RUN)') .go/work.test internal/work $(WORK_SECONDS); \
	else echo '$@: internal/work is not selected'; fi
# The pm service's tests and RACE_TESTS under -race, the latter sharded as go-test-work's; make -j2 runs both at once.
# No -trimpath: a race build shares no compile with go-build's, and the race cache pm-go.yml saves has none.
# RACE_SERVICE_RUN (go test's -run) and RACE_RUN (names joined by |) narrow them.
RACE_SERVICE_RUN ?= .
RACE_RUN ?= $(RACE_TESTS)
go-test-race: go-test-race-service go-test-race-work
go-test-race-service:
	if $(call selected,./internal/service); then \
	  CGO_ENABLED=1 go test -race -tags gms_pure_go $(GO_TEST_LINK) -run '$(RACE_SERVICE_RUN)' ./internal/service; \
	else echo '$@: internal/service is not selected'; fi
go-test-race-work:
	if $(call selected,./internal/work); then \
	  CGO_ENABLED=1 go test -c -race -tags gms_pure_go $(GO_TEST_LINK) -o .go/work-race.test ./internal/work && \
	  python3 .github/scripts/go_shards.py --shards $(WORK_SHARDS) --run '$(RACE_RUN)' .go/work-race.test internal/work $(WORK_SECONDS); \
	else echo '$@: internal/work is not selected'; fi

# macOS (pm-go.yml). On a push to main every test runs there, RACE_TESTS only under -race (go-test-race), as the owner
# decided: they are half of internal/work's CPU on the 3-core runner. A pull request runs there only the tests whose
# behaviour can differ by OS, the os rows of .github/scripts/go-os-tests.txt, which classifies every test (make
# go-os-check); Linux runs every test on both. Each list is taken before the tests run, so a list that fails fails the
# target instead of testing nothing. Given SELECTED, they run its packages' os tests: the selection intersected with
# the os set.
go-os-check:
	$(OS_TESTS) check
go-test-work-macos:
	run=$$($(OS_TESTS) names --package internal/work --skip '$(RACE_TESTS)') && $(MAKE) go-test-work WORK_RUN="$$run"
IN = $(if $(SELECTED),--in '$(SELECTED)')
go-test-but-work-os:
	pkgs=$$($(OS_TESTS) packages --os --but internal/work $(IN)) && if [ -z "$$pkgs" ]; then echo '$@: no package selected'; \
	  else run=$$($(OS_TESTS) names --os --anchored --but internal/work $(IN)) && \
	  CGO_ENABLED=1 go test $(GO_TEST_FLAGS) $(GO_TEST_LINK) -run "$$run" $$pkgs; fi
go-test-work-os:
	run=$$($(OS_TESTS) names --os --package internal/work --skip '$(RACE_TESTS)') && $(MAKE) go-test-work WORK_RUN="$$run"
go-test-race-os:
	svc=$$($(OS_TESTS) names --os --anchored --package internal/service) && \
	  work=$$($(OS_TESTS) names --os --package internal/work --only '$(RACE_TESTS)') && \
	  $(MAKE) -j2 go-test-race RACE_SERVICE_RUN="$$svc" RACE_RUN="$$work"
