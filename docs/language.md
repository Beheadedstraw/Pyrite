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
`net`, `time`, `sys`, and local modules such as `scoring`. Imported modules can
define functions and classes. Core functions such as `print` are available
without an import.

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
raw: bytes = b"AR\x00\xff"
scores: list[int] = [10, 32]
values: list[any] = ["Ada", 42, 3.5, True]
tokens: list[Token] = []
table: dict = dict()
counts: dict[int] = dict()
seen: set = set()
text: string_builder = string_builder()
data: bytes_builder = bytes_builder()
user: object = {"kind": "student"}
out: file = file.open("score.txt", "w").defer()
tcp: socket = net.tcp("127.0.0.1", 8080).defer()
server: listener = net.listen("127.0.0.1", 8080).defer()
lock: mux = mux()
maybe_token: Token = None
```

First-version annotations:

- `int`
- `float`
- `string` or `str`
- `bytes`
- `bool`
- `any`
- `list[int]`
- `list[any]`
- `list[T]` for typed lists backed by `any` values, including class types
- `dict`
- `dict[T]` for string-keyed dictionaries with typed values
- `set`
- `string_builder`
- `bytes_builder`
- `object`
- `file`
- `socket`
- `listener`
- `mux`

Custom class names can be used as annotations after their class is declared:

```pyrite
token: Token = Token(0, "name")
tokens: list[Token] = []
by_name: dict[Token] = dict()
```

`None` can initialize nullable class fields and annotated class variables. A
field first assigned `None` can later take its concrete class type.

Expressions can continue across physical lines while inside `()`, `[]`, or
`{}`. Trailing commas are accepted in call arguments, list literals, and object
literals:

```pyrite
tokens: list[Token] = [
    Token(TokenKind.IDENT, "name"),
    Token(TokenKind.NUMBER, "42"),
]

value = parse_call(
    tokens[0],
    tokens[1],
)
```

## Enums

Enums define integer constants with dotted names:

```pyrite
enum TokenKind:
    IDENT
    NUMBER
    STRING = 10

kind: int = TokenKind.IDENT
```

Members auto-increment from zero unless a value is assigned.

Boolean literals accept `True` and `False`. Lowercase `true` and `false` are
also accepted for now. Unary `not` is supported for boolean-style checks:

```pyrite
if not cursor.done():
    print("more input")
```

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
