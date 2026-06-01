# captn

## TODO:
- [ ] Add a new type for import resolutions
- [ ] Add LSP and AST tests
  - [ ] Resolve imports and symbols
  - [ ] Best to add snapshot testing with some unit tests on the tree structure
- [ ] Populate "Semantic Graph" from ASTNodes
  - [ ] https://github.com/dominikbraun/graph
  - [ ] Subgraphs for symbol relationships?
  - [ ] Assign layered hashes to each edge
- [ ] Add observation backend
  - [ ] Claude Code will be the first provider
- [ ] Persist graph observations as inline log
  - [ ] Allow for growing write sharding, reads all logs
  - [ ] Use file location to decide which shard to write to
    - [ ] Reduces conflicts
    - [ ] Start with 10 shards
- [ ] Assign meaning to edges and cache
- [ ] Restore graph
- [ ] Support Comments
- [ ] MCP Server
- [ ] Integrate with Claude Code as COG Frontend and Backend
- [ ] Add support for python, typescript, TSX
- [ ] Add OpenAI SDK and test with Ollama


Document value prop:
- Guaranteed freshness with computational caching
- n * x reduction in token usage and time
- lazy observation graph allowing for longer and cheaper sessions