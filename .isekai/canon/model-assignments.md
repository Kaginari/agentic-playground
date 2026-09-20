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

## The court triad (E3 — dispatched, disposable context)

| Office | Job (from its own template) | Suggested model | Why |
|---|---|---|---|
| **Great Sage** | Perception without judgment: read wide, report thin, `file:line` anchors + one-line facts, no decoration, eyes-only. High dispatch volume, low reasoning depth. | **Haiku 4.5** | This is bulk retrieval and terse extraction, not judgment — exactly the fast/cheap profile. Its own office laws forbid it from doing anything that would need more (no writes, no verdicts, no carried-away reasoning). Upgrade to Sonnet 5 only if a codebase is dense/ambiguous enough that facts are getting missed. |
| **Raphael** | Judges: a verdict without `file:line` evidence "is not a verdict — it is gossip," BLOCK beats a lazy PASS. Correctness-critical — a wrong verdict either lets a bad change land or blocks a good one. | **Sonnet 5** | Real reasoning is the job here, not retrieval. For domains where a bad verdict is expensive (security, payments, migrations, anything hard to roll back), bump to **Opus 5**. |
| **Ciel** | Speaks: turns facts into durable, journal-ready, human-facing prose under nature 7's siphon (full courtesy to Veldora) and strict house formats. Prose quality and voice consistency matter more than deep reasoning. | **Sonnet 5** | Strong, reliable prose at a lower cost than Opus. **Fable 5.1** is worth trialing specifically for this office — voice/narrative consistency is its whole job — but nothing in this session's context differentiates Fable's actual capabilities, so treat that as an experiment, not a settled recommendation. |

## Base ranks (from `isekai.md`'s "The world" table)

| Rank | Job | Suggested model | Why |
|---|---|---|---|
| **Elf** | The shared mind *and* voice — thinks across domains, routes work, rules disputes, drafts every outward message. Closest rank to Rimuru in reasoning load. | **Sonnet 5** default | Balanced reasoning + writing. Bump to **Opus 5** for large worlds with many orcs and genuine cross-domain conflict to arbitrate — that's where an elf's judgment calls get expensive to get wrong. |
| **Orc** | Rules its domain and *is* the actual landing gate (not advisory like Raphael): verifies the right slime authored, traits hold, duties done, doc truthful. Getting this wrong either lands a bad change or blocks a good one — same shape as Raphael's job, but with real teeth. | **Sonnet 5** default | Same reasoning-critical profile as Raphael, for the same reason. **Opus 5** for high-stakes domains (security, financial correctness, infra you can't cheaply revert). |
| **Slime** | One narrow zone, deep but narrow, authors changes directly. Many exist per world; frequently loaded/reloaded (rule 4's first-action gate). | **Haiku 4.5** default | Cost efficiency at volume — most zones are narrow and well-defined enough not to need more. **Sonnet 5** when a specific zone's own logic is intrinsically complex (a gnarly algorithm, ambiguous legacy code) — decide per-slime, not per-world; a world can reasonably mix both. |

## Ascended ranks

Not enough signal to tune these confidently — High Orc, Dark Elf and Kijin are rare,
human-declared, and long-lived (their memory "is an archive, not a desk"), which argues for
the same or higher tier than the rank they ascended from rather than a cost-optimized choice.
Default to **Opus 5** for all three until there's a concrete reason to do otherwise — their
scarcity means the cost difference barely matters.
