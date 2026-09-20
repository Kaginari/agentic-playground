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
