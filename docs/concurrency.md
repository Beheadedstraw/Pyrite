# Concurrency And Routines

Pyrite uses `routine(...)` for lightweight concurrent work. Import the
`routines` module before starting routines:

```pyrite
import routines

def main():
    routines.workers(8)
    print_lock: mux = mux()
    routine(print("background work"), print_lock)
    routine(testr(5), print_lock)
    async(testr(10))
    print("main work")
    return 0

def testr(value: int):
    print(f"value={value}")
    return 1
```

## Routines

- `routine(print(value))` starts a concurrent print task.
- `routine(helper(args))` starts a concurrent helper function call.
- `async(helper(args))` is the same fire-and-forget scheduling model without
  the optional mux argument. It is the preferred spelling for independent work.
- `routine(print(value), lock)` locks `lock` while printing.
- `routine(helper(args), lock)` locks `lock` while the helper runs.
- `routines.workers(count)` sets the runtime worker pool size before the first
  routine or async task starts. If it is not called, Pyrite picks a default from
  the CPU count. The `PYRITE_WORKERS` environment variable can also set the
  default worker count for a compiled program.
- Routines are automatically joined before normal `main` return cleanup.
- Routine helper arguments can include runtime handles such as `socket`; the
  helper should own and close the handle it receives.
- Output order between routines and main code is intentionally nondeterministic.

The current C backend schedules routines on a runtime worker pool implemented
with native threads. The language-level model is Go-routine shaped: start work
with `async(...)` or `routine(...)` and let the runtime manage execution.

## Mux Locks

`mux()` creates a lock:

```pyrite
lock: mux = mux()
```

Manual locking:

```pyrite
routines.lock(lock)
print("critical section")
routines.unlock(lock)
```

`routines.lock` and `routines.unlock` are declared in `stdlib/routines.pyr`
with native runtime bindings.

If you pass a mux into `routine(...)`, the routine locks around that operation.
Do not also lock the same mux around the call unless you intentionally want a
larger critical section; nested locking can deadlock because muxes are not
reentrant.
