class StringStressBench:
    def __init__(self, limit: int):
        self.limit = limit
        self.raw = "  Ada Lovelace wrote notes for the analytical engine  "
        self.old = "ADA"
        self.new = "AUGUSTA"

    def run(self):
        i = 0
        hits = 0

        while i < self.limit:
            cleaned = self.raw.strip().upper().replace(self.old, self.new)
            if self.new in cleaned:
                hits = hits + 1
            i = i + 1

        return hits


def main():
    bench = StringStressBench(5_000_000)
    print(bench.run())


if __name__ == "__main__":
    main()
