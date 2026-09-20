---
description: Genesis — scan the codebase and birth Elves, Orcs and Slimes based on observed need
argument-hint: "[path] [--dry-run] [--depth code|deep]  (no args: scans the directory holding the nearest .isekai/; depth defaults to code)"
allowed-tools: Bash(find:*), Bash(ls:*), Bash(grep:*), Bash(wc:*), Bash(cat:*), Bash(git log:*), Bash(git show:*), Read, Write, Edit, Glob, Grep
---

# /genesis — birth a world's population from observed need

Where `/isekai` founds the world and `/don`/`/mint` import creatures from elsewhere, `/genesis`
looks at the code the world actually contains and decides, itself, which Elves, Orcs and
Slimes that code needs — then births them.

`/isekai` already runs this same flow automatically the moment it founds a brand-new world
(see its own Steps). Invoke `/genesis` directly to re-survey a world that already exists, or
to fill in creatures for code that showed up after the initial reincarnation.

This is Nature 4 (Genesis) invoked directly: a deliberate order from Veldora stands in for the
"named twice" rule — the command's own invocation is both namings at once. It never births
silently; it always shows its reasoning before writing anything.

**Argument:** `$ARGUMENTS`
- No arguments → operate on the directory holding the nearest `.isekai/` (search upward from
  cwd). If none is found, tell the user to run `/isekai` first and stop.
- A path → use that directory's `.isekai/` instead.
- `--dry-run` → print the proposed population without writing any files or touching `log.md`.
- `--depth code` (default) → survey the code as it stands now (step 3 below).
- `--depth deep` → also do step 4 below: mine each zone's git history to understand how it
  got this way, not just what it currently looks like.

## Steps

1. **Load the convention.** Read `.isekai/isekai.md` for this world and honor whatever it
   says over the defaults below — a world may have amended its own Nine Natures or renamed
   ranks.
2. **Load the existing population.** List `.isekai/{elf,orc,slime}/*/` — these creatures
   already exist; Genesis only adds what's missing, never duplicates or overwrites a living
   creature's doc.
3. **Survey the code — by reading it, not just listing it.** Walk the target directory (skip
   `.git/`, `.isekai/`, build/vendor/dependency dirs, and anything `.gitignore` excludes). File
   names, doc mirrors and commit counts (`git log --oneline -- <path> | wc -l`, a cheap
   recency/activity proxy) tell you *where* the boundaries probably are — they are a map to
   where to read, never a substitute for reading. For every candidate zone, actually open its
   source before deciding anything about it: read files under ~500 lines in full, sample the
   representative functions in a larger one. Note what it does, what it depends on, what
   depends on it, and any concrete invariant, gotcha or inconsistency the code itself reveals —
   a flag that's only half-wired, two sibling functions that quietly diverge, a value hardcoded
   where a schema implies it's configurable, a no-op that looks like real logic. That's what
   step 7's `## Traits` section is for.
4. **Deep survey — git archaeology (`--depth deep` only, skip otherwise).** For each candidate
   zone from step 3, walk its history: `git log --follow --oneline -- <path>` for the commit
   list, then look closer — `git show <sha> -- <path>` — at the commits whose message signals
   something worth understanding: `fix`, `bug`, `revert`, `workaround`, `hack`, a version tag
   (an in-code comment like `@since v0.0.9` is a hint there's more history behind that line
   than the current diff shows), or a spot touched by several commits in a row (a sign of
   instability worth naming). Skip mechanical commits — formatting, dependency bumps, pure
   renames. The goal is evolution, not archaeology for its own sake: why does this zone look
   the way it does, what was tried and abandoned, what incident or bug shaped a current
   default or a workaround that looks stranger than it is. A fact that's true *right now*
   still goes in step 7's `## Traits`; a fact about *how it got here* — a reversal, a recurring
   fix, an incident — goes in that creature's dated `## Thoughts` instead, and if the same kind
   of history shows up across more than one zone, that's Nature 5 analysis that should flow up
   (mention it in step 8's log entry so it reaches the Orc/Elf level).
5. **Classify by need, not by file count:**
   - **Elf** — every world gets at least one, regardless of how many Orcs it needs. The chain
     of command is fixed (Principles: Rimuru → Elf → Orc → Slime) — an Orc reporting straight
     to Rimuru skips a rank, not just a formality. A single-domain world still gets exactly one
     Elf commanding its one Orc; mint more only where something is genuinely cross-domain
     (shared types/utilities more than one Orc's territory depends on, project-wide
     conventions, docs/tests that cut across domains) and multiple Orcs actually need
     arbitrating. When there's no natural cross-domain name to give it, `elf-core` is a lawful
     default — don't invent a false cross-domain purpose just to justify a fancier name.
   - **Orc** — a domain wide enough to need a ruler: it contains multiple Slime-sized zones, or
     it's the boundary other domains talk through (an API layer, a provider's resource
     registry, a CLI's command set). Name it `orc-<domain>`. Always reports to an Elf, never
     directly to Rimuru.
   - **Slime** — a narrow area that is the one source of ground truth for something specific:
     a single resource/model/schema, a single config parser, a single integration point. Name
     it `slime-<zone>` after the thing it's ground-truth for.
   - Err toward fewer creatures beyond the mandatory Elf. A dozen one-file Slimes is noise, not
     symbiosis (Nature 2) — merge sibling zones under one Slime when they're always touched
     together, split a Slime that's clearly doing two unrelated jobs.
6. **Draft the population map** and show it to the user before writing anything: for each
   proposed creature, its id, its territory (paths), a one-line reason, and which existing
   creature (if any) it reports to. Also list what's staying unrouted on purpose (e.g. CI
   config, README) and why. On `--dry-run`, stop here.
