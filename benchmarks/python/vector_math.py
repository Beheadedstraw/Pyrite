def add2(a, b):
    return [a[0] + b[0], a[1] + b[1]]


def sub2(a, b):
    return [a[0] - b[0], a[1] - b[1]]


def scale2(v, scalar):
    return [v[0] * scalar, v[1] * scalar]


def dot2(a, b):
    return a[0] * b[0] + a[1] * b[1]


def length_sq2(v):
    return dot2(v, v)


def add3(a, b):
    return [a[0] + b[0], a[1] + b[1], a[2] + b[2]]


def sub3(a, b):
    return [a[0] - b[0], a[1] - b[1], a[2] - b[2]]


def scale3(v, scalar):
    return [v[0] * scalar, v[1] * scalar, v[2] * scalar]


def dot3(a, b):
    return a[0] * b[0] + a[1] * b[1] + a[2] * b[2]


def cross3(a, b):
    return [
        a[1] * b[2] - a[2] * b[1],
        a[2] * b[0] - a[0] * b[2],
        a[0] * b[1] - a[1] * b[0],
    ]


def length_sq3(v):
    return dot3(v, v)


class VectorMathBench:
    def __init__(self, limit: int):
        self.limit = limit

    def run(self):
        i = 0
        total = 0
        a = [3, 4]
        b = [10, 2]
        c = [1, 2, 3]
        d = [4, 5, 6]

        while i < self.limit:
            sum2 = add2(a, b)
            diff2 = sub2(a, b)
            scaled2 = scale2(a, 3)
            dot_2 = dot2(a, b)
            len2 = length_sq2(a)

            sum3 = add3(c, d)
            diff3 = sub3(c, d)
            scaled3 = scale3(c, 2)
            dot_3 = dot3(c, d)
            cross = cross3(c, d)
            len3 = length_sq3(c)

            total = total + dot_2 + dot_3 + len2 + len3
            total = total + sum2[0] + diff2[1] + scaled2[0]
            total = total + sum3[2] + diff3[0] + scaled3[1] + cross[1]
            i = i + 1

        return total


def main():
    bench = VectorMathBench(200_000)
    print(bench.run())


if __name__ == "__main__":
    main()
