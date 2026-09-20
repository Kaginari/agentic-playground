# Canon transcriptions

The files in this folder are faithful transcriptions from photographs of a fuller `ISEKAI.md`
canon than the version currently installed by the `/isekai` command (`.isekai/isekai.md`,
`.claude/commands/isekai.md`, `.opencode/commands/isekai.md`). The installed skill template
only carries the Crest, the Nine Natures, Principles, the wire-language rules, Instruments,
Minds & Bodies, the world table, the gate, and six Laws.

The photographed source has more structure than that: the same Nine Natures, but underneath
them a numbered rule set (1–21, grouped into Chapters I–VII) and a separate E1–E6 protocol
set for Minds & Bodies specifically. Rimuru's own throne-body instructions describe the full
canon chain as **Nature laws 0–8 → rules 1–21 → E-protocols → colony map** — all three layers
live together in one `ISEKAI.md`, not as separate documents. The source also documents the
`/isekai` command's own logic in more detail than what's installed (two phases — audit-only
vs. populate — plus an `AGENTS.md` template it appends).

These files were transcribed rather than merged into `.isekai/isekai.md` / the installed
`/isekai` command directly, because reconciling two differently-structured numbering schemes
(Laws 1–6 vs. rules 1–21), and adding the Phase 2a/2b + `AGENTS.md` + `tools/tempest.js`
logic the installed command doesn't have, without guessing at the parts the photos didn't
capture, risked introducing contradictions into the live convention. Treat this folder as raw
source material for a future, more complete `ISEKAI.md` and `/isekai` command — not yet
operative. One piece of it *has* been built already: `tempest.js` itself
(`.isekai/tools/tempest.js`), implemented from `instruments-tempest.md` since that spec was
unambiguous enough to build directly.

## What's here

- [`rules-chapters.md`](rules-chapters.md) — the E1–E6 protocols and Chapters I–VII (rules 1–21)
- [`ascended-races.md`](ascended-races.md) — fuller prose on high orc and dark elf
- [`rimuru-throne-body.md`](rimuru-throne-body.md) — the OpenCode-side global "throne body" agent spec
- [`isekai-phases.md`](isekai-phases.md) — the `/isekai` command's real Phase 2a (audit-only)
  and Phase 2b (populate) logic — richer than what's installed
- [`templates.md`](templates.md) — the `AGENTS.md` colony-block template Phase 2b appends
- [`instruments-tempest.md`](instruments-tempest.md) — the full `tempest.js` instrument-board
  spec — **implemented** at [`../tools/tempest.js`](../tools/tempest.js)
- [`agents/great-sage.md`](agents/great-sage.md), [`agents/raphael.md`](agents/raphael.md),
  [`agents/ciel.md`](agents/ciel.md) — templates for all three "court triad" bodies
  (perceive / judge / speak); `ciel.md` is complete, the other two have small gaps (see below)

## Known gaps

The source photos cut off before capturing:
- The full text of rules 12–13 (Chapter V — Tracking) — only the tail of 13 survived.
- The Kijin paragraph in the ascended-races section (High orc and Dark elf are complete; Kijin
  is not).
- Rules 1–3 of Raphael's own office laws (only 4–6 and its Wire mouth section survived).
- Great Sage's own Wire mouth commissions (only its office laws survived).
- The `.opencode/skills/` seed template that Phase 2b step 3 refers to ("seed template
  below") was never itself photographed — only the reference to it.

Complete: `ciel.md` (all three court-triad members now transcribed in full), and Phase 2a +
Phase 2b of the `/isekai` command (both phases and the `AGENTS.md` template captured in full).

Don't fill these gaps by inference — if you find the rest of this source, transcribe it
directly; otherwise leave the gap marked.
