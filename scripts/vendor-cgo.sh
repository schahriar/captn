#!/usr/bin/env bash
set -euo pipefail

GOPATH=$(go env GOPATH)
GOMOD="$(dirname "$0")/../go.mod"

version_of() {
  # A bit brittle but saves maintenance on dependency bumps
  grep "^\s*$1 " "$GOMOD" | awk '{print $2}'
}

TS_GO_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-go")
TS_JAVA_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-java")
TS_PHP_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-php")
TS_PYTHON_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-python")
TS_RUBY_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-ruby")
TS_SWIFT_VERSION=$(version_of "github.com/alex-pinkus/tree-sitter-swift")
TS_TYPESCRIPT_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-typescript")
TS_CSS_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-css")
TS_HTML_VERSION=$(version_of "github.com/tree-sitter/tree-sitter-html")
GO_TS_VERSION=$(version_of "github.com/tree-sitter/go-tree-sitter")

if [ -z "$TS_GO_VERSION" ] || [ -z "$TS_JAVA_VERSION" ] || [ -z "$TS_PHP_VERSION" ] || [ -z "$TS_PYTHON_VERSION" ] || [ -z "$TS_RUBY_VERSION" ] || [ -z "$TS_SWIFT_VERSION" ] || [ -z "$TS_TYPESCRIPT_VERSION" ] || [ -z "$TS_CSS_VERSION" ] || [ -z "$TS_HTML_VERSION" ] || [ -z "$GO_TS_VERSION" ]; then
  echo "error: could not detect tree-sitter versions from go.mod" >&2
  exit 1
fi

echo "tree-sitter/tree-sitter-go: $TS_GO_VERSION"
echo "tree-sitter/tree-sitter-java: $TS_JAVA_VERSION"
echo "tree-sitter/tree-sitter-php: $TS_PHP_VERSION"
echo "tree-sitter/tree-sitter-python: $TS_PYTHON_VERSION"
echo "tree-sitter/tree-sitter-ruby: $TS_RUBY_VERSION"
echo "alex-pinkus/tree-sitter-swift: $TS_SWIFT_VERSION"
echo "tree-sitter/tree-sitter-typescript: $TS_TYPESCRIPT_VERSION"
echo "tree-sitter/tree-sitter-css: $TS_CSS_VERSION"
echo "tree-sitter/tree-sitter-html: $TS_HTML_VERSION"
echo "tree-sitter/go-tree-sitter: $GO_TS_VERSION"

vendor_copy() {
  local src="$1" dst="$2"
  chmod -R u+w "$dst" 2>/dev/null || true
  rm -rf "$dst"
  # Subgrammar sources nest below the module root, which go mod vendor
  # does not create for them
  mkdir -p "$(dirname "$dst")"
  cp -r "$src" "$dst"
  chmod -R u+w "$dst"
}

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-go@$TS_GO_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-go/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-java@$TS_JAVA_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-java/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-python@$TS_PYTHON_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-python/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-ruby@$TS_RUBY_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-ruby/src

# Swift has no official grammar; the maintained one keeps its generated parser
# on a side branch, so the pin is a pseudo-version rather than a release
vendor_copy "$GOPATH/pkg/mod/github.com/alex-pinkus/tree-sitter-swift@$TS_SWIFT_VERSION/src" \
  vendor/github.com/alex-pinkus/tree-sitter-swift/src

# The php repo holds two subgrammars whose scanners share common/scanner.h;
# the Go binding package compiles both
vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-php@$TS_PHP_VERSION/php/src" \
  vendor/github.com/tree-sitter/tree-sitter-php/php/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-php@$TS_PHP_VERSION/php_only/src" \
  vendor/github.com/tree-sitter/tree-sitter-php/php_only/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-php@$TS_PHP_VERSION/common" \
  vendor/github.com/tree-sitter/tree-sitter-php/common

# The typescript repo holds two subgrammars whose scanners share common/scanner.h
vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-typescript@$TS_TYPESCRIPT_VERSION/typescript/src" \
  vendor/github.com/tree-sitter/tree-sitter-typescript/typescript/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-typescript@$TS_TYPESCRIPT_VERSION/tsx/src" \
  vendor/github.com/tree-sitter/tree-sitter-typescript/tsx/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-typescript@$TS_TYPESCRIPT_VERSION/common" \
  vendor/github.com/tree-sitter/tree-sitter-typescript/common

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-css@$TS_CSS_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-css/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/tree-sitter-html@$TS_HTML_VERSION/src" \
  vendor/github.com/tree-sitter/tree-sitter-html/src

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/go-tree-sitter@$GO_TS_VERSION/include" \
  vendor/github.com/tree-sitter/go-tree-sitter/include

vendor_copy "$GOPATH/pkg/mod/github.com/tree-sitter/go-tree-sitter@$GO_TS_VERSION/src" \
  vendor/github.com/tree-sitter/go-tree-sitter/src
