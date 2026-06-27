# captn

## TODO:
- [ ] Persist graph observations as inline log
  - [ ] Allow for growing write sharding, reads all logs
  - [ ] Use file location to decide which shard to write to
    - [ ] Reduces conflicts
    - [ ] Start with 10 shards
    - [ ] Or create a new cog per directory
  - [ ] Attach hashes of git author on the first prefix of logs to avoid conflicts
    - [ ] Dedupe
  - [ ] Add tests for freshness checks
- [ ] Restore graph
- [ ] Modify the MCP query to prefer paths
- [ ] Have graph exploration without resolution
  - [ ] Provides a semantic tree to frontend to query
- [ ] Add a variety of possible queries instead of just explain
  - [ ] Hash queries too to ensure queries stay consistent
- [ ] n-degree of separation on node queries
- [ ] Retries for observation backend
- [ ] Add postprocessing validation step to ensure a quality batch
- [ ] Assign meaning to edges and cache
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