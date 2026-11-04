GO ?= go
SCENARIOS ?= node-crash one-way-partition recovered-crash lone-witness
REF ?= 14

# The explainer page lives beside this repository rather than in it. CI checks
# out this repository alone, so it is a separate target with no lab to run.
LAB ?= ../aegis-lab

.PHONY: all build fmt fmt-check vet test race check check-all lab-check cover replay demo explain clean hooks

all: check

build:
	$(GO) build ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required for:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

check: fmt-check vet build test race

# The page's own proof: it re-measures this repository's gate, compares all four
# fixtures against it byte for byte, and builds. It runs `make replay` itself, so
# a `check-all` run replays the scenarios twice — once here, once for the card.
lab-check:
	@if [ ! -d "$(LAB)" ]; then \
		echo "lab not found at $(LAB); set LAB=/path/to/aegis-lab"; \
		exit 1; \
	fi
	@command -v npm >/dev/null 2>&1 || { echo "npm is required to check the lab"; exit 1; }
	cd "$(LAB)" && npm run check

# Both sides, one command: the Go gate first, then the page that explains it.
#
# Serial on purpose even under -j: the lab's gate re-measures this repository, and
# running that while the repository's own race tests are in flight buys nothing
# and reads as one gate instead of two.
.NOTPARALLEL: check-all

check-all: check lab-check

# GitHub cannot gate a direct push to main with required status checks: it
# rejects the push with GH006, since the checks only exist after the push they
# would reject. The gate therefore runs before the push leaves the machine.
# Optional, because it writes to your git config; `git push --no-verify` skips
# it when the push is not about code.
hooks:
	git config core.hooksPath hooks
	@echo "pre-push now runs make check-all (bypass: git push --no-verify)"

# Two runs of every scenario must produce identical bytes. Comparing parsed
# events would miss reordering, which is exactly the defect this target exists to
# catch, and a scenario that is deterministic only by luck is not deterministic.
replay:
	@for s in $(SCENARIOS); do \
		first=$$(mktemp); second=$$(mktemp); \
		$(GO) run ./cmd/aegis run --scenario $$s --format jsonl > "$$first"; \
		$(GO) run ./cmd/aegis run --scenario $$s --format jsonl > "$$second"; \
		if diff -u "$$first" "$$second" > /dev/null; then \
			echo "replay $$s: identical"; \
			rm -f "$$first" "$$second"; \
		else \
			diff -u "$$first" "$$second"; \
			rm -f "$$first" "$$second"; \
			exit 1; \
		fi; \
	done

demo:
	$(GO) run ./cmd/aegis run --scenario node-crash --format table

explain:
	$(GO) run ./cmd/aegis explain -scenario node-crash -ref $(REF)

clean:
	$(GO) clean ./...
