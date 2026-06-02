# Pyrite Standard

Pyrite's standard documentation has been split into smaller topic files under
[docs](docs/).

- [Overview](docs/README.md)
- [Language Basics](docs/language.md)
- [Self-Hosting](docs/self-hosting.md)
- [Native Bindings](docs/native-bindings.md)
- [Functions And Control Flow](docs/functions-control-flow.md)
- [Concurrency And Routines](docs/concurrency.md)
- [Files](docs/files.md)
- [Network Sockets](docs/networking.md)
- [Collections](docs/collections.md)
- [Text, Strings, Regex, Random, And Built-ins](docs/text-random-builtins.md)

The compiler is still intentionally small, so these docs distinguish between the
language contract and the current compiler-backed subset where needed. The
frontend now uses lexer/parser, AST, semantic-analysis, HIR, and C-emission
phases, and the repository includes the `src/pyritec2` Pyrite-written compiler
seed.
