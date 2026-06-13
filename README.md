# captn

## TODO:
- [ ] Add observation backend
  - [ ] Claude Code will be the first provider
  - [ ] Add caching to ResolveObservationsToGraph
  - [ ] Explain subgraphs as a batch including relationships etc
  - [ ] Subgraphs for symbol relationships?
  - [ ] Block-based observations
- [ ] Hash questions too to ensure queries stay consistent
- [ ] Implement LSP server re-use rather than spawn
- [ ] Assign layered hashes to each edge
- [ ] Persist graph observations as inline log
  - [ ] Allow for growing write sharding, reads all logs
  - [ ] Use file location to decide which shard to write to
    - [ ] Reduces conflicts
    - [ ] Start with 10 shards
  - [ ] Add tests for freshness checks
- [ ] Assign meaning to edges and cache
- [ ] Restore graph
- [ ] Add a per language import to package name and path resolver
  - [ ] Possibly in AST?
- [ ] Support Comments
- [ ] MCP Server
- [ ] Integrate with Claude Code as COG Frontend and Backend
- [ ] Add support for python, typescript, TSX
- [ ] Add OpenAI SDK and test with Ollama


Document value prop:
- Guaranteed freshness with computational caching
- n * x reduction in token usage and time
- lazy observation graph allowing for longer and cheaper sessions