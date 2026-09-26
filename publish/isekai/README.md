<p align="center"><img src="portraits/rimuru.png" width="96" alt="Rimuru"></p>

<h1 align="center">Isekai</h1>

<p align="center"><b>An agent harness where the law is the harness.</b><br>
One Go binary. The convention isn't a prompt the model may skip — the gate, the log, the human
gate, the sandbox and the memory are code paths it cannot route around.</p>

---

## Why

Coding agents read their project's rules and then, sometimes, don't follow them. Isekai turns a
directory into a **world** governed by [`ISEKAI.md`](ISEKAI.md) and runs the agent inside that
law:

- **The gate** — every turn that changed the world is checked before it's reported done: the right
  creature authored it, its invariants hold, its duties are done, its doc changed with it. The
  verdict lands in an append-only `log.md`.
- **The human gate** — every action is classified (`read` · `write` · `outward` · `destructive`)
  before it runs. Outward and destructive acts ask you; per-command rules (`bash:git push*`) can
  allow, ask or deny. The model's claim about a command can only make it stricter.
- **The sandbox** — `bash` runs in a persistent shell under `bwrap`: the filesystem read-only
  except the world, no network unless the gate let the act out, secrets scrubbed from the env.
- **Memory, native** — recall before every step, remember after; knowledge the agent held but never
  wrote down (*the unsaid*) is surfaced and routed to where it belongs. When context fills, it is
  **drained** into those homes by pointer, not summarized away.
- **Ranks and offices** — the session (Rimuru) dispatches Court Bodies to Elves, Orcs and Slimes;
  each office (Great Sage reads, Raphael judges, Ciel writes) can run on its own model.
- **Everything on, config takes away** — every tool, law feature and instrument is switchable in
  YAML or JSON, and a switched-off law is always shown, never silent.

## Install

This repository is private: download a release with the GitHub CLI.

```sh
gh release download --repo Kaginari/isekai --pattern "isekai_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz"
tar -xzf isekai_*.tar.gz isekai && install -m 755 isekai ~/.local/bin/
isekai version
```

Or build it: `CGO_ENABLED=0 go build -o isekai ./cmd/isekai` (Go ≥ 1.23, standard library only).

## Quick start

```sh
cd your-project
isekai init                      # found a world here: .isekai/ with the law and a config
export ANTHROPIC_API_KEY=…        # or any OpenAI-compatible endpoint (vLLM, Ollama, OpenRouter)
isekai                           # the live session — the board opens at http://127.0.0.1:7411
isekai run "add a health check endpoint and its test"
```

## The CLI

| Command | What it does |
|---|---|
| `isekai` / `repl` | the live session: talk while Courts work; `/agents`, `/send`, `/usage`, `/status`, `/compact` |
| `run "<ask>"` | one ask to its end, non-interactive; `--json` for machines |
| `resume <id>` · `sessions` | continue or list sessions |
| `status` | the instrument board and the off-list (every law feature that is switched off, and where) |
| `config show\|explain\|check\|path\|patch` | the effective config with the origin of every value |
| `memory` · `toolbox` · `onto` | the three memories, the two-level toolbox, the creature graph |
| `usage` | tokens and cost by body, office, rank, model and day |
| `bench` | a fixed task set on every configured model |
| `board` | the board without a session |
| `selftest` · `version` · `init` | |

## Configuration

`.isekai/config.yaml` (or `.json`), layered over `~/.config/isekai/`, env and flags. A taste:

```yaml
providers:
  vllm: { type: openai, baseURL: https://vllm.internal/v1, apiKeyEnv: VLLM_API_KEY }
models:
  default: anthropic/claude-opus-5
  offices: { great-sage: anthropic/claude-haiku-4-5, raphael: anthropic/claude-opus-5, ciel: anthropic/claude-fable-5-1 }
permissions:
  rules:
    - { match: "bash:git push*", action: ask }
    - { match: "bash:git push --force*", action: deny }
tools:
  custom:
    kube-pods: { run: [kubectl, get, pods, -n, "{{ns}}"], params: { ns: { type: string } }, class: outward }
rules:
  - { text: "Tests must pass before a change lands.", check: "go test ./..." }
```

The full schema: [`docs/canon/config.md`](docs/canon/config.md). The design: [`docs/canon/binary.md`](docs/canon/binary.md).

## Benchmarks

Benchmarks run under [Harbor](https://github.com/harbor-framework/harbor) in Docker — one container
per task, the agent inside it, the task's own tests deciding. Every launch is recorded in
[`bench/runs.jsonl`](bench/runs.jsonl); [`bench/RESULTS.md`](bench/RESULTS.md) is generated from it.
The €0 smoke (`bench/smoke.sh`) runs on every commit in CI against a fake OpenAI-compatible server.

## License

MIT — see [LICENSE](LICENSE). Bootstrap 5 (embedded in the board) is MIT.
