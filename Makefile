GO ?= go
PNPM ?= pnpm
BIN := bin

.PHONY: verify deps fmt-check vet lint test build install clean

verify: fmt-check vet lint test build

deps:
	$(PNPM) install --frozen-lockfile

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt pendente em:"; echo "$$out"; exit 1; fi
	$(PNPM) exec prettier --check .

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

test:
	$(GO) test ./...

build:
	$(GO) build -o $(BIN)/ocgs ./cmd/ocgs
	$(GO) build -o $(BIN)/ccs ./cmd/ccs

install:
	$(GO) install ./cmd/ocgs ./cmd/ccs

clean:
	rm -rf $(BIN)
