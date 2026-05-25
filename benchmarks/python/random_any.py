import random


class RandomAnyBench:
    def __init__(self, limit: int):
        self.limit = limit

    def run(self):
        random.seed(123)
        i = 0
        values = ["Ada", 42, 3.5, True, "Pyrite"]
        picked = "none"

        while i < self.limit:
            picked = random.choice(values)
            i = i + 1

        return i


def main():
    bench = RandomAnyBench(200_000)
    print(bench.run())


if __name__ == "__main__":
    main()
