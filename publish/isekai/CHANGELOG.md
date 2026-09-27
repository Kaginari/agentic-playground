# Changelog

All notable changes to __TITLE__ are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org).

## [Unreleased]

## [0.2.0] - __DATE__

### Highlights

**A terminal in the class of Claude Code**, on Charm v2: the mascot drops in; the model's thinking
streams and folds; each rank has a face and its own verbs, and the orc weighs the verdict at the gate;
tool calls are class-coloured cards and diffs keep the code's colours; `ctrl+t` watches every body,
live; `/board` full screen with the reasoned ontology as a graph; a `ctrl+k` palette and toasts; the
window title and tab progress follow the session; the transcript reprints on resize.

**Safety in code**: a global dangerous-command guard refused before any gate (151-case corpus;
`guard install` wires it into Claude Code and OpenCode); the gate runs the pre-turn verify lines, keeps
the tests intact, and sends a failed gate back to the model once.

**New commands**: `goal`, `review`, `handoff`, `board --ssh`, `--containered`, a setup form at `init`;
config one file per section (`guards.yaml`, `rules.yaml`…).

**Fixes**: an upstream `finish_reason: "error"` is a failed (retryable) call, not an answer; a TUI that
cannot start says why; a piped slash command's turn can no longer be skipped by the next line.

## [0.1.5] - __DATE__

### Highlights

Works with Google Gemini's OpenAI-compatible API, free tier included: each tool call's `thought_signature` travels back on the next turn (multi-step tool use failed before), 429 and 5xx answers are retried with backoff (honouring `Retry-After`), and Gemini's array-form errors are shown instead of "unreadable body". The session answers you in plain language; only dispatched Court Bodies answer on the wire.

## [0.1.4] - __DATE__

### Highlights

`run "<ask>"` no longer reads stdin — an inherited pipe that never closes (CI, another agent, cron) could hang it; stdin is read only for `run` with no ask or `run -`. Background jobs killed by a timeout no longer print a stray "Killed" line into the next command's output.

## [0.1.3] - __DATE__

### Highlights

The board's off-list now names switched-off tools (it said "everything on" while a tool was off), and long origins wrap inside their card.

## [0.1.2] - __DATE__

### Highlights

A patch release for two defects found by running the released binary in a plain world.

### Fixed
- A machine-wide agent file named after the throne (e.g. OpenCode's `~/.config/opencode/agents/rimuru.md`) was imported as a creature with rank `elf`, so the session ran with the elf's office, model route and tool shelf. The session's name is now reserved.
- The toolbox's level-2 hint named `node .isekai/tools/toolbox.js`, which releases do not ship; it names `isekai toolbox` unless the world ships the JS instrument.

## [0.1.1] - __DATE__

### Highlights

A patch release: the default model is `anthropic/claude-opus-5` (0.1.0 shipped an outdated default),
and the live REPL no longer races the session's context reading while a turn perceives it.

### Fixed
- Default model `anthropic/claude-opus-5`.
- A data race between the status line / board and the running turn's context reading.
- The shared Harbor smoke task uses neutral wording.

## [0.1.0] - __DATE__

### Highlights

First release: the isekai convention as a Go binary. The law is enforced by the harness — the end-of-turn gate with its verdict in `log.md`, the human gate with per-command permission rules, a `bwrap`-sandboxed persistent shell, and native memory with compaction by drain. Ranks and Court dispatch are native, each office can run its own model, and every feature is switchable in YAML or JSON with switched-off law always reported. Ships the board (eight responsive pages), MCP (stdio and HTTP), Claude Code / OpenCode compatibility for instructions, skills, commands and agents, and a Harbor benchmark adapter.
