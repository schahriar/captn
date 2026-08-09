---
name: add-treesitter-language
description: Add captn support for a new tree-sitter language, or review/change an existing language. Covers the parser transformer over the closed AST vocabulary, LSP server wiring, import classification, extension dispatch, cgo vendoring, and tests. Load before touching pkg/languages, pkg/ast, or any language PR.
---

# Add tree-sitter language support to captn

Language support means mapping a tree-sitter grammar's node kinds onto captn's closed AST vocabulary, plus LSP wiring and import classification. The reference implementation is `pkg/languages/golang/` (~380 lines across two files) — read both files first; a new language should look like them.

> Note that `pkg/languages/golang/` is the reference implementation written by a human, including the latest best practices and the overall design rationale to the entire system. While you should reference other implementations for consistency, the golang implementation is the one that is correct and should be followed.

## The AST vocabulary is closed — keep it that way

Exactly nine node kinds exist: Module, Block, Import, FuncExpression, FuncArgument, Declaration, CallExpression, ReturnStatement, Symbol (the `ASTVisitor` interface in `pkg/ast/tree.go`). Map grammar constructs onto these; let everything else fall through to the default walk. Do not add a tenth kind, and do not add wrapper nodes to "preserve structure". Block is never emitted by a transformer — it exists only as the auto-created container you narrow inside FuncExpression; control-flow and block statements fall through.

This is load-bearing, not taste:

- A node's cache identity is `hash(filePath + rawSource + position + Kind)` chained through every ancestor (`ast.GetHash`, `pkg/ast/tree.go`). Observations are keyed on these hashes and shared through git. Every extra node deepens every descendant's parent chain, so AST complexity directly lowers team-wide cache hit rates, and any AST shape change invalidates existing shared observations.
- The interval index keeps one node per exact byte range; on a collision the deeper node wins and the outer one becomes invisible to range queries. Only Module's own shadowing (by its Block) is compensated, and only in `FindTightestEnclosingNode` (`pkg/cog/queries.go`) — every other shadowed node is simply gone from range queries. Harmless when the loser is a container nothing queries for; harmful when it is a FuncExpression or a Symbol the search needs.
- Two same-Kind nodes at the same range under the same parent panic with `knownerr.HashCollision` at parse time, and search hard-errors unless each LSP-resolved range contains exactly one indexed node (`expected 1 node for dependency`, `pkg/cog/workspace.go`).

If a grammar genuinely cannot fit the nine kinds, stop: changing `pkg/ast` requires the adversarial review the author's rules in CLAUDE.md call for. Present the conflict to the developer instead of working around it.

## What you must map — everything else may fall through

Downstream queries read very few fields, but those few are unforgiving:

| Grammar construct | Emit | Load-bearing detail |
|---|---|---|
| import / require / include | `ASTImportStatement` | Dependency resolution asks the LSP for a definition at the Import node's own range — it never reads `.Reference`. The range must start at LSP-resolvable text (the quoted path or module spec), not at a keyword. `.Reference`/`.Namespace` surface only in debug output and snapshots; grouping keys on the import node's range. Set them when the grammar makes it cheap. |
| function and method definitions, lambdas | `ASTFuncExpression` | Only FuncExpression and Module become observation-graph vertices (`IsNodeOfInterest`, `pkg/cog/workspace.go`). A language that never emits FuncExpression still parses, but every observation silently collapses onto one coarse file-level node. `Name` must be a Symbol positioned exactly on the identifier: cross-file definition ranges must contain exactly one node. Always narrow `.Block` before `WalkChildrenInto` — see below. |
| call expressions | `ASTCallExpression` | `Symbol` must be non-nil and sit on the callee identifier — `SearchSnippet` dereferences it without a nil check. When a call form has no identifiable callee, do not emit a CallExpression at all; fall through and its children still index under the enclosing block. |
| variable / const declarations | `ASTDeclaration` | Add a Symbol to `.Names` for each identifier an LSP definition can land on (think `const f = () => ...`); the exactly-one-node check needs a node at that spot. Walk the right-hand side into the declaration so calls land in `.Virtual`. |
| module / package name | `trx.Root.Name` | Display only. |
| return statements | `ASTReturnStatement` | Optional — traversal and debug output only. Skipping it is the cache-friendlier choice. |

Narrowing `.Block`: `NewASTFuncExpression` pre-creates its Block with the function's own range. Left un-narrowed, the Block shadows the FuncExpression in the interval index (same range, deeper node wins) and the function disappears from range queries even though it was emitted. Narrow to the body node for braced bodies; for expression bodies (arrows, lambdas) narrow to the expression node — that collides with the emitted expression instead, which only shadows the Block and is benign.

