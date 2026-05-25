#!/usr/bin/env python3
import argparse
import subprocess
import sys
import time
from pathlib import Path


def rss_mib(pid: int) -> float | None:
    status = Path(f"/proc/{pid}/status")
    try:
        for line in status.read_text().splitlines():
            if line.startswith("VmRSS:"):
                return int(line.split()[1]) / 1024
    except FileNotFoundError:
        return None
    return None


def run(label: str, cmd: list[str], interval: float) -> tuple[float, float, list[tuple[float, float]]]:
    start = time.perf_counter()
    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    samples: list[tuple[float, float]] = []
    peak = 0.0

    while proc.poll() is None:
        rss = rss_mib(proc.pid)
        now = time.perf_counter() - start
        if rss is not None:
            peak = max(peak, rss)
            samples.append((now, rss))
        time.sleep(interval)

    output = proc.stdout.read() if proc.stdout else ""
    elapsed = time.perf_counter() - start
    rss = rss_mib(proc.pid)
    if rss is not None:
        peak = max(peak, rss)
        samples.append((elapsed, rss))

    if proc.returncode != 0:
        print(output, file=sys.stderr)
        raise SystemExit(f"{label} failed with exit code {proc.returncode}")

    return elapsed, peak, samples


def summarize(label: str, elapsed: float, peak: float, samples: list[tuple[float, float]]) -> None:
    print(f"{label}: elapsed={elapsed:.3f}s peak={peak:.2f}MiB samples={len(samples)}")
    if not samples:
        return
    checkpoints = 8
    if len(samples) <= checkpoints:
        chosen = samples
    else:
        step = max(1, len(samples) // checkpoints)
        chosen = samples[::step]
        if chosen[-1] != samples[-1]:
            chosen.append(samples[-1])
    for t, rss in chosen:
        print(f"  {t:8.3f}s  {rss:8.2f}MiB")


def main() -> None:
    parser = argparse.ArgumentParser(description="Sample RSS while stress benchmarks run.")
    parser.add_argument("--interval", type=float, default=0.05, help="RSS sample interval in seconds")
    parser.add_argument("--skip-python", action="store_true", help="Only run the Pyrite stress benchmark")
    args = parser.parse_args()

    root = Path(__file__).resolve().parents[1]
    pyritec = root / "build" / "pyritec"
    pyrite_out = root / "build" / "benchmarks" / "string_stress"
    pyrite_src = root / "benchmarks" / "pyrite" / "string_stress.pyr"
    python_src = root / "benchmarks" / "python" / "string_stress.py"

    (root / "build" / "benchmarks").mkdir(parents=True, exist_ok=True)
    subprocess.run(["go", "build", "-o", str(pyritec), "./cmd/pyritec"], cwd=root, check=True)
    subprocess.run([str(pyritec), str(pyrite_src), "-o", str(pyrite_out)], cwd=root, check=True)

    elapsed, peak, samples = run("pyrite", [str(pyrite_out)], args.interval)
    summarize("pyrite", elapsed, peak, samples)

    if not args.skip_python:
        elapsed, peak, samples = run("python", [sys.executable, str(python_src)], args.interval)
        summarize("python", elapsed, peak, samples)


if __name__ == "__main__":
    main()
