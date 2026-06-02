# Self-Hosting

The active production compiler is still the Go compiler in `cmd/pyritec`, but
the repository now includes a Pyrite-written compiler seed in `src/pyritec2`.

Build it with the Go compiler:

```sh
build/pyritec src/pyritec2/main.pyr -o build/pyritec2
```

Use it to emit C for the starter subset:

```sh
build/pyritec2 examples/hello.pyr -o build/hello.c
gcc -std=c11 -Wall -Wextra -O2 -o build/hello-from-pyritec2 build/hello.c
```

Current `pyritec2` support:

- `import ...` lines are accepted and skipped
- `def main():`
- `const name = integer_expression`
- inferred integer locals such as `value = 42`
- annotated integer locals such as `value: int = 42`
- assignment and arithmetic expression passthrough for integer-shaped values
- `print("text")`
- `print(value)`
- `return value`

This is not a replacement compiler yet. It is the bootstrap path for growing a
Pyrite compiler written in Pyrite. The next milestones are helper functions,
typed string locals, `if` / `while`, a real token stream, and enough expression
parsing for `pyritec2` to compile its own modules.
