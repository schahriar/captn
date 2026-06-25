# captn

## TODO:
- [ ] Integrate with Claude Code as COG Frontend
  - captn claude
  - [ ] Integrate grep MCP tool so finds are done in captn
- [ ] n-degree of separation on node queries
- [ ] Retries for observation backend
- [ ] Add postprocessing validation step to ensure a quality batch
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