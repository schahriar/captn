# captn

## TODO:
- [x] Auto-install LSP servers
- [x] Have `captn` work with `--prompt` in no TUI mode
- [ ] Fix port already in use error
- [ ] Fix `captn` system prompt not propagating to sub-agents
  - [ ] Use hooks + (system prompts and skill) to ensure Claude doesn't get shocked on the non-grep outputs
  - [ ] Explore modifying `agents` explorer config
- [ ] Add tool to collect automated chat sessions for experimentation
- [ ] Benchmark using predefined set of changes in popular git repos
- [ ] Add subgraph exporter to debug behavior
- [ ] Add support for python, typescript, Java, TSX, Swift, Rust, C, C++, Ruby, PHP, CSS, Shell, HTML
- [ ] A new cog per directory
- [ ] Add tests for freshness checks
- [ ] Dedupe and GC
- [ ] Tracing and better error file
- [ ] Modify the child spawns to prefer paths
- [ ] n-degree of separation on node queries
- [ ] Add postprocessing validation step to ensure a quality batch
- [ ] Add Codex support
- [ ] Support Comments
- [ ] Add OpenAI SDK and test with Ollama

Document value prop:
- Guaranteed freshness with computational caching
- n * x reduction in token usage and time
- lazy observation graph allowing for longer and cheaper sessions