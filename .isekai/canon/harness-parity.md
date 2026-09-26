# Harness parity — what a daily driver must ship

`binary.md` says what the `isekai` binary is: the law as a harness, one engine, two distributions
(`isekai`, `agent-one`) that differ only by lexicon. This file is the capability inventory that
makes the harness *usable* — the floor a person expects because Claude Code and OpenCode both
clear it — and, for each capability, the smallest version this binary ships and where the law
touches it. `config.md` is the switchboard for everything named here.

Two sources are cited. **CC** is Claude Code as installed on this machine (`~/.claude/`, this
repo's `.claude/`). **OC** is OpenCode, cited as paths under the read-only clone
(`packages/opencode/src/…`, `packages/core/src/…`). Neither is copied; both are read for the
shape a user's hands already know.

**The ruthless rule.** A capability is MUST only if a day of real work without it means reaching
for another tool. Everything else is LATER, and a LATER item enters only when a session names the
need twice (Nature 4). No TUI, no LSP, no web UI, no plugin runtime, no share, no IDE bridge.

## The inventory

| Capability | CC | OC | Ours (minimal) | v0 | Law |
|---|---|---|---|---|---|
| **read** | Read: offset/limit, 2000-line default, images, PDFs | `tool/read.ts:14-34` — offset/limit, 2000 lines, 50 KB cap, 2000-char lines | text files, offset/limit, 2000 lines / 50 KB; a directory read lists it | MUST | class `read`; a path outside the world is `outward` (`isekai/tool/builtins.go:55`) |
| **write** | Write: whole file, must Read first | `tool/write.ts` — permission `edit`, external-dir check | whole file; refuses a record (`log.md`, `isekai.md`, canon, `notes.jsonl`) as `destructive` | MUST | Vitality check 4 at turn end; territory refusal (Law 2); a record overwrite is Law 6 |
| **edit** | Edit: exact old→new, `replace_all`, must Read first | `tool/edit.ts:48-58` — `oldString/newString/replaceAll`, fuzzy matchers `edit.ts:682-715` | exact match, `replaceAll`, refuse ambiguous (n>1 without `replaceAll`); no fuzzy pass | MUST | same as write; the edit is gated by the class of its *path*, never of its text |
| **multi-edit / patch** | none (Edit ×N) | `tool/apply_patch.ts` — unified-patch tool with per-file permission (`:204-207`) | none: N edits in one turn are N calls | LATER | — |
| **bash** | Bash: `timeout` (ms, max 600 s), `run_in_background`, persistent cwd | `tool/shell.ts:434-618` — timeout, workdir, kill on overrun; `tool/shell/prompt.ts:18` | `command`, `cwd`, `timeoutMs` (default 120 s, max 600 s), `background: true` returns a job id, output tail to `.isekai/tmp/jobs/<id>` | MUST | per-command classifier (`isekai/tool/classify.go:116`) — `read`/`write`/`outward`/`destructive`; outward and destructive always ask; cwd outside the world is outward |
| **glob / grep** | Glob, Grep (ripgrep, output modes, context) | `tool/glob.ts:49` and `tool/grep.ts:63-86` — ripgrep, 100-hit cap | Go stdlib walk + regexp; 100-hit cap; `-n` lines; respects `.gitignore` roughly | MUST | class `read`; results are pointers (`path:line`), never payloads — the compass, point 1 |
| **tool-output overflow** | truncates in place, offers path | `tool/truncate.ts:14-15` — 2000 lines / 50 KB, rest to disk, 7-day retention | same limits; the rest to `.isekai/tmp/tool-out/<id>`; the tool returns head + path | MUST | `@DUMP` — overflow travels by reference (Absolute Rule II) |
| **web fetch** | WebFetch (markdown, 15-min cache) | `tool/webfetch.ts:9-50` — 5 MB, 120 s, text/markdown/html | GET only, 5 MB, 60 s, text as-is | MUST | always `outward` (Nature 7): one gate question per host per session |
| **web search** | WebSearch | `tool/websearch.ts`, provider-gated (`tool/registry.ts:58`) | none | LATER | outward |
| **todo / working memory** | TodoWrite | `tool/todo.ts:14-29` — per-session list | the **desk**: `.isekai/instruments/desk/<session>.md`, ~5 dated thoughts; the model writes it with `desk` | MUST | Nature 5 — it *is* working memory; compaction pass 4 writes it; over ~5 is a stress reading |
| **ask the user** | AskUserQuestion (options) | `tool/question.ts:14-41` — questions with options | `question`: free text or options, on the TTY; in `run` with no TTY it returns `@?` and ends the turn | MUST | Absolute Rule III — Rimuru asks Veldora rather than guess; the same prompt the human gate uses |
| **subagent dispatch** | Agent tool: `subagent_type`, background, fork, resume by id | `tool/task.ts:56-62` — `subagent_type`, `background`, `task_id` resume; depth limit `task.ts:111` (`subagent_depth`) | `dispatch`: body name, commission in the wire, foreground; depth 1 | MUST | a **Court Body** (`binary.md` §Ranks): fresh context, tools cut to rank + territory, one wire report back, `@U` required or flagged |
| **project instructions** | `CLAUDE.md` walk-up (project + parents), `~/.claude/CLAUDE.md`, `@imports`, `.claude/rules/` | `session/instruction.ts:60-68,123-133` — `AGENTS.md`, `CLAUDE.md`, `~/.config/opencode/AGENTS.md`, `~/.claude/CLAUDE.md`; first project match wins | the **law loader** first (crest always, code sections on demand), then `AGENTS.md` / `CLAUDE.md` walk-up, then the global pair; nearest wins, no stacking | MUST | the crest is not an instruction file: it is loaded even when instruction discovery is off |
| **skills** (two-tier) | `.claude/skills/<n>/SKILL.md`, `~/.claude/skills`, plugin skills; description resident, body on invoke | `skill/index.ts:21-25,170-215` — `.claude/skills`, `.agents/skills`, `.opencode/skill(s)`, config `skills.paths`; `tool/skill.ts` loads by name | the **toolbox**: registry over every skill dir (native + CC + OC), level 1 `@T` manifest under a budget, level 2 `skill <name>` loads | MUST | §Minds & Bodies — a Mind's body never enters a prompt uninvited; every load journaled |
| **slash commands** | `.claude/commands/*.md` (+ `~/.claude/commands`), frontmatter, `$ARGUMENTS`, `$1..`, `@file`, `` !`cmd` `` | `config/command.ts:15-24` — `{command,commands}/**/*.md`; `command/index.ts:36-44` hints `$N`/`$ARGUMENTS`; `config/markdown.ts:5-6` `@file`, `` !`cmd` `` | `/name args`: template + `$ARGUMENTS` + `$N`; `@file` inlines a pointer; `` !`cmd` `` LATER (it is a bash act) | MUST | a command is a Mind worn for one turn; `!` shell is classified like any bash |
| **subagent definitions** | `.claude/agents/*.md`: `name`, `description`, `tools`, `model` | `config/agent.ts:11-32` — `{agent,agents}/**/*.md`; `core/src/v1/config/agent.ts:12-41` — `mode`, `model`, `permission`, `steps` | creature docs `.isekai/{elf,orc,slime}/<n>/` are the native bodies; CC/OC agent files are read as bodies too (`mode` default `subagent`) | MUST | a Body is minted named for a rank; territory from its doc; one Keeper per world |
| **permissions** | `permissions.allow/deny/ask` as `Tool(pattern)`, `defaultMode`, settings ladder | `permission/index.ts:28-38` — rulesets, last match wins, default `ask`; `agent/agent.ts:119-136` per-agent defaults; `subagent-permissions.ts` | the **class gate** is the floor; rules overlay it: `deny` and `ask` only tighten; `allow` on `write` silences the strict prompt; `allow` on `outward`/`destructive` is an explicit pre-approval and must be a pattern, never `*` | MUST | §The loop — nothing auto-approved, a declaration only tightens; CC's `Bash(git push:*)` in this repo's `.claude/settings.local.json` is exactly such a pre-approval |
| **hooks** | `settings.hooks`: PreToolUse, PostToolUse, Stop, SessionStart, UserPromptSubmit, PreCompact…; shell commands, JSON on stdin, exit 2 blocks | plugin hooks `packages/plugin/src/index.ts:261-334` — `permission.ask`, `tool.execute.before/after`, `chat.message`, `session.compacting`… (JS, not shell) | shell hooks only: `preTool`, `postTool`, `sessionStart`, `stop`, `preCompact`; JSON on stdin, exit 2 = block with stderr as reason | MUST (pre/post tool), LATER (rest) | a hook is a machine mouth: its stdout may speak the wire (`@F`, `@?`) and is folded into the journal, never into the prompt raw |
| **MCP client** | `.mcp.json` (project), `claude mcp add` (user), stdio + http/sse, OAuth | `mcp/index.ts:342-391` local (stdio) and `:238-286` remote (streamable-http, SSE, OAuth); config `core/src/v1/config/mcp.ts` | stdio JSON-RPC only; tools listed into the toolbox as `external`; each call classified `outward` unless the server is marked `inward` in config | MUST (stdio), LATER (http, OAuth, prompts, resources) | Nature 7: an MCP server is a mouth outside the world unless the human says otherwise in config |
| **sessions** | `~/.claude/projects/<slug>/<id>.jsonl`; `--continue`, `--resume`, `/resume` picker | SQLite (`session/session.ts:11`), `cli/cmd/session.ts:45-81` list/delete, `run --continue/--session/--fork` (`cli/cmd/run.ts:147-160`), export/import | JSONL, one file per session under the data dir, per world; `session list`, `run --continue`, `run --session <id>`; no fork, no export | MUST | files are truth: the transcript is readable without the binary; the loop journal is separate and append-only |
| **context compaction** | auto-compact near the window, `/compact [focus]`, PreCompact hook — a model-written summary | `session/compaction.ts:28-33,160-242` — summary by a `compaction` agent; `overflow.ts:8-34` triggers at window − reserved; `prune` clears old tool outputs (`:114-156`) | the **drain** (`binary.md` §Compaction): pointerize → trim spent → surface the unsaid → write the desk → record the episode → verify; `summary` strategy as fallback | MUST | Nature 5 + §The unsaid: a summary loses the unsaid; a drain sends each piece home. Trigger is a reading (threshold), never a provider overflow error |
| **non-interactive + JSON** | `claude -p "…"`, `--output-format text\|json\|stream-json`, `--max-turns` | `opencode run "…"`, `--format json` (`cli/cmd/run.ts:15-18,170-175`), stdin piping | `run "<ask>"`: stdin appended; `--format text\|json\|wire`; `--max-steps`; exit codes = loop's (0 done, 4 denied, 5 budget, 6 `@?`) | MUST | the wire is a first-class output format; toward a human the default stays prose |
| **streaming** | token streaming in TUI and `stream-json` | SSE event bus, `run` streams parts | text deltas to stdout as they arrive; tool calls printed as one line each; JSON mode emits one event per line | MUST | — |
| **providers / models** | Anthropic (+ Bedrock, Vertex) | `provider/`, models.dev catalog, many auth plugins (`plugin/index.ts:67-86`) | `anthropic`, `openai`-compatible (OpenRouter, Ollama, any base URL), `mock`; model as `provider/model`; keys from env only | MUST | the provider call is the one standing outward act; a key never enters a file the world tracks |
| **plan mode** | Shift+Tab / `--permission-mode plan`: read-only tools, plan file, exit prompt | `agent/agent.ts:156-181` — `plan` agent denies edit except `.opencode/plans/*.md`; `tool/plan.ts` exit asks the user | `--mode plan` = a permission preset: every `write` denied except `.isekai/tmp/plans/*.md`; leaving plan mode is a `question` | MUST (preset), LATER (plan file UX) | plan mode is the Elf's seat — routing written down before a body runs |
| **file snapshots / undo** | `~/.claude/file-history/<session>/<hash>@vN`, `/rewind` | `snapshot/index.ts` — shadow git dir per worktree, `session/revert.ts` revert/unrevert | before every write/edit the prior bytes go to `.isekai/tmp/undo/<session>/<hash>@vN`; `undo [path]` restores the last version; git is the real record | MUST (copy + undo), LATER (diff view, per-turn revert) | Law 6 — an undo is a write of an old version, not a history rewrite; records are still refused |
| **doom-loop check** | none | `session/processor.ts:29,356-373` — same tool+args 3× asks `doom_loop` | same call three times in a row is an instrument reading: the turn pauses with `@?` | LATER (cheap) | Nature 9 — stress is measured, not felt |
| **background jobs** | `run_in_background`, notifications | `background/`, `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS` (`tool/task.ts:98`) | bash `background` only; dispatch is foreground | MUST (bash), LATER (dispatch) | — |
| **LSP / diagnostics** | IDE bridge | `tool/lsp.ts`, `lsp/` | none | OUT | — |
| **TUI / web / share / IDE / ACP** | full TUI, IDE extensions | `cli/tui`, `server/`, `share/`, `acp/`, `ide/` | line REPL only | OUT | §Principles of the build |

## Reading the table

- **Ours (minimal)** is a spec, not a wish: each cell is what the Go package must do and no more.
  Where CC and OC differ, the smaller shape wins unless the larger one is what the law needs (the
  drain over the summary; the desk over the todo list).
- **v0** is the first daily-driver build. LATER items are named here once; the second naming — a
  session hitting the wall — is the birth (Nature 4).
- **Law** is the column that makes this harness different from the two it learned from: in CC and
  OC the law is a document the model may read; here every row's law cell is a code path.

## What the two harnesses do that this one refuses

- **Auto-approve modes** — CC's `--dangerously-skip-permissions`, OC's `--auto` / `--yolo`
  (`cli/cmd/run.ts:83-97`). There is no flag that silences the gate for `outward` or `destructive`.
  The lawful equivalents are `--approve <class>` per run and pattern pre-approvals in config,
  both logged as such; `--dry-run` says what would be asked.
- **A private store** — OC's SQLite, CC's opaque state dirs. Every file this binary writes is
  plain text a person can read, and the ones that matter live in the world.
- **A summary as memory** — see the compaction row.
- **A generic subagent** — CC's `general-purpose`, OC's `general` (`agent/agent.ts:182-195`).
  Every dispatched body is named for a rank and cut to a territory.

## Compatibility promises

The harness reads what the other two write, so a world set up under CC or OC keeps working:

| Written by | Read as | Where |
|---|---|---|
| `CLAUDE.md`, `AGENTS.md`, `~/.claude/CLAUDE.md`, `~/.config/opencode/AGENTS.md` | project instructions | `discovery.instructions` |
| `.claude/skills/*/SKILL.md`, `.opencode/skill(s)/*/SKILL.md`, `.agents/skills`, `~/.claude/skills`, `~/.config/opencode/skill(s)` | Minds | `discovery.skills` |
| `.claude/commands/*.md`, `.opencode/command(s)/*.md`, the global pair | commands | `discovery.commands` |
| `.claude/agents/*.md`, `.opencode/agent(s)/*.md`, the global pair | Bodies (`mode` default `subagent`) | `discovery.agents` |
| `.claude/settings.json` `permissions.allow/deny/ask` (`Tool(prefix:*)`), `.claude/settings.local.json` | permission rules | `permissions.import.claudeCode` |
| `opencode.json(c)` `permission` block | permission rules | `permissions.import.opencode` |
| `.mcp.json` (CC project scope), `opencode.json(c)` `mcp` block | MCP servers (stdio only in v0) | `mcp.import` |

Imports are read-only translations: the binary never writes to another harness's files. Every
import is a switch in `config.md`, and a switched-off import is shown in `status` like any other
switched-off instrument (Nature 9).
