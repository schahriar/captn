#!/usr/bin/env bash
set -euo pipefail

GOPATH=$(go env GOPATH)
GOMOD="$(dirname "$0")/../go.mod"

version_of() {
  # A bit brittle but saves maintenance on dependency bumps
  grep "^\s*$1 " "$GOMOD" | awk '{print $2}'
}

TS_GO_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-go")
TS_PYTHON_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-python")
TS_SWIFT_VERSION=$(version_of "github.com/alex-pinkus/tree-sitter-swift")
GO_TS_VERSION=$(version_of "github.com/tree-sitter/go-tree-sitter")

if [ -z "$TS_GO_VERSION" ] || [ -z "$TS_PYTHON_VERSION" ] || [ -z "$TS_SWIFT_VERSION" ] || [ -z "$GO_TS_VERSION" ]; then
  echo "error: could not detect tree-sitter versions from go.mod" >&2
  exit 1
fi

echo "tree-sitter/tree-sitter-go: $TS_GO_VERSION"
echo "tree-sitter/tree-sitter-python: $TS_PYTHON_VERSION"
echo "alex-pinkus/tree-sitter-swift: $TS_SWIFT_VERSION"
echo "tree-sitter/go-tree-sitter: $GO_TS_VERSION"

vendor_copy() {
  local src="$1" dst="$2"
  chmod -R u+w "$dst" 2>/dev/null || true
  rm -rf "$dst"
  cp -r "$src" "$dst"
  chmod -R u+w "$dst"
}

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-go@$TS_GO_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-go/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-python@$TS_PYTHON_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-python/src

# Swift has no official grammar; the maintained one keeps its generated parser
# on a side branch, so the pin is a pseudo-version rather than a release
vendor_copy "$GOPATH/pkg/mod/github.com/alex-pinkus/tree-sitter-swift@$TS_SWIFT_VERSION/src" \
  vendor/github.com/alex-pinkus/tree-sitter-swift/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/go-tree-sitter@$GO_TS_VERSION/include" \
  vendor/github.com/tree-sitter/go-tree-sitter/include

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/go-tree-sitter@$GO_TS_VERSION/src" \
  vendor/github.com/tree-sitter/go-tree-sitter/src
