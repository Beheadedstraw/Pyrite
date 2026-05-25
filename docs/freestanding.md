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

The first freestanding slice supports basic Pyrite code: integers, booleans,
strings, lists, helper functions, `if`, `while`, `foreach`, `print`, and the
`kernel` module.

Freestanding strings support `left + right`, `chr(code)`, `value.len()`, and
`value.slice(start, end)`, which is enough to build small input buffers in
Pyrite code.

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
- `kernel.panic(message)`
- `kernel.halt()`
- `kernel.write_port(port, value)`
- `kernel.read_port(port)`
- `kernel.outb(port, value)`
- `kernel.inb(port)`

The built-in `print(...)` also works in freestanding mode and writes through
`pyrite_kernel_putchar`.
