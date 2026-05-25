# Pyrite Docs

Pyrite is a Python-shaped language that compiles to native code. The active
compiler is written in Go, transpiles Pyrite to C, and invokes `gcc`.

Pyrite is not Python compatibility mode. The goal is familiar syntax with a
small compiled runtime.

## Modules

- [Language Basics](language.md): imports, variables, typing, constants, globals
- [Native Bindings](native-bindings.md): writing Pyrite modules backed by runtime C
- [Functions And Control Flow](functions-control-flow.md): `def`, returns,
  `if`, `while`, loops, `try` / `except`
- [Concurrency And Routines](concurrency.md): `routine(...)`, `mux()`, locking
- [Files](files.md): file handles, reads, writes, `.defer()`, cleanup tracing
- [Network Sockets](networking.md): TCP/UDP sockets and pure Pyrite HTTP helpers
- [Collections](collections.md): lists, dictionaries, inheritance
- [Classes](classes.md): first-version class declarations, fields, methods
- [Data Formats](data-formats.md): JSON and XML helpers
- [Vector Math](vector-math.md): pure Pyrite vector helpers
- [Text, Strings, Regex, Random, And Built-ins](text-random-builtins.md):
  printing, f-strings, string/numeric methods, regex, random, common functions

## Compiler Subset

The current compiler recognizes core syntax, module loading, native bindings,
helper functions, routines, resources, lists, object-shaped dictionary defaults,
module functions, and module globals. Standard APIs such as files, sockets,
regex, random, routine locks, JSON, and XML now enter through `.pyr` modules.
Unsupported syntax fails at compile time instead of being silently ignored.
