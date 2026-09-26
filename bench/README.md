# bench — the agent under Harbor

Benchmarks run through [Harbor](https://github.com/harbor-framework/harbor): one Docker container
per task, the agent inside it, the task's own tests deciding pass or fail.

- `harbor/Dockerfile` — the Harbor runner image (`<dist>-harbor`); task containers are siblings
  started through the host's Docker socket.
- `harbor/harbor_agent.py` — the adapter: installs the static binary, seeds a world, runs
  `<dist> run --json`, reports tokens and cost from the usage journal.
- `fakevllm/` — an OpenAI-compatible stand-in for a hosted vLLM that plays a fixed script, for
  the €0 plumbing test (`FAKE_MODE=right|wrong`).
- `tasks/` — local tasks (`hello-file`: the plumbing smoke task).
- `configs/` — the exact agent config each kind of run uses.
- `runs.jsonl` — one line per launch, append-only; `RESULTS.md` is generated from it by
  `record.py`, which also asserts expected rewards (`--expect`).
- `smoke.sh` — the €0 smoke run: the fake vLLM scripted right must score 1.0, wrong must score 0.0.
- `jobs/` — Harbor's raw output, not tracked.

Run the runner with the repository mounted at the same absolute path, so the paths Harbor hands
to the Docker daemon resolve on the host:

    R=$(git rev-parse --show-toplevel)
    docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v $R:$R -w $R/bench \
      --group-add $(stat -c %g /var/run/docker.sock) <dist>-harbor run -p tasks/hello-file -a oracle -o jobs -y