Rules for every node you emit:

- It must carry a valid in-buffer range. Hashing and indexing dereference positions without checks at parse time; a nil or out-of-range position is a raw panic, not a friendly error.
- Never emit a node that shares its exact byte range with a FuncExpression or with a Symbol that search must find (see the shadowing rule above). Known deviation in the reference: golang's call-argument FuncArguments reuse their Symbol's container — the Symbol wins there, so it stays findable; do not copy the pattern anyway.
- `AppendChild` is safe on Module, Block, FuncExpression, CallExpression, Declaration, ReturnStatement. Import, FuncArgument, and Symbol panic.
- Construct nodes only through their `New*` constructors — `banstructlit` (run by `make build`) rejects direct struct literals, and the constructors pre-create Blocks so nil-guards downstream hold.

## Walker semantics (pkg/parsers/walker.go)

The transformer is one function switching on `node.Kind` (see `GolangTransformer`):

- `trx.Emit(n)` attaches `n` to the current parent. `trx.WalkChildrenInto(ctx, n)` descends with `n` as the new parent; `trx.WalkChildren(ctx)` descends keeping the current parent.
- The `default:` case must `return trx.WalkChildren(ctx)`. Anything else silently stops traversal below that node — captn's guarantee is information retention over correctness (`pkg/ast/AST.md`).
- Cases may rely on the parent set by an earlier case: Go's `parameter_list` case fills arguments only because the enclosing `function_declaration` case emitted the FuncExpression and walked into it first. Note that ordering when you reorganize.
- Broken or partial code must still parse. Prefer folding an unrecognizable subtree into the enclosing block over failing (`pkg/parsers/README.md`). On a truly unexpected shape return an error — do not panic: the transformer runs outside `ASTPanicBoundary` (that boundary guards visitor passes like indexing and queries, where `knownerr`-recoverable panics get a node trail), so a transformer panic escapes with no AST context.

## Checklist — every place a new language touches

1. `pkg/languages/<lang>/` — new package with the transformer + `LanguageSupport` definition (mirror `golang_parser.go`) and an LSP locate helper (mirror `golang_gopls.go`).
2. `pkg/languages/languages.go` — add `var <Lang> LanguageSupport = languages_<lang>.New<Lang>LanguageSupportDefinition()`.
3. `pkg/cog/parse_file.go` `ParseSource` — add the extension(s) to the dispatch. `filepath.Ext` yields only the final segment (`.d.ts` files arrive as `.ts`; `.pyi` is its own extension). Cover every extension the language's LSP can resolve a definition into: in the shipped search flow all per-match errors are swallowed, so an unsupported searched file drops one match and resolution into an unsupported file kills that match's whole graph — both surface as silently missing results.
4. `pkg/common/source.go` `GetLanguage` — add the extension → display-name mapping. Skipping this fails silently: viz shows `file_type="unknown"`.
5. Grammar dependency: `go get github.com/tree-sitter/tree-sitter-<lang>` (prefer official tree-sitter org grammars), import its `bindings/go` package in the parser, run `make mod`, and add a `version_of` + `vendor_copy` block for it in `scripts/vendor-cgo.sh` — `go mod vendor` drops the C sources the binding compiles. Multi-parser grammar repos (typescript, php, markdown) have no top-level `src/`: read the binding's cgo `#include` lines and add one `vendor_copy` per subgrammar `src/` plus the shared `common/` when present. Forgetting the script fails `make build` at the banstructlit vet step with `fatal error: '../../src/parser.c' file not found`; a warm `.gocache` can mask it, so verify on a clean cache.
6. `GetLanguageID()` — return the exact LSP-spec identifier (`"go"`, `"python"`, `"typescript"`), not captn's display name. It becomes the `didOpen` languageId and the LSP client cache key; strict servers ignore documents with the wrong id. Must be unique across languages. Sibling dialects (typescript/tsx) need one LanguageSupport per dialect — same transformer, different `GetTreeSitterLanguage()` and `GetLanguageID()` (`"typescript"` vs `"typescriptreact"`), same LSP server — with each extension dispatched to its dialect.
7. `GetLSPServerRequirement()` — first choose the language's de-facto LSP server (pyright for Python, typescript-language-server for TypeScript, rust-analyzer for Rust); it must speak stdio and support `textDocument/definition` and `didOpen` at minimum. Install it on your machine now by running its install command yourself — captn never installs servers, and the LSP-backed verification below needs the binary present. Then fill the requirement: `Name` (binary name; also the process-wide memo key, so unique), `InstallCommand` (the same one-line install command; captn surfaces it in the missing-server error for the coding agent to run), `Locate` (resolve the binary: `exec.LookPath` first, then ask the ecosystem where it installs binaries the way `goplsPath` asks `go env` rather than trusting env vars; append `.exe` on Windows for direct file checks — npm shims resolve via LookPath/PATHEXT). Put `// banstructlit:ignore` on the line directly above the returned struct literal.
8. `NewLSPServer()` — spawn through the same locate function `Locate` uses so they cannot disagree. The transport is stdio: node-based servers need an explicit `--stdio` flag or captn hangs at initialize, and server logs must never reach stdout (they break Content-Length framing) — wire `cmd.Stderr` to `os.Stderr`.
9. `ClassifyImportType()` — return only `LocalDependency`, `PackageDependency`, or `StandardLibraryDependency`; the enum is closed and unknown values get silently relabeled downstream. Input is the resolved definition file's on-disk path — not the import string — and it arrives in two forms: workspace-relative (may climb out via `../`) and absolute. Normalize with `filepath.ToSlash(filepath.Clean(path))` and match interior substrings; never anchor on the workspace root. Derive patterns from paths the server actually returns (run `ListDependencies` against a fixture and look): stub-serving servers resolve stdlib into their own installation, not the runtime's — pyright into its bundled `typeshed-fallback/`, typescript-language-server into `node_modules/typescript/lib/`. stdlib and package prune graph traversal — misclassifying them as local makes captn crawl the whole dependency tree.

