# The isekai binary — the law as a harness

`isekai` is a single Go binary that runs an agent session on a world. The convention was written
for harnesses that only *read* it (Claude Code, OpenCode); a model can read a law and still skip
it. Here the law is the harness: the gate, Vitality, the log, the human gate, the instruments and
the wire are code paths the model cannot route around, and every creature is a native type.

Source: `isekai/` at the repository root (Go module; stdlib plus the Charm libraries for the
terminal UI, `canon/tui.md`). Build with
`go build -o bin/isekai ./cmd/isekai` from `isekai/`. Toolchain: Go ≥ 1.23.

## Principles of the build

- **Minimal.** Standard library only — except the terminal UI, built on the Charm libraries
  (`canon/tui.md`). No LSP, no plugin system. A feature enters when a session names the need twice
  (Nature 4).
- **Files are truth.** The binary reads and writes the same files the JS instruments do
  (`log.md`, `memory/`, `toolbox/`, `instruments/`) in the same formats. It never keeps a private
  store the world cannot read without it.
- **The law is enforced, not recited.** Every rule the binary can check, it checks in code; what
  it cannot check it puts in front of the model at the moment it applies, not once at start.
- **Containment by construction.** Nothing reaches outside the world except the model provider
  call itself and actions that passed the human gate.

## Packages

