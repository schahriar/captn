## Rules for Parsers

- Parsers should attempt to self-recover
  - While Captn can gracefully recover from parser panics, it is encouraged to default to virtual blocks when parsing fails so incomplete / incorrect code (be it syntax, semantics, etc.) is still indexed at the best effort.
  - An an implementor of a language parser you should provide additional thought on whether the tree can be safely folded into a generic virtual that effectively acts as a snippet without the syntax subtree of the node. In many cases this only **impales the ability** for Captn to make deterministic decisions but should not be an outright file error or fatal. (TODO: Provide example)