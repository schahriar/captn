BINARY  := captn
GOBIN_DIR := $(shell go env GOBIN)
ifeq ($(strip $(GOBIN_DIR)),)
  GOBIN_DIR := $(shell go env GOPATH)/bin
endif

GO_ENV = PATH="$(GOBIN_DIR):$$PATH" \
         CGO_ENABLED=1

.PHONY: mod build test generate coverage

all: mod build generate init debug

mod:
	go mod tidy
	go mod vendor

sign:
	@OS=$$(uname -s); \
	if [ "$$OS" = "Darwin" ]; then \
		codesign -f -s - bin/$(BINARY); \
	fi;

build: mod
	@$(GO_ENV) go build -mod=mod -v -o bin/$(BINARY) cmd/main.go
	make sign

# Keep -count=1 static as snapshot tests shouldn't be cached
test:
	@$(GO_ENV) go test -count=1 -v -failfast ./pkg/tests

coverage:
	@$(GO_ENV) go test -count=1 -v -failfast \
		-coverpkg=./pkg/... \
		-coverprofile=coverage.out \
		./tests
	@$(GO_ENV) go tool cover -html=coverage.out

trace:
	go tool trace trace.out

# Keep -count=1 static as snapshot tests shouldn't be cached
# TODO: Separate snapshot testing from unit tests
test:
	@$(GO_ENV) go test -count=1 -v -failfast ./tests

# Install delve first -> go install github.com/go-delve/delve/cmd/dlv@latest
debug: build
	@$(GO_ENV) dlv exec ./bin/captn -- run ./cmd/main.go
