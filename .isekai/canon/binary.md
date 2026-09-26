# The isekai binary — the law as a harness

`isekai` is a single Go binary that runs an agent session on a world. The convention was written
for harnesses that only *read* it (Claude Code, OpenCode); a model can read a law and still skip
it. Here the law is the harness: the gate, Vitality, the log, the human gate, the instruments and
the wire are code paths the model cannot route around, and every creature is a native type.

Source: `isekai/` at the repository root (Go module, stdlib only). Build with
`go build -o bin/isekai ./cmd/isekai` from `isekai/`. Toolchain: Go ≥ 1.23.

## Principles of the build

- **Minimal.** Standard library only. No TUI, no LSP, no web, no plugin system. A line REPL and a
  one-shot mode. A feature enters when a session names the need twice (Nature 4).
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
| `gate` | the human gate: TTY prompt, `--approve <class>` pre-approval, `--dry-run` | Nature 7, Law 6: outward and destructive always ask; nothing auto-approved; a denial stops the turn |
| `loop` | the turn engine: perceive → recall → plan → act → verify → record per tool step; journal to `.isekai/instruments/loop/<run-id>.jsonl` | §The loop — budgets are readings; past one, checkpoint and stop honestly |
| `instrument` | context occupancy from provider-reported usage, 200k budget / 180k stress zone; `status` board | Nature 9, §Instruments — Rimuru's stress is a reading |
| `memory` | port of `memory.js`: status, index, recall (meaning + relation rank), remember (`--kind law|colony|territory`) | §Memory tiers — same files, same formats |
| `toolbox` | port of `toolbox.js`: registry build, `brief` (level-1 `@T` manifest under a budget), `load` (level 2, journaled) | §Minds & Bodies — the toolbox |
| `wire` | parse and emit the envelope (`@S @F @V @? @U @E`, `@ROOT @SCOPE @ASK @CAP @DUMP`); `@CAP` enforced | Absolute Rule II, Nature 8 |
| `world` | world discovery, the law loader (crest always; code sections on demand), ranks and bodies from `.isekai/{elf,orc,slime}/`, `log.md` append-only writer, the Orc gate and the Vitality check, Court dispatch | Natures 1–4, the gate, Laws 2–4 |
| `onto` | the ontology: schema, graph, reasoning, validation, projection | §The ontology below |
| `cmd/isekai` | CLI: `isekai` (REPL), `isekai run "<ask>"`, `isekai status`, `isekai onto …`, `isekai memory …`, `isekai toolbox …`, `isekai selftest` | — |

## Ranks and bodies, native

- The session is **Rimuru**. A creature is a record read from its doc under
  `.isekai/<race>/<name>/`: race, territory (path globs), traits, worn minds, orc (for a slime).
- A **Court Body** is `dispatch`: a fresh loop with its own context, a commission in the wire
  (`@ROOT @SCOPE @ASK @CAP`), a tool set cut to its rank and territory, and one wire report back.
  Its context dies with the call; only what it wrote to disk and its report survive. A report
  with no `@U` line is flagged to the dispatcher (the unsaid).
- **Territory is enforced.** A Slime's `write`/`edit` outside its territory is refused, not
  warned (Law 2). Escalation is a `@?` to its dispatcher, one hop (Absolute Rule III).

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
- **Validation** — shapes: every Slime has exactly one `truth` edge; every Mind is worn or is
  nobody's; no two Slimes own an overlapping path (Nature 2). A violation is a finding.
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

## Tests

Every package carries Go tests; providers are exercised through `mock`. `isekai selftest` runs
the in-binary checks the way the JS instruments' `selftest` does and answers on the wire
(`@S PASS n checks`). Interop tests run the binary and the JS tools against the same files.
