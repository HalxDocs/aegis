GO ?= go

.PHONY: all build fmt fmt-check vet test race check replay demo clean

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

check: fmt-check vet build test race

# Two runs of one scenario must produce identical bytes. Comparing parsed events
# would miss reordering, which is exactly the defect this target exists to catch.
replay:
	@first=$$(mktemp); second=$$(mktemp); \
	$(GO) run ./cmd/aegis run --scenario node-crash --format jsonl > "$$first"; \
	$(GO) run ./cmd/aegis run --scenario node-crash --format jsonl > "$$second"; \
	if diff -u "$$first" "$$second"; then \
		echo "replay: identical"; \
		rm -f "$$first" "$$second"; \
	else \
		rm -f "$$first" "$$second"; exit 1; \
	fi

demo:
	$(GO) run ./cmd/aegis run --scenario node-crash --format table

clean:
	$(GO) clean ./...
