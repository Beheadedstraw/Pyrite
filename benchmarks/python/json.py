import importlib
import sys
from pathlib import Path

sys.path = [p for p in sys.path if Path(p or ".").resolve() != Path(__file__).resolve().parent]
json = importlib.import_module("json")


class JsonBench:
    def __init__(self, limit: int):
        self.limit = limit
        self.payload = '{"name":"Ada","score":42,"active":true}'

    def run(self):
        i = 0
        total = 0
        values = ["Ada", 42, 3.5, True]

        while i < self.limit:
            encoded = json.dumps(values, separators=(",", ":"))
            decoded = json.loads(encoded)
            data = json.loads(self.payload)
            name = data["name"]
            score = data["score"]
            active = data["active"]

            if active:
                total = total + score

            i = i + 1

        return total


def main():
    bench = JsonBench(100_000)
    print(bench.run())


if __name__ == "__main__":
    main()
