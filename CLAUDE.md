# Agent instructions - captn

<!-- BEGIN AUTHOR HAND WRITTEN SECTION — DO NOT EDIT -->
## Author's rules, do not modify
Captn is a computational cache layer for coding agent reasoning thoughts. It works by parsing any language supported by treeparser into a abstract AST (doubly abstracted) and then attaching answers to generic reasoning questions to the graph nodes on-demand (as the coding agent demands).

Cached nodes are called COGs (cached observation graph), where an observation is a answer to a question (referred to as query in this repo).

> Note that a complex AST makes the final result of this repo less cache friendly. Use careful judgement and adverserial reviews when adding / changing language support or in rare occassions updating the AST>

There few important product decrees you must follow:
- **Captn must be not change developers's workflow**, at times we provide additional hints while the coding agent waits for captn but overall the development workflow must remain mostly the same.
- **Captn must integrate seamlessly into existing workflows**, no special instructions, complex behaviors. Our goal is to cache reasoning for any coding agent to cut down on planning and research time.
- **Captn observations must be shareable**, they go into git in a log file, one person producing an observation should benefit everyone who takes the log file through git.
- **Captn observations must not create git conflicts**, no matter what, captn should never product git conflicts over its state (COG log files) for developers to resolve.
  

And a few technical rules to follow:
1. **You are not writing a programming language**, it's easy to make this mistake. When we build an AST we are not writing an AST that compiles into code, you are writing a tree that resolves into blocks that can contain meaningful information about the repository. Think modules, functions, sections.
2. **Clarity over correctness**, this repo is complex and has several sections written by humans with careful intent. When planning a new implementation focus on clarity over covering rare edge cases and scenarios. Yet, let the developer you are assisting know of edge cases not covered in favor of clarity and this rule.
3. **Plan for interoperability**, captn must work on Mac, Linux, Window and with Claude Code, Codex, and some future harness. While maintaining implementation clarity, ensure we don't miss out on future compatibility.
4. **Build and reuse debug tools**, help the developer by using / reusing debug tools in the `tools` folder or implementing simple new ones.
5. **Every word must make sense**, do not author long documentation, pull requests, commit messages, etc. where the entire context must be read for a human to understand what you wrote. Every section must make sense in isolation, if you need to spend an extra generation round on this rule do not hesitate.
6. **Output errors that help with reproducibility**, this repo has `knownerr` package for this exact purpose. Ensure that we return known errors to the developer in case they need to report an issue.
7. **Do not over-comment**, retain the priviledge of commenting in code for exceptional cases needing an explanation. A developer can always ask you for an explanation later.
8. **Do not change more than you need to, unless the develoepr asks**, if constructing a new PR / feature stick to the developers ask. You can ask them in a multi-choice question if they want the approach to implementation to be focused or potentially make larger changes. 
9. **Search for existing libraries**, if a library exists that can achieve a task, prefer it.
<!-- END AUTHOR HAND WRITTEN SECTION — DO NOT EDIT -->