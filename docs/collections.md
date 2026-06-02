# Collections

## Lists

Lists use Python-like literal and indexing syntax:

```pyrite
def main():
    nums = [10, 32]
    values: list[any] = ["Ada", 42, 3.5, True]
    print(nums[0])
    print(values[0])
    return nums[0] + nums[1]
```

Current compiler-backed support includes integer lists, mixed `list[any]`
lists, and typed lists such as `list[string]` or `list[Token]`:

- `items = [a, b, c]`
- `values: list[any] = ["Ada", 42, 3.5, True]`
- `tokens: list[Token] = []`
- `items[index]`
- `items.len() -> int`
- `items.get(index) -> item`
- `items.at(index) -> item`
- `items.set(index, value) -> list`
- `items.push(value) -> list`
- `items.pop() -> list`
- `items.peek() -> item`
- `for item in items:`
- `foreach(items):`
- `foreach(items, name):`
- `foreach([1, 2, 3]):`
- printing and f-string formatting for `list[int]`, `list[any]`, and typed
  lists

`list[any]` and typed non-int lists currently store ints, floats, bools,
strings, bytes, class objects, and nested list values.

Typed class lists can be used for compiler-style data:

```pyrite
tokens: list[Token] = []
tokens = tokens.push(Token(TokenKind.IDENT, "name"))
first: Token = tokens[0]
print(first.text)
```

List update operations are persistent-style in this compiler generation. Assign
the returned list back when you want to keep the change:

```pyrite
items: list[int] = [1, 2]
items = items.push(3)
items = items.set(0, 9)
items = items.pop()
```

## Bytes

`bytes` stores binary data without treating `0` as the end of the value:

```pyrite
def main():
    data: bytes = b"AR\x00\xff"
    more: bytes = bytes([1, 2, 255])
    joined: bytes = data + more.push(7)
    print(joined.len())
    print(joined.get(3))
    print(joined.slice(0, 2).to_string())
```

Supported operations:

- `b"..."` byte literals
- `bytes([1, 2, 255])`
- `left + right`
- `data[index]`
- `data.len() -> int`
- `data.get(index) -> int`
- `data.at(index) -> int`
- `data.slice(start, end) -> bytes`
- `data.push(value) -> bytes`
- `data.to_string() -> string`

## Dictionaries

Pyrite now has a runtime dictionary type for string-keyed lookup:

```pyrite
def main():
    table: dict = dict()
    table = table.set("name", "Ada")
    table = table.set("score", 42)
    print(table.has("name"))
    print(table.get_string("name"))
    print(table.get_int("score"))

    counts: dict[int] = dict()
    counts = counts.set("tokens", 3)
    print(counts.get("tokens"))
```

Supported dictionary operations:

- `dict()`
- `table.set(key, value) -> dict`
- `table.get(key) -> any`, or the annotated value type for `dict[T]`
- `table.get_string(key) -> string`
- `table.get_int(key) -> int`
- `table.has(key) -> bool`
- `table.remove(key) -> dict`
- `table.len() -> int`

Dictionary values can be typed with `dict[T]`, including class names:

```pyrite
tokens: dict[Token] = dict()
tokens = tokens.set("first", Token(0, "name"))
first: Token = tokens.get("first")
```

Dictionary updates are also persistent-style:

```pyrite
table = table.set("kind", "token")
table = table.remove("kind")
```

## Variable Inheritance

Object-shaped values still exist separately from `dict`. They are used by the
`scoring` example for inherited defaults and dotted field access:

```pyrite
defaults = {"kind": "student", "active": true}
user = defaults
user.name = "Ada"
user.score = 42

print(user.kind)
print(user.name)
```

Lookup checks the child first, then walks to the inherited parent. Assignments
write to the child only.

Use `object` when annotating this older dotted-field shape:

```pyrite
user: object = defaults
```

## Sets

Sets store unique strings:

```pyrite
def main():
    seen: set = set()
    seen = seen.add("main")
    seen = seen.add("main")
    print(seen.len())
    print(seen.has("main"))
```

Supported set operations:

- `set()`
- `seen.add(value) -> set`
- `seen.remove(value) -> set`
- `seen.has(value) -> bool`
- `seen.len() -> int`

## Builders

Builders make repeated output assembly less noisy than chained `+` expressions.
They are persistent-style like lists and dictionaries.

```pyrite
def main():
    text: string_builder = string_builder()
    text = text.write("hello")
    text = text.write(" world")
    print(text.string())

    data: bytes_builder = bytes_builder()
    data = data.push(65)
    data = data.write(b"R")
    print(data.bytes().to_string())
```

String builder operations:

- `string_builder()`
- `builder.write(value: string) -> string_builder`
- `builder.string() -> string`
- `builder.to_string() -> string`
- `builder.len() -> int`

Bytes builder operations:

- `bytes_builder()`
- `builder.write(value: bytes) -> bytes_builder`
- `builder.push(value: int) -> bytes_builder`
- `builder.bytes() -> bytes`
- `builder.to_bytes() -> bytes`
- `builder.len() -> int`
