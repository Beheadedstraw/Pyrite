#!/usr/bin/env python3
import random
import re
import sys


app_name = "Ada"
max_bonus = 6


class InheritedUser:
    def __init__(self, defaults):
        self._defaults = defaults
        self._own = {}

    def __getattr__(self, name):
        if name in self._own:
            return self._own[name]
        if name in self._defaults:
            return self._defaults[name]
        raise AttributeError(name)

    def __setattr__(self, name, value):
        if name in {"_defaults", "_own"}:
            object.__setattr__(self, name, value)
            return
        self._own[name] = value


def scoring_add(a, b):
    return a + b


def main():
    name = app_name
    scores = [10, 32]
    bonus = random.randint(1, max_bonus)
    defaults = {"kind": "student", "active": True}
    user = InheritedUser(defaults)
    user.name = name
    user.score = scoring_add(scores[0], scores[1])
    message = f"{user.name} scored {user.score}"

    print(message)
    print(f"scores = {scores}")
    print(f"random bonus = {bonus}")
    print(f"inherited kind = {user.kind}")
    for score in scores:
        print(f"score item = {score}")

    if user.score >= 40:
        print("score status = passing")
    else:
        print("score status = retry")

    with open("score.txt", "w", encoding="utf-8") as out:
        out.write(f"{message}\n")
        out.flush()

    with open("score.txt", "r", encoding="utf-8") as in_file:
        saved = in_file.read()

    print(f"saved: {saved.rstrip()}")
    matched = re.search("Ada", saved) is not None
    print(f"regex matched Ada: {str(matched).lower()}")

    return user.score


if __name__ == "__main__":
    sys.exit(main())
