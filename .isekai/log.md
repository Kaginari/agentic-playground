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
