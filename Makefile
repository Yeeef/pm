UV ?= uv
# pm runs from its package at the repo root, in the environment uv keeps at .venv.
PMRUN := $(UV) run --quiet

.PHONY: test test-full test-live test-go test-go-suite go-build

# ARGS goes to pytest: make test-full ARGS="-k serve" runs only the integration tests a change touches.
ARGS ?=

# Run the light harness tests (pm CLI, records, hooks) in parallel against temp repos and a fake bd; run while
# working.
test:
	$(PMRUN) python tests/run.py -n auto -m "not integration" $(ARGS)

# Run the harness tests ARGS selects, the integration ones too (the pm service, pm init, push, the session-start
# hook). CI runs the whole integration set on every PR, so locally it refuses to run without ARGS; CI=1 forces it.
test-full:
	@if [ -z "$(ARGS)" ] && [ -z "$(CI)" ]; then \
	  echo 'make test-full: CI runs every integration test on each PR; do not run the whole set locally.' >&2; \
	  echo '  Run the tests your change touches: make test-full ARGS="-k <name>"' >&2; \
	  echo '  Watch CI instead: gh pr checks <n> --watch. Force the whole set: CI=1 make test-full' >&2; \
	  exit 2; fi
	$(PMRUN) python tests/run.py -n auto $(ARGS)

# Check the owner-request Stop hook's Haiku judge on its labelled cases with live model calls; run after editing
# its prompt. PM_LIVE_RUNS sets the runs per case (default 3).
test-live:
	PM_LIVE_TESTS=1 $(PMRUN) python tests/run.py -s -k owner_request_prompt_live

# Build Go pm as the release builds it (cgo, -tags gms_pure_go, stripped). A release takes its version from its tag
# (release/build.sh); this build stamps the Python package's version instead, so that the shared suite's config
# check and launcher see the version the tests pin, until the cut-over. Then write Python pm's pages, check results
# and reference outputs into .go/parity (go_parity_corpus.py; PM_PARITY_LIVE=<a clone that uses pm> adds its records
# and Beads data, which needs bd), then run go vet,
# the Go tests (with the same tag: the embedded Dolt needs it) against that corpus (PM_PARITY) and the parity tests
# against Python pm (PM_GO). The parity tests' reference help layout is Python 3.13's argparse; CI runs them on 3.13.
GO_PM := $(CURDIR)/.go/pm
GO_VERSION_FLAG = -X github.com/Yeeef/pm/internal/buildinfo.Version=$(shell sed -n 's/^version = "\(.*\)"/\1/p' pyproject.toml)
go-build:
	CGO_ENABLED=1 go build -trimpath -tags gms_pure_go -ldflags "-s -w $(GO_VERSION_FLAG)" -o $(GO_PM) ./cmd/pm

GO_PARITY := $(CURDIR)/.go/parity
# Both sides of the parity corpus render in a zone far from UTC, so a UTC date where a local one belongs (day pages)
# differs, on CI's UTC runners too.
PARITY_TZ ?= Pacific/Auckland
test-go: go-build
	TZ=$(PARITY_TZ) $(PMRUN) python tests/go_parity_corpus.py $(GO_PARITY) $(if $(PM_PARITY_LIVE),--live $(PM_PARITY_LIVE))
	CGO_ENABLED=1 go vet -tags gms_pure_go ./... && TZ=$(PARITY_TZ) PM_PARITY=$(GO_PARITY) CGO_ENABLED=1 go test -tags gms_pure_go ./...
	PM_GO=$(GO_PM) $(PMRUN) python -m pytest -q -p no:cacheprovider -n auto tests/test_go_parity.py $(ARGS)

# Run the shared suite, the integration tests too, on Go pm (PM_IMPL=go) against tests/go-expected-failures.txt:
# a listed test must fail (strict xfail), so the list only shrinks. Then compare the transcripts of the tests that pass
# on Go with Python pm's (compare_transcripts.py). Go pm fails most tests at once, so the whole run takes seconds.
test-go-suite: go-build
	PM_IMPL=go PM_GO_BIN=$(GO_PM) $(PMRUN) python tests/run.py -n auto $(ARGS)
	$(PMRUN) python tests/compare_transcripts.py
