# Functions And Control Flow

## Functions

Functions use `def`:

```pyrite
def main():
    return 0

def add(a: int, b: int):
    return a + b
```

Current compiler-backed rules:

- `def main():` is the program entrypoint.
- `def name(args):` creates a helper function.
- Helper calls such as `testr(print_lock)` are valid statements.
- Helper calls can be used as expressions when the helper returns a value.
- Parameter types are inferred from calls, or can be written explicitly.
- Helper return types are inferred from `return` statements.

Example:

```pyrite
def main():
    value = answer()
    print(value)
    return value

def answer():
    return 7
```

## Cleanup

Resource-returning calls can register cleanup with `.defer()`:

```pyrite
f = file.open("notes.txt", "r").defer()
tcp = net.tcp("127.0.0.1", 8080).defer()
```

Tracked runtime heap memory is cleaned up before `main` returns. Helper-function
temporary cleanup is not yet scoped per helper.

## If / Else

```pyrite
if user.score >= 40:
    print("score status = passing")
else:
    print("score status = retry")
```

Supported comparison operators:

- `==`
- `!=`
- `<`
- `<=`
- `>`
- `>=`

## While

```pyrite
i = 0

while i < 3:
    print(i)
    i = i + 1

while True:
    print("loop forever")
```

## Switch / Case

`switch` evaluates the target once and checks `case` values in order. String
switches compile to `strcmp` checks; integer and boolean switches compile to
numeric comparisons.

```pyrite
switch path:
    case "/api/ping":
        return http.json("{\"ok\":true}")
    case "/api/text":
        return http.text("hello\n")
    default:
        return http.not_found()
```

Supported first-version forms:

- `switch value:`
- `case value:`
- `default:`
- string, int, and bool switch values

## For Each

```pyrite
scores = [10, 32]

for score in scores:
    print(f"score item = {score}")

foreach(scores):
    print(item)

foreach(scores, score):
    print(score)

foreach([1, 2, 3]):
    print(item)
```

`foreach(items):` uses an implicit `item` variable. `foreach(items, name):`
uses the provided item variable name. The current compiler supports `list[int]`
and `list[any]` variables or list literals.

## Try / Except

```pyrite
try:
    raise "could not score user"
except err:
    print(err)
```

Supported first-version forms:

- `try:`
- `except:`
- `except name:`
- `raise "message"`
- Resource acquisition raises to the nearest active `try` on failure. This
  includes `file.open`, `net.tcp`, `net.udp`, `net.listen`, and `net.accept`
  when assigned to a resource variable.

Exception values are strings for now.
