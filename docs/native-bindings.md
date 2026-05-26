# Native Bindings

Pyrite modules can expose runtime-backed functions with `native def`. This is
the bridge for features that need operating-system calls or low-level runtime
support before they can be written entirely in Pyrite.

```pyrite
native def tcp(host: string, port: int) -> socket = pyrite_socket_open_tcp
native def read(sock: socket, max_bytes: int) -> string = pyrite_socket_read
```

The left side is the Pyrite function signature. The right side is the C symbol
provided by the runtime. When user code imports the module, calls such as
`net.tcp("127.0.0.1", 8080)` type-check against the Pyrite signature and compile
to the bound runtime call.

Native functions currently require typed parameters and an explicit return type.
Supported return types are the normal Pyrite runtime types, including `int`,
`float`, `string`, `bytes`, `bool`, `any`, `socket`, `file`, `mux`,
`list[int]`, `list[any]`, `list[T]`, `dict`, `dict[T]`, `set`,
`string_builder`, `bytes_builder`, `object`, and `listener`.

Current standard modules using this path:

- `stdlib/file.pyr`
- `stdlib/net.pyr`
- `stdlib/random.pyr`
- `stdlib/regex.pyr`
- `stdlib/routines.pyr`
- `stdlib/time.pyr`
