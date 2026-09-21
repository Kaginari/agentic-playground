# Toolbox — a big registry, a small prompt

The law's own sentence is `isekai.md § Minds & Bodies`: "Loading is already two-tiered — a Mind's
`description` is the only part that sits in context by default; its full body loads only when the
Mind is actually donned." This file is that sentence made into an instrument, extended past Minds
to commands, world tools, Bodies and external tools, and made budgeted and measured. The
instrument is `.isekai/tools/toolbox.js`; Nature 2 (Symbiosis) is the biology: each tool does
exactly its role in the turn that needs it, and none rides along uninvited.

## Two levels

| Level | What crosses | When | Cost | Command |
|---|---|---|---|---|
| **1 · Manifest** | name · one-line description · declared triggers · path · the price of level 2 | every turn, within a token budget (default 1,500) | *resident* ≈ bytes(name + description) / 4 | `pick`, `brief` |
| **2 · Load** | the full body — a `SKILL.md`, a command, a tool's header, a Body's agent file, an external's usage notes — whole or one section by anchor | only on the body's own decision to use the tool, never pre-emptively | *full* ≈ bytes(body) / 4 — or one section's | `load <name> [--map \| --sec N]` |

Level 1 is what a Mind's frontmatter description already is today: a pointer that sits in context.
The toolbox makes that pointer **selected** (only the entries that fit the ask), **budgeted** (the
manifest never exceeds a known token count) and **priced** (every pointer says what level 2 would
cost). Level 2 is what donning already is: the body loads on demand — and every load is
journaled, so the pattern has an honest measure: offered against loaded.

## The registry — derived, never hand-maintained

`toolbox.js index` harvests everything a body could pick up into `.isekai/toolbox/registry.json`
(gitignored, rebuilt at will — files are the truth, the index is derived, as `memory-tiers.md`
draws the boundary):

| Kind | Source | Description from | Level 2 body |
|---|---|---|---|
| `mind` | `.claude/skills/*/SKILL.md`, `.opencode/skill(s)/*/SKILL.md` | frontmatter `description` | the whole `SKILL.md`, section-addressable |
| `command` | `.claude/commands/*.md`, `.opencode/command(s)/*.md` | frontmatter `description`, else the `# title` | the whole command file |
| `tool` | `.isekai/tools/*` | the leading comment block's first paragraph | the leading comment block (the header, not the code) |
| `body` | `.claude/agents/*.md`, `.opencode/agent(s)/*.md` | frontmatter `description` (+ `mode`, `model`) | the agent file |
| `external` | `.isekai/toolbox/extra.jsonl` — hand-written, tracked | the line's `description` | the line's `usage` notes |

A thing installed in two homes (a Mind in both `.opencode/skills/` and `.claude/skills/`) is one
entry with two `srcs`, never two entries (Nature 2). Each entry carries:

- **triggers** — the phrases that name this tool outright: frontmatter `triggers:` / `when:` when
  present; else the phrases the description itself quotes (`"review since X"`, `"check contrast"`);
  else the description's first sentence. The entry's name is always a trigger (`memory.js` →
  `memory.js`, `memory`; `code-review` → `code-review`, `code review`).
