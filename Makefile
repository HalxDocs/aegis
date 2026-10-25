GO ?= go
SCENARIOS ?= node-crash one-way-partition

.PHONY: all build fmt fmt-check vet test race check cover replay demo clean

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

clean:
	$(GO) clean ./...
