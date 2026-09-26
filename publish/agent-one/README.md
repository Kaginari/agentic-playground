<p align="center"><img src="portraits/orchestrator.png" width="96" alt="Orchestrator"></p>

<h1 align="center">Agent-One</h1>

<p align="center"><b>A coding-agent harness that enforces its policies in code.</b><br>
One static Go binary. Review gates, an append-only change log, human approval for risky actions,
a sandboxed shell and native memory are part of the runtime — not instructions the model may
ignore.</p>

---

## Why

Agents read a project's rules and then, sometimes, don't follow them. Agent-One turns a directory
into a **workspace** governed by [`AGENT-ONE.md`](AGENT-ONE.md) and runs the agent inside it:

- **Review gate** — every turn that changed the workspace is checked before it's reported done:
  the owning member made the change, its invariants hold, the requested work is complete, and the
  owning doc changed with the code. The verdict is appended to `log.md`.
- **Human approval** — every action is classified (`read` · `write` · `outward` ·
  `destructive`) before it runs. Outward and destructive actions ask you; per-command rules
  (`bash:git push*` → allow / ask / deny) tune it. A model can only make a classification stricter.
- **Sandbox** — `bash` runs in a persistent shell under `bwrap`: read-only filesystem except the
  workspace, no network unless the action was approved as outward, secrets stripped from the env.
- **Native memory** — recall before every step, record after; knowledge the agent held but never
  wrote down is surfaced and filed. When the context window fills, it is **drained** to memory by
  reference instead of being summarized away.
- **Roles and model routing** — the orchestrator dispatches ephemeral subagents to coordinators,
  domain owners and zone workers; the analyst, judge and drafter roles can each run on their own
  model (a cheap model to read, a strong one to judge).
- **Everything on, configuration takes away** — every tool, policy and instrument can be switched
  off in YAML or JSON, and a disabled policy is always reported, never silent.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/Kaginari/agent-one/main/install.sh | sh
# or
go install github.com/Kaginari/agent-one/cmd/agent-one@latest
# or
docker run --rm -it -v "$PWD:/work" ghcr.io/kaginari/agent-one:latest
```

Release archives for linux/macOS × amd64/arm64 ship with `checksums.txt` and build provenance:
`gh attestation verify <archive> --repo Kaginari/agent-one`.

## Quick start

```sh
cd your-project
agent-one init                    # onboard: .agent-one/ with the policies and a config
export ANTHROPIC_API_KEY=…         # or any OpenAI-compatible endpoint (vLLM, Ollama, OpenRouter)
agent-one                         # the live session — the dashboard opens at http://127.0.0.1:7411
agent-one run "add a health check endpoint and its test"
```

## The CLI

| Command | What it does |
|---|---|
| `agent-one` / `repl` | the live session: keep talking while subagents work; `/agents`, `/send`, `/usage`, `/status`, `/compact` |
| `run "<task>"` | one task to completion, non-interactive; `--json` for automation |
| `resume <id>` · `sessions` | continue or list sessions |
| `status` | the instrument board and every policy that is switched off (and where) |
| `config show\|explain\|check\|path\|patch` | the effective configuration with the origin of every value |
| `memory` · `toolbox` · `onto` | memory tiers, the two-level tool registry, the ownership graph |
| `usage` | tokens and cost by agent, role, model and day |
| `bench` | a fixed task set on every configured model |
| `board` | the dashboard without a session |
| `selftest` · `version` · `init` | |

## Configuration

`.agent-one/config.yaml` (or `.json`), layered over `~/.config/agent-one/`, environment and flags:

```yaml
providers:
  vllm: { type: openai, baseURL: https://vllm.internal/v1, apiKeyEnv: VLLM_API_KEY, toolCalls: native }
models:
  default: vllm/Qwen/Qwen3-Coder-480B-A35B-Instruct
permissions:
  rules:
    - { match: "bash:git push*", action: ask }
    - { match: "bash:git push --force*", action: deny }
tools:
  custom:
    kube-pods: { run: [kubectl, get, pods, -n, "{{ns}}"], params: { ns: { type: string } }, class: outward }
rules:
  - { text: "Tests must pass before a change lands.", check: "go test ./..." }
budgets:
  session: { usd: 5 }
```

## Benchmarks

Benchmarks run under [Harbor](https://github.com/harbor-framework/harbor) in Docker — one container
per task, the agent inside it, the task's own tests deciding. Every launch is recorded in
[`bench/runs.jsonl`](bench/runs.jsonl); [`bench/RESULTS.md`](bench/RESULTS.md) is generated from it.
CI runs a no-cost smoke benchmark (`bench/smoke.sh`) on every commit against a fake
OpenAI-compatible server.

## License

MIT — see [LICENSE](LICENSE). Role badges use [Tabler Icons](https://tabler.io/icons) (MIT);
the dashboard embeds Bootstrap 5 (MIT).