- **serves** — whom it serves, derived the law's way from real wearers, never hand-assigned: a Mind
  named in a creature doc is worn; wearers all Slimes → `zone`, all Orcs → `verdict`, Elves or
  Kijin → `global`; mixed or nobody → `shared`. Commands, tools and externals are `shared`; a Body
  serves its own rank. A race-prefixed Mind (a creature's hat) serves its race's lane.
- **cost** — `resident` (level 1) and `full` (level 2), both ≈ bytes / 4: an estimate, the same
  one `context-check.sh` and tempest's chip use, not a billing figure. Known **before** injection —
  that is the whole point: a body decides to load knowing the price.
- **installed** — for externals, measured at index time (the `path` exists, or the name is found on
  `PATH`), never believed from the file's own claim; a claim that disagrees is a `@?`.

**Staleness.** The registry records the mtime of every source file it read. Any source changed,
appeared or vanished since the build → `@? registry older than its sources` on every command that
reads it (Nature 9: a document's claim about current state is a memory, not a fact). No registry on
disk → answered from a live harvest and named as a hole. Written atomically (temp + rename).

## The pick — meaning, trigger, relation, then the budget

`toolbox.js pick "<the turn's ask>" [--as <body>] [--budget <tokens>] [-k N] [--kind …] [--min <score>]`

1. **Meaning** — BM25 over name + description + triggers, `memory.js`'s scorer unchanged: a real
   token vocabulary, normalised 0..1 as the share of the ask's ideal match, so a 3-word description
   and a 500-word one compete fairly. Local, model-free, nothing leaves the world (Nature 7).
2. **Trigger** — a trigger phrase present in the ask (every content word of it, lightly stemmed so
   "render" meets "rendering"): **+0.8** for a declared trigger (frontmatter or quoted), **+0.5**
   for the name, **+0.4** for a first-sentence trigger; +0.05 per further hit, at most +0.15. The
   strong signal: an explicit trigger is the author's own declaration, and it surfaces the entry
   even at zero similarity.
3. **Relation** (`--as <body>`) — `memory.js`'s typed bonds, read off the creature docs'
   `Reports to:` / `Territory:` lines: worn by the asker +0.15 (else the entry names the asker
   +0.10) · serves the asker's lane +0.08 · a Body that is the asker's parent or child +0.10 (else
   names one +0.05) · names a path in the asker's zone +0.08 — capped at 0.3. Relation nudges and
   breaks ties; it never surfaces an entry alone. Rimuru gets none: the throne wears everything.
4. **Fit** — one word in common is not a fit. An entry is offered only on a trigger hit or on at
   least two of the ask's content words (any one word when the ask has fewer than three). `--min`
   adds an optional score floor. `-k` caps the count (default 5); it never fills — an ask that fits
   one tool gets one `@T` line.
5. **Budget** — the ranked list fills a token budget (default 1,500, `--budget`) greedily by score:
   each entry's `resident` cost is added if it fits, skipped if not. What fits the turn but not the
   budget is a `@?` naming the entries and their costs, never a silent drop. A budget cut therefore
   drops the lowest-scoring picks, not the cheapest ones.

Every pick is journaled as an *offer* (names, resident cost, budget) in the loads instrument.

## The tags — `@T`, `@TOOLS`

Dialect tags of this world, born in the journal entry that first rides them; they graduate into the
law's register when they serve across two worlds or three sessions, per the register's own rule.

| Tag | Meaning |
|---|---|
| `@T <kind> <name> — <path> — <cost> — <why or description>` | one **pointer** to a tool: kind ∈ mind\|command\|tool\|body\|external; the path (or `not installed`); the cost — `pick` says `<resident>tok (load≈<full>)` and why it was picked (`trigger "<phrase>"`, `meaning`, `relation <bond>`) with its score; `brief` says `load≈<full>tok`, the one-line description and the declared triggers in `⟨ ⟩`. Never a payload. |
| `@TOOLS as=<body> k=<n> cost=<tokens>/<budget> — <how to load>` | opens a level-1 manifest block: the `@T` lines that follow are the tools this turn may load, and the head line names the exact level-2 command for each mouth. |

The instrument's own answers ride the core envelope: `@S PICK k=<n> cost=<c>/<b>` · `@S LOAD
<kind> <name> sec=<all|N> tokens=<t>` followed by the body text · `@S MAP <name> sections=<n>` with
one `@F #n — heading — tokens` per section · `@S EXPLAIN` · `@S INDEXED` · `@?` per hole · `@E`.

## The three mouths — injection without bloat

The toolbox never pastes a tool's body into a prompt. It hands over names and paths with known
costs (the compass: point, don't carry); the receiving body loads on demand.

- **A Court Body brief** — `toolbox.js brief "<ask>" --as <body>` prints the `@TOOLS` block,
  ready to paste under the commission. The block's head line says how to load: `node
  .isekai/tools/toolbox.js load <name> [--map|--sec N]`. The Court reads the manifest, works, and
  dons a Mind only when it needs it — `--map` first for a long one, then the section it wants.
  Its context dies with the task (dispatch-and-discard), so whatever it loaded costs the
  dispatcher nothing.
- **Claude Code** — the picks map onto the host's own on-demand loading, named honestly: a
  project Mind is a skill the host invokes by name (`Skill <name>`); a deferred host tool is
  loaded by name (`ToolSearch "select:<name>"`); a Body is dispatched by name (`Agent <body>`).
  The host keeps every skill's description resident on every turn; the toolbox does not change
  that — it tells the body **which** names to load for this turn, so a registry can grow past
  what any host would keep resident.
- **OpenCode** — the same, through its skill loader: the manifest names the skill, the body
  loads it when it decides to. `load <name>` is the mouth-independent fallback for both.

## The loads instrument — offered against loaded

`.isekai/instruments/toolbox/loads.jsonl` — machine-local, gitignored, append-only single-line
writes (atomic below `PIPE_BUF`), overwritten freely like any instrument. Two kinds of line:

- `{at, by, ev:"offer", cmd, ask, names, tokens, budget}` — one per `pick`/`brief` that offered anything.
- `{at, by, ev:"load", name, kind, tokens, sec}` — one per `load` (a `--map` crosses nothing and is not counted).

`toolbox.js status` reads it back: *offered 12 (7 distinct, 6 picks) · loaded 2 (2 distinct) ·
resident cost 876 tok · loaded cost 2,568 tok*. Offered far above loaded is the pattern working;
loaded far above offered is a body reaching past its manifest — both are readings, not feelings.

## `extra.jsonl` — the world's external tools

One JSON object per line, hand-written and tracked. The registry measures `installed`; the file's
own `installed` is a claim kept beside the measurement.

```json
{"kind":"external","name":"soffice","path":"/usr/bin/soffice",
 "description":"LibreOffice headless — renders a .pptx to PDF or PNG so a slide can be checked.",
 "triggers":["render","pdf","pptx","deck","slide"],
 "cost":{"resident":60,"full":90},
 "usage":"soffice --headless --convert-to pdf --outdir .isekai/tmp/<scratch> <file.pptx>",
 "installed":true}
```

`name` and `description` are required; `path` (absolute; `null` means "look on PATH"), `triggers`,
`cost` (`resident` and `full`, or one number for resident), `usage` (what `load` hands over),
`serves`, `installed` are optional. An MCP server is an external whose `full` cost is its tool
schema — the number the pattern exists to keep out of the prompt until it is needed. A malformed
line is skipped, never fatal.

## The instrument

`node .isekai/tools/toolbox.js [root] <command>` — stdlib Node, Node 16-compatible, one file.

| Command | Reads / writes | Answer |
|---|---|---|
| `index [--json]` | sources → `toolbox/registry.json` | `@S INDEXED n entries — mind · command · tool · body · external`; `@?` per external not installed |
| `status [--budget N] [--json]` | registry, `extra.jsonl`, the loads journal | registry size and staleness; resident cost if everything were injected vs. the budget; full cost if everything were loaded; externals installed/missing; offered vs. loaded |
| `pick "<ask>" [--as X] [--budget N] [-k N] [--kind k[,k]] [--min s] [--json]` | registry (or a live harvest); appends one offer | `@S PICK` · `@T …` per pick · `@?` · `@E` |
| `brief "<ask>" … [--json]` | same | the `@TOOLS` block, ready to paste; `--json` gives it as one line — the head, the `@T` lines and the picks as priced pointers (what `loop.js` reads) |
| `load <name> [--as X] [--kind k] [--map \| --sec N] [--json]` | registry; appends one load | `@S LOAD` + the body (or one section; `--sec 0` is the preamble) · `@S MAP` the section index |
| `explain <name> [--kind k] [--json]` | registry | path(s), both costs, lane and wearers, triggers with their source, the description |
| `selftest` | a throwaway world under `.isekai/tmp/toolbox-selftest/` (Law 5), removed after | `@S PASS n checks` on Node 16 and 22 |

A name shared by two kinds (`--kind` disambiguates) is a `FAIL`, not a guess; so is `-k 0`, a
budget of 0, an unknown kind, a section past the end (it points at `--map`).

## The law's text

The bullet in `isekai.md § Minds & Bodies` after "Loading is already two-tiered", kept beside the design it states:

**The toolbox — two levels, one budget.** A body never carries the whole shelf. Everything it
could pick up — Minds, commands, world tools, Bodies, and the externals `.isekai/toolbox/extra.jsonl`
names — is indexed by `.isekai/tools/toolbox.js` into a derived registry, each entry priced before
anything is injected. A turn receives **level 1, the manifest**: names, one-line descriptions,
triggers, paths and the cost of level 2 — only the entries that fit the ask (meaning, trigger,
relation) and a token budget, on the wire as `@T` lines under `@TOOLS`. **Level 2, the load**, is
the body's own decision to use the tool, never pre-emptive: `toolbox.js load <name>` (whole, or one
section by anchor), a Mind invoked by name, a Body dispatched — and every load is journaled, so
`status` reports what was loaded against what was merely offered. The toolbox never pastes a tool's
body into a prompt; it hands over pointers with known costs, and the receiving body loads on demand.
This is "loading is already two-tiered" made an instrument and extended past Minds to commands,
tools, Bodies and externals: the registry may grow without bound; the prompt does not.
