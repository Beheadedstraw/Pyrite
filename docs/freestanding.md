# Freestanding Kernel Target

Pyrite can emit a freestanding object file for kernel experiments:

```sh
build/pyritec --target freestanding examples/kernel_hello.pyr -o build/kernel_hello.o
```

`--freestanding` is a shorthand for `--target freestanding`.

The freestanding target emits:

- a `long kmain(void)` entrypoint instead of hosted `int main(void)`
- a small C runtime without libc, POSIX, pthreads, sockets, files, regex, or time
- a relocatable object file compiled with `-ffreestanding`, `-fno-stack-protector`,
  `-fno-pic`, `-mno-red-zone`, and `-c`
- weak hooks named `pyrite_kernel_putchar` and `pyrite_kernel_hang`

A real kernel should provide those hooks from C or assembly:

```c
void pyrite_kernel_putchar(int ch) {
    /* write ch to serial, VGA, framebuffer console, etc. */
}

__attribute__((noreturn)) void pyrite_kernel_hang(void) {
    for (;;) {
        __asm__ volatile("hlt");
    }
}
```

The freestanding slice supports basic Pyrite code: integers, booleans, strings,
bytes, lists, dictionaries, sets, string/bytes builders, helper functions, `if`,
`while`, `foreach`, `print`, and the `kernel` module.

Freestanding strings support `left + right`, `chr(code)`, `value.len()`, and
`value.slice(start, end)`, which is enough to build small input buffers in
Pyrite code.

Freestanding bytes support `b"..."`, `bytes([1, 2, 255])`, `left + right`,
indexing, `.len()`, `.get()`, `.slice()`, `.push()`, and `.to_string()`.

Freestanding collections use the same persistent-style update API as hosted
Pyrite:

```pyrite
items = items.push(3)
table = table.set("name", "Ada")
seen = seen.add("main")
builder = builder.write("chunk")
```

Avoid these hosted modules in kernel code for now: `file`, `net`, `http`,
`regex`, `routines`, `random`, and `time`.

## Kernel Module

`stdlib/kernel.pyr` keeps names Python-like: lower-case, snake_case, and small
verbs.

Current bindings:

- `kernel.print(value)`
- `kernel.println(value)`
- `kernel.cls()`
- `kernel.input(prompt)`
- `kernel.color(value)`
- `kernel.panic(message)`
- `kernel.halt()`
- `kernel.write_port(port, value)`
- `kernel.read_port(port)`
- `kernel.outb(port, value)`
- `kernel.inb(port)`
- `kernel.outw(port, value)`
- `kernel.inw(port)`
- `kernel.read64(address)`
- `kernel.write64(address, value)`
- `kernel.read8(address)`
- `kernel.write8(address, value)`
- `kernel.read_cr3()`
- `kernel.write_cr3(value)`
- `kernel.flush_page(address)`
- `kernel.shr(value, bits)`
- `kernel.shl(value, bits)`
- `kernel.ptr(value)`
- `kernel.string_at(address)`
- `kernel.string_byte(value, index)`
- `kernel.setup_user_mode(kernel_cr3)`
- `kernel.enter_user(space, entry, stack)`

The built-in `print(...)` also works in freestanding mode and writes through
`pyrite_kernel_putchar`.

## Artemis Program Target

Pyrite also has an Artemis user-program image target:

```sh
build/pyritec --target artemis programs/hello.pyr -o build/programs/hello.apx
```

This target emits an `ARTNAT1` native image for Artemis OS. The current user
program subset supports a simple `return N` from `main`; `print("...")` is
rejected until Artemis exposes user-space stdout syscalls.
