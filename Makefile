VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GO ?= go

.PHONY: build test lint e2e clean tidy install

build:
	CGO_ENABLED=0 $(GO) build -ldflags='$(LDFLAGS)' -o hm ./cmd/hm

install:
	CGO_ENABLED=0 $(GO) install -ldflags='$(LDFLAGS)' ./cmd/hm

test:
	$(GO) test ./...

lint:
	$(GO) vet ./...
	@command -v golangci-lint >/dev/null && golangci-lint run ./... || echo "golangci-lint not installed; skipping"

e2e:
	$(GO) test -tags=e2e -timeout=20m ./e2e/...

tidy:
	$(GO) mod tidy

clean:
	rm -f hm
	rm -rf dist/
