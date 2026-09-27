# Changelog

All notable changes to __TITLE__ are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org).

## [Unreleased]

## [0.2.0] - __DATE__

### Highlights

**A terminal in the class of Claude Code**, on Charm v2: the mascot drops in; the model's thinking
streams and folds; each rank has a face and its own verbs, and the domain owner reviews the change at the gate;
tool calls are class-coloured cards and diffs keep the code's colours; `ctrl+t` watches every agent,
live; `/board` full screen with the ownership graph; a `ctrl+k` palette and toasts; the
window title and tab progress follow the session; the transcript reprints on resize.

**Safety in code**: a global dangerous-command guard refused before any gate (151-case corpus;
`guard install` wires it into Claude Code and OpenCode); the gate runs the pre-turn check lines, keeps
the tests intact, and sends a failed gate back to the model once.

**New commands**: `goal`, `review`, `handoff`, `board --ssh`, `--containered`, a setup form at `init`;
config one file per section (`guards.yaml`, `rules.yaml`…).

**Fixes**: an upstream `finish_reason: "error"` is a failed (retryable) call, not an answer; a TUI that
cannot start says why; a piped slash command's turn can no longer be skipped by the next line.

## [0.1.3] - __DATE__

### Highlights

Works with Google Gemini's OpenAI-compatible API, free tier included: each tool call's `thought_signature` travels back on the next turn (multi-step tool use failed before), 429 and 5xx answers are retried with backoff (honouring `Retry-After`), and Gemini's array-form errors are shown instead of "unreadable body". The orchestrator answers you in plain language; only dispatched subagents answer on the wire. HTTP messages no longer say "agent" where they mean the request or response body.

## [0.1.2] - __DATE__

### Highlights

`run "<task>"` no longer reads stdin — an inherited pipe that never closes (CI, another agent, cron) could hang it; stdin is read only for `run` with no task or `run -`. Background jobs killed by a timeout no longer print a stray "Killed" line into the next command's output. The dashboard's off-list now names switched-off tools (it said "everything on" while a tool was off), long origins wrap inside their card, and `memory status` reads "working N note sets".

## [0.1.1] - __DATE__

### Highlights

A patch release for two defects found by running the released binary in a plain workspace.

### Fixed
- A machine-wide agent definition named `orchestrator` could be imported as a member, so the session would run with that member's role, model route and tools. The orchestrator's name is now reserved.
- The tool registry's load hint named a JavaScript tool that is not shipped; it now names `agent-one toolbox`.
- The dashboard's theme variables and the default model (`anthropic/claude-opus-5`) match the tested build.

## [0.1.0] - __DATE__

### Highlights

First release: a coding-agent harness that enforces its policies in code — the review gate with its verdict in `log.md`, human approval with per-command permission rules, a `bwrap`-sandboxed persistent shell, and native memory with context compaction by drain. Roles and subagent dispatch are native, the analyst, judge and drafter roles can each run their own model, and every feature is switchable in YAML or JSON with disabled policies always reported. Ships the dashboard (eight responsive pages), MCP (stdio and HTTP), compatibility with Claude Code and OpenCode instructions, skills, commands and agents, and a Harbor benchmark adapter.