| Package | Holds | Law it carries |
|---|---|---|
| `provider` | one `Provider` interface (messages + tool calls + usage); `anthropic` (Messages API), `openai` (chat-completions — also OpenRouter, Ollama, any compatible endpoint), `mock` (scripted, for tests) | the provider call is the one standing outward act |
| `tool` | `Tool{Name, Description, Schema, Class, Run}`; built-ins `read`, `write`, `edit`, `bash`, `glob`, `grep`, `dispatch` | every tool declares a class; `bash` is classified per command (port of `loop.js`'s classifier) — a declared class only tightens |
| `gate` | the human gate: the TUI's choice block or the TTY prompt (`Gate.Answer` is the seam), `--approve <class>` pre-approval, `--dry-run` | Nature 7, Law 6: outward and destructive always ask; nothing auto-approved; a denial stops the turn |
| `loop` | the turn engine: perceive → recall → plan → act → verify → record per tool step; journal to `.isekai/instruments/loop/<run-id>.jsonl` | §The loop — budgets are readings; past one, checkpoint and stop honestly |
| `instrument` | context occupancy from provider-reported usage, 200k budget / 180k stress zone; `status` board | Nature 9, §Instruments — Rimuru's stress is a reading |
| `memory` | port of `memory.js`: status, index, recall (meaning + relation rank), remember (`--kind law|colony|territory`) | §Memory tiers — same files, same formats |
| `toolbox` | port of `toolbox.js`: registry build, `brief` (level-1 `@T` manifest under a budget), `load` (level 2, journaled) | §Minds & Bodies — the toolbox |
| `wire` | parse and emit the envelope (`@S @F @V @? @U @E`, `@ROOT @SCOPE @ASK @CAP @DUMP`); `@CAP` enforced | Absolute Rule II, Nature 8 |
| `world` | world discovery, the law loader (crest always; code sections on demand), ranks and bodies from `.isekai/{elf,orc,slime}/`, `log.md` append-only writer, the Orc gate and the Vitality check, Court dispatch | Natures 1–4, the gate, Laws 2–4 |
| `onto` | the ontology: schema, graph, reasoning, validation, projection | §The ontology below |
| `config` | the switchboard (`canon/config.md`): layers, origins, refusals, permission rules, models per office/rank/task, ranks as data | the honesty rule; a disabled law is a finding |
| `compact` | the drain: pointerize, trim, the unsaid, the desk, the episode, verify | §Compaction below |
| `sandbox` · `shell` | bwrap containment and env scrubbing; one persistent bash per body with background jobs | Nature 7, §Bash below |
| `mcp` · `discover` | the MCP client (stdio, streamable HTTP); what other harnesses wrote (instructions, Minds, commands, Bodies, MCP imports) | §MCP below; `harness-parity.md` |
| `board` | the world, seen: an HTTP handler over the same instruments | §The board below |
| `app` | the one engine behind both binaries: config → world → providers, shelf, MCP, discoveries, hooks; sessions (JSONL), the live session (the TUI on a terminal, the line REPL on a pipe or `--plain`), `run`, `bench`, `selftest`, `init`, `board` | everything above, wired |
| `tui` | the terminal UI (`canon/tui.md`): the view model (blocks from the loop's events), the Bubble Tea program, the choice block the gate and the `ask` tool answer through | the one siphon that never narrows — the human's |
| `cmd/isekai` · `cmd/agent-one` | the two distributions: one `main` each, differing only by the name they hand `app.Main` (lexicon, world dir, env prefix, law file follow) | — |

## Bash — the model's shell, the world's rules

- **Anthropic-defined tools on Anthropic models.** With an `anthropic` provider the binary declares
  Claude's own trained tools — `{"type":"bash_20250124","name":"bash"}` and
  `{"type":"text_editor_20250728","name":"str_replace_based_edit_tool"}` — schema-less, so the model
  drives the shell and editor it was trained on. With an `openai` provider (OpenRouter, Ollama,
  vLLM) the same executors sit behind custom-schema `bash` / `edit` tools. The model chooses the
  words; the binary always executes.
- **One persistent session per body.** The shell keeps its cwd and exported variables between
  calls; `{"restart": true}` resets it. A Court Body gets its own session and loses it with its task.
- **Sandboxed by default.** Each command runs under `bwrap`: the filesystem is read-only except the
  world root and a private `/tmp`, no network unless the command passed the human gate as
  `outward`. No `bwrap` on the machine → `sandbox: none`, reported as `@?` in `status`.
- **The whole binary in a container — `--containered`.** The binary re-runs itself under Docker
  (`docker run --rm --init`): the world mounted read-write at its own path, this binary and what a
  session reads from the host mounted read-only (`~/.config/<dist>`, `~/.<dist>`, the Claude and
  OpenCode instruction/skill/command/agent dirs, git's identity — only those that exist), the
  session store `~/.local/share/<dist>` read-write (sessions must save), `$HOME` otherwise an empty
  tmpfs (no `~/.ssh`, no other secrets), the caller's uid, the host network (the model API and the
  board). Keys cross by name (`-e NAME`), never by value: the terminal's variables, the dist's own
  `<PREFIX>*`, every `*_API_KEY` / `*_BASE_URL`. The image is built on first use from the Dockerfile
  embedded in the binary (debian slim + bash, git, ripgrep, python3, jq, make, curl…), tagged by the
  Dockerfile's hash so a changed runtime is rebuilt; `--image <ref>` runs a richer one. Inside,
  `<PREFIX>CONTAINERED=1` makes the flag a no-op, and `status` says `container: docker <image>` in
  place of the sandbox line: the container is the boundary, bwrap does not nest in it.
- **Classified before it runs.** The classifier settles the class; permission rules and the human
  gate decide; the model's own claim only tightens. `git` is read through its global options
  (`git -C . push`, `git -c k=v push`, `git --no-pager push` are the verb's class). A path is
  read for where it points: a symlink out of the world, and a file yet to be created under
  one, are outward for `read`, `write`, `edit` and `patch` alike; the editor refuses them.
- **Secrets never cross.** The command's environment drops `*_API_KEY`, `*_TOKEN`, `*_SECRET`,
  `*_PASSWORD` and every provider's `apiKeyEnv`; config holds an allow-list.
- **Nothing outlives its call unannounced.** Each command runs in its own process group; a timeout
  kills the group. `background: true` starts a named, logged background job with its own output
  file; jobs are listed in `status` and killed when the session ends.

## The default toolset — everything on, config takes away

The binary ships the tools a daily-driver harness needs, all enabled by default (profile `max`);
config disables, never has to discover:

| Tool | Class | Does |
|---|---|---|
| `bash` | classified per command | persistent sandboxed shell; `background` jobs |
| `read` · `ls` · `glob` · `grep` | read | files, directories, patterns, content search |
| `write` · `edit` · `multiedit` · `patch` | write | create, `str_replace`, several replaces atomically, apply a unified diff |
| `git` | classified per subcommand | `status`/`diff`/`log`/`show` read; `commit` write; `push` outward; `reset --hard` destructive |
| `webfetch` · `websearch` | outward | fetch a URL as text; search via a configured backend |
| `ask` | read | ask the human a question with options, answer returned to the model |
| `dispatch` | per rank | a Court Body (§Ranks and bodies) |
| `recall` · `remember` | read · write | the memory tiers |
| `toolbox` | read | search the registry, load a skill/command/tool body (level 2) |
| `onto` | read | query the creature graph: who owns a path, what a creature may see |
| `skill` | read | don a Mind by name |
| MCP tools | declared by server, classified like any other | every configured MCP server |

Custom tools from config (`tools.custom`) join the same table on the same terms.

## MCP

The binary is an MCP client (JSON-RPC 2.0, stdlib): **stdio** and **streamable HTTP** transports;
OAuth for remote servers comes later — until then a remote server authenticates by header from
an env var.
- **Servers** come from `mcp.servers` in config, and — each behind its own `mcp.import` switch —
  from Claude Code's `.mcp.json` and OpenCode's `opencode.json(c)` `mcp` block, read as-is.
- **Tools** are named `mcp__<server>__<tool>` and enter the toolbox registry as level-1 entries:
  the model sees names and one-line descriptions under the budget, never fifty full schemas; a
  schema is sent to the model once the toolbox picks it for the turn.
- **Classes.** A server is `outward` unless config marks it `inward` (Nature 7 — it is a mouth
  outside the world). Within that, MCP tool annotations only tighten: `destructiveHint` →
  destructive, `openWorldHint` → outward; `readOnlyHint` lowers nothing on its own — only config
  may (`mcp.servers.<s>.tools.<t>.class`). Every call passes the same permission rules and gate.
- **Containment.** A stdio server runs under the same sandbox and env scrubbing as `bash`, with
  its own `env` / `envAllow` and a `network: true|false`; a server that fails to start or dies is
  a `@?` in `status`, and its tools report the fault instead of vanishing.
- **Resources and prompts.** Resources are readable through the `read` tool as
  `mcp://<server>/<uri>`; prompts become slash commands `/<server>:<prompt>`.

## When a tool is missing

The model never meets a silent failure, and never loops on one:
1. **Unknown or disabled tool called** → one error result naming why (`disabled by
   tools.webfetch.enabled in .isekai/config.yaml:12` or `no such tool`) and the closest enabled
   alternatives. The same missing call twice in a turn stops the turn (doom-loop guard).
2. **A capability the body doesn't hold** → `toolbox` search first: the registry covers disabled
   tools, MCP servers, skills and commands too, so the model learns what exists, what is off, and
   what loading it costs.
3. **Composable from what is on** → it is composed (e.g. `ls` via `bash` when `ls` is off), under
   the same classification and gate — a disabled tool is never re-enabled by the back door: a
   `bash` call that reproduces a disabled *outward* tool is still outward and still asks.
4. **Not composable** → the need is named: a `@?` hole to the dispatcher, one hop (Absolute Rule
   III), and a shared note of kind `colony` recording the missing capability.
5. **Named twice** (Nature 4 — Genesis) → the binary proposes the tool: a drafted `tools.custom`
   entry or an `enabled: true` patch, shown to the human as a config diff. Config is the human's
   written word, so it lands only on their yes; then it reloads live.

## Open models

Open-weight models (served by vLLM, Ollama, SGLang, OpenRouter) differ from Claude in four ways
the binary adapts to, per provider in config:
- **Tool shape.** A `minimal` tool profile: few tools, flat schemas, short descriptions. Editing is
  the `str_replace` family (`view` / `create` / `str_replace` / `insert`) as a custom tool — the
  shape most open coding models met in training through SWE-agent/OpenHands-style scaffolds.
- **Tool-call transport.** `toolCalls: native` uses the server's function calling (on vLLM it
  needs `--enable-auto-tool-choice` and the model's `--tool-call-parser`); `toolCalls: text`
  asks for calls as tagged text and parses them in the binary, for servers or models whose native
  calling is broken. A malformed call gets one error result naming the fault, never a guess.
- **Wire by grammar.** Where the server supports guided decoding (`response_format` with a JSON
  schema), a Court Body's report is constrained to the envelope's schema and rendered to the wire
  by the binary — a small model cannot drop `@S` or `@U`. Elsewhere the wire is parsed and a
  malformed report is re-asked once.
- **The real window.** `contextWindow: auto` reads the served maximum (vLLM's `/v1/models`
  `max_model_len`); budgets, the stress zone and the drain trigger scale to it, so a 32k model
  drains early instead of overflowing.

## Models per office, rank and task

The throne never chooses its horse: the session (Rimuru) runs on the model the human picked
(`models.default`, or `--model`). Every other call is routed by what the task demands
(`canon/model-assignments.md`):
- **The triad is chosen by the ask.** The wire already names the office: `@ASK findings` is
  Great Sage's (perceive), `@ASK verdict` Raphael's (judge), `@ASK draft` Ciel's (speak). A
  `dispatch` runs on `models.offices.<office>`; the binary's own calls are routed the same way —
  the end-of-turn gate verdict to Raphael, log entries and the drain's desk to Ciel, recall
  summaries to Great Sage.
- **Resolution, most specific first:** creature (`models.creatures.<name>`) → office → rank
  (`models.ranks.<race>`) → task (`models.tasks.<drain|gate|log|bench>`) → `models.default`. Each
  entry is `provider/model` with optional `effort` and `fallback`. `status` shows the resolved
  model for every office and rank, with the origin of each.
- **The lineage holds.** Great Sage → Raphael → Ciel is one escalating chain: Ciel's model must
  never sit below Raphael's, nor Raphael's below Great Sage's. Providers declare a `tier` per
  model; config that breaks the order is refused at load, and a model with no declared tier is a
  `@?`, never a guess.
- **A script beats a model.** Anything deterministic (journal lines, verdict records, pointer
  tables) is code, and never reaches any office.

## Ranks and bodies, native

- The session is **Rimuru**. A creature is a record read from its doc under
  `.isekai/<race>/<name>/`: race, territory (path globs), traits, worn minds, orc (for a slime).
- A **Court Body** is `dispatch`: a fresh loop with its own context, a commission in the wire
  (`@ROOT @SCOPE @ASK @CAP`), a tool set cut to its rank and territory, its own shell, and one
  wire report back. Its context dies with the call; only what it wrote to disk and its report
  survive. A Court's writes are gated by the Court itself — the end-of-turn gate runs on its
  own account and its verdict is recorded under its own name; nothing propagates upward for a
  second gate. A report with no `@U` line is flagged to the dispatcher (the unsaid). The
  `@U` lines of a background Court are landed by the dispatcher when the report wakes it.
- **Ranks are data, the law's table is the default.** The binary carries the ranks of
  `isekai.md` §The world (Elf, Orc, Slime; Kijin; High Elf, High Orc, Dark Elf) as its built-in
  table: parent, job, whether the rank authors, whether it holds a gate, body mode, default office,
  tools, model. Config (`ranks.<name>`) overrides a field of a built-in rank or adds a new rank; with
  `rankSet: replace` the ranks listed in config are the whole hierarchy and the built-in table is
  not loaded (the default, `extend`, merges over it).
  The law's shape is checked at load and a config that breaks it is refused: the hierarchy is a
  tree rooted at Rimuru; a rank that authors has a gate-holding rank above it (Law 3); a path has
  exactly one gate holder; every rank has an escalation parent (Absolute Rule III); ascended ranks
  keep everything their base rank held. A rank from config becomes an ontology class with its
  bonds, so knowledge flows along it. `status` lists every rank that differs from the law.
- **Territory is enforced.** A Slime's `write`/`edit` outside its territory is refused, not
  warned (Law 2). Escalation is a `@?` to its dispatcher, one hop (Absolute Rule III).

## Live session — talk while the court works

- **Two faces, one engine.** On a terminal the session is the TUI (`canon/tui.md`); on a pipe,
  under `TERM=dumb`, or with `--plain`, it is the line REPL. Both run the same engine, sessions,
  Courts and gate.
- **The human is never locked out.** A turn runs in the background. A line typed mid-turn is
  queued and delivered to the running body at its next tool step (the way Claude Code does);
  `esc` (TUI) or `Ctrl-C` interrupts the turn, a second `Ctrl-C` exits.
- **A stopped turn is not a dead session.** When a turn ends on its tool results (denied,
  escalated, checkpointed), the next ask rides that user message rather than following it — two
  user messages in a row is a shape no provider takes.
- **Courts run in the background.** `dispatch` may be asynchronous: the dispatcher keeps working
  and is woken by the Court's report. A running Court stays addressable (`/send <court> <text>`)
  until its dispatcher accepts the report; then its context dies (§Minds & Bodies — the task is
  over when the report is taken).
- **The court is visible.** `/agents` (and a status line above the prompt) shows every live body:
  rank, office, model, state (thinking · tool · waiting on gate · done), elapsed, context
  occupancy against its window, tokens and cost so far. Starts and reports are announced between
  prompts, one line each (the TUI draws each Court as a block with its report rendered).
- **Consumption is an instrument** (Nature 9). Every provider call's usage (input, output, cache
  read, cache write) is journaled to `.isekai/instruments/usage/<session>.jsonl`, priced from the
  provider's per-model `price` in config (unpriced models show tokens, never a guessed cost), and
  rolled up per body, office, rank, model and session. `/usage` and `isekai usage [--session
  <id>|--since <date>]` read it; `tempest.js` can read the same file. The session's own calls
  carry no office — the throne is not an office; an office labels a dispatched Court and the
  binary's own routed calls (verdict → raphael, drain and log → ciel, recall → great-sage).
- **Budgets are readings too.** `budgets.session.tokens|usd` and `budgets.court.tokens|usd` stop a
  body honestly at the line (checkpoint, report, resume command), never mid-write.

## The gate and Vitality, in code

At the end of any turn that wrote files, before the turn is reported done:
1. **Right slime authored** — every written path maps to the territory of the body that wrote it.
2. **Traits hold** — each touched creature's `verify` commands (from its doc) run; exit codes are
   the reading.
3. **Duties done** — the commission's `@ASK` is answered (`@S` present, holes named as `@?`).
4. **Doc truthful** — a change under a territory with no change to its owning doc fails
   (Nature 1). The owning doc is the Slime's doc, else the Orc's.

The verdict (pass / fail + reason) is appended to `log.md` by the binary. A world with no orcs
records `Gate: n/a (no orcs)` and still runs check 4 against any doc it can find.

**Every write is seen, whatever tool made it.** A tool that knows its paths reports them
(`write`, `edit`, `patch`); the shell, a custom tool and an MCP server do not. So the engine
stamps the world tree — size and mtime per file, `.git` never walked, bounded at 200k files — at
the start of a turn, again after every write-class step (the difference is that step's writes
when it named none), and once more at the turn's end (a write a read-classified command slipped
in, `xargs touch`). The world dir's own instruments and records are not the model's writes and
are not stamped: `instruments/`, `tmp/`, `memory/`, `toolbox/registry.json`,
`ontology/graph/unsaid.ttl`, `log.md`. A tree past the cap is not watched and the hole is named
in the report. A turn's writes are gated once; the next turn of the same session gates only its
own. The `dispatch` step names what its Court wrote, and the dispatcher's gate leaves those paths
alone — one verdict per landing, the Court's. A background Court still writing when its
dispatcher's turn ends is the one case the dispatcher's gate may see a Court's write.

## The ontology

Knowledge passes between creatures along a typed graph of their relations. The graph is
formal (so it can be checked) and projected to terse text (so a model reads it cheaply).

- **Schema** — `.isekai/ontology/schema.ttl`, a Turtle subset: classes (`Creature` ⊃ `Rimuru`,
  `Elf`, `Orc`, `Slime`, `Kijin`; `Mind`; `Doc`; `Fact` ⊃ `Law`, `Colony`, `Territory`), properties
  with domain, range, `subPropertyOf`, `inverseOf`, transitivity, and cardinality shapes. The
  bonds are the world's own: `truth` (slime ⇒ orc), `verdict` (orc ⇒ elf), `wears` (body ⇌ mind),
  `owns` (creature → path), `knows` (creature → fact), `about` (fact → creature | path).
- **Graph** — asserted triples in `.isekai/ontology/graph/*.ttl`, plus triples *derived* at load
  from the world itself (creatures, territories, worn minds, orcs). Derived triples are never
  written back; files are truth.
- **Reasoning** — forward-chaining to a fixpoint over RDFS-style rules (subclass, subproperty,
  domain/range typing, inverse, transitive). Small, total, deterministic.
- **Validation** — shapes: every Slime has exactly one `truth` edge; every Mind is worn, or is
  shared (a host skill with no race prefix); no two Slimes own an overlapping path (Nature 2). A violation is a finding.
- **Flow** — analysis flows up: a `Fact` a Slime `knows` is visible to every creature up its
  `truth`/`verdict` chain. Wisdom flows down: a `Law` known above is visible below. A Court's `@U`
  lines become asserted `Fact`s, typed by their kind, attached to the reporting body.
- **Projection** — `isekai onto project <creature> --budget N` walks what that creature may see
  and emits one plain line per fact, nearest first, under the budget:
  `slime-auth ⇒truth orc-security · token TTL is 15m (territory)`. This is what a dispatched Court
  receives; the Turtle never enters a prompt (Tokens, not eyes — no IRIs, no prefixes).

## Compaction — drain, don't summarize

A generic harness compacts by asking the model to summarize the whole conversation into one blob:
lossy, blind to what is already on disk, and the unsaid dies with the context. Here the memory
tiers already say where every kind of knowledge lives, so compaction is a **drain**: each piece of
context moves to its home, and the new context is rebuilt from those homes by pointer.

**Trigger — a reading, never an overflow.** The context instrument crosses the configured
threshold (default: the 180k stress zone of the 200k budget, or a fraction of the model's real
window, whichever is lower). Never on a provider "context too long" error — by then it is too late
to drain honestly.

**The drain, in order** — mechanical passes first, the model only where judgment is needed:
1. **Point, don't carry** (no model). Every tool result that is a file read, grep or glob is
   replaced by a pointer: `path:line-range · sha256-prefix`. A file already on disk is never kept
   twice. Duplicate reads collapse to the latest.
2. **Trim the spent** (no model). Tool outputs of finished steps shrink to their journal line
   (exit code, verdict, tail) — the full output is already in `.isekai/instruments/loop/`.
3. **Surface the unsaid** (model, one call). The model is commissioned `@ASK findings +unsaid`
   over the turns being drained and answers in the wire. Each `@U law|colony|territory` goes home:
   a shared note (`memory remember --kind`), an ontology Fact attached to the current creature, and
   for territory a proposed edit to the Slime's doc (gated like any write). `@F` lines worth
   keeping land as shared notes.
4. **Write the desk** (same call). The working memory — at most ~5 dated thoughts: the goal, the
   plan's position, decisions taken and why, open holes (`@?`), the next step. This is the
   hippocampal buffer, not a summary of history.
5. **Record the episode** (no model). If the drained turns landed changes, their `log.md` entry is
   appended now, not at the end of the session.

**The rebuilt context**, in order: the crest · the commission (the human's ask, verbatim) · the
desk · the ontology projection for the current creature · the recall manifest for the current ask
(memory hits + toolbox `@T` lines, pointers only) · the pointer table · the last N turns verbatim.

**Nothing is lost that can be recalled.** The drained turns are indexed into the short tier's
semantic cache for this session; `recall` can bring any of them back on demand, and any pointer
can be re-read. A pointer whose file changed since (digest mismatch) is flagged, not trusted
(Nature 9).

**Verify the drain.** After rebuilding, the binary checks: the ask is present verbatim; every `@?`
open before is still present; every pointer resolves; tokens after < tokens before. A failed check
aborts the compaction and keeps the old context. Every drain is journaled with before/after tokens,
pointers made, notes and facts landed.

**Dispatch first, drain second.** In the stress zone the cheapest compaction is not to accumulate:
heavy work goes to a Court Body whose context dies with its task (§Minds & Bodies). Compaction is
for what dispatch could not avoid.

Every pass is switchable in config (`compaction.passes.*`); a `summary` strategy exists as the
generic fallback, and `status` shows which strategy and passes are live.

## The board — the world, seen

`isekai` lights the board when it starts (`ui.board.autostart`, default on; `isekai board` alone
serves it without a session). It is served by the binary itself on `127.0.0.1` (`ui.board.port`),
reads the same instruments the session writes, and updates live over server-sent events. It
supersedes `tempest.js` once it shows everything tempest shows.
- **Pages, not one scroll:** Overview · Court (live bodies, states, context, cost) · Usage (tokens
  and cost by body, office, rank, model, day) · Colony (the creature graph from the ontology) ·
  Memory (tiers, notes, unsaid by kind) · Toolbox (registry, what was offered vs loaded) · Log ·
  Config (effective config with origins, the off-list).
- **The grid is Bootstrap's.** Bootstrap 5 CSS is embedded in the binary — the 12-column grid,
  its breakpoints (`sm` 576 · `md` 768 · `lg` 992 · `xl` 1200 · `xxl` 1400), utilities and
  components — so every page is responsive from a phone to a wide screen, offline. Isekai's own
  look is a thin theme over it (colour tokens for each rank and lane, dark and light), never a
  second layout system. Charts and the colony graph are inline SVG sized by their grid column.

## Tests

Every package carries Go tests; providers are exercised through `mock`. `isekai selftest` runs
the in-binary checks the way the JS instruments' `selftest` does and answers on the wire
(`@S PASS n checks`). Interop tests run the binary and the JS tools against the same files.
