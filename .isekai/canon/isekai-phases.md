# The /isekai command — Phase 2a and Phase 2b

Transcribed from photographs. This is the actual two-phase logic of the full `/isekai`
command — considerably richer than the version currently installed at
`.claude/commands/isekai.md` / `.opencode/commands/isekai.md` / `.isekai/isekai.md`. The
installed version only ever does the equivalent of Phase 2b's step 3 (write `isekai.md` and
`log.md`, copy portraits); it never surveys the stack, never appends an `AGENTS.md` colony
block, never creates `.isekai/name`, never copies `tools/tempest.js`, never touches
`.gitignore`, and never lights the instruments as its last act. See [`README.md`](README.md)
— not yet merged into the operative command, kept here as reference for that future pass.

---

## Phase 2a — Present → AUDIT ONLY (change nothing)

Read the canon, the map block, every mind, and run the instruments:
`node .isekai/tools/tempest.js . --json` (absent → note it and read desks by hand). Report a
findings table:

1. **Map drift** — routing rows without a mind dir; mind dirs without a row.
2. **Malformed minds** — missing load trigger, elder, Discipline/no-push lines, `## Thoughts`.
3. **Territory** — overlapping file claims between slimes; top-level areas with no orc routed.
4. **Stress & diet** — from the board: desks over limit (5 ground / 10 dark elf; archive
   complete with T#/D# + every-3rd-decision verdict), docs over the 6KB diet, genesis-watch
   ♦ names.
5. **Versioning** — `.gitignore` divergence from rule 12.
6. **Foreign soil** — plain skills present; any creature treating one as more than a hint is
   a violation.
7. **Cascade (rule 19)** — orc decrees cited in every subject slime's doc; elf decrees in
   every orc's; contradictions of higher law are void — report them.
8. **Bodies** — E6 violations; keeper diary exists and is append-only; chronicle last-entry
   date; the machine-global throne body (`~/.config/opencode/agents/rimuru.md`) present —
   absent → note it (a world drifts less when any session can Tab to the throne).
9. **Instruments** — board file present, `--ensure` idempotent, `metrics/` writable.

End with a numbered list of recommended fixes. Change nothing unless the human asks.

## Phase 2b — Absent → POPULATE (offer-first)

1. **Survey** the stack: `go.mod` / `package.json` / `pyproject.toml` / `Cargo.toml`,
   top-level layout, test setup — infer natural domains (orcs) and zones (slimes).
2. **Present the plan and wait for the human's yes:** the minds roster (one `elf-colony` +
   orcs + slimes with file territories), the home tree, and the OPTIONAL bodies offer (a
   Keeper `mode: all`; the triad — `<name>-great-sage` reads / `<name>-raphael` verdicts /
   `<name>-ciel` drafts, genomes cloned from the seed's `offices/` with this world's name +
   root, riding the office templates' current mounts) — bodies are never minted silently; the
   plan names exactly which, and the human may take minds-only. The throne needs no minting:
   `rimuru` (`~/.config/opencode/agents/rimuru.md`, `mode: all`, machine-global since
   2026-09-20) Tabs into every world already — populate verifies it exists and reports it;
   absent → tell the human how to restore it.
3. On yes, write — **filling gaps only, never overwriting**:
   - `ISEKAI.md` ← copy the canon asset verbatim. Exists already → do not touch, report it.
   - `AGENTS.md` ← **append** the colony block below (create the file if absent); never
     replace existing content.
   - `.isekai/` home: `log.md` from the chronicle template (replace `{{DATE}}` — ISO 8601 —
     and `{{TARGET}}` — the directory name); `name` ← the world's chosen name (one line;
     default the directory name, jura-style names lawful — the human may rename; the board
     answers at `localhost:<port>/<name>/`); `portraits/` ← copy the 8 faces (skip any
     present); `tools/tempest.js` ← copy the instrument (skip if present); `metrics/`,
     `tmp/`, `agents-diaries/` each with an empty `.gitkeep`.
   - `.opencode/skills/` minds per the approved plan — seed template below (not itself
     captured — see `README.md`). Facts are harvested from the survey; territories must not
     overlap.
   - `.gitignore` ← ensure rule 12's block exactly (append missing lines; never delete a
     human's rules). If the target is a git repo (`git rev-parse --is-inside-work-tree`) and
     an existing rule excludes more, tell the human instead of editing it.
   - Chronicle the genesis: append the founding entry to `.isekai/log.md`.
4. **Light the instruments:** `node .isekai/tools/tempest.js <target> --ensure` (nature law
   8 — the world is born observed). The answer names the board URL —
   `http://localhost:<port>/<world-name>/`; put it in the summary AND open the app for the
   human (`open <url>` on macOS; headless → just print it). The board sleeps with the world
   (30-min silence lease; an open page holds it awake; `--stop`); sessions re-run `--ensure`
   as a heartbeat at landmarks.
5. Show the resulting tree + a one-line summary: what was created, what already existed, the
   board URL, and — if a git repo — the exact add/commit/push commands **the human** must run
   (you never push).

## Templates

### `AGENTS.md` colony block

See [`templates.md`](templates.md) — the full template, transcribed separately since it's
referenced from more than one place in the canon.
