BINARY  := captn
GOBIN_DIR := $(shell go env GOBIN)
ifeq ($(strip $(GOBIN_DIR)),)
  GOBIN_DIR := $(shell go env GOPATH)/bin
endif

GO_ENV = PATH="$(GOBIN_DIR):$$PATH" \
         CGO_ENABLED=1

.PHONY: mod build test generate coverage vet

BANSTRUCTLIT_BIN := $(CURDIR)/bin/banstructlit

all: mod build generate init debug graphviz

mod:
	chmod -R u+w vendor/ 2>/dev/null || true
	go mod tidy
	go mod vendor
	bash scripts/vendor-cgo.sh

sign:
	@OS=$$(uname -s); \
	if [ "$$OS" = "Darwin" ]; then \
		codesign -f -s - bin/$(BINARY); \
	fi;

build: mod vet
	@$(GO_ENV) go build -mod=mod -v -o bin/$(BINARY) cmd/main.go
	make sign

# Keep -count=1 static as snapshot tests shouldn't be cached
test:
	@$(GO_ENV) go test -count=1 -v -failfast ./pkg/tests

accept_snapshots:
	UPDATE_SNAPS=true $(MAKE) test

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

graphviz:
	cd ./tools/viz && npm start

$(BANSTRUCTLIT_BIN):
	cd tools/banstructlit && go build -o $(BANSTRUCTLIT_BIN) .

vet: $(BANSTRUCTLIT_BIN)
	@$(GO_ENV) $(BANSTRUCTLIT_BIN) ./...

# Install delve first -> go install github.com/go-delve/delve/cmd/dlv@latest
debug: build
	@$(GO_ENV) dlv exec ./bin/captn -- run ./cmd/main.go
