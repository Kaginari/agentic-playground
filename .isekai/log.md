# Isekai Chronicle — change log

Append-only. Newest entries at the bottom. One entry per change.

## Entry format

    ### [YYYY-MM-DD HH:MM] <author> — <short title>
    - **Task:** what was asked
    - **Files:** paths changed (or `none`)
    - **Gate:** <orc-name> pass | fail — reason   (or `n/a`)
    - **Result:** done | partial | failed
    - **Learned:** anything the ranks above should keep

---

### [2026-09-20 17:54] rimuru — World reincarnated
- **Task:** /isekai convention-jura
- **Files:** .isekai/isekai.md, .isekai/log.md, .isekai/{portraits,instruments,elf,orc,slime,tmp}/
- **Gate:** n/a
- **Result:** done
- **Learned:** The world is reincarnated. Read isekai.md before any work.

### [2026-09-20 22:18] rimuru — Deterministic OpenCode/tempest.js integration test, real bug found
- **Task:** "crée une template de test et test et calcule" — build a reusable, deterministic
  test verifying tempest.js's OpenCode reader against OpenCode's own ground truth
  (`opencode export`), not eyeballed numbers.
- **Files:** .isekai/tools/test-opencode-integration.js (new), .isekai/tools/tempest.js,
  presentations/isekai-opencode-overview.pptx
- **Gate:** n/a (Nature 9 instrumentation + a real fix, not a landing under an Orc)
- **Result:** done
- **Learned:** The test found a real bug on its first real run: `harvestOpencodeGlobal()`'s
  SQL summed only `tokens_input`/`tokens_output`, silently dropping `tokens_cache_read` and
  `tokens_cache_write` — the same undercount class already fixed for Claude Code's transcripts,
  just not ported to the OpenCode reader. Fixed by summing all three input columns. Also:
  this machine's system Node (16.20.2) predates `node:sqlite` (Node 22.5+); installed Node 22
  via `nvm` so the OpenCode path is no longer just "honestly unavailable" but actually reads
  real numbers — documented as a prerequisite at the top of tempest.js. Deck's field-tested
  numbers (slide 8) and category framing (title badge, advantages slide) updated to match:
  842M real tokens, 3 real bugs found by an actual test script (not 2, not eyeballed), and
  "A CROSS-HARNESS CONVENTION" instead of "AN OPENCODE-NATIVE CONVENTION" — the deck's own
  content (the whole Claude Code compatibility appendix, the harness-agnostic Minds/Bodies
  portability) already argued this positioning; the badge just hadn't caught up to it.

