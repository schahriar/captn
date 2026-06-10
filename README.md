# captn

## TODO:
- [ ] Populate "Semantic Graph" from ASTNodes
  - [ ] Resolve imports to final path
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