# board — the workspace, seen

`board.New(Options{WorkspaceRoot, WorkspaceDir, Names, Sources})` is an `http.Handler`; `board.Serve(ctx,
addr, opts)` listens until the context ends. Everything a page needs is embedded (Bootstrap
5.3.8 under `assets/`, the theme in `static/board.css`, the page script in `static/board.js`,
`html/template` pages under `templates/`), so a page works offline.

Pages, one question each: Overview · Subagent · Usage · Team · Memory · Toolbox · Log · Config.
Labels come from `Options.Names` (keys in `DefaultNames`); a distribution passes its own words.

## Feeds

`Sources` is a struct of functions; a nil field falls back to the file-backed reader
(`FileSources`). The integrator wires what files cannot know:

| field | answers | default |
|---|---|---|
| `Subagent() []Agent` | the live agents (rank, role, model, state, started, context, usage, usd) | silent |
| `Config() any` | the effective config; rendered generically as a tree, a `{value, origin}` map is a leaf with its origin badge | silent |
| `Off() []Off` | policy features disabled by config, with the origin | silent |
| `Session() Session` | the running session | the last session in the usage journal |
| `Usage`, `Loop`, `Log`, `Team`, `Memory`, `Toolbox`, `Doc` | files: `instruments/usage/*.jsonl`, `instruments/loop/*.jsonl`, `log.md`, packages `onto`, `memory`, `toolbox` | file-backed |

Every figure carries a `Reading` (source, last time, lit or silent, why). A silent or missing
instrument renders as "silent since …" or "never lit", never as zero.

## The usage journal

`<workspaceDir>/instruments/usage/<session>.jsonl`, one JSON object per provider call:

```json
{"ts":"2026-09-26T08:00:00Z","session":"s-2026-09-26","agent":"zone-auth","rank":"zone",
 "role":"findings","model":"claude-opus-5","provider":"anthropic",
 "input":800,"output":900,"cacheRead":0,"cacheWrite":0,"usd":0.0159}
```

| field | type | meaning |
|---|---|---|
| `ts` | RFC 3339 | when the call returned |
| `session` | string | session id (defaults to the file name) |
| `agent` | string | the agent that made the call (`orchestrator`, `zone-auth`, …) |
| `rank` | string | its rank (`orchestrator`, `coord`, `domain`, `zone`, `service`, or a config rank) |
| `role` | string | `findings` · `verdict` · `draft` · `""` |
| `model`, `provider` | string | as resolved for the call |
| `input`, `output`, `cacheRead`, `cacheWrite` | int | tokens as the provider reported them |
| `usd` | number or `null` | priced from config; `null` = unpriced (shown as tokens, never a guessed cost) |

Unreadable lines are skipped and counted in the reading. Rollups (by agent, role, rank, model,
day) treat `null` as unpriced: the USD column is a floor and says how many calls it misses.

## Live

`/events` is server-sent events: `usage`, `loop`, `log` (a file changed under the workspace dir)
and `subagent` (the live feed's JSON changed; the payload carries the agents). A page declares
which events it patches on (`data-live`) and refetches itself with `?partial=1`.

## Theme

`static/board.css` layers CSS custom properties over `--bs-*`: rank tokens (`--r-zone`,
`--r-domain`, `--r-coord`, `--r-auditor`, `--r-principal`, `--r-service`), lane tints (`--l-zone`,
`--l-review`, `--l-global`, `--l-shared`) and chart series (`--c-1..4`), dark and light each
re-stepped, `data-bs-theme` following the system unless the reader picks. The rank and lane
hexes are board.js's audited tokens carried over; `--c-2` (dark) is re-stepped to `#b3841f`
so the four chart series pass every palette-audit gate in both modes.
