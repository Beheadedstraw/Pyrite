# Vector Math

Pyrite includes a pure `.pyr` vector math module:

```pyrite
import vectors

def main():
    a = [3, 4]
    b = [10, 2]

    print(vectors.add2(a, b))
    print(vectors.dot2(a, b))
    print(vectors.length_sq2(a))
    return 0
```

Vectors are represented as `list[int]` values for now. The module is pure
Pyrite and does not use native bindings.

Current APIs:

- `vectors.add2(a: list[int], b: list[int]) -> list[int]`
- `vectors.sub2(a: list[int], b: list[int]) -> list[int]`
- `vectors.scale2(v: list[int], scalar: int) -> list[int]`
- `vectors.dot2(a: list[int], b: list[int]) -> int`
- `vectors.length_sq2(v: list[int]) -> int`
- `vectors.add3(a: list[int], b: list[int]) -> list[int]`
- `vectors.sub3(a: list[int], b: list[int]) -> list[int]`
- `vectors.scale3(v: list[int], scalar: int) -> list[int]`
- `vectors.dot3(a: list[int], b: list[int]) -> int`
- `vectors.cross3(a: list[int], b: list[int]) -> list[int]`
- `vectors.length_sq3(v: list[int]) -> int`

The compiler now supports numeric `+`, `-`, `*`, and `/` expressions. Division
returns a `float`.

Planned:

- `list[float]`
- dynamic vector lengths
- appendable lists
- normalized vectors and square-root length helpers
