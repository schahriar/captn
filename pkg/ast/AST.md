# AST

An Abstract Syntax Tree but doubly abstracted to represent a partial parsing of snippets rather than a full construction intended for compiler passes / checks or lowering.

Goals:
- Provide a import / block / symbol resolution for any language
- Guaranteed information retention over correctness.
  - No correctness pass can cover complete / incomplete code across 30+ languages.
  - Our main use-cases are computational caching, block filters, and graph construction most of which don't need semantic correctness, but approximate relationships to reduce the search space.

Constraints:
- AST should be able to represent any tree-sitter supported programming language
- It must not enforce syntax correctness, broken code should parse with best effort
  - Use Virtual nodes when needed. They will act as a source code container.
- It must retain the ability to reconstruct full source from the combination of all nodes
- It must retain non-executable syntax (comments, hints)