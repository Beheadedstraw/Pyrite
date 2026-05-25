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

Current compiler-backed support includes integer lists and mixed `list[any]`
lists:

- `items = [a, b, c]`
- `values: list[any] = ["Ada", 42, 3.5, True]`
- `items[index]`
- `for item in items:`
- `foreach(items):`
- `foreach(items, name):`
- `foreach([1, 2, 3]):`
- printing and f-string formatting for `list[int]` and `list[any]`

`list[any]` currently stores ints, floats, bools, and strings.

Planned list operations:

- `len(items) -> int`
- `items[index] = value`
- `items.append(value) -> int`
- `items.pop() -> any`
- `items.insert(index, value) -> int`
- `items.remove_at(index) -> any`
- `items.clear() -> int`

## Dictionaries

Dictionaries use Python-like literal and indexing syntax:

```pyrite
def main():
    user = {"name": "Ada", "score": 42}
    user.score = user.score + 1
    return user.score
```

Current compiler-backed object support is intentionally narrow and is used by
the `scoring` example. The language contract keeps dictionary syntax open for a
full implementation.

## Variable Inheritance

Dictionary-shaped values inherit by default when assigned into a new variable:

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

Planned dictionary helpers:

- `dict.parent(child) -> dict`
- `dict.has_own(child, key) -> bool`
- `dict.copy(parent) -> dict`
