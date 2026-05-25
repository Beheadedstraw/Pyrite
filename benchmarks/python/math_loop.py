class MathLoopBench:
    def __init__(self, limit: int):
        self.limit = limit

    def run(self):
        i = 0
        total = 0

        while i < self.limit:
            total = total + 1
            i = i + 1

        return total


def main():
    bench = MathLoopBench(2_000_000)
    print(bench.run())


if __name__ == "__main__":
    main()
