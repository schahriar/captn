# captn

## TODO:
- [ ] Add observation backend
  - [ ] **Provided a snippet and file path** explain subgraphs as a batch including relationships etc
    - [ ] Block-based observations (should filter to relevant high level scopes)
    - [ ] Resolve references for Blocks, gets n-degree of separation
- [ ] MCP Server
- [ ] Integrate with Claude Code as COG Frontend and Backend
- [ ] Assign layered hashes to each edge
  - [ ] Hash questions too to ensure queries stay consistent
- [ ] Persist graph observations as inline log
  - [ ] Allow for growing write sharding, reads all logs
  - [ ] Use file location to decide which shard to write to
    - [ ] Reduces conflicts
    - [ ] Start with 10 shards
    - [ ] Or create a new cog per directory
  - [ ] Add tests for freshness checks
- [ ] Assign meaning to edges and cache
- [ ] Restore graph
- [ ] Add Codex support
- [ ] Add a per language import to package name and path resolver
  - [ ] Possibly in AST?
- [ ] Support Comments
- [ ] Add support for python, typescript, TSX
- [ ] Add OpenAI SDK and test with Ollama


Document value prop:
- Guaranteed freshness with computational caching
- n * x reduction in token usage and time
- lazy observation graph allowing for longer and cheaper sessions