# Agent instructions - captn

<!-- BEGIN AUTHOR HAND WRITTEN SECTION — DO NOT EDIT -->
## Author's rules, do not modify
Captn is a computational cache layer for coding agent reasoning thoughts. It works by parsing any language supported by treeparser into a abstract AST (doubly abstracted) and then attaching answers to generic reasoning questions to the graph nodes on-demand (as the coding agent demands).

Cached nodes are called COGs (cached observation graph), where an observation is a answer to a question (referred to as query in this repo).

> Note that a complex AST makes the final result of this repo less cache friendly. Use careful judgement and adverserial reviews when adding / changing language support or in rare occassions updating the AST>

There few important product decrees you must follow:
- **Captn must be not change developers's workflow**, at times we provide additional hints while the coding agent waits for captn but overall the development workflow must remain mostly the same.
- **Captn must integrate seamlessly into existing workflows**, no special instructions, complex behaviors. Our goal is to cache reasoning for any coding agent to cut down on planning and research time.
- **Captn observations must be shareable**, they go into git in a log file, one person producing an observation should benefit everyone who takes the log file through git.
- **Captn observations must not create git conflicts**, no matter what, captn should never product git conflicts over its state (COG log files) for developers to resolve.
  

And a few technical rules to follow:
1. **You are not writing a programming language**, it's easy to make this mistake. When we build an AST we are not writing an AST that compiles into code, you are writing a tree that resolves into blocks that can contain meaningful information about the repository. Think modules, functions, sections.
2. **Clarity over correctness**, this repo is complex and has several sections written by humans with careful intent. When planning a new implementation focus on clarity over covering rare edge cases and scenarios. Yet, let the developer you are assisting know of edge cases not covered in favor of clarity and this rule.
3. **Plan for interoperability**, captn must work on Mac, Linux, Window and with Claude Code, Codex, and some future harness. While maintaining implementation clarity, ensure we don't miss out on future compatibility.
4. **Build and reuse debug tools**, help the developer by using / reusing debug tools in the `tools` folder or implementing simple new ones.
5. **Every word must make sense**, do not author long documentation, pull requests, commit messages, etc. where the entire context must be read for a human to understand what you wrote. Every section must make sense in isolation, if you need to spend an extra generation round on this rule do not hesitate.
6. **Output errors that help with reproducibility**, this repo has `knownerr` package for this exact purpose. Ensure that we return known errors to the developer in case they need to report an issue.
7. **Do not over-comment**, retain the priviledge of commenting in code for exceptional cases needing an explanation. A developer can always ask you for an explanation later.
8. **Do not change more than you need to, unless the develoepr asks**, if constructing a new PR / feature stick to the developers ask. You can ask them in a multi-choice question if they want the approach to implementation to be focused or potentially make larger changes. 
9. **Search for existing libraries**, if a library exists that can achieve a task, prefer it.
<!-- END AUTHOR HAND WRITTEN SECTION — DO NOT EDIT -->

## Learnings

Things that were expensive to discover. Each one cost a wrong turn.

### The graph is the explanation

`ObservationGraph.QueryWithDepth` builds its answer from *every vertex* ("X does the following") and
*every edge* ("X uses Y with the following answer"). Dropping a vertex drops the explanation of a
dependency, not just a node. Do not filter the dependencies `SearchSnippet` resolves — a stdlib or
package definition is often the thing that explains the behaviour being asked about.

Only `ASTFuncExpression` and `ASTModule` become vertices (`IsNodeOfInterest`). Anything a language
maps to `ASTDeclaration` collapses onto its nearest enclosing function or, failing that, the whole
module. That is why Swift maps classes, protocols, extensions and even bodyless protocol
requirements to `ASTFuncExpression`: a vertex per type and per function, not one per file.

### Identity

`ast.GetHash` reads only the node's own path, source, position and kind, plus its parent's hash.
Siblings never participate. Two consequences: a partial parse that builds only the spine down to one
node yields byte-identical hashes, and any edit anywhere in a file changes every hash in that file,
because the module's source is the whole file and it sits in every chain.

Identity is *not* portable outside the workspace. `Source.RelativePath` resolves an outside path by
climbing out with `../`, and the number of segments depends on how deep the checkout sits, so the
same SDK file hashes differently for every developer. Observations anchored there can never hit for
anyone else. See `pkg/cog/CACHE.md` before designing anything that shares parsed state.

### Language servers disagree about ranges

`textDocument/definition` may answer with the identifier's extent (gopls, pyright) or with a
zero-width position at its start (sourcekit-lsp). Repair the shape in the language's own
`NormalizeDefinitionRange`, which runs in `NewResolvedDependencyFromURI` while the target file's
`Source` is in hand — the range is resolved by containment afterwards, so a range spanning no bytes
resolves to nothing at all. Do not teach the shared range queries a second shape; a server quirk
belongs to the server.

Never write `ClassifyImportType` from documentation. Run the server against a fixture and look at
the paths it actually returns — sourcekit-lsp resolves every cross-module definition into a
generated interface under a temp directory, which no plausible guess would have matched.

### Measuring

Benchmark `B/op` is allocation churn, not size; a parse that reported 34 GB retained 4 MB. Go's
`runtime.MemStats` cannot see cgo, so a tree-sitter tree is invisible to it — use process RSS, and
measure a marginal slope across several held trees rather than one before/after reading, since freed
C memory is not returned to the OS.

### Grammars have holes; do not paper over them

tree-sitter-swift cannot parse a function without a body, so every declaration in a `.swiftinterface`
is an ERROR node. Error recovery groups an arbitrary run of declarations, so reading structure back
out of it invents relationships the grammar never established. Keep the name so definitions still
resolve, accept the coarse vertex, and say so.