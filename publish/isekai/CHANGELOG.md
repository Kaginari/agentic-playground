# Changelog

All notable changes to __TITLE__ are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org).

## [Unreleased]

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
