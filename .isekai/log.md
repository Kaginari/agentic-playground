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
  - **Caveat named by the human, not yet tested:** this measured one task, one cold session
    per trial — it could not detect prompt-cache amortization (repeat reads of the same
    docs get cheap within a session) or accumulated-knowledge amortization (task 2 in the
    same zone shouldn't need to redo task 1's archaeology) across a *sequence* of tasks,
    since every trial started fresh with no continuation. The real "does cost drop over a
    session" question is still open — a follow-up would need `claude -c`/`--continue` to
    chain a second, different real task onto each of these same 6 trial directories and
    compare marginal (not total) cost.
