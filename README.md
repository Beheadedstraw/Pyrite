# Pyrite

Pyrite is a Python-shaped language that compiles to native code instead of being
interpreted. The active compiler is now written in Go. It transpiles Pyrite to C,
then invokes `gcc` to produce a native executable.

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
- `def main():` and simple helper functions like `def testr(m):`
- inferred variables and optional static annotations like `name: string = "Ada"`
- `int`, `float`, `string`, `bool`, `any`, `list[int]`, `list[any]`, `dict`, `file`, `socket`, `listener`, and `mux` annotations
- immutable bindings with `const`
- module globals with `global`
- integer addition
- integer lists and `for item in list:`
- `foreach(items):`, `foreach(items, item_name):`, and `foreach([1, 2, 3]):` list iteration
- mixed `list[any]` lists with ints, floats, bools, and strings
- object-shaped values with inherited defaults from imported Pyrite modules
- first-version classes with `__init__`, fields, and methods
- `if` / `else` and `while` comparisons
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
- string methods like `value.strip()`, `value.upper()`, `value.replace(...)`, `value.slice(...)`
- numeric methods like `value.abs()`, `value.clamp(...)`, `ratio.round()`
- `native def` declarations for runtime-backed Pyrite modules
- `return <integer expression>` from `main` and helper functions
- bare helper calls like `testr(print_lock)`
- helper calls in expressions, such as `value = answer()`
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

The compiler is still intentionally small and line-oriented. The next milestone
is a real lexer/parser so these features become general instead of pattern-based
for the current syntax subset.