## Verify

- Fixtures under `tests/fixtures/<lang>/` mirroring the golang layout: `baseproj/` (one simple function file, one method/receiver file) and `multidep/` (an entry file importing one stdlib module and one aliased local package, plus that package). Fixtures must form a project the language's LSP server can actually resolve — that can mean installed dependencies, not just manifests (`@types/node` in a real `node_modules/`, a venv). Unresolvable imports fail silently as absent dependencies, not as errors.
- Structural tests in `tests/` mirroring `parser_unit_test.go`. The minimal contract to assert: Module → FuncExpression (Name) → Block → CallExpression (Symbol), plus the `GetParent` chain with `assert.Same`. Extend the walk to ReturnStatement / Arguments / ReturnType only where the language chose to map them — do not add mappings just to satisfy a test shape. testify `assert` only, never `require`; guard with `if !assert.X(...) { return }`.
- Snapshots: one-line `checkSnapshot`-style tests (`snaps.MatchYAML(t, pf.Module.Debug())`); bless with `make accept_snapshots`. Snapshots embed absolute checkout paths, so expect full-file churn when regenerating.
- Import resolution tests (`ListDependencies`, `GroupByPackage`) require the language server binary installed locally — the golang suite mixes them into the same files, so the split is convention, not mechanism: keep structural and snapshot tests passing without the server.
- Eyeball the graph (author's rule: reuse debug tools): `graph.gv` is written only by `ObservationGraph.QueryWithDepth` (`pkg/cog/observation_graph.go`), which the CLI/MCP flow does not call — produce one from a throwaway test that builds a graph via `SearchSnippet` or `Workspace.QueryWithDepth` and calls `WriteToFile`. Then `cd tools/viz && npm install && npm start` and drop the file. Wrong import classification shows as wrong node colors; missing FuncExpressions show everything hanging off the module node.
- Full loop: `make mod` → `make build` (runs the banstructlit vet) → `make test`.

## Failure signatures

| Symptom | Cause |
|---|---|
| Empty or thinner-than-expected search results, no error | Extension missing from `ParseSource` dispatch, or any per-match error — everything except a missing LSP server is swallowed |
| nil pointer panic in `SearchSnippet` | CallExpression emitted with nil `Symbol` |
| `expected 1 node for dependency ... received 0` (or 2) | No Symbol at the resolved identifier (missing `Declaration.Names` / `FuncExpression.Name`), or two nodes sharing one byte range |
| `knownerr.HashCollision` panic at parse time | Same-Kind nodes emitted twice at one range under one parent |
| `make build` fails in banstructlit vet on missing `parser.c` | Grammar missing from `scripts/vendor-cgo.sh` (a warm `.gocache` can mask it) |
| Hang at LSP initialize | Server not in stdio mode (missing `--stdio`) or logging to stdout |
| Every node "local" in viz | `ClassifyImportType` patterns don't match the LSP-resolved cache paths |
| Observations root at the file, not at functions | Transformer never emits `ASTFuncExpression`, or `.Block` left un-narrowed so it shadows the function (typical with expression-bodied lambdas) |
