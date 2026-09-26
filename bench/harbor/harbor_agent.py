"""Harbor adapter for the agent binary.

Harbor gives one task container and one instruction; this adapter installs the static Go binary,
seeds a minimal world, runs `<dist> run --json`, and reports tokens and cost from the usage
journal. Load it with:  harbor run --agent harbor_agent:HarborAgent  (PYTHONPATH=bench/harbor)

Host environment it reads:
  BENCH_BIN             path to the linux/amd64 static binary to install (required)
  BENCH_DIST            the binary's name (required); its world dir is .<dist>, its env prefix
                        the name upper-cased with - → _ (my-agent → MY_AGENT_)
  BENCH_CONFIG          path to the bench config (YAML), written into the world as config.local.yaml
  BENCH_FORWARD_ENV     comma-separated env var names to forward (API keys by name, never values
                        written to disk), e.g. VLLM_API_KEY
"""

from __future__ import annotations

import base64
import json
import os
import shlex
from pathlib import Path

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

WORLD = "/app"  # the task's working directory; the world dir is seeded inside it


class HarborAgent(BaseInstalledAgent):
    @staticmethod
    def name() -> str:
        return os.environ.get("BENCH_DIST", "agent")

    def _dist(self) -> str:
        dist = os.environ.get("BENCH_DIST")
        if not dist:
            raise ValueError("BENCH_DIST must name the agent binary")
        return dist

    def _dist_dir(self) -> str:
        return "." + self._dist()

    def _env_prefix(self) -> str:
        return self._dist().upper().replace("-", "_") + "_"

    def get_version_command(self) -> str | None:
        return f"{self._dist()} version"

    async def install(self, environment: BaseEnvironment) -> None:
        binary = os.environ.get("BENCH_BIN")
        if not binary or not Path(binary).is_file():
            raise ValueError("BENCH_BIN must point at the static linux/amd64 agent binary")
        await environment.upload_file(binary, f"/usr/local/bin/{self._dist()}")
        await self.exec_as_root(environment, command=f"chmod 755 /usr/local/bin/{self._dist()}")

    def _run_env(self) -> dict[str, str]:
        env: dict[str, str] = {}
        for name in filter(None, (n.strip() for n in os.environ.get("BENCH_FORWARD_ENV", "").split(","))):
            if name in os.environ:
                env[name] = os.environ[name]
        return env

    @with_prompt_template
    async def run(self, instruction: str, environment: BaseEnvironment, context: AgentContext) -> None:
        env = self._run_env()
        d, dist = self._dist_dir(), self._dist()
        # The bench config is a file inside the world (the project-local layer): only a config file
        # is the human's written word for the gate, never an env var.
        cfg = os.environ.get("BENCH_CONFIG")
        write_cfg = ""
        if cfg:
            b64 = base64.b64encode(Path(cfg).read_bytes()).decode()
            write_cfg = f" && echo {b64} | base64 -d > {WORLD}/{d}/config.local.yaml"
        await self.exec_as_agent(
            environment,
            command=f"mkdir -p {WORLD}/{d} && cd {WORLD} && {dist} init --bench > /logs/agent/init.txt 2>&1{write_cfg}",
            env=env,
            timeout_sec=60,
        )
        model = f"--model {shlex.quote(self.model_name)} " if self.model_name else ""
        await self.exec_as_agent(
            environment,
            command=(
                f"cd {WORLD} && {dist} run --json {model}{shlex.quote(instruction)} "
                "> /logs/agent/run.json 2> /logs/agent/run.stderr; rc=$?; "
                f"mkdir -p /logs/agent/usage && cp -r {d}/instruments/usage/. /logs/agent/usage/ 2>/dev/null; "
                f"cp {d}/log.md /logs/agent/log.md 2>/dev/null; exit $rc"
            ),
            env=env,
        )

    def populate_context_post_run(self, context: AgentContext) -> None:
        usage_dir = self.logs_dir / "usage"
        if not usage_dir.is_dir():
            return
        n_in = n_out = n_cache = 0
        cost, priced = 0.0, True
        for f in sorted(usage_dir.glob("*.jsonl")):
            for line in f.read_text().splitlines():
                if not line.strip():
                    continue
                r = json.loads(line)
                cache_read, cache_write = int(r.get("cacheRead") or 0), int(r.get("cacheWrite") or 0)
                n_in += int(r.get("input") or 0) + cache_read + cache_write
                n_cache += cache_read
                n_out += int(r.get("output") or 0)
                if r.get("usd") is None:
                    priced = False
                else:
                    cost += float(r["usd"])
        context.n_input_tokens = n_in
        context.n_cache_tokens = n_cache
        context.n_output_tokens = n_out
        context.cost_usd = cost if priced else None
