# Classes

Pyrite supports a first-version class model with Python-shaped syntax:

```pyrite
class Player:
    def __init__(self, name: string, score: int):
        self.name = name
        self.score = score

    def add(self, amount: int):
        self.score = self.score + amount
        return self.score

    def label(self):
        return f"{self.name}: {self.score}"

def main():
    player = Player("Ada", 40)
    print(player.name)
    print(player.label())
    return player.add(2)
```

Current support:

- `class Name:`
- classes declared in imported modules
- methods declared with `def method(self, ...)`
- constructor-style calls through `Name(...)`
- `__init__(self, ...)` for initialization
- field assignment through `self.field = value` and `object.field = value`
- field reads through `object.field`
- method calls through `object.method(...)`
- nullable class fields initialized with `None`
- typed class variables and lists, such as `token: Token` and `list[Token]`
- indexed member reads such as `tokens[0].text`

Current field values can be `int`, `float`, `bool`, `string`, `bytes`, `any`,
class objects, and typed lists. Method parameters other than `self` should be
type annotated for now.

Compiler-shaped class containers are supported:

```pyrite
class Token:
    def __init__(self, kind: int, text: string):
        self.kind = kind
        self.text = text

class TokenStream:
    def __init__(self, tokens: list[Token]):
        self.tokens = tokens
        self.current = None

    def first(self):
        self.current = self.tokens[0]
        return self.current
```

Planned:

- inheritance between classes
- class fields declared outside methods
- private fields and methods
- richer object cleanup for long-running programs
