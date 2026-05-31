# captn

## TODO:

- [ ] Add LSP and AST tests
  - [ ] Resolve imports and symbols
  - [ ] Best to add snapshot testing with some unit tests on the tree structure
- [ ] Modify function names to be a symbol to track position
- [ ] Interval tree for each symbol for fast lookups https://github.com/rdleal/intervalst
- [ ] Populate "Semantic Graph" from ASTNodes
  - [ ] https://github.com/dominikbraun/graph
- [ ] Assign hashes to each node and layered hashes to each edge
- [ ] Add observation backend
  - [ ] Claude Code will be the first provider
- [ ] Persist graph observations as inline log
- [ ] Assign meaning to edges and cache
- [ ] Restore graph
- [ ] Support Comments
- [ ] MCP Server
- [ ] Integrate with Claude Code as COG Frontend and Backend
- [ ] Add support for python, typescript, TSX
- [ ] Add OpenAI SDK and test with Ollama