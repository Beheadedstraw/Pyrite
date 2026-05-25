#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PYTHON_BIN="${PYTHON_BIN:-python3}"

mkdir -p build/benchmarks
go build -o build/pyritec ./cmd/pyritec

benches=(
  math_loop
  string_methods
  random_any
  json
  vector_math
  file_io
)

printf "%-18s %-10s %-12s %-10s %-12s\n" "benchmark" "pyrite(s)" "pyrite(MiB)" "python(s)" "python(MiB)"
printf "%-18s %-10s %-12s %-10s %-12s\n" "---------" "---------" "-----------" "---------" "-----------"

run_measured() {
  local out_file="$1"
  shift
  "$PYTHON_BIN" - "$out_file" "$@" <<'PY'
import resource
import subprocess
import sys
import time

out_file = sys.argv[1]
cmd = sys.argv[2:]

start = time.perf_counter()
with open(out_file, "w", encoding="utf-8") as out:
    subprocess.run(cmd, stdout=out, stderr=subprocess.STDOUT, check=True)
elapsed = time.perf_counter() - start

# Linux reports ru_maxrss in KiB. macOS reports bytes; normalize roughly.
rss = resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss
if sys.platform == "darwin":
    mib = rss / (1024 * 1024)
else:
    mib = rss / 1024

print(f"{elapsed:.4f} {mib:.2f}")
PY
}

for bench in "${benches[@]}"; do
  pyr_src="benchmarks/pyrite/${bench}.pyr"
  pyr_out="build/benchmarks/${bench}"
  py_src="benchmarks/python/${bench}.py"

  build/pyritec "$pyr_src" -o "$pyr_out"

  read -r pyrite_time pyrite_mem < <(run_measured "/tmp/pyrite_bench_${bench}.out" "$pyr_out")
  read -r python_time python_mem < <(run_measured "/tmp/python_bench_${bench}.out" "$PYTHON_BIN" "$py_src")

  printf "%-18s %-10s %-12s %-10s %-12s\n" "$bench" "$pyrite_time" "$pyrite_mem" "$python_time" "$python_mem"
done
