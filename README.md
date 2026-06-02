# Pyrite

Pyrite is a Python-shaped language that compiles to native code instead of being
interpreted. The active compiler is now written in Go. It transpiles Pyrite to C,
then invokes `gcc` to produce a native executable.

The compiler now uses a real lexer/parser and an AST-driven pipeline with
explicit load/bind, analyze, HIR, and emit phases. A Pyrite-written bootstrap
compiler seed lives under [src/pyritec2](src/pyritec2); it is intentionally
small, but it is compiled by Pyrite and can emit C for the current starter
subset.

The older assembly compiler files remain under `src/` as historical bootstrap
work, but `make` builds [cmd/pyritec](cmd/pyritec).

## Documentation

The language and standard library docs are split by topic under [docs](docs/).
[STANDARD.md](STANDARD.md) is now a short index for those files.

## Working Compiler Subset

This smaller example is compiled from input today:

```pyrite
import console

def main():
    const a: int = 10
    b = 32
    c: int = a + b
    print("hello from pyrite")
    print("this was emitted from source")
    print(c)
    return c
```

Supported today:

- `import name`, loading sibling modules or `stdlib/name.pyr`
- module-local functions and classes, including classes declared in imported
  modules
- `def main():` and helper functions like `def testr(m):`
- inferred variables and optional static annotations like `name: string = "Ada"`
- `int`, `float`, `string`, `bool`, `bytes`, `any`, `list[int]`, `list[any]`, `list[T]`, `dict`, `dict[T]`, `set`, builders, resources, and class annotations
- immutable bindings with `const`
- module globals with `global`
- `None` for nullable class fields
- Python-style unary `not`
- integer addition
- integer lists and `for item in list:`
- `foreach(items):`, `foreach(items, item_name):`, and `foreach([1, 2, 3]):` list iteration
- mixed `list[any]` lists and typed lists/dicts with primitive or class values
- class fields that hold typed lists, including `list[Token]`
- object-shaped values with inherited defaults from imported Pyrite modules
- first-version classes with `__init__`, fields, and methods
- typed class lists such as `tokens: list[Token] = []`
- indexed member expressions like `tokens[0].text`
- enums with explicit or expression-based integer values
- `if` / `else`, `while`, `switch`, and `match` comparisons
- `try` / `except`, string `raise`, and catchable `file.open` failures
- standard `json` and `xml` modules for first-version data format handling
- pure Pyrite `vectors` module for fixed 2D/3D integer vector math
- catchable resource acquisition failures for files, sockets, listeners, and accepts
- `import routines`, `routine(print(...))`, `routine(helper(...))`, and `mux()` for lightweight concurrent work
- `print(...)`, including selected f-string forms
- stdlib-backed `file.open(...).defer()`, `file.write`, `file.flush`, `file.read_all`
- stdlib-backed `net.tcp`, `net.udp`, `net.listen`, `net.accept`, `net.write`, and `net.read` for sockets
- compiler flag `--trace-defer` for cleanup tracing
- stdlib-backed `regex.match`
- stdlib-backed `random.int`
- `random.seed`, `random.float`, and `random.choice`
- stdlib-backed `time.sleep(seconds)`
- process arguments through `sys.arg_count()` and `sys.arg(index)`
- string methods like `value.strip()`, `value.upper()`, `value.replace(...)`, `value.slice(...)`, `value[index]`, `value.byte(index)`, `value.to_int()`, `value.is_digit()`, `value.is_alpha()`, `value.is_alnum()`, and `value.is_space()`
- numeric methods like `value.abs()`, `value.clamp(...)`, `ratio.round()`
- `native def` declarations for runtime-backed Pyrite modules
- `return <integer expression>` from `main` and helper functions
- bare helper calls like `testr(print_lock)`
- helper calls in expressions, such as `value = answer()`
- multiline expressions inside `()`, `[]`, and `{}`, including trailing commas
- blank lines and `#` comments

Unsupported syntax now fails instead of being silently ignored.

## Build

```sh
make
make run
```

The default command compiles [examples/base.pyr](examples/base.pyr) into
`build/base` and runs it.

Compile a specific input file:

```sh
build/pyritec examples/base.pyr -o build/base
```

Compile with defer cleanup tracing:

```sh
build/pyritec --trace-defer examples/base.pyr -o build/base
```

When enabled, deferred cleanup prints to stderr when file handles close and
when tracked heap memory is released.

or through Make:

```sh
make compile SRC=examples/hello.pyr OUT=build/hello
make run SRC=examples/hello.pyr OUT=build/hello
make run SRC=examples/routines.pyr OUT=build/routines
make compile SRC=examples/sockets.pyr OUT=build/sockets
make compile SRC=examples/errors.pyr OUT=build/errors
make compile SRC=examples/strings.pyr OUT=build/strings
make compile SRC=examples/numbers.pyr OUT=build/numbers
make compile SRC=examples/random.pyr OUT=build/random
make compile SRC=examples/any.pyr OUT=build/any
make compile PYRITEC_FLAGS=--trace-defer
```

Compile a freestanding kernel object:

```sh
build/pyritec --target freestanding examples/kernel_hello.pyr -o build/kernel_hello.o
```

The freestanding target emits `long kmain(void)` and a tiny no-libc runtime.
See [docs/freestanding.md](docs/freestanding.md).

## Pyrite-Written Compiler Seed

The repository now includes a Pyrite-written bootstrap compiler seed:

```sh
build/pyritec src/pyritec2/main.pyr -o build/pyritec2
build/pyritec2 examples/hello.pyr -o build/hello.c
gcc -std=c11 -Wall -Wextra -O2 -o build/hello-from-pyritec2 build/hello.c
```

`pyritec2` currently compiles a small subset: `import` skipping, `def main`,
integer `const`, integer locals, integer assignment/arithmetic passthrough,
`print("text")`, `print(value)`, and `return value`. This is the active
self-hosting path, not a replacement for the Go compiler yet.

## Benchmarks

Paired Pyrite/Python benchmark files live under [benchmarks](benchmarks/).

```sh
scripts/bench.sh
```

The runner compiles the Pyrite benchmark files, runs each Pyrite executable,
runs the matching Python file, and prints wall-clock seconds.

Expected output:

```text
Ada scored 42
scores = [10, 32]
random bonus = <1..6>
inherited kind = student
score item = 10
score item = 32
score status = passing
while loop
0
1
caught missing file
file.open failed: missing-score.txt
saved: Ada scored 42
regex matched Ada: true
exit=42
```

The compiler is still intentionally small, but it is no longer line-oriented:
the frontend now tokenizes, parses into AST nodes, performs semantic analysis,
lowers into a first HIR layer, and emits C from structured compiler state. The
next self-hosting milestone is growing `pyritec2` until it can compile its own
modules.
