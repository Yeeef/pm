UV ?= uv
# The harness (tests/, pytest) runs in the environment uv keeps at .venv, from pyproject.toml's dev group.
PYRUN := $(UV) run --quiet

.PHONY: test test-full test-live test-go go-build go-fmt go-vet go-test go-test-but-work go-test-race merge-ready \
  lint lint-go lint-deadcode lint-py

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

# Check the owner-request Stop hook's Haiku judge on its labelled cases with live model calls; run after editing
# its prompt. PM_LIVE_RUNS sets the runs per case (default 3).
test-live: go-build
	PM_LIVE_TESTS=1 $(PYRUN) python tests/run.py -s -k owner_request_prompt_live

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
RATCHET := errcheck,gocyclo,gocognit,funlen,revive,reassign
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
go-build:
	CGO_ENABLED=1 go build $(call GO_FLAGS,$(VERSION)) -o .go/pm ./cmd/pm
	CGO_ENABLED=1 go build $(call GO_FLAGS,$(VERSION)) -o .go/render-pages ./tests/render-pages
	CGO_ENABLED=1 go build $(call GO_FLAGS,$(VERSION_BEFORE)) -o .go/pm-before ./cmd/pm

# The work store's concurrency tests, which make test-go runs again under the race detector (about a minute; the whole
# internal/work package under -race would take several): the write lock, the slot, gc and merges racing writers.
RACE_TESTS := Racing|Migrating|Eight|Opposite|OutsideTheLock|NotStarved|Outlasting|HungFetch|WriteLock|PmLock|ASecondLock
# Build, then the gofmt check, go vet and the Go tests (with the same tag: the embedded Dolt needs it), then the pm
# service and the work store's concurrency tests (RACE_TESTS) again under -race. pm-go.yml runs the parts as parallel
# jobs: go-build, go-fmt, go-vet and go-test-but-work, go-test on internal/work (GO_PKGS), and go-test-race.
test-go: go-build go-fmt go-vet go-test go-test-race

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

# GO_PKGS narrows go-test: make go-test GO_PKGS=./internal/work.
GO_PKGS ?= ./...
go-vet:
	CGO_ENABLED=1 go vet -tags gms_pure_go ./...
go-test:
	CGO_ENABLED=1 go test -tags gms_pure_go $(GO_PKGS)
# go-test on every package but internal/work, which pm-go.yml tests in a job of its own. The package list is taken
# before the test runs, so a go list that fails fails the target instead of testing nothing.
go-test-but-work:
	pkgs=$$(go list -tags gms_pure_go ./...) && \
	  CGO_ENABLED=1 go test -tags gms_pure_go $$(printf '%s\n' $$pkgs | grep -v '/internal/work$$')
go-test-race:
	CGO_ENABLED=1 go test -race -tags gms_pure_go ./internal/service && CGO_ENABLED=1 go test -race -tags gms_pure_go -run '$(RACE_TESTS)' ./internal/work
