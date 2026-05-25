# Files

Files are opaque `file` handles. Use `.defer()` to close them automatically
before the current function exits.

```pyrite
def main():
    out: file = file.open("score.txt", "w").defer()
    file.write(out, "Ada scored 42\n")
    file.flush(out)

    in_file = file.open("score.txt", "r").defer()
    saved = file.read_all(in_file)
    print(f"saved: {saved}")
    return 0
```

## File Functions

- `file.open(path, mode) -> file`
  Opens a file. `mode` follows C/POSIX style: `"r"`, `"w"`, `"a"`, `"r+"`,
  `"w+"`, or `"a+"`.

- `file.open(path, mode).defer() -> file`
  Opens a file and registers it to close before the current function returns.
  If the open fails inside `try`, the error is caught by `except`.

- `file.close(file) -> int`
  Planned manual close operation.

- `file.read_all(file) -> string`
  Reads from the current file position to end of file.

- `file.write(file, value)`
  Writes a string and returns the byte count. Use an f-string to format
  numbers or other values before writing.

- `file.flush(file)`
  Flushes buffered writes and returns the runtime status code.

These functions are declared in `stdlib/file.pyr` with native runtime bindings.

- `file.last_error() -> string`
  Returns the most recent runtime error message for checked file operations.

## Defer Tracing

Compile with:

```sh
pyritec --trace-defer input.pyr -o output
```

Generated programs print cleanup events to stderr when deferred handles close
and when tracked heap allocations are released. Normal builds keep cleanup
silent.