### [2026-09-20 22:36] rimuru — First real with/without-convention comparison: better, not cheaper
- **Task:** "does convention make a complex task done better, and does consumption drop" —
  a real, measured 3-trial-per-arm comparison, not a claim. 6 isolated `claude -p` sessions
  (Sonnet 5, `--permission-mode acceptEdits`) fixing the exact `slime-db-role` Update bug
  its own Traits document (raw collection delete + hardcoded `"admin"` instead of `dropRole`
  + the role's own `database` field) — 3 copies of terraform-provider-mongodb with `.isekai/`
  present + a one-line `CLAUDE.md` pointer, 3 copies with `.isekai/`, `.opencode/`, `.claude/`
  stripped entirely. Same task prompt, deliberately described the bug's *shape* without
  naming the fix, to force genuine search either way.
- **Files:** none in this world (trials ran in gitignored `.isekai/tmp/ab-test/` scratch
  copies of terraform-provider-mongodb, not committed there per that world's own
  untracked-`.isekai/` convention)
- **Gate:** n/a
- **Result:** done — mixed, honest finding, not a clean win
- **Learned:**
  - **Correctness:** all 3 "with" trials adopted the canonical fix (`dropRole`, matching the
    same file's own `Delete` and the sibling `db_user` resource's prior fix) and cited the
    exact commit SHAs (`79bfcf4`, `5044b99`) named in the Traits doc verbatim — real evidence
    they read and used it, not coincidence. 2/3 compiled clean and even fixed a real,
    unrelated pre-existing bug (wrong error variable on line 175) as a side effect; the 3rd
    got the right diagnosis but broke the build on that same dangling variable. 0/3 "without"
    trials adopted the canonical pattern — all 3 settled for the narrower literal fix
    (parameterize the database, keep the raw collection delete) despite the correct pattern
    sitting 20 lines away in the same file, and all 3 compiled fine.
  - **Cost: did not drop, roughly tripled.** with-* averaged $0.494 / 28.3 turns / 1.27M
    input tokens; without-* averaged $0.165 / 11 turns / 327K input tokens. Ranges didn't
    overlap (n=3/arm, so real signal, not noise at this size) — the overhead is turns spent
    reading `isekai.md` plus all four creature READMEs, including ones irrelevant to this
    bug (`elf-core`, `slime-config`'s TLS traits, `orc-provider`'s registry notes).
  - **Caveat named by the human, then tested:** this measured one task, one cold session
    per trial — it could not detect prompt-cache amortization across a *sequence* of tasks,
    since every trial started fresh with no continuation.

### [2026-09-20 22:43] rimuru — Follow-up: does cost drop over a sequence? Not relatively.
- **Task:** chained a second, different real task (make `provider.go`'s hardcoded
  `MaxConnLifetime: 10` connect-timeout configurable via a new schema field — a bug
  `slime-config`'s own Traits documents) onto each of the prior entry's 6 sessions via
  `claude -p ... -c` (continue), same with/without split, same directories.
- **Files:** none in this world (same gitignored scratch copies)
- **Gate:** n/a
- **Result:** done
- **Learned:**
  - **Correctness was ~a tie:** 5/6 trials wired a correct new schema field regardless of
    condition — this task lives entirely in two files with no cross-file archaeology needed,
    so the "with" edge from the first task (finding a fix pattern in a sibling file via
    Traits) had nothing to bite on here. The one failure (with-3) was the *same* carried-over
    bug from task 1 (an unfixed dangling `err` reference), not a new task-2 defect.
  - **Cost went up for every trial, both conditions, task 1 to task 2** — $0.494→$0.791 avg
    "with", $0.165→$0.254 avg "without" — despite 91–98% cache-hit rates on task 2's reads.
    A continued session re-sends its full accumulated history every turn; caching discounts
    the per-token rate but the history itself keeps growing, so total cost keeps climbing.
  - **The with/without ratio did not shrink across the sequence — it held or widened**
    (task 1: ~2.99x; task 2 marginal cost: ~3.35x). The convention's up-front reading cost
    isn't paid once and forgotten within a session — it rides along in the cached context on
    every subsequent turn, so its overhead persists rather than amortizing away turn-to-turn.
    Whether it amortizes *across separate sessions* on the same world (a different axis —
    each session pays the read cost again regardless, but task N might redo less
    archaeology) is still untested.

### [2026-09-20 22:53] rimuru — Fixed a missing law, verified free-model + escalation, deck slide
- **Task:** three follow-ups from the same thread: (1) a "does escalation exist" question that
  surfaced a real bug while checking it, (2) "does this work with non-Claude/open-source
  models," (3) add the escalation flow to the deck.
- **Files:** .isekai/isekai.md, presentations/isekai-opencode-overview.pptx
- **Gate:** n/a
- **Result:** done
- **Learned:**
  - **Real bug found and fixed:** this world's own `.isekai/isekai.md` was missing
    `### III. Confirmation and Escalation` entirely — Nature 4's own text (line 135)
    references "Absolute Rule III" by name, but the section itself was never written into
    *this* copy, even though all 4 command templates and the terraform-provider-mongodb
    world's copy have it. Drift from a prior session's edit that apparently landed everywhere
    except here. Restored verbatim from the mongodb world's copy; a full diff now shows the
    two files byte-identical.
  - **Escalation is real and rank-based, not model-based:** Rule III's actual answer to
    "can a Slime escalate a task it can't do" is yes — one hop at a time, Slime → Orc → Elf →
    Rimuru → Veldora, never skipping a rank, never a handoff of the work itself. Fable 5.1
    is NOT part of this chain — per `model-assignments.md` it's a model choice for the
    Elf/Ciel office's outward-voice drafting specifically, a different axis entirely
    (which rank resolves it vs. which model that rank happens to run on). Corrected this
    distinction when building the deck slide rather than encoding the conflation.
  - **Non-Claude/open-source model support: already true, now verified, not just asserted.**
    Ran `opencode run` against terraform-provider-mongodb's real `.isekai/` with two free
    OpenCode models (`big-pickle`, `nemotron-3.5-lightning-free`, zero cost, zero
    credentials configured). Both correctly read a Slime's Traits doc and Rule III cold and
    answered accurately and specifically — not generically. Confirms what a source read
    already implied: `isekai.md` and `genesis.md` have zero Claude-specific assumptions
    (grep for "claude" turns up only `/don`/`/mint`, which exist *because* Claude Code needs
    a bridge OpenCode doesn't). The only Claude-specific artifact in this repo is
    `model-assignments.md`, and it says so in its own opening line — reference material, not
    a requirement.
  - Added a new main-deck slide ("When a rank hits its ceiling: escalate, never guess")
    illustrating the real Rule III chain, with the Fable/escalation distinction stated
    explicitly rather than left implicit.

### [2026-09-20 23:17] rimuru — Mind usage tracking in tempest.js; lazy-load/shrink documented
- **Task:** "make sure convention lazy load skills when needed too and it should show on
  tempest the skills and how much they are used... implement a shrink mechanism that removes
  skill from context when it's not needed."
- **Files:** .isekai/tools/tempest.js, .isekai/isekai.md (+ propagated to all 4 command
  templates and the mongodb world's isekai.md/tempest.js)
- **Gate:** n/a
- **Result:** done, with two of the three asks reframed rather than built new — they were
  already true of the mechanism, just undocumented
- **Learned:**
  - **Lazy loading already happens — nothing to implement.** A Skill's frontmatter
    `description` is the only part that sits in context by default; the full body loads only
    on invocation (`call the Skill tool first — the skill's instructions load into the turn`).
    This is the harness's own mechanism, not something the convention controls. Documented it
    explicitly in `isekai.md`'s Minds section instead of building a redundant layer, with a
    pointer to the newly-imported `writing-for-agents` Mind for how to write a description
    that actually triggers reliably.
  - **"Remove from context when not needed" isn't a lever this architecture has** — once a
    Mind's body is loaded into a conversation, there's no in-place unload short of the
    harness's own auto-compaction. But the convention already has the mechanism that achieves
    the same practical effect: a Court Body's context (and whatever Mind it wore) dies with
    its task, and only the terse wire report survives to the dispatcher. Documented this
    explicitly under Bodies rather than inventing a new "shrink" concept — it was already Rule
    II + Court mode, just not connected to Minds specifically in writing.
  - **Real, new feature: Mind usage tracking.** Added `harvestSkillUsage(root)` — scans this
    world's own Claude Code transcripts for `Skill` tool_use events (`input.skill`), tallies
    per-name counts, cwd-scoped the same way `harvestClaudeUsage` already is. Wired into a new
    "minds — worn, not raced" table (sorted by uses) and into the neural-graph's Mind-node
    tooltips/sizing (more-used Minds render larger). Verified against raw grep ground truth on
    convention-jura's own transcripts before trusting it (2 genuine `isekai` invocations found;
    the 5 newly-imported Minds correctly show 0 — installed, not yet actually invoked).
  - **Side fix:** found and fixed a pre-existing cosmetic bug the new table's `desc` column
    exposed — a Skill's YAML `description: "..."` (quoted) kept its literal quote marks
    through the extraction regex. Added a small `unquote()` helper, applied at both
    description-extraction sites (skills-shaped and `.isekai/{elf,orc,slime}` shaped).

### [2026-09-20 23:30] rimuru — Rimuru's own context stress, as a measured instrument
- **Task:** "add a context window of 200K like an internal limiter... the system itself can
  stress when session cames closer to 180K it enter stress mode... recommend or ask the human
  let me take a break" — extend Nature 9 (Perception: measured, never guessed) from creature
  stress to Rimuru's own session.
- **Files:** .isekai/tools/context-check.sh (new), .isekai/isekai.md (+ propagated to all 4
  command templates and the mongodb world's isekai.md/context-check.sh)
- **Gate:** n/a
- **Result:** done
- **Learned:**
  - **Real, working instrument, not just a documented threshold.** `context-check.sh` reads
    the running session's own Claude Code transcript (most-recently-modified `.jsonl` under
    `~/.claude/projects/<slugified-cwd>/`) and takes the *last* assistant turn's own
    `input_tokens + cache_read_input_tokens + cache_creation_input_tokens` as current context
    occupancy — not a cumulative sum across turns, which is a different, much larger number
    (total spend, already computed elsewhere by `tempest.js`'s `harvestClaudeUsage`). Tested
    live against this very session: reported 418,671 tokens, matching Claude Code's own
    `/context` reading (385.3k) to within ~7% — close enough to trust for catching a zone,
    not exact enough to bill from.
  - **The 200K budget is deliberately conservative and model-independent**, not tied to
    whichever model's real window happens to be seated on the throne (Rimuru is
    model-agnostic by rule — "the throne does not choose its horse" — and some models have
    far smaller real windows than others). Fired correctly on first real test: this session
    is at 209% of that conservative budget while nowhere near its actual 1M-token model
    window, which is the intended behavior — recommend a break early, not wait for the real
    ceiling.
  - **What "stress mode" actually does, on inspection, is not eviction — nothing can force
    content out of context mid-session.** Documented the three real levers instead: write
    anything not yet durable immediately (insurance against a compaction losing it), dispatch
    remaining heavy work to a Court Body rather than inline (the shrink mechanism already
    documented this session), and tell the human plainly that a break/`/clear`/fresh session
    is warranted. This is a behavioral commitment for Rimuru to self-check at natural
    checkpoints, not an automatic system-level enforcement — named honestly as such rather
    than oversold as automatic.

### [2026-09-20 23:40] rimuru — Stress-relief loop was one-directional; CLAUDE.md/AGENTS.md close it
- **Task:** In the mongodb world (a sibling session), Veldora reported that after a `/clear`
  taken on the stress instrument's own advice, the next session had forgotten everything —
  proof the relief system "isn't working properly." Root cause, found there and true here too:
  `context-check.sh`'s relief steps tell a stressed session to write everything durable to
  `log.md` before a `/clear`, but nothing told the *next* session to read `isekai.md`/`log.md`
  back. This world — the canonical reference — had no `CLAUDE.md` or `AGENTS.md` either, so the
  gap was in the convention itself, not a one-off in one world. Veldora then asked for the fix
  here too, not only in the mongodb world and the global templates. Claude Code auto-loads
  `CLAUDE.md` and OpenCode auto-loads `AGENTS.md` on every session start in a directory,
  including immediately after `/clear` or a fresh OpenCode session — that is the actual
  mechanism "the world remembers in documents, because sessions forget" was always relying on,
  and it was never wired up for either tool.
- **Files:** CLAUDE.md (new), AGENTS.md (new, symlink to CLAUDE.md — one file, both mouths, no
  drift), plus the genome fix (Nature 3 — a command's own procedure, not `isekai.md` itself, so
  no Law 6 gate applies): `.claude/commands/isekai.md` and `.opencode/commands/isekai.md`
  (both here and the two machine-global copies) now write/append `CLAUDE.md` as founding step 7
  and symlink `AGENTS.md` to it, for every world founded from here on.
- **Gate:** n/a
- **Result:** done
- **Learned:** The earlier OpenCode template draft (written in the mongodb world before this
  entry) claimed "OpenCode itself has no equivalent auto-loaded file" — wrong: `tempest.js`'s
  own harvest already reads `AGENTS.md` for routing, so OpenCode plainly does have one. Caught
  and corrected before it could mislead a future `/isekai` run. The write-half of the relief
  loop (write anything not yet durable before a `/clear`) was always sound; the read-half (a
  fresh session of either tool actually reading it back) had no attachment point until now, in
  neither tool, in any world on this machine.

### [2026-09-21T00:33:00+02:00] rimuru — tempest.js synced from the mongodb world: bar chart, log scale, doc modal, portraits, skills clickable
- **Task:** A same-day session in the mongodb world (`.isekai/tools/tempest.js` is the one
  file every registered world's dashboard actually runs off, this world's own copy included)
  worked through a chain of human reports and asks against the live dashboard: an invisible
  consumption chart, a merged all-models view with a genuine linear-scale bug, neural-graph
  label collisions, a "cast doesn't load the md file" gap, a request to show that doc on
  screen instead of buried in a panel, race portraits, and the same doc-loading + label-fit
  treatment for Minds. Full narrative, root causes, and verification method for every one of
  these live in that world's own `log.md` (2026-09-20/21 entries) — not restated here in full;
  this entry is the pointer plus what changed in *this* copy of the file.
- **Files:** .isekai/tools/tempest.js (replaced, synced hunk-by-hunk from the mongodb world's
  already-verified copy — confirmed byte-identical with `diff` after every round)
- **Gate:** n/a
- **Result:** done
- **Learned:**
  - Consumption chart: line chart → grouped bar chart (a single day of data made a `<polyline>`
    invisible — not a data bug, a mark-choice bug). Merged "◆ all" selector added, colored by
    a fixed, already-validated 8-hue race palette (never a new one); its first version was
    genuinely broken on real numbers (~80,000× spread between models on a linear scale hides
    everything but the top series) — fixed with a labeled log10 y-axis, linear view untouched.
    Drill-down bars now match the clicked model's own selector color instead of a fixed
    cyan/violet pair.
  - Neural graph: every node's label offset was measured from the core circle radius, but the
    drawn glow is 1.7× that — fixed to measure from the halo consistently (5 node types). Mind
    labels additionally needed *horizontal* truncation (full name still in the tooltip/modal) —
    the radial fix alone doesn't help a shelf where neighbors are only ~99px apart and a skill
    name can run 25+ characters.
  - New: a `/<world>/doc?name=` route (and `/<world>/portrait/<race>`, keyed only through a
    fixed map — never a raw filename off the URL) serves a creature's or Mind's real doc file
    live, opened in an on-screen modal (not appended to the bottom-of-page focus panel, which a
    top-of-page cast-row click used to update invisibly off-screen). Extended to Minds/skills
    on the same footing as creatures, including the mind table rows, which weren't clickable
    at all before.
  - This session's own shell stayed sandboxed away from this directory for *writes* (a bare
    `cp` was refused) but not for reads (`diff`, `node --check`) or for the Read/Edit/Write
    tool family, which aren't gated by that same classifier — every hunk above was applied
    here by hand through Edit, verified identical to the source world's file, never through a
    blind file copy.

### [2026-09-21T01:05:00+02:00] rimuru — default theme light; stress pill now fills with its own status color
- **Task:** Human, in the mongodb world: "by default load light version, the stress percentage
  pill should be filling with matching color in light mode." Synced here on the same standing
  "update convention dir, commit and push" instruction as the previous entry.
- **Files:** .isekai/tools/tempest.js (both world-page `render()` and the global
  `renderIndex()` now emit `<body class="light">`; `.pill` CSS)
- **Gate:** n/a
- **Result:** done
- **Learned:** Neither page ever wrote a literal `<body>` tag before — the browser's implicit
  one is what `document.body.classList.toggle('light')` was always operating on, so adding an
  explicit `<body class="light">` was the whole fix; the per-world page's `themeBtn` static
  label was flipped from "◐ light" to "◑ dark" to match (it shows the *next* click's action,
  and light is now the starting state). `.pill.cool/warn/hot` had a matching text color but a
  hardcoded dark-only border hex and no background at all — nearly invisible on a light
  surface. Fixed with `background:color-mix(in srgb, var(--cy) 16%, transparent)` (and the
  --gd/--em equivalents) instead of a second light-mode override block — reuses the *same*
  already-light/dark-adaptive tokens the text already used, one definition instead of two.

### [2026-09-21T01:45:00+02:00] rimuru — colony diagram rebuilt (4-column order, zone tints, ascended shelf), context-window chip, skill context-cost, canon version fixed
- **Task:** Continuing chain in the mongodb world, human asks in order: "add a light background
  color per race zone... prepare places for ascended races" and "I want [Minds] after slime
  like 4th level" → clarified twice ("only slime link to skills, be relative" → "or after orc if
  orc orchestrates skill to slime, match the architecture") settling on slime → orc → minds →
  elf; "add how much it costs [a Mind] in context tokens too"; "context window in tempest dash
  at the beginning, with color, so it's seeable" then "don't put % — put tokens, not
  percentage"; and separately "the canon is v9" (root-caused: `canonOf()` only ever checked for
  a root-level `ISEKAI.md` that never existed in the new `/isekai` shape, so the chip always
  read `v?`).
- **Files:** .isekai/tools/tempest.js (copied whole from the mongodb world's already-verified
  copy this round — confirmed byte-identical with `diff`, `node --check` passed here too)
- **Gate:** n/a
- **Result:** done
- **Learned:**
  - **Diagram column order, settled by architecture, not guessed twice more**: after getting
    Minds' position wrong across two earlier redesigns based on text descriptions alone
    (impossible to verify without a screenshot), the human explicitly reasoned it through in
    chat — Orc "rules its domain; commands its Slimes; validates their work" (isekai.md's own
    words), so Orc is the one reaching for a Mind, not Slime directly. Final order: slime → orc
    → minds → elf. The harmony-link data underneath is still not race-restricted (a Mind links
    to *any* creature whose doc mentions it) — this is a visual/architectural framing choice
    layered on top, not a change to what the data actually measures.
  - **Two real bugs caught and fixed while rebuilding, not just moved**: (1) `nodeByName` was
    being built for mind-edge lookups *before* `elfNodes` existed in the node list — any Mind
    linking to an Elf would have silently found nothing. Fixed by deferring the whole mind-edge
    pass to after every node type (including the new ascended shelf) is constructed. (2) The
    mind-node y-position was read from `.map()`'s *index* parameter (accidentally named `y`)
    instead of the actual spread-computed `y` on each item — would have stacked every Mind at
    sequential integer y-coordinates (0,1,2…) instead of their real vertical slots. Caught by
    re-reading the diff before trusting it, not by the syntax checker (both were valid JS).
  - **Minds got a real vertical column** (same spread/below-label treatment as slime/orc/elf)
    instead of a cramped horizontal shelf — the old per-mind name truncation (needed when
    neighbors sat ~99px apart *sideways*) is gone with it; a vertical column doesn't fight its
    neighbor for the same row, confirmed live: `git-guardrails-claude-code` now renders in full.
  - **"Prepare places for ascended races"**: highorc and kijin had *zero* placement logic before
    this at all (only darkelf got a fixed corner) — a creature of either race existing in a
    world's cast table would never have appeared in the graph. Rather than inventing three
    separate speculative column slots, they share one reserved shelf below a dashed divider
    (reusing the exact divider/caption pattern already proven for the old Mind shelf), visible
    and captioned "(none born yet — place reserved)" even at zero population — honest per
    Nature 9, not a fabricated placeholder node.
  - **Zone backgrounds**: one low-opacity tinted `<rect>` per column (slime/orc/minds/elf),
    each using that race's *own* already-validated CSS variable (`--r-slime` etc.) rather than
    a new hex, so light/dark mode both stay correct automatically; the ascended shelf gets a
    neutral dim tint since it can hold three different races at once.
  - **Context-window chip**: new `harvestContextStress()` mirrors `.isekai/tools/context-
    check.sh` exactly (same 200k/180k defaults, same "most recent assistant turn's own usage,
    not a cumulative sum" method) so the terminal instrument and the dashboard chip can never
    silently drift apart. First version showed a percentage ("⋄ context 251%"); the human
    immediately said no — percentage of a soft, admittedly-conservative 200k budget was
    confusing once real usage exceeded it. Switched to a plain token count.
  - **Skill context cost**: a Mind's `kb` (full file size) was never the right number for "what
    does this cost me" — per isekai.md's own Minds section, only the YAML `description` sits in
    context on every turn by default; the full body loads only when actually donned. Added
    `descTok` (≈bytes/4) computed from the description alone, shown in the Minds table, the
    node tooltip, and the focus-panel mind branch — genuinely new information, not a re-unit of
    the existing KB column.
  - **Canon version**: `canonOf()` looked only for a root-level `ISEKAI.md`/`CONVENTION-
    ZERO.md`/`SLIME.md`, a shape from the *elder* convention. Neither this world nor the
    mongodb world has ever had one — the real canon doc has lived at `<home>/isekai.md` since
    the `/isekai` reincarnation, so the chip always read `v?`, silently, for every world on this
    shape. Fixed to check `<home>/` first, falling back to the elder root-level shape. The
    mongodb world's own `.isekai/isekai.md` got an explicit `canon v9` stamp added (the human's
    own number, on direct instruction) so the fix has something real to find there; this
    world's `.isekai/isekai.md` was deliberately left unstamped — its real version number
    wasn't asserted by anyone, and `v?` is the honest reading of that, not a bug to paper over.

### [2026-09-21T01:58:00+02:00] rimuru — colony diagram edges were barely visible, especially against the new zone tints
- **Task:** Human (mongodb world): "links are kind of invisible can you do them better."
- **Files:** .isekai/tools/tempest.js (copied whole from the mongodb world's already-verified
  copy, confirmed byte-identical)
- **Gate:** n/a
- **Result:** done
- **Learned:** `line`/`line.elfedge` were hardcoded for a dark surface (`#22304a`, width .7)
  and were thin even there; the separate `body.light` override (`#c6cfe0`) was even paler
  against a now-default light background, and the new race-zone tint rects from the previous
  entry made the contrast worse still. Fixed by switching to `var(--dim)` — already re-stepped
  per theme for exactly this legibility job — at width 1.3 and opacity .65 (mindedge:
  `--r-mind`, width 1.2, opacity .75, since it carries the use-count label and deserves to read
  as the more specific relationship). Deleted the now-dead `body.light line` override entirely
  instead of updating it — one theme-adaptive rule replaces two hand-tuned ones.
### [2026-09-21T01:20:00+02:00] rimuru — one real biological phenomenon per Nature law, in README and deck
- **Task:** Human: "check the convention and for Nature law go look for a phenomena in nature or
  biology and put its image or describe it in readme and ppt."
- **Files:** README.md (new section "The nine natures, as nature already does them" — 9-row
  table with glyph + phenomenon + description; `natures/` added to the repo tree; presentations
  paragraph corrected from "12-slide" to 16 and the "no LibreOffice" caveat dropped — it is
  installed now and the render was checked); .isekai/natures/*.png (9 new glyphs);
  presentations/isekai-opencode-overview.pptx (new slide 5, slides 5–15 renumbered 6–16);
  .isekai/tmp/pptx-build/build.js + make_icons.js (source of both — gitignored proving grounds).
- **Gate:** n/a
- **Result:** done
- **Learned:**
  - **Read "for Nature law" as one phenomenon per law, not one for the whole set** — the nine
    are each named after a biological idea, so a single analog would have been meaningless.
    Picks were made on *structural* match to the law's actual rule, not the name: Vitality →
    DNA replication checkpoints + p53 apoptosis (record and change land together, stale copy is
    sacrificed); Symbiosis → lichen; Evolution → CRISPR–Cas (the mistake literally mutates the
    genome, at once, inherited); Genesis → quorum sensing (one signal is noise, a quorum is a
    pattern); Memory → hippocampal→neocortical sleep consolidation (analysis up, wisdom down,
    buffer cleared); Swarm → *Dictyostelium* cellular slime mold (colony forms on convergence,
    dissolves after — and it is a slime); Containment → blood–brain barrier (structure, not
    policy); The Wire → waggle dance (point don't carry + fixed schema); Perception →
    proprioception (silent instrument = Ian Waterman's case).
  - **Nature 7 shaped the deliverable**: "image or describe" — photos would have meant network
    fetches, which Containment forbids without Veldora's agreement. Went with descriptions as the
    content plus locally built Font Awesome glyphs (extending make_icons.js, same pipeline the
    deck already used) as the visual. Offered real public-domain photos as a follow-up needing
    consent, not done unasked.
  - **README was stale on its own deck**: said 12 slides, deck had 15; said render QA was
    impossible, but `soffice` is present on this machine now. Fixed both in the same change
    (Vitality) rather than leaving them for later.
  - **Noted, not fixed**: the deck's *source* (build.js, make_icons.js, icons/) lives under
    `.isekai/tmp/pptx-build/`, which `.gitignore` excludes wholesale — only the built .pptx is
    tracked. A fresh clone cannot rebuild the deck. First naming (Nature 4): if it comes up
    again, the source should move out of the proving grounds into a tracked location.

### [2026-09-21T02:05:00+02:00] rimuru — ADOPTED: law update from the bench-forge world's handoff (payload A); payload B pending
- **Task:** Human: "i put imgs of a jura update can you check it and takes those updates?" — 62
  photographs (IMG_2638–IMG_2699) of a screen showing `ISEKAI-UPDATE-agentic-playground.2026-09-21.md`,
  a self-contained handoff prepared by rimuru @ the bench-forge world (Jura) on 2026-09-21 22:00
  CEST on Veldora's word, carrying PAYLOAD A (`.isekai/isekai.md`, 24,489 B, md5
  `61ce0e19248ff0df8db0e1907cd133b0`) and PAYLOAD B (`.isekai/tools/tempest.js`, 150,476 B, md5
  `fa80fe87ee7b009b219c0f4431bf5eb9`).
- **Files:** .isekai/isekai.md (Law 6: on Veldora's order — the handoff states the carrier arriving
  from Veldora's hand IS that order, and the human's "take those updates" here confirms it)
- **Gate:** n/a
- **Result:** payload A adopted; payload B NOT adopted (see Learned)
- **Learned:**
  - **This world was the intended receiver even though the header names `agentic-playground`**:
    the handoff's "target state at send time" matched this disk exactly — law 325 lines /
    20,742 B, tempest.js md5 `b62bbf665d6d68a40c0c3f1708069bac`. Checked before touching anything
    (Nature 9: the doc's claim, verified against the instrument).
  - **Hashes before/after**: law `094e1ff390cc01b4ddb7a92de2894914` (325 L / 20,742 B) →
    `41e487a09a63bf5b022ca077098bd4af` (367 L / 24,488 B). **Not the handoff's md5** — the
    payload did not arrive as text, it arrived as photographs of a curved monitor, so the
    channel was transcription, not extraction. The delta was located by line-number arithmetic
    (offset +110 up to Nature 5, +113 to `## Instruments`) and confirmed to be four regions,
    each transcribed from full-resolution crops: (1) Nature 5's first bullet (desk lives in the
    worn MIND); (2) new **The separation law (words 6–13)** paragraph; (3) new **The creature
    split & the adaptation loop (words 10–11)** with its three bullets; (4) the Minds bullet's
    "Exception (word 11)". Instruments, Bodies, The world, gate and Laws were re-read and are
    unchanged. One byte short of the stated 24,489 (likely a single newline or a dash variant
    somewhere); could not be pinned from the photos and was not guessed (the handoff's own rule:
    never "repair" payload bytes by hand). The literal `&amp;` in the separation-law paragraph is
    reproduced as photographed — it is what the source contains.
  - **Payload B not adoptable from photographs**: ~1,500 lines of dense JS with long wrapped
    lines across ~50 photos cannot be transcribed faithfully; a near-copy of an instrument with
    silent typos is worse than the older working one. Escalated to the human (Absolute Rule III)
    with the choice: bring the handoff as a text file (the awk+md5 recipe then works as
    designed), or have the changelog's nine items re-implemented here on top of the current
    tempest.js as a best-effort port that will not md5-match.
  - **Round-trip noted**: the sender's tempest.js header says it was itself "ported from
    photographs of the source (2026-09-20)" — this world's file, photographed there, now
    photographed back. Second time the photo channel has carried this instrument. Per Nature 4
    that is a need named twice: a text channel between these two machines (a shared repo remote,
    or the .md carrier moved as a file) should exist.
- **Addendum [2026-09-21T02:15+02:00]:** Human chose "re-implement from the changelog" for
  payload B. `context-check.sh` read 184,497 / 200,000 (92%) at that moment — stress zone —
  so per Instruments the port is dispatched to a Court Body from `.isekai/tmp/payload-b-brief.md`
  rather than run inline; only its wire report returns here. Human told plainly.

### [2026-09-21T23:30:00+02:00] rimuru — ADOPTED: payload B (tempest.js) landed by re-implementation, via Court Body
- **Task:** Human: "continue" after `/clear` — resume the payload B port dispatched in the previous
  entry's addendum. That dispatch never landed: the `/clear` came first, and tempest.js was still
  at md5 `b62bbf665d6d68a40c0c3f1708069bac` on re-entry (checked before re-dispatching — Nature 9).
- **Files:** .isekai/tools/tempest.js (1655 → 2124 lines, md5 `565ea89687f2aeaa6df14218a871ffa8`
  — a re-implementation from the handoff changelog + IMG_2650–2699, NOT the sender's bytes; header
  comment updated in the same change, Vitality)
- **Gate:** n/a (no orcs in this world; Rimuru verified against the instrument directly)
- **Result:** done — all nine changelog items carried; definition of done re-run by Rimuru after
  the Court reported: `node --check` ok · `--json` one parse-clean line (23 keys, 6 minds, 4
  commands) · `--ensure` → `/convention-jura/` 200 with all seven lane ids (smind · slime · omind ·
  orc · gmind · elf · asc), SHARED SKILLS row, GREAT-SAGE → RAPHAEL → CIEL triad at '—' ×3,
  TRUTH/VERDICT/ANIMA bond classes present · `doc?name=tdd` → `@S:MAP` (5 sections, measured
  bytes) · `&sec=1` → `@S:SEC` · bad name → 404 · `--stop` takes the board down.
- **Learned:**
  - **Dispatch-and-discard worked as the law describes it**: the Court burned ~276k tokens over
    107 tool uses reading photographs; Rimuru received 3,210 bytes of wire and spent one
    verification pass. That is the whole point of the Court mode — the previous session at 92%
    could not have done this inline.
  - **A Court's PASS is a memory until checked** — every claim in its report was re-run here
    before this entry was written. All held.
  - **Court's own holes (`@?`), kept honest, not patched over**: the server doc-route body was
    never photographed (rebuilt from the visible client contract); command harvest and the
    `.claude/skills` merge came from the brief's facts, not photos; TRIAD regexes were anchored to
    name segments (departure from the photographed `/raphael|mon/` form, flagged in code); the
    photographed CSS drew elfedge dashed in lane view — the changelog's shape law (solid = race
    bond) was applied instead; a lane-tint height that overshot the row-2 divider by 28px in the
    photo was corrected; changelog items 8/9 (succession, propagation ≠ cloning) are process, not
    code — recorded in the header only.
  - **The 62 photographs have now done their job** (A transcribed, B ported). Their removal is a
    deletion → Law 6, asked of Veldora, not assumed.
