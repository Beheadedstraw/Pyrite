# Network Sockets

Pyrite exposes TCP/UDP client sockets and first-version TCP server listeners
through the `net` module.

```pyrite
import net

def main():
    tcp: socket = net.tcp("127.0.0.1", 8080).defer()
    net.write(tcp, "GET / HTTP/1.0\r\n\r\n")
    tcp_reply = net.read(tcp, 1024)
    print(f"tcp reply = {tcp_reply}")

    udp: socket = net.udp("127.0.0.1", 9000).defer()
    net.write(udp, "ping")
    udp_reply = net.read(udp, 1024)
    print(f"udp reply = {udp_reply}")

    return 0
```

## Socket Functions

- `net.tcp(host, port) -> socket`
  Opens a TCP client socket and connects to `host:port`.

- `net.udp(host, port) -> socket`
  Opens a UDP socket and connects it to `host:port`, so `net.write` and
  `net.read` use that peer by default.

- `net.tcp(host, port).defer() -> socket`
  Opens a TCP client socket and closes it before the current function returns.
  If the open fails inside `try`, the error is caught by `except`.

- `net.udp(host, port).defer() -> socket`
  Opens a UDP socket and closes it before the current function returns.
  If the open fails inside `try`, the error is caught by `except`.

- `net.write(socket, data) -> int`
  Sends a string and returns the byte count. Use an f-string when formatting
  numbers or other values into the payload.

- `net.read(socket, max_bytes) -> string`
  Reads up to `max_bytes` bytes and returns a string.

- `net.close(socket) -> int`
  Closes a socket immediately. This is useful in server loops where deferring
  each accepted client would keep descriptors open until the function returns.

- `net.set_nonblocking(socket, enabled) -> int`
  Enables or disables nonblocking mode for a connected socket.

- `net.read_ready(socket, timeout_ms) -> bool`
  Waits until a socket is readable or the timeout expires.

## TCP Server Functions

```pyrite
import net

def main():
    server: listener = net.listen("127.0.0.1", 8080).defer()
    client: socket = net.accept(server).defer()

    request = net.read(client, 1024)
    print(f"request = {request}")
    net.write(client, "HTTP/1.0 200 OK\r\n\r\nhello from Pyrite\n")

    return 0
```

- `net.listen(host, port) -> listener`
  Opens a TCP listener with the default backlog.

- `net.listen_backlog(host, port, backlog) -> listener`
  Opens a TCP listener with an explicit backlog.

- `net.listen(host, port).defer() -> listener`
  Opens a TCP listener and closes it before the current function returns.
  If the listen fails inside `try`, the error is caught by `except`.

- `net.accept(listener) -> socket`
  Blocks until a client connects, then returns a connected socket.
  If the accept fails inside `try`, the error is caught by `except`.

- `net.set_listener_nonblocking(listener, enabled) -> int`
  Enables or disables nonblocking mode for a listener.

- `net.accept_ready(listener, timeout_ms) -> bool`
  Waits until a listener has a pending connection or the timeout expires.

- `net.serve_delimited(host, port, delimiter, handler, should_close, max_messages) -> int`
  Runs a generic evented TCP request/response loop. The runtime reads from
  sockets until `delimiter`, calls `handler(frame) -> string`, writes the
  returned response, and calls `should_close(frame) -> bool` to decide whether
  to close that client. This is deliberately a generic `net` primitive; HTTP
  behavior such as request parsing and headers stays in `stdlib/http.pyr`.

```pyrite
import net

def echo(frame: string):
    return frame

def close_after(frame: string):
    return True

def main():
    return net.serve_delimited("127.0.0.1", 9000, "\n", echo, close_after, 1000)
```

- `net.close_listener(listener) -> int`
  Closes a listener immediately.

- `net.last_error() -> string`
  Returns the most recent runtime error message for checked network operations.

The public `net` API is declared in `stdlib/net.pyr` with native bindings into
the runtime socket helpers.

UDP server binding is not implemented yet.

## HTTP Helpers

The `http` module is written in Pyrite and builds on `net`.

```pyrite
import http
import routines

def handle_client(client: socket):
    request = http.read_request(client)
    if http.get(request, "/"):
        reply = http.text("hello from Pyrite\n")
    else:
        reply = http.not_found()
    http.respond(client, reply)
    return 0

def main():
    routines.workers(16)
    server: listener = http.listen("127.0.0.1", 8081).defer()
    client: socket = http.accept(server)
    async(handle_client(client))
    return 0
```