7. **Birth each new creature.** For each one not already present:
   - `mkdir -p .isekai/<race>/<name>/`
   - Write `.isekai/<race>/<name>/README.md`:
     ```markdown
     <img src="../../portraits/<race>.png" width="64" alt="<race>">

     # <race>-<name>

     - **Rank:** Elf | Orc | Slime
     - **Territory:** <paths this creature owns>
     - **Reports to:** <Rimuru if this is the Elf; its Elf if this is an Orc; its Orc if this
       is a Slime — an Orc never reports directly to Rimuru>
     - **Purpose:** <one paragraph, from step 5's reasoning>

     ## Traits
     - <at least one concrete, verified fact from step 3 (and step 4, on `--depth deep`): an
       invariant, a gotcha, an inconsistency between sibling files, a hardcoded value, a
       silent no-op. Leave this empty only when the territory is genuinely trivial (a stub, a
       constant, an empty package) — for anything with real logic, an empty Traits section
       means the survey wasn't deep enough; go back and read it.>

     ## Thoughts

     ### [<date>]
     Born by /genesis. <one line: why this territory needed a creature now>
     ```
   - For the Elf, also note which Orc(s) it commands; for an Orc, also note which Slimes it
     commands (fill in as they're born in the same run).
8. **Record it.** Append one entry to `.isekai/log.md` for the whole run (not one per
   creature): task, every creature born (id + territory), gate `n/a` (birth, not a landing),
   result `done`, and anything learned about the codebase's shape worth keeping — on
   `--depth deep`, name the depth used and summarize any cross-zone evolution pattern found.
9. **Report** the population as a tree, plus the one-paragraph reasoning per creature so the
   user can veto or rename anything before it's used for real work.

## Notes

- Genesis never births a Kijin, High Elf, High Orc or Dark Elf — those are earned through
  Nature 5 (ascension) or observed convergence (Nature 6), never assigned up front.
- Genesis never deletes or merges an existing creature — that's Nature 2/3 territory and needs
  its own review; Genesis only fills gaps.
- Re-running `/genesis` later is safe: it re-surveys the code and proposes only what's still
  missing or what's drifted enough to flag (a new top-level domain with no owner, a Slime whose
  territory no longer exists) — it never silently touches a creature it didn't just create.
- Reading beats inferring, always. A creature born from file names and commit counts alone is
  a placeholder, not ground truth — Genesis isn't done until every non-trivial creature's
  `## Traits` holds something only a reader of the actual code would know.
- Every population includes exactly one Elf at minimum, even a single-Orc world — it's a
  required link in the fixed chain of command, not an optional convenience for large worlds.
- `--depth deep` costs more time and context per zone — reach for it when the code's current
  shape doesn't fully explain itself (an odd hardcoded value, a version-tagged comment with no
  other context, two siblings that diverge for no visible reason), or when the human asks for
  the world's memory to include how the code got here, not just what it does today.
  `--depth code` (the default) is enough for most worlds.
