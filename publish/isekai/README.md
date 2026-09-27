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

Or build it: `CGO_ENABLED=0 go build -o isekai ./cmd/isekai` (Go ≥ 1.26; the standard library plus the Charm libraries for the terminal).

## Quick start

```sh
cd your-project
isekai init                      # found a world here: .isekai/ with the law and a config
export ANTHROPIC_API_KEY=…        # or any OpenAI-compatible endpoint (vLLM, Ollama, OpenRouter)
isekai                           # the live session — the board opens at http://127.0.0.1:7411
isekai run "add a health check endpoint and its test"
```

## The terminal

A terminal UI in the class of Claude Code, built on the Charm libraries: the conversation flows into
your scrollback, and every event of the loop is drawn — thinking, tool calls, diffs, Courts, the gate.

![welcome](docs/screens/isekai-welcome.png)

The model's thinking streams while it arrives and folds when the answer starts; each rank has its own
face and verbs beside the spinner:

![thinking](docs/screens/isekai-thinking.png)

When a turn wrote files, the orc weighs the verdict before the turn is done:

![the gate](docs/screens/isekai-gating.png)

Tool calls are cards edged in their class's colour; diffs keep the code's own colours:

![tool cards and a diff](docs/screens/isekai-tools.png)

`ctrl+t` watches every body — the session and each Court, its thinking and its steps, live:

![every body](docs/screens/isekai-bodies.png)

`/board` shows the world full screen — the agents, the reasoned ontology as a graph, the offices, the
usage — and `ctrl+k` opens a fuzzy command palette:

![the ontology graph](docs/screens/isekai-board-graph.png)

![the palette and a toast](docs/screens/isekai-overlay.png)

| Key | |
|---|---|
| `enter` · `shift+enter` | send · newline |
| `/` · `ctrl+k` | commands · the palette |
| `ctrl+t` | every body, live |
| `ctrl+o` | expand the last folded block |
| `esc` | interrupt the turn |

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
| `goal --validate "<cmd>" "<objective>"` | work turn after turn until the command passes; the binary runs it, the model cannot skip it |
| `review [range]` | two reviewers on two models in parallel, one merged shortlist; nothing fixed before you approve |
| `handoff [focus]` | a handoff note for a fresh session (`/handoff read` picks it up) |
| `guard check\|test\|show\|install` | the global dangerous-command guard; `install` wires it into Claude Code and OpenCode |
| `board [--ssh [addr]]` | the board without a session — on the web, and over SSH (keys in `~/.ssh/authorized_keys` only) |
| `--containered` | the whole binary in a Docker container: the world read-write, the rest read-only |
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


Any section can live in its own file beside `config.yaml` — `models.yaml`, `providers.yaml`,
`rules.yaml`, `guards.yaml` — and `config explain` names the file every value came from. `guards.yaml`
may be just a list of patterns, added to the built-in denylist of catastrophic commands:

```yaml
# .isekai/guards.yaml
- '(^|[[:space:]])terraform[[:space:]]+destroy'
- 'kubectl[[:space:]]+delete[[:space:]]+(ns|namespace)'
```

The full schema: [`docs/canon/config.md`](docs/canon/config.md). The design: [`docs/canon/binary.md`](docs/canon/binary.md).

## Benchmarks

Benchmarks run under [Harbor](https://github.com/harbor-framework/harbor) in Docker — one container
per task, the agent inside it, the task's own tests deciding. Every launch is recorded in
[`bench/runs.jsonl`](bench/runs.jsonl); [`bench/RESULTS.md`](bench/RESULTS.md) is generated from it.
The €0 smoke (`bench/smoke.sh`) runs on every commit in CI against a fake OpenAI-compatible server.

## License

MIT — see [LICENSE](LICENSE). Bootstrap 5 (embedded in the board) is MIT.
