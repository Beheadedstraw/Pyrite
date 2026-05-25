from pathlib import Path


class FileIOBench:
    def __init__(self, path: str, limit: int):
        self.path = Path(path)
        self.limit = limit

    def run(self):
        i = 0

        with self.path.open("w") as out:
            while i < self.limit:
                out.write("Ada scored 42\n")
                i = i + 1

        data = self.path.read_text()
        return len(data)


def main():
    bench = FileIOBench("build/bench_io.txt", 20_000)
    print(bench.run())


if __name__ == "__main__":
    main()
