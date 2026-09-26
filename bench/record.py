#!/usr/bin/env python3
"""Append one Harbor job to bench/runs.jsonl and regenerate bench/RESULTS.md.

    bench/record.py <jobs/JOB_DIR> --purpose "<why this launch>" [--expect <mean>]

--expect makes it an assertion: a mean reward other than the expected one exits 1 (used by the
smoke test). runs.jsonl is append-only; RESULTS.md is generated from it and never hand-edited.
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path

BENCH = Path(__file__).resolve().parent


def git(*args: str) -> str:
    try:
        return subprocess.run(["git", *args], cwd=BENCH, capture_output=True, text=True, check=True).stdout.strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        return "unknown"


def summarize(job_dir: Path, purpose: str) -> dict:
    r = json.loads((job_dir / "result.json").read_text())
    (name, ev), *more = r["stats"]["evals"].items()
    if more:
        raise SystemExit(f"{job_dir}: one agent/dataset per job expected, got {len(more) + 1}")
    agent, _, dataset = name.partition("__")
    model = None
    cfg = job_dir / "config.json"
    if cfg.exists():
        c = json.loads(cfg.read_text())
        agents = c.get("agents") or []
        if agents:
            model = agents[0].get("model_name")
    s = r["stats"]
    return {
        "ts": r["started_at"],
        "job": job_dir.name,
        "repo": git("rev-parse", "--short", "HEAD"),
        "dirty": bool(git("status", "--porcelain")),
        "agent": agent,
        "model": model,
        "dataset": dataset or "adhoc",
        "trials": r["n_total_trials"],
        "errors": ev.get("n_errors", 0),
        "mean_reward": (ev.get("metrics") or [{}])[0].get("mean"),
        "n_input_tokens": s.get("n_input_tokens"),
        "n_cache_tokens": s.get("n_cache_tokens"),
        "n_output_tokens": s.get("n_output_tokens"),
        "cost_usd": s.get("cost_usd"),
        "purpose": purpose,
    }


def render(runs: list[dict]) -> str:
    def cell(v):
        return "—" if v is None else (f"{v:.4f}" if isinstance(v, float) else str(v))

    out = ["# Benchmark results", "", "Generated from `runs.jsonl` by `record.py` — do not edit.", "",
           "| when | agent | model | dataset | trials | mean reward | errors | input tok | output tok | cost $ | commit | purpose |",
           "|---|---|---|---|---|---|---|---|---|---|---|---|"]
    for r in reversed(runs):
        commit = r.get("repo", "?") + ("+dirty" if r.get("dirty") else "")
        out.append("| " + " | ".join(cell(x) for x in [
            r.get("ts", "")[:16].replace("T", " "), r.get("agent"), r.get("model"), r.get("dataset"),
            r.get("trials"), r.get("mean_reward"), r.get("errors"), r.get("n_input_tokens"),
            r.get("n_output_tokens"), r.get("cost_usd"), commit, r.get("purpose")]) + " |")
    return "\n".join(out) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("job_dir", type=Path)
    ap.add_argument("--purpose", required=True)
    ap.add_argument("--expect", type=float)
    a = ap.parse_args()
    line = summarize(a.job_dir, a.purpose)
    runs_path = BENCH / "runs.jsonl"
    with runs_path.open("a") as f:
        f.write(json.dumps(line) + "\n")
    runs = [json.loads(x) for x in runs_path.read_text().splitlines() if x.strip()]
    (BENCH / "RESULTS.md").write_text(render(runs))
    print(json.dumps(line))
    if a.expect is not None and line["mean_reward"] != a.expect:
        print(f"expected mean reward {a.expect}, got {line['mean_reward']}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
