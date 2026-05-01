.PHONY: build test test-race run \
	lint lint-fix lint-new \
	coverage vulncheck \
	tools-install hooks-install

# -----------------------------------------------------------------------
# Build / run
# -----------------------------------------------------------------------

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

run:
	go run ./cmd/pingpong \
	  -config cmd/pingpong/pingpong.example.yaml \
	  -self-port 5001 -peer-port 5002

# -----------------------------------------------------------------------
# Code quality — lint, coverage, vulnerability scan
#
# Thresholds in .golangci.yml are anchored on Go-community conventions
# (gocyclo/cyclop/gocognit/funlen/lll/dupl defaults and Uber Go Style
# Guide).
# -----------------------------------------------------------------------

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

# lint-new is the CI gate: fail only on issues in code changed against
# origin/main. Old code is surfaced but not blocked.
lint-new:
	golangci-lint run --new-from-rev=origin/main ./...

coverage:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out | tail -1

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# -----------------------------------------------------------------------
# One-shot setup: install developer-side quality tools and git hooks
# -----------------------------------------------------------------------

tools-install:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Also install golangci-lint (see https://golangci-lint.run/welcome/install/)"

hooks-install:
	./scripts/install-hooks.sh
