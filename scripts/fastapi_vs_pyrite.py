#!/usr/bin/env python3
import argparse
import concurrent.futures
import collections
import os
import shutil
import socket
import subprocess
import sys
import time
import venv
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
VENV = ROOT / "build" / "fastapi-bench-venv"
TARGET = ROOT / "build" / "fastapi-bench-pkgs"

PATHS = [
    "/api/ping",
    "/api/score",
    "/api/text",
    "/api/user",
    "/api/items",
    "/api/compute",
]

PROFILES = {
    "micro": PATHS,
    "realworld": [
        "OPTIONS /api/auth",
        "OPTIONS /api/search?q=ada",
        "/api/auth",
        "/api/search?q=ada",
        "/api/report",
        "/api/profile",
        "/api/transform",
        "/api/dashboard",
    ],
    "mixed": PATHS + [
        "/api/auth",
        "/api/search?q=ada",
        "/api/report",
        "/api/profile",
        "/api/transform",
        "/api/dashboard",
    ],
}


def venv_python():
    if os.name == "nt":
        return VENV / "Scripts" / "python.exe"
    return VENV / "bin" / "python"


def ensure_fastapi_env():
    system_check = subprocess.run(
        [sys.executable, "-c", "import fastapi, uvicorn, httptools, uvloop"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    if system_check.returncode == 0:
        return sys.executable, os.environ.copy()

    python = venv_python()
    have_ensurepip = subprocess.run(
        [sys.executable, "-c", "import ensurepip"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    ).returncode == 0

    if have_ensurepip and not python.exists():
        VENV.parent.mkdir(parents=True, exist_ok=True)
        try:
            venv.EnvBuilder(with_pip=True).create(VENV)
        except Exception:
            if VENV.exists():
                shutil.rmtree(VENV, ignore_errors=True)

    if have_ensurepip and python.exists():
        check = subprocess.run(
            [str(python), "-c", "import fastapi, uvicorn, httptools, uvloop"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        if check.returncode != 0:
            pip_check = subprocess.run(
                [str(python), "-m", "pip", "--version"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            if pip_check.returncode != 0:
                shutil.rmtree(VENV, ignore_errors=True)
            else:
                subprocess.run(
                    [str(python), "-m", "pip", "install", "--upgrade", "pip", "fastapi", "uvicorn[standard]"],
                    cwd=ROOT,
                    check=True,
                )
                return str(python), os.environ.copy()
        else:
            return str(python), os.environ.copy()

    if subprocess.run([sys.executable, "-m", "pip", "--version"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
        TARGET.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            [sys.executable, "-m", "pip", "install", "--target", str(TARGET), "fastapi", "uvicorn[standard]"],
            cwd=ROOT,
            check=True,
        )
        env = os.environ.copy()
        env["PYTHONPATH"] = str(TARGET) + os.pathsep + env.get("PYTHONPATH", "")
        return sys.executable, env

    pipx = shutil.which("pip3")
    if pipx:
        TARGET.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            [pipx, "install", "--target", str(TARGET), "fastapi", "uvicorn[standard]"],
            cwd=ROOT,
            check=True,
        )
        env = os.environ.copy()
        env["PYTHONPATH"] = str(TARGET) + os.pathsep + env.get("PYTHONPATH", "")
        return sys.executable, env

    raise RuntimeError(
        "FastAPI benchmark dependencies are unavailable. Install python3-venv or pip, "
        "then rerun: sudo apt install python3.13-venv  # or: sudo apt install python3-pip"
    )


def wait_for_http(host, port, timeout=10.0):
    deadline = time.perf_counter() + timeout
    last_error = None
    while time.perf_counter() < deadline:
        try:
            response = request_once(host, port, "/api/ping")
            if is_ok_response(response):
                return
        except OSError as exc:
            last_error = exc
        time.sleep(0.025)
    raise RuntimeError(f"server did not start on {host}:{port}: {last_error}")


def is_ok_response(response):
    return (
        response.startswith(b"HTTP/1.1 200")
        or response.startswith(b"HTTP/1.0 200")
        or response.startswith(b"HTTP/1.1 204")
        or response.startswith(b"HTTP/1.0 204")
    )


def request_target(path):
    if path.startswith("OPTIONS "):
        return "OPTIONS", path[len("OPTIONS "):]
    if path.startswith("GET "):
        return "GET", path[len("GET "):]
    return "GET", path


def request_once(host, port, path):
    method, target = request_target(path)
    cors_headers = ""
    if method == "OPTIONS":
        cors_headers = (
            "Origin: https://app.example\r\n"
            "Access-Control-Request-Method: GET\r\n"
            "Access-Control-Request-Headers: authorization,content-type\r\n"
        )
    payload = (
        f"{method} {target} HTTP/1.1\r\n"
        f"Host: {host}:{port}\r\n"
        f"{cors_headers}"
        "Connection: close\r\n"
        "\r\n"
    ).encode("ascii")

    with socket.create_connection((host, port), timeout=5.0) as sock:
        sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        sock.settimeout(5.0)
        sock.sendall(payload)
        chunks = []
        while True:
            chunk = sock.recv(65536)
            if not chunk:
                break
            chunks.append(chunk)
    return b"".join(chunks)

def read_http_response(sock, pending):
    while b"\r\n\r\n" not in pending:
        chunk = sock.recv(65536)
        if not chunk:
            raise ConnectionResetError("connection closed before response headers")
        pending += chunk
    header_end = pending.index(b"\r\n\r\n") + 4
    headers = pending[:header_end]
    content_length = 0
    for line in headers.split(b"\r\n"):
        if line.lower().startswith(b"content-length:"):
            content_length = int(line.split(b":", 1)[1].strip())
            break
    total = header_end + content_length
    while len(pending) < total:
        chunk = sock.recv(65536)
        if not chunk:
            raise ConnectionResetError("connection closed before response body")
        pending += chunk
    response = pending[:total]
    return response, pending[total:]


def keep_alive_worker(host, port, paths, concurrency, worker_id, count):
    worker_ok = 0
    worker_bytes = 0
    worker_errors = collections.Counter()
    worker_latency = 0.0
    pending = b""
    try:
        with socket.create_connection((host, port), timeout=5.0) as sock:
            sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
            sock.settimeout(5.0)
            for n in range(count):
                i = worker_id + n * concurrency
                method, target = request_target(paths[i % len(paths)])
                cors_headers = ""
                if method == "OPTIONS":
                    cors_headers = (
                        "Origin: https://app.example\r\n"
                        "Access-Control-Request-Method: GET\r\n"
                        "Access-Control-Request-Headers: authorization,content-type\r\n"
                    )
                req_start = time.perf_counter()
                payload = (
                    f"{method} {target} HTTP/1.1\r\n"
                    f"Host: {host}:{port}\r\n"
                    f"{cors_headers}"
                    "Connection: keep-alive\r\n"
                    "\r\n"
                ).encode("ascii")
                try:
                    sock.sendall(payload)
                    response, pending = read_http_response(sock, pending)
                except OSError as exc:
                    worker_errors[type(exc).__name__] += 1
                    worker_latency += time.perf_counter() - req_start
                    continue
                good = is_ok_response(response)
                worker_ok += 1 if good else 0
                worker_bytes += len(response)
                worker_latency += time.perf_counter() - req_start
                if not good:
                    worker_errors["non_200"] += 1
    except OSError as exc:
        worker_errors[type(exc).__name__] += count
    return worker_ok, worker_bytes, worker_errors, worker_latency


def run_client(host, port, requests, concurrency, paths, keep_alive):
    start = time.perf_counter()
    ok = 0
    bytes_read = 0
    errors = collections.Counter()
    total_latency = 0.0

    def one(i):
        req_start = time.perf_counter()
        try:
            response = request_once(host, port, paths[i % len(paths)])
        except OSError as exc:
            return 0, 0, type(exc).__name__, time.perf_counter() - req_start
        good = is_ok_response(response)
        return 1 if good else 0, len(response), "" if good else "non_200", time.perf_counter() - req_start

    def worker(worker_id, count):
        if keep_alive:
            return keep_alive_worker(host, port, paths, concurrency, worker_id, count)
        worker_ok = 0
        worker_bytes = 0
        worker_errors = collections.Counter()
        worker_latency = 0.0
        for n in range(count):
            good, size, error, latency = one(worker_id + n * concurrency)
            worker_ok += good
            worker_bytes += size
            worker_latency += latency
            if error:
                worker_errors[error] += 1
        return worker_ok, worker_bytes, worker_errors, worker_latency

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
        futures = []
        base = requests // concurrency
        extra = requests % concurrency
        for worker_id in range(concurrency):
            count = base + (1 if worker_id < extra else 0)
            if count == 0:
                continue
            futures.append(pool.submit(worker, worker_id, count))
        for future in concurrent.futures.as_completed(futures):
            worker_ok, worker_bytes, worker_errors, worker_latency = future.result()
            ok += worker_ok
            bytes_read += worker_bytes
            total_latency += worker_latency
            errors.update(worker_errors)

    elapsed = time.perf_counter() - start
    return elapsed, ok, bytes_read, errors, total_latency


def start_pyrite(port, workers):
    pyritec = ROOT / "build" / "pyritec"
    server = ROOT / "build" / "benchmarks" / "http_api_server"
    source = ROOT / "benchmarks" / "pyrite" / "http_api_server.pyr"
    subprocess.run(["go", "build", "-o", str(pyritec), "./cmd/pyritec"], cwd=ROOT, check=True)
    subprocess.run([str(pyritec), str(source), "-o", str(server)], cwd=ROOT, check=True)
    env = os.environ.copy()
    if workers > 0:
        env["PYRITE_WORKERS"] = str(workers)
    return subprocess.Popen([str(server)], cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)


def start_fastapi(port):
    python, env = ensure_fastapi_env()
    app = "benchmarks.fastapi.http_api_server:app"
    return subprocess.Popen(
        [
            str(python),
            "-m",
            "uvicorn",
            app,
            "--host",
            "127.0.0.1",
            "--port",
            str(port),
            "--loop",
            "uvloop",
            "--http",
            "httptools",
            "--log-level",
            "warning",
        ],
        cwd=ROOT,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


def stop(proc):
    if proc.poll() is not None:
        return
    proc.terminate()
    try:
        proc.wait(timeout=3.0)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=3.0)


def process_peak_mib(pid):
    status = Path("/proc") / str(pid) / "status"
    try:
        text = status.read_text(encoding="utf-8")
    except OSError:
        return 0.0
    for key in ("VmHWM:", "VmRSS:"):
        for line in text.splitlines():
            if line.startswith(key):
                parts = line.split()
                if len(parts) >= 2:
                    return int(parts[1]) / 1024
    return 0.0


def bench_server(name, start_fn, host, port, requests, concurrency, warmup, paths, keep_alive):
    proc = start_fn(port)
    elapsed = 0.0
    ok = 0
    bytes_read = 0
    errors = collections.Counter()
    peak_mib = 0.0
    total_latency = 0.0
    try:
        wait_for_http(host, port)
        if warmup > 0:
            run_client(host, port, warmup, min(concurrency, warmup), paths, keep_alive)
        elapsed, ok, bytes_read, errors, total_latency = run_client(host, port, requests, concurrency, paths, keep_alive)
        returncode = proc.poll()
        peak_mib = process_peak_mib(proc.pid)
    finally:
        stop(proc)
    return {
        "name": name,
        "requests": requests,
        "ok": ok,
        "seconds": elapsed,
        "rps": requests / elapsed if elapsed > 0 else 0.0,
        "wall_us": (elapsed / requests * 1000000) if requests > 0 else 0.0,
        "avg_ms": (total_latency / requests * 1000) if requests > 0 else 0.0,
        "bytes": bytes_read,
        "errors": sum(errors.values()),
        "first_error": errors.most_common(1)[0][0] if errors else "",
        "rss_mib": peak_mib,
        "exit": returncode,
    }


def main():
    parser = argparse.ArgumentParser(description="Benchmark Pyrite HTTP against FastAPI/Uvicorn.")
    parser.add_argument("--requests", type=int, default=2000)
    parser.add_argument("--concurrency", type=int, default=16)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--pyrite-port", type=int, default=8091)
    parser.add_argument("--fastapi-port", type=int, default=8093)
    parser.add_argument("--server", choices=["both", "pyrite", "fastapi"], default="both")
    parser.add_argument("--warmup", type=int, default=100)
    parser.add_argument("--pyrite-workers", type=int, default=64)
    parser.add_argument("--profile", choices=sorted(PROFILES), default="micro", help="Named request path profile.")
    parser.add_argument("--paths", default="", help="Comma-separated request paths to cycle through. Overrides --profile.")
    parser.add_argument("--keep-alive", action=argparse.BooleanOptionalAction, default=True, help="Reuse one HTTP connection per client worker.")
    args = parser.parse_args()
    if args.paths:
        paths = [path.strip() for path in args.paths.split(",") if path.strip()]
    else:
        paths = PROFILES[args.profile]
    if not paths:
        raise SystemExit("--paths must include at least one path")

    rows = []
    if args.server in ("both", "pyrite"):
        rows.append(bench_server("pyrite", lambda port: start_pyrite(port, args.pyrite_workers), args.host, args.pyrite_port, args.requests, args.concurrency, args.warmup, paths, args.keep_alive))
    if args.server in ("both", "fastapi"):
        try:
            rows.append(bench_server("fastapi", start_fastapi, args.host, args.fastapi_port, args.requests, args.concurrency, args.warmup, paths, args.keep_alive))
        except RuntimeError as exc:
            if args.server == "fastapi":
                raise SystemExit(f"fastapi benchmark unavailable: {exc}") from None
            print(f"fastapi benchmark skipped: {exc}", file=sys.stderr)

    print(
        f"{'server':<10} {'requests':>8} {'conc':>5} {'ok':>8} "
        f"{'errors':>8} {'seconds':>10} {'req/s':>10} {'wall_us':>9} {'avg_ms':>9} {'MiB*':>8} {'bytes':>10} {'first_error':>16}"
    )
    for row in rows:
        print(
            f"{row['name']:<10} {row['requests']:>8} {args.concurrency:>5} {row['ok']:>8} "
            f"{row['errors']:>8} {row['seconds']:>10.4f} {row['rps']:>10.1f} "
            f"{row['wall_us']:>9.2f} {row['avg_ms']:>9.3f} {row['rss_mib']:>8.2f} {row['bytes']:>10} {row['first_error']:>16}"
        )
    print(f"*Warmup requests per server: {args.warmup}")
    print(f"*Keep-alive: {args.keep_alive}")
    print("*MiB is the server process high-water RSS from /proc when available.")


if __name__ == "__main__":
    main()
