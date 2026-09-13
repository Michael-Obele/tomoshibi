#!/usr/bin/env python3
"""Search load benchmark for Cinder vs Firecrawl.

Fires concurrent /v1/search requests with varied queries and reports success
rate plus latency percentiles, so a comparison between engines is backed by
numbers rather than vibes.

Usage:
    python3 scripts/search-bench.py --url http://localhost:8080/v1/search \
        --concurrency 10 --duration 30 --label cinder
"""
import argparse
import json
import statistics
import threading
import time
import urllib.request

QUERIES = [
    "golang concurrency patterns",
    "svelte 5 runes guide",
    "cloudflare workers durable objects",
    "go 1.27 generic methods",
    "firecrawl self hosted docker",
    "searxng json api",
    "web scraping best practices 2026",
    "llm ready markdown extraction",
    "postgresql connection pooling",
    "redis rate limiting",
]


def run_worker(url, queries, duration, results, lock, label):
    end = time.time() + duration
    i = 0
    while time.time() < end:
        q = queries[i % len(queries)]
        i += 1
        body = json.dumps({"query": q, "limit": 5}).encode()
        req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"})
        t0 = time.time()
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                status = resp.status
        except urllib.error.HTTPError as e:
            status = e.code
        except Exception as e:
            status = 0  # connection error
            with lock:
                results["errors"][type(e).__name__] = results["errors"].get(type(e).__name__, 0) + 1
        lat = (time.time() - t0) * 1000  # ms
        with lock:
            results["statuses"][status] = results["statuses"].get(status, 0) + 1
            results["lats"].append(lat)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--concurrency", type=int, default=10)
    ap.add_argument("--duration", type=int, default=30)
    ap.add_argument("--label", default="bench")
    args = ap.parse_args()

    results = {"statuses": {}, "lats": [], "errors": {}}
    lock = threading.Lock()
    threads = [
        threading.Thread(target=run_worker, args=(args.url, QUERIES, args.duration, results, lock, args.label))
        for _ in range(args.concurrency)
    ]
    t0 = time.time()
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    elapsed = time.time() - t0

    lats = sorted(results["lats"])
    n = len(lats)
    pct = lambda p: lats[min(n - 1, int(n * p))] if n else 0
    ok = sum(v for k, v in results["statuses"].items() if 200 <= k < 300)
    fail = n - ok
    print(f"\n=== {args.label} — {args.url}")
    print(f"  requests: {n} in {elapsed:.0f}s ({n/elapsed:.1f} req/s) | concurrency {args.concurrency}")
    print(f"  success:  {ok}/{n} ({100*ok/n:.1f}%) | failures: {fail}")
    print(f"  statuses: {dict(sorted(results['statuses'].items()))}")
    print(f"  latency:  p50={pct(0.5):.0f}ms  p95={pct(0.95):.0f}ms  p99={pct(0.99):.0f}ms  max={lats[-1]:.0f}ms" if n else "  no requests")
    if results["errors"]:
        print(f"  errors:   {results['errors']}")
    return ok, n


if __name__ == "__main__":
    main()