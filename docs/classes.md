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
- methods declared with `def method(self, ...)`
- constructor-style calls through `Name(...)`
- `__init__(self, ...)` for initialization
- field assignment through `self.field = value` and `object.field = value`
- field reads through `object.field`
- method calls through `object.method(...)`

Current field values can be `int`, `float`, `bool`, `string`, and `any`.
Method parameters other than `self` should be type annotated for now.

Planned:

- class annotations like `player: Player`
- inheritance between classes
- class fields declared outside methods
- private fields and methods
- richer object cleanup for long-running programs
