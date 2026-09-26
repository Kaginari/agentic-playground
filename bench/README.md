# bench — isekai under Harbor

Benchmarks run through [Harbor](https://github.com/harbor-framework/harbor): one Docker container
per task, the agent inside it, the task's own tests deciding pass or fail.

- `harbor/Dockerfile` — the Harbor runner image (`isekai-harbor`); task containers are siblings
  started through the host's Docker socket.
- `harbor/isekai_agent.py` — the adapter: installs the static binary, seeds a world, runs
  `isekai run --json`, reports tokens and cost from the usage journal.
- `fakevllm/` — an OpenAI-compatible stand-in for a hosted vLLM that plays a fixed script, for
  the €0 plumbing test (`FAKE_MODE=right|wrong`).
- `tasks/` — local tasks (`hello-file`: the plumbing smoke task).
- `configs/` — the exact isekai config each kind of run uses.
- `runs.jsonl` — one line per launch, append-only; `RESULTS.md` is generated from it.
- `jobs/` — Harbor's raw output, not tracked.

Run the runner with the repository mounted at the same absolute path, so the paths Harbor hands
to the Docker daemon resolve on the host:

    R=$(git rev-parse --show-toplevel)
    docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v $R:$R -w $R/bench \
      --group-add $(stat -c %g /var/run/docker.sock) isekai-harbor run -p tasks/hello-file -a oracle -o jobs -y
