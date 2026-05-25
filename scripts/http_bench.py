#!/usr/bin/env python3
import argparse
import socket
import subprocess
import sys
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def wait_for_port(host, port, timeout=5.0):
    deadline = time.perf_counter() + timeout
    last_error = None
    while time.perf_counter() < deadline:
        try:
            with socket.create_connection((host, port), timeout=0.1):
                return
        except OSError as exc:
            last_error = exc
            time.sleep(0.02)
    raise RuntimeError(f"server did not start on {host}:{port}: {last_error}")


def request_once(host, port, path):
    payload = (
        f"GET {path} HTTP/1.1\r\n"
        f"Host: {host}:{port}\r\n"
        "Connection: close\r\n"
        "\r\n"
    ).encode("ascii")
    with socket.create_connection((host, port), timeout=2.0) as sock:
        sock.sendall(payload)
        chunks = []
        while True:
            chunk = sock.recv(65536)
            if not chunk:
                break
            chunks.append(chunk)
    return b"".join(chunks)


def run_server(cmd, host, port, requests):
    proc = subprocess.Popen(cmd, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        wait_for_port(host, port)
        start = time.perf_counter()
        ok = 0
        for i in range(requests):
            path = "/api/ping" if i % 3 == 0 else "/api/score" if i % 3 == 1 else "/api/text"
            response = request_once(host, port, path)
            if response.startswith(b"HTTP/1.1 200 OK"):
                ok += 1
        elapsed = time.perf_counter() - start
        return elapsed, ok
    finally:
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=2.0)
            except subprocess.TimeoutExpired:
                proc.kill()


def main():
    parser = argparse.ArgumentParser(description="Benchmark the fixed-request Pyrite HTTP API server.")
    parser.add_argument("--requests", type=int, default=200)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--pyrite-port", type=int, default=8091)
    parser.add_argument("--python-port", type=int, default=8092)
    parser.add_argument("--python", default=sys.executable)
    args = parser.parse_args()

    pyritec = ROOT / "build" / "pyritec"
    pyrite_server = ROOT / "build" / "benchmarks" / "http_api_server"
    pyrite_source = ROOT / "benchmarks" / "pyrite" / "http_api_server.pyr"
    python_source = ROOT / "benchmarks" / "python" / "http_api_server.py"

    subprocess.run(["go", "build", "-o", str(pyritec), "./cmd/pyritec"], cwd=ROOT, check=True)
    subprocess.run([str(pyritec), str(pyrite_source), "-o", str(pyrite_server)], cwd=ROOT, check=True)

    pyrite_elapsed, pyrite_ok = run_server([str(pyrite_server)], args.host, args.pyrite_port, args.requests)
    python_elapsed, python_ok = run_server([args.python, str(python_source)], args.host, args.python_port, args.requests)

    print(f"{'server':<10} {'requests':>8} {'ok':>8} {'seconds':>10} {'req/s':>10}")
    print(f"{'pyrite':<10} {args.requests:>8} {pyrite_ok:>8} {pyrite_elapsed:>10.4f} {args.requests / pyrite_elapsed:>10.1f}")
    print(f"{'python':<10} {args.requests:>8} {python_ok:>8} {python_elapsed:>10.4f} {args.requests / python_elapsed:>10.1f}")


if __name__ == "__main__":
    main()
