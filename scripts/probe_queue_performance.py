"""Measure queue admission, real Worker completion and event reads on an isolated ForgeOS deployment."""
import argparse
import http.client
import json
import math
import os
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from contextlib import closing
from datetime import datetime
from urllib.parse import urlsplit


def timings(values):
    ordered = sorted(values)
    return {"samples": len(values), **{
        key: round(ordered[math.ceil(q * len(ordered)) - 1], 3)
        for key, q in (("p50_ms", .50), ("p95_ms", .95), ("p99_ms", .99), ("max_ms", 1))
    }}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", required=True)
    parser.add_argument("--detail-task", type=int, required=True, help="a completed task with recorded events")
    parser.add_argument("--projects", type=int, nargs="+", required=True, help="project IDs for new git_head tasks")
    parser.add_argument("--jobs", type=int, required=True)
    parser.add_argument("--clients", type=int, default=4)
    parser.add_argument("--samples", type=int, default=50, help="reads per endpoint per client per round")
    parser.add_argument("--timeout", type=int, default=180, help="seconds to wait for the Workers")
    args = parser.parse_args()
    base = urlsplit(args.url)
    if base.scheme not in ("http", "https") or not base.hostname or base.path not in ("", "/") or base.username or base.password or base.query or base.fragment:
        parser.error("provide an HTTP(S) control-plane origin")
    if min(args.jobs, args.clients, args.samples, args.timeout, args.detail_task, *args.projects) < 1:
        parser.error("counts, timeout and IDs must be positive")
    token = os.environ.get("FORGEOS_OWNER_TOKEN")
    if not token:
        parser.error("set FORGEOS_OWNER_TOKEN")
    conn = http.client.HTTPSConnection if base.scheme == "https" else http.client.HTTPConnection

    def connect():
        return closing(conn(base.hostname, base.port, timeout=15))

    def request(client, path, body=None):
        data = None if body is None else json.dumps(body).encode()
        start = time.perf_counter()
        client.request("GET" if data is None else "POST", path, data,
                       {"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        response = client.getresponse()
        raw = response.read()
        ms = 1000 * (time.perf_counter() - start)
        if response.status != (200 if data is None else 201):
            raise RuntimeError(f"HTTP {response.status} for {path}")
        return json.loads(raw), ms

    detail = f"/tasks/{args.detail_task}"
    listing = "/tasks?limit=30"
    with connect() as client:
        first, _ = request(client, listing)
        history, _ = request(client, detail)
    if history["status"] not in ("DONE", "FAILED", "CANCELLED", "SUPERSEDED") or not history["events"]:
        parser.error("select a terminal task with events")
    total = first["counts"]["ALL"]

    def reads(expected_total):
        def reader(_):
            lists, details = [], []
            with connect() as client:
                for i in range(args.samples + 1):
                    page, lm = request(client, listing)
                    task, dm = request(client, detail)
                    if page["counts"]["ALL"] != expected_total or task["events"] != history["events"]:
                        raise RuntimeError("task count or historical events changed during the read benchmark")
                    if i:  # Warm each connection before collecting percentiles.
                        lists.append(lm)
                        details.append(dm)
            return lists, details

        start = time.perf_counter()
        with ThreadPoolExecutor(max_workers=args.clients) as pool:
            rows = list(pool.map(reader, range(args.clients)))
        elapsed = time.perf_counter() - start
        return {"list": timings([v for ls, _ in rows for v in ls]),
                "history": timings([v for _, ds in rows for v in ds]),
                "wall_seconds_including_warmup": round(elapsed, 3)}

    before = reads(total)
    created, admission, queued, running = {}, [], [], []
    with connect() as client:
        start = time.perf_counter()
        for i in range(args.jobs):
            project = args.projects[i % len(args.projects)]
            task, ms = request(client, "/tasks", {"kind": "git_head", "project_id": project})
            if task["project_id"] != project or task["id"] in created:
                raise RuntimeError("unexpected task admission")
            created[task["id"]] = project
            admission.append(ms)
        print("Created tasks " + ", ".join(map(str, created)), file=sys.stderr, flush=True)
        pending = set(created)
        deadline = time.monotonic() + args.timeout
        while pending:
            for task_id in sorted(pending):
                task, _ = request(client, f"/tasks/{task_id}")
                if task["status"] in ("READY", "RUNNING"):
                    continue
                events = task["events"]
                result = task["execution"]["result"] if task.get("execution") else {}
                if (task["status"] != "DONE" or len(task["attempts"]) != 1 or not result.get("commit")
                        or result["exit_code"] != 0 or task["project_id"] != created[task_id]
                        or [event["type"] for event in events] != ["task_created", "execution_claimed", "execution_completed"]):
                    raise RuntimeError(f"unexpected receipt for task {task_id} ({task['status']})")
                timestamps = [datetime.fromisoformat(event["at"].replace("Z", "+00:00")) for event in events]
                queued.append((timestamps[1] - timestamps[0]).total_seconds() * 1000)
                running.append((timestamps[2] - timestamps[1]).total_seconds() * 1000)
                pending.remove(task_id)
            if pending:
                if time.monotonic() >= deadline:
                    raise TimeoutError(f"unfinished tasks: {sorted(pending)}")
                time.sleep(.25)
        elapsed = time.perf_counter() - start
    after = reads(total + args.jobs)
    print(json.dumps({"initial_tasks": total, "historical_events": len(history["events"]), "created": list(created),
                      "before": before, "after": after, "create": timings(admission), "queue_wait": timings(queued),
                      "claim_to_completion": timings(running), "batch_seconds": round(elapsed, 3),
                      "observed_jobs_per_second": round(args.jobs / elapsed, 2)}, indent=2))


if __name__ == "__main__":
    main()
