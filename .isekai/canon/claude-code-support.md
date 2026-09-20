# tempest.js: Claude Code support

**Not transcribed from source** — like `model-assignments.md`, this is new reasoning, added
2026-09-20, not a photograph transcription. It documents an asymmetry `tempest.js` had and how
it was closed: the instrument board was built OpenCode-first and silently returned empty data
for a Claude Code world, rather than failing loudly — the kind of gap Nature 9 (Perception)
exists to catch, once someone actually looked at the board and asked where the population went.

## What was OpenCode-only, and what each gap actually was

1. **Population discovery** (`harvest()`'s `creatures`/`orcs`). Originally read
   `.opencode/skills/<name>/SKILL.md` for creatures and a root `AGENTS.md` routing table for
   the Orc→Slime tree — the fuller, not-yet-merged canon shape (see `isekai-phases.md`'s Phase
   2b). The *operative* `/isekai` + `/genesis` commands never produce either: they write
   `.isekai/{elf,orc,slime}/<name>/README.md` directly, no `AGENTS.md`. A world populated the
   operative way showed zero creatures on the board — not because nothing was there, but
   because the board was looking in a shape that command doesn't write.
   **Fix:** `harvest()` now also scans `.isekai/{elf,orc,slime}/*/` for a `README.md` (or the
   first `.md` file present), merged in alongside any `.opencode/skills/`-shaped creatures
   (not a replacement — a world could have both). The Orc→Slime tree is read straight off each
   Orc doc's own `- **Commands:** slime-a, slime-b` line instead of requiring `AGENTS.md`.
2. **Minted bodies** (`agentsDir`, the "Suggested models" and agents-ledger sections). Only
   ever looked at `.opencode/agents/`. Claude Code's own Bodies, brought in by `/mint`, land at
   `.claude/agents/` — a world with only Claude-minted Bodies showed none.
   **Fix:** now scans both `.opencode/agents/` and `.claude/agents/`, merged. Claude Code
   sub-agent frontmatter carries no `mode:` field (no primary/all distinction the way OpenCode
   has) — every `.claude/agents/*.md` file defaults to `mode: subagent` (Court-shaped, Agent-tool
   invoked) instead of showing an unhelpful `?`.
3. **Live token usage** (`live`, `agentUse` — the dashboard's usage tables and the
   "Suggested models" card's actual-usage column). Read only from OpenCode's session store,
   `~/.local/share/opencode/opencode.db` (SQLite: a `session` table with `model`, `agent`,
   `tokens_input`, `tokens_output`, `time_created`, `directory`). Claude Code has no such
   database.
   **Fix:** `harvestClaudeUsage(root)` reads Claude Code's own session transcripts instead —
   one JSONL file per session at `~/.claude/projects/<slugified-cwd>/<sessionId>.jsonl`.
   Assistant-turn lines carry `message.model`, `message.usage.{input,output}_tokens`, and a
   `timestamp`; `cwd` is matched against `root` with the same prefix semantics as the
   OpenCode SQL query's `directory LIKE root + '%'`. Its output is merged additively into
   `live`/`agentUse` via `mergeLive`/`mergeAgentUse`, not a replacement — a machine running
   both ecosystems against the same world sees combined numbers.

## The one place this is honestly coarser, not just different

OpenCode's `session.agent` column names the actual mounted body (`orc-provider`,
`slime-config-file`, ...) that answered each session — precise, per-body attribution. Claude
Code's transcripts carry no equivalent field for "which minted sub-agent handled this turn";
the only signal available is `isSidechain` (a sub-agent fork vs. the main session). So
`harvestClaudeUsage`'s agent identity is coarse: `'subagent'` vs `'main session'`, not a body
name. This is deliberate, not an oversight — inventing a fake per-body label from
`attributionSkill` (which names a *skill*, not a *sub-agent*) would have been more precise-
*looking* and less honest. If Claude Code transcripts ever gain a reliable per-sub-agent field,
tighten this — don't work around its absence by guessing.

## What wasn't touched

`/holidays` relief-run dispatch (`spawn(cli.bin, cli.args(...))`) already had its own
Claude/OpenCode fallback before this pass — that fix predates this doc and isn't repeated here.
Nothing about `/genesis`, `/isekai`, or the `.isekai/{elf,orc,slime}/` doc shape changed; this
pass only taught the *board* to read what those commands already produce.
