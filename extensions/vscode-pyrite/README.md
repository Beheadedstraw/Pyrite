# Pyrite Language Support

VS Code support for Pyrite `.pyr` files.

## Features

- Registers `.pyr` files as the `pyrite` language
- Syntax highlighting for functions, classes, enums, strings, byte strings,
  f-strings, typed containers, standard modules, comments, and operators
- Language-server-backed code completion for keywords, types, built-ins,
  standard modules, standard module members, snippets, and local document
  symbols
- Auto indentation for `def`, `class`, `enum`, `if`, `else`, `while`, `for`,
  `foreach`, `switch`, `match`, `try`, and `except`
- Snippets for common Pyrite constructs
- Commands:
  - `Pyrite: Compile Current File`
  - `Pyrite: Compile Current File With Defer Trace`
  - `Pyrite: Compile And Run Current File`

## Local Development

Open this folder in VS Code and press `F5` to launch an Extension Development
Host.

The compile commands use:

1. `pyrite.compilerPath` when configured
2. `<workspace>/build/pyritec` when it exists
3. `pyritec` on `PATH`

Compiled binaries are written to `pyrite.outputDirectory`, which defaults to
`build`.

## Language Server

The extension starts `server/server.js` through the Language Server Protocol.
Today the server provides completions. It is intentionally small, but it gives
Pyrite a clean place to add diagnostics, hover text, definitions, workspace
symbols, and compiler-powered checks later.
