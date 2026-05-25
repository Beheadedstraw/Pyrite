# Language Basics

## Calls

Functions use Python-like calls:

```pyrite
value = add(x, 2)
print(value)
print(f"value = {value}")
```

## Imports

Modules are imported at the top of a file:

```pyrite
import file
import regex
import random
import routines
import net
import time
import scoring
```

Imports first look for a sibling `<name>.pyr` file next to the input file, then
for `stdlib/<name>.pyr`. Module functions are called with dotted names:

```pyrite
import scoring

def main():
    total = scoring.add(2, 3)
    return total
```

Current compiler-backed imports include `file`, `regex`, `random`, `routines`,
`net`, `time`, and local modules such as `scoring`. Core functions such as
`print` are available without an import.

Planned import forms:

- `import name`
- `import name as alias`
- `from module import name`
- `from module import a, b`

## Variables

Variables are assigned without `let`:

```pyrite
x = 40
y = add(x, 2)
name = "Ada"
```

Types are inferred by default. Optional annotations use Python-style syntax:

```pyrite
x: int = 40
ratio: float = 0.75
name: string = "Ada"
scores: list[int] = [10, 32]
values: list[any] = ["Ada", 42, 3.5, True]
user: dict = {"kind": "student"}
out: file = file.open("score.txt", "w").defer()
tcp: socket = net.tcp("127.0.0.1", 8080).defer()
server: listener = net.listen("127.0.0.1", 8080).defer()
lock: mux = mux()
```

First-version annotations:

- `int`
- `float`
- `string` or `str`
- `bool`
- `any`
- `list[int]`
- `list[any]`
- `dict`
- `file`
- `socket`
- `listener`
- `mux`

Boolean literals accept `True` and `False`. Lowercase `true` and `false` are
also accepted for now.

## Constants And Globals

Use `const` for immutable bindings:

```pyrite
const max_bonus: int = 6
```

Use `global` for module-level values:

```pyrite
global app_name: string = "Ada"
```

Rules:

- `const name = value` creates an immutable binding.
- `const name: type = value` creates an immutable binding with a checked type.
- Reassigning a const name is a compile-time error.
- `global name = value` creates a module-level value.
- Module-level `const` creates a module-level immutable value.
