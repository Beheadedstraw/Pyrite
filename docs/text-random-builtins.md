# Text, Regex, Random, And Built-ins

## Print

`print` writes to standard output and appends a newline:

```pyrite
print("hello")
print(42)
print(f"value={value}")
```

Current compiler-backed support is single-argument `print(value)`.

## F-Strings

F-strings use Python-like `f"..."` syntax:

```pyrite
name = "Ada"
score = 42
print(f"{name} scored {score}")
print(f"{score} testing int plus string")
```

Current interpolation supports simple variables, member expressions, function
call results, bools, floats, strings, and integer lists.

Format specifiers such as `{value:04}` and conversion flags such as `{value!r}`
are intentionally out of scope for the first version.

## Regex

Regular expression functions live under `regex.*`:

```pyrite
import regex

def main():
    text = "Ada scored 42"
    matched = regex.match("Ada", text)
    print(f"matched={matched}")
    return 0
```

Current standard module support:

- `regex.match(pattern, text) -> bool`

Planned:

- `regex.find(pattern, text) -> string`
- `regex.replace(pattern, text, replacement) -> string`
- `regex.split(pattern, text) -> list`

## Strings

String helpers are available as Python-like methods and as functions from the
`strings` module:

```pyrite
import strings

def main():
    raw = "  Ada Lovelace  "
    needle = "Love"
    cleaned = raw.strip()
    print(cleaned.upper())
    print(cleaned.replace("Ada", "Augusta"))
    print(f"contains Love = {cleaned.contains(needle)}")
    print(strings.lower("MODULE CALL"))
    return cleaned.len()
```

Current support:

- `value.strip()`, `value.lstrip()`, `value.rstrip()`
- `value.upper()`, `value.lower()`
- `value.len() -> int`
- `value.find(needle) -> int`
- `value.contains(needle) -> bool`
- `value.startswith(prefix) -> bool`
- `value.starts_with(prefix) -> bool`
- `value.endswith(suffix) -> bool`
- `value.ends_with(suffix) -> bool`
- `value.replace(old, replacement) -> string`
- `value.slice(start, end) -> string`

The same operations are also available as `strings.strip(value)`,
`strings.upper(value)`, `strings.replace(value, old, replacement)`, and so on.

## Integers And Floats

Numeric helpers are available as methods and as functions from `ints` and
`floats`:

```pyrite
import ints
import floats

def main():
    count = -7
    ratio: float = -3.75

    print(count.abs())
    print(count.clamp(0, 10))
    print(ratio.round())
    print(ratio.to_string())
    print(ints.max(3, 9))
    print(floats.floor(2.75))
    return ratio.abs().round()
```

Integer support:

- `value.abs() -> int`
- `value.min(other) -> int`
- `value.max(other) -> int`
- `value.clamp(min, max) -> int`
- `value.to_float() -> float`
- `value.to_string() -> string`
- `value.is_even() -> bool`
- `value.is_odd() -> bool`

Float support:

- `value.abs() -> float`
- `value.min(other) -> float`
- `value.max(other) -> float`
- `value.clamp(min, max) -> float`
- `value.round() -> int`
- `value.floor() -> int`
- `value.ceil() -> int`
- `value.trunc() -> int`
- `value.to_int() -> int`
- `value.to_string() -> string`

The same operations are also available as `ints.abs(value)`,
`ints.clamp(value, min, max)`, `floats.round(value)`,
`floats.clamp(value, min, max)`, and so on.

## Random

Random functions live under `random.*`:

```pyrite
import random

def main():
    random.seed(123)
    scores = [10, 20, 30]
    bonus = random.int(1, 6)
    ratio = random.float()
    picked = random.choice(scores)
    print(f"random bonus = {bonus}")
    print(f"random float = {ratio}")
    print(f"random choice = {picked}")
    return bonus
```

Current standard module support:

- `random.int(min, max) -> int`
- `random.seed(value) -> int`
- `random.float() -> float`
- `random.choice(items: list[any]) -> any`

Integer lists are converted at the call boundary, so `random.choice([1, 2, 3])`
also works.

`regex` and `random` are declared in `stdlib/regex.pyr` and
`stdlib/random.pyr` with native runtime bindings.

## Time

Time helpers live under `time.*`:

```pyrite
import time

def main():
    print("before sleep")
    time.sleep(0.25)
    print("after sleep")
    return 0
```

Current standard module support:

- `time.sleep(seconds: float) -> int`

`time.sleep` pauses the current thread for at least the requested number of
seconds and returns `0`.

## Common Built-ins

- `print(value)`
- `len(value) -> int` is planned for strings, lists, and dictionaries.