- `http.response(status, content_type, body) -> string`
  Builds a basic HTTP/1.1 response with `Content-Length` and `Connection: close`.

- `http.response_with_connection(status, content_type, body, connection) -> string`
  Builds a response with an explicit `Connection` header, such as `keep-alive`.

- `http.ok_text(body) -> string`
  Builds a `200 OK` text response.

- `http.ok_json(body) -> string`
  Builds a `200 OK` JSON response.

- `http.json(body) -> string`
  Alias for a `200 OK` JSON response.

- `http.text(body) -> string`
  Alias for a `200 OK` text response.

- `http.created_json(body) -> string`
  Builds a `201 Created` JSON response.

- `http.accepted_json(body) -> string`
  Builds a `202 Accepted` JSON response.

- `http.no_content() -> string`
  Builds a `204 No Content` response.

- `http.not_found() -> string`
  Builds a JSON `404 Not Found` response.

- `http.bad_request() -> string`
  Builds a JSON `400 Bad Request` response.

- `http.method_not_allowed() -> string`
  Builds a JSON `405 Method Not Allowed` response.

- `http.server_error() -> string`
  Builds a JSON `500 Internal Server Error` response.

- `http.listen(host, port) -> listener`
  Opens an HTTP TCP listener with the default backlog.

- `http.listen_backlog(host, port, backlog) -> listener`
  Opens an HTTP TCP listener with an explicit backlog.

- `http.accept(listener) -> socket`
  Accepts one connected client from an HTTP listener.

- `http.close(socket) -> int`
  Closes a connected client socket.

- `http.close_listener(listener) -> int`
  Closes an HTTP listener immediately.

- `http.request_method(request) -> string`
  Returns the method from the request line, such as `GET`.

- `http.request_path(request) -> string`
  Returns the raw target from the request line, such as `/api/ping?x=1`.

- `http.path(request) -> string`
  Returns the target path without the query string.

- `http.query(request) -> string`
  Returns the query string without `?`, or an empty string.

- `http.request_target(request) -> string`
  Alias for `http.request_path`.

- `http.request_version(request) -> string`
  Returns the HTTP version from the request line, such as `HTTP/1.1`.

- `http.read_request(socket) -> string`
  Reads a basic request from a connected client socket.

- `http.write_response(socket, response) -> int`
  Writes a response string to a connected client socket.

- `http.respond(socket, response) -> int`
  Writes a response and closes the connected client socket.

- `http.should_close(request) -> bool`
  Returns true when a request is empty or asks for `Connection: close`.

- `http.response_connection(request) -> string`
  Returns `close` or `keep-alive` for response construction.

- `http.is_preflight(request) -> bool`
  Returns true for an `OPTIONS` CORS preflight request.

- `http.cors_allowed_origin(request, allowed_origins) -> string`
  Returns the matching origin from a `list[any]` of allowed origins, or an
  empty string when the request origin is not allowed.

- `http.cors_request_allowed(request, allowed_origins) -> bool`
  Checks origin, requested method, and requested headers for a CORS preflight.

- `http.cors_preflight(request, allowed_origins) -> string`
  Builds a `204 No Content` CORS response for allowed preflights, or a `403`
  response when the origin/method/headers do not match.

- `http.route(request, method, path) -> bool`
  Returns true when the request method and path exactly match.

- `http.match(method, path, route_method, route_path) -> bool`
  Returns true when already-parsed request method/path values exactly match.

- `http.match_get(path, route_path) -> bool`
  Returns true when an already-parsed request path exactly matches a GET route
  path. Use this when the surrounding router already checked or assumes `GET`.

- `http.get(request, path) -> bool`
  Returns true for an exact `GET` route match.

- `http.post(request, path) -> bool`
  Returns true for an exact `POST` route match.

- `http.put(request, path) -> bool`
  Returns true for an exact `PUT` route match.

- `http.delete(request, path) -> bool`
  Returns true for an exact `DELETE` route match.

Apps own their routing. For example, [examples/http_api_server.pyr](../examples/http_api_server.pyr)
defines `/api/ping`, `/api/score`, and `/api/text` for benchmark testing using
the generic helpers above.
