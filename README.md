# captn

## TODO:
- [ ] Provide detailed logs on what Captn does
- [ ] Attach hashes of git author on the first prefix of logs to avoid conflicts
  - [ ] Dedupe and GC
- [ ] Have graph exploration without resolution
  - [ ] Provides a semantic tree to frontend to query
  - [ ] Allow for Claude to call different questions
- [ ] Add tests for freshness checks
- [ ] A new cog per directory
- [ ] Modify the child spawns to prefer paths
- [ ] n-degree of separation on node queries
- [ ] Retries for observation backend
- [ ] Add postprocessing validation step to ensure a quality batch
- [ ] Add Codex support
- [ ] Support Comments
- [ ] Add support for python, typescript, TSX
- [ ] Add OpenAI SDK and test with Ollama

Document value prop:
- Guaranteed freshness with computational caching
- n * x reduction in token usage and time
- lazy observation graph allowing for longer and cheaper sessions