GO ?= go

.PHONY: all build fmt fmt-check vet test race check demo clean

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

demo:
	$(GO) run ./cmd/aegis run --scenario node-crash --format table

clean:
	$(GO) clean ./...
