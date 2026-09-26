# Changelog

All notable changes to __TITLE__ are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org).

## [Unreleased]

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
