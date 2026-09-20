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

**Retuned again 2026-09-20** (human: "journaling and deterministic task should be haiku and
always try to create deterministic scripts for haiku to use"). One more question belongs
*before* picking a tier at all: **can this be a deterministic script instead of a model call,
or reduced to one?** `tempest.js`'s `harvest()`/`render()` and `palette-audit`'s
`validate_palette.js` are already exactly this — computed, not judged; the instrument board's
whole reason to exist (Nature 9) is replacing "feels stressed" with a number nobody had to
reason their way to. A script beats even Haiku: zero variance, free, instant. Where a model
call is still needed because the input varies, but the *shape* of the output is fixed and the
facts going in are already fully known — that's still not a judgment task, and it should run
on Haiku 4.5, not a bigger model by default. Below, this is why the most mechanical Ciel-shaped
inputs (a raw field dump, no prose) should bypass the Ciel office entirely rather than lower
its tier — see the ordering constraint under the court triad for why dropping Ciel's own floor
was the wrong fix.

## The court triad (E3 — dispatched, disposable context)

**Known gap, named rather than silently inherited:** `isekai.md`'s own world table
(Rimuru/Elf/Orc/Slime, ascended Kijin/High Orc/High Elf/Dark Elf) never mentions Great Sage,
Raphael or Ciel at all — they exist only in the transcribed `canon/agents/*.md` templates,
which are explicitly not-yet-merged reference material. This table inherits that ambiguity
rather than resolving it: whether the court triad is a mode any base rank's Body can run in,
or a separate dispatch pattern layered on top of the base ranks, isn't settled by `isekai.md`
as written. Don't read this table as implying an answer either way — that merge is a separate,
larger pass (see `canon/README.md`'s own caution about not guessing at the photographed
source's gaps).

**Ordering constraint** (human: "match complexity and task in convention with... Ciel is more
intelligent than Raphael and Great Sage, got it?"). This isn't just a preference — it's in the
canon's own framing. `ciel.md` calls Ciel *"the last evolution: where Great Sage perceived and
Raphael judged, Ciel SPEAKS"* — one lineage, escalating: Great Sage → Raphael → Ciel. Whatever
tiers this table assigns, **Ciel's floor must never sit below Raphael's, and Raphael's must
never sit below Great Sage's.** That constraint caught a real mistake in the previous pass
(below): defaulting Ciel to Haiku 4.5 for "simple" journal entries put its floor at Great
Sage's tier, breaking the ordering the canon itself states.

| Office | Job (from its own template) | Suggested model | Why |
|---|---|---|---|
| **Great Sage** | Perception without judgment: read wide, report thin, `file:line` anchors + one-line facts, no decoration, eyes-only. High dispatch volume, low reasoning depth. | **Haiku 4.5** | This is bulk retrieval and terse extraction, not judgment — exactly the fast/cheap profile. Its own office laws forbid it from doing anything that would need more (no writes, no verdicts, no carried-away reasoning). Upgrade to Sonnet 5 only if a codebase is dense/ambiguous enough that facts are getting missed. |
| **Raphael** | Judges: a verdict without `file:line` evidence "is not a verdict — it is gossip," BLOCK beats a lazy PASS. Correctness-critical — a wrong verdict either lets a bad change land or blocks a good one. | **Sonnet 5** | Real reasoning is the job here, not retrieval. For domains where a bad verdict is expensive (security, payments, migrations, anything hard to roll back), bump to **Opus 5**. |
| **Ciel** | Speaks: turns facts into durable, journal-ready, human-facing prose under nature 7's siphon (full courtesy to Veldora) and strict house formats. Prose quality and voice consistency matter more than deep reasoning. | **Sonnet 5** floor (never below Raphael's); **Opus 5** for heavy synthesis; **Fable 5.1** when voice is the point | Corrected: the deterministic-script principle above still applies to Ciel's most mechanical inputs, but it now resolves the other way — as a reason those cases **bypass Ciel entirely**, not as a reason to drop Ciel's own tier below Raphael's. A `celebrations.jsonl` append with no prose at all is already pure code in `tempest.js` — no office dispatched, Ciel or otherwise. The moment the *Ciel office itself* is actually invoked — turning facts into house-format prose, even a short `log.md` entry — that's voice work by definition (it's why Ciel exists instead of a template-fill script), and per the ordering constraint above it runs no lower than Raphael's floor: **Sonnet 5**. **Opus 5** for heavier synthesis still: a session report or handoff weaving many threads into one coherent narrative. **Fable 5.1**, independent of complexity, when voice-consistency itself is the point (diary-style entries, anything meant to read as one consistent character's voice over time) — confirmed available 2026-09-20. Don't stack tiers: one model runs the call, so when a piece is both complex *and* voice-critical, default to Fable. |

## Base ranks (from `isekai.md`'s "The world" table)

| Rank | Job | Suggested model | Why |
|---|---|---|---|
| **Elf** | The shared mind *and* voice — thinks across domains, routes work, rules disputes, drafts every outward message. Closest rank to Rimuru in reasoning load. | **Sonnet 5** default (coordination); **Opus 5** for genuine cross-domain conflict; **Fable 5.1** for the outward voice specifically | isekai.md's own table names Elf, not Ciel, as the rank that "keeps... the world's voice" and "is the one who speaks back up" — Rule II names the same surfaces (session reports, diary voices, journal legibility) that justify Fable for Ciel. The earlier version of this table applied the voice/Fable reasoning only to Ciel and left Elf out, even though Elf is the rank the core convention itself assigns voice to. Split the same way Ciel is: **Sonnet 5** for the coordination and routing that's most of an Elf's actual work, **Opus 5** when arbitrating genuine cross-domain conflict, **Fable 5.1** specifically when drafting the outward-facing report or handoff itself and voice-consistency is what's being asked for — not for the reasoning that produces its content. |
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

## Open questions from Fable's own review of this convention

Ordered by request, Fable 5.1 (this document's own subject) reviewed `isekai.md` and this file
2026-09-20 and surfaced findings beyond what's already folded into the tables above. Named
here rather than silently acted on or silently dropped:

- **A cheaper alternative to stacking tiers on Ciel/Elf:** instead of betting one model on
  both synthesis and voice for a piece that needs both, a two-call pipeline — Sonnet or Opus
  synthesizes the facts into a draft, Fable does a voice pass on that draft — mirrors a pattern
  the convention already uses elsewhere (a Slime authors, an Orc gates: two passes, not one
  model doing both jobs). Not adopted as a default here — it changes the "one model per
  office per call" shape the rest of this table assumes, and that's a real architecture
  decision, not a tuning tweak. Worth prototyping if a genuinely complex-and-voice-critical
  case shows up often enough to justify it.
- **No discovery/index layer:** Genesis births creatures, but nothing tells a fresh session
  which Slime owns a given file without cold-scanning `.isekai/slime/*` every time. Outside
  this document's scope (it's a `/genesis`/`tempest.js` question, not a model-tuning one) —
  named so it isn't lost.
- **No concurrency protocol for `log.md`:** Rule I's "living memory stays per machine" implies
  more than one machine or session can work the same world, but nothing in `isekai.md`
  addresses simultaneous appends or overlapping gate checks on the same doc. Also outside this
  document's scope — same reason, named for the same reason.
