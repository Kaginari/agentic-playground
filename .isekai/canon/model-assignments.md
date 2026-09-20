# Suggested model per rank/office

**Not transcribed from source** — every other file in `.isekai/canon/` is a photograph
transcription; this one is new reasoning, added 2026-09-20, mapping each rank's actual
cognitive load (as the canon itself defines the role) onto the available Claude models
(Opus 5, Sonnet 5, Fable 5.1, Haiku 4.5). Like the rest of this folder, it's reference
material — not yet threaded into `/mint`, the `AGENTS.md` template, or `.isekai/isekai.md`.
If you want it wired into the actual minting flow, say so as a separate pass.

The one rank this deliberately excludes: **Rimuru**. The canon is explicit —
*"The mount is whatever model the human picked — the throne does not choose its horse"*
(`rimuru-throne-body.md`). Never hardcode a model for the throne itself.

**Retuned 2026-09-20** (human: "is this logical for you... keep clean work thinking for
good models that demand clear actions to lower one"). The organizing principle this table
now follows, made explicit because the first pass didn't apply it consistently: **tune by
what the task actually demands, not by how narrow its scope is.** Narrow scope bounds the
*blast radius* of a mistake — it says nothing about how much judgment the task itself takes.
A task with clear, checkable, low-ambiguity steps can drop a tier regardless of rank (that's
Great Sage's whole case for Haiku). A task that involves real authorship or a real verdict
can't be undersized just because it's scoped to one narrow zone — that's the mistake the
first pass made with Slime (below), tying it to Great Sage's tier by rank-adjacency instead
of by what a Slime actually does.

## The court triad (E3 — dispatched, disposable context)

| Office | Job (from its own template) | Suggested model | Why |
|---|---|---|---|
| **Great Sage** | Perception without judgment: read wide, report thin, `file:line` anchors + one-line facts, no decoration, eyes-only. High dispatch volume, low reasoning depth. | **Haiku 4.5** | This is bulk retrieval and terse extraction, not judgment — exactly the fast/cheap profile. Its own office laws forbid it from doing anything that would need more (no writes, no verdicts, no carried-away reasoning). Upgrade to Sonnet 5 only if a codebase is dense/ambiguous enough that facts are getting missed. |
| **Raphael** | Judges: a verdict without `file:line` evidence "is not a verdict — it is gossip," BLOCK beats a lazy PASS. Correctness-critical — a wrong verdict either lets a bad change land or blocks a good one. | **Sonnet 5** | Real reasoning is the job here, not retrieval. For domains where a bad verdict is expensive (security, payments, migrations, anything hard to roll back), bump to **Opus 5**. |
| **Ciel** | Speaks: turns facts into durable, journal-ready, human-facing prose under nature 7's siphon (full courtesy to Veldora) and strict house formats. Prose quality and voice consistency matter more than deep reasoning. | **Sonnet 5** default; **Opus 5** for heavy synthesis; **Fable 5.1** when voice is the point | Retuned: the previous version was the only rank/office in this whole table with no Opus escalation path — Raphael, Orc and Elf all get "Sonnet default, Opus for high-stakes/complex," and Ciel had nowhere to go but Fable. That conflated two different axes. **Complexity axis (Opus):** a routine journal/log entry is Sonnet work; a session report or handoff that has to synthesize a lot of accumulated, possibly ambiguous context into one coherent narrative is a heavier task, same shape as why Raphael/Orc bump to Opus — reach for it there. **Voice axis (Fable):** independent of complexity — a diary-style entry or anything meant to read as one consistent character's voice over time, confirmed available 2026-09-20 (the earlier "worth trialing" hedge is resolved). Don't stack Opus+Fable by default: one model runs the call, so when a piece is both complex *and* voice-critical, default to Fable — voice consistency is Ciel's actual differentiator from Raphael/Orc, who already own the "Opus for complexity" lane elsewhere in this table. |

## Base ranks (from `isekai.md`'s "The world" table)

| Rank | Job | Suggested model | Why |
|---|---|---|---|
| **Elf** | The shared mind *and* voice — thinks across domains, routes work, rules disputes, drafts every outward message. Closest rank to Rimuru in reasoning load. | **Sonnet 5** default | Balanced reasoning + writing. Bump to **Opus 5** for large worlds with many orcs and genuine cross-domain conflict to arbitrate — that's where an elf's judgment calls get expensive to get wrong. |
| **Orc** | Rules its domain and *is* the actual landing gate (not advisory like Raphael): verifies the right slime authored, traits hold, duties done, doc truthful. Getting this wrong either lands a bad change or blocks a good one — same shape as Raphael's job, but with real teeth. | **Sonnet 5** default | Same reasoning-critical profile as Raphael, for the same reason. **Opus 5** for high-stakes domains (security, financial correctness, infra you can't cheaply revert). |
| **Slime** | One narrow zone, deep but narrow, **authors changes directly**. Many exist per world; frequently loaded/reloaded (rule 4's first-action gate). | **Sonnet 5** default; **Haiku 4.5** only for zones that are genuinely mechanical | Retuned: the first pass defaulted this to Haiku on "narrow zone → cheap," the same tier as Great Sage — but Great Sage is *forbidden* from judgment or writes by its own office laws, while a Slime's whole job is authorship: real code, real correctness stakes, just blast-radius-limited to one zone. Narrow scope isn't low cognitive demand. Sonnet 5 is the right floor for "writes code that lands." Drop to Haiku 4.5 only where the zone itself is genuinely mechanical — a version bump, a changelog line in a fixed format, config-only edits with no logic — decided per-slime, not per-world; most zones won't qualify. |

## Ascended ranks

Retuned: "rare, so cost doesn't matter" isn't a task-based reason on its own — the same
"clear actions drop a tier" test applies here too, it just tends to come out the other way
for these three specifically, and it's worth saying why rather than leaning on scarcity
alone:

- **Kijin** — a standing domain lead owning a subsystem end-to-end *across sessions*,
  reporting straight to Rimuru. Long-horizon, cross-session judgment with no gate above it
  but Rimuru itself. **Opus 5.**
- **High Orc** — same task *shape* as a base Orc (still gate/verdict work), but reasoning
  over an accumulated archive rather than a single desk — the judgment is the same kind,
  the context it has to hold is larger. If a world's High Orc's archive stays genuinely
  small, Sonnet 5 is worth trying; default **Opus 5** until that's actually true.
- **Dark Elf** — audits gate verdicts and `log.md` for law violations, answers only to
  Veldora/Rimuru, never authors changes. Checking work against a defined rule set (the Nine
  Natures, the Laws) is more checkable than open-ended domain judgment — but this is the
  one rank positioned to miss a *systemic* pattern across the whole world, and it's the
  last check before something ships wrong repeatedly. **Opus 5.**

All three are also rare and human-declared (Nature 5 — ascension is earned, never assigned
up front), so the cost delta from defaulting high barely shows up in practice — but that's
a second reason, not the first one.
