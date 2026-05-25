# Benchmarks

These benchmark pairs compare the current compiled Pyrite subset against
roughly equivalent Python code. The workloads are class-based on both sides so
they exercise constructor calls, field access, and method dispatch.

Run all benchmarks:

```sh
scripts/bench.sh
```

Run the long string memory stress test:

```sh
scripts/memory_stress.sh --skip-python
```

Run the local HTTP API server benchmark:

```sh
scripts/http_bench.py --requests 1000
```

Run the larger Pyrite vs FastAPI benchmark:

```sh
scripts/fastapi_vs_pyrite.py --requests 2000 --concurrency 16
```

Run a more application-like HTTP profile:

```sh
scripts/fastapi_vs_pyrite.py --profile realworld --requests 3000 --concurrency 8
```

The FastAPI comparison uses a warmup phase by default and reports both
throughput and average per-request latency:

```sh
scripts/fastapi_vs_pyrite.py --requests 10000 --concurrency 32 --warmup 500
```

Tune Pyrite's HTTP worker pool without editing source:

```sh
scripts/fastapi_vs_pyrite.py --requests 10000 --concurrency 64 --pyrite-workers 64
```

The FastAPI comparison uses HTTP keep-alive by default so each benchmark worker
reuses one connection. To force the old one-request-per-connection behavior:

```sh
scripts/fastapi_vs_pyrite.py --requests 3000 --concurrency 32 --no-keep-alive
```

Current benchmark pairs:

- `math_loop`: while-loop integer increments
- `string_methods`: strip/upper/replace/contains in a loop
- `random_any`: seeded random choice from a mixed `list[any]`
- `json`: stringify/parse a mixed scalar array and read flat object fields
- `vector_math`: fixed 2D/3D vector add/subtract/scale/dot/cross helpers
- `file_io`: write/read a small text file
- `string_stress`: long-running string loop used by `memory_stress.sh`
- `http_api_server`: blocking HTTP API server with `/api/ping`, `/api/score`,
  `/api/text`, `/api/user`, `/api/items`, `/api/compute`, and real-world style
  `/api/auth`, `/api/search`, `/api/report`, `/api/profile`, `/api/transform`,
  and `/api/dashboard` routes, driven by `scripts/http_bench.py` or
  `scripts/fastapi_vs_pyrite.py`

HTTP benchmark profiles:

- `micro`: the original small API routes.
- `realworld`: header scanning, query/search scoring, report generation,
  CORS preflight processing, profile computation, string transformation, and
  dashboard aggregation.
- `mixed`: both groups together.

The runner reports wall-clock seconds and peak resident memory in MiB. Memory is
read via Python's `resource.getrusage`, so it reflects the platform's child
process peak RSS reporting. These are simple smoke benchmarks, not a full
statistical benchmark harness.
