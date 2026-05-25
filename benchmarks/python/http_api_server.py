import socket


def response(status, content_type, body):
    body_bytes = body.encode("utf-8")
    header = (
        f"HTTP/1.1 {status}\r\n"
        f"Content-Type: {content_type}\r\n"
        f"Content-Length: {len(body_bytes)}\r\n"
        "Connection: close\r\n"
        "\r\n"
    )
    return header.encode("utf-8") + body_bytes


def route_api(request):
    if request.startswith("GET /api/ping"):
        return response("200 OK", "application/json", '{"ok":true,"service":"python"}')
    if request.startswith("GET /api/score"):
        return response("200 OK", "application/json", '{"name":"Ada","score":42}')
    if request.startswith("GET /api/text"):
        return response("200 OK", "text/plain", "hello from Python\n")
    return response("404 Not Found", "application/json", '{"error":"not found"}')


def serve_fixed(host, port, requests):
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as server:
        server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        server.bind((host, port))
        server.listen(128)
        handled = 0
        while handled < requests:
            client, _ = server.accept()
            with client:
                request = client.recv(4096).decode("utf-8", errors="replace")
                client.sendall(route_api(request))
            handled += 1
    return handled


if __name__ == "__main__":
    serve_fixed("127.0.0.1", 8092, 100000)
