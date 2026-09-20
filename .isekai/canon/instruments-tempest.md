# 8. Instruments — the world measures itself

Transcribed from photographs. This is the detailed spec behind Nature 9 (Perception) and the
`.isekai/instruments/` folder that `/isekai` already scaffolds.

`.isekai/tools/tempest.js` started as an implementation of this prose spec, with no source
code ever seen. It is now, instead, a **faithful port of the actual source** — photographed
across 20 images on 2026-09-20 (see `.isekai/canon/README.md`'s note on that session), roughly
lines 2–747 of it continuous, missing only the HTTP route table at the very end (which this
session wrote itself, built to satisfy exactly what the fully-visible client-side script
calls). The port includes things this prose spec never even hinted at: the real creature
source is `.opencode/skills/*/SKILL.md` (not a separate `.isekai/<race>/<name>/` tree), an
elder `.convention-zero`/`CONVENTION-ZERO.md`/`SLIME.md` fallback for older worlds, real
per-model/per-agent token stats read live from OpenCode's own `~/.local/share/opencode/
opencode.db` SQLite store, a canon-vs-`.drawio`-chart version-debt check, an interactive SVG
"neural-layer" colony graph (slimes → orcs → elf, radius ∝ doc weight, glow = stress), and a
harm-fenced relief system: snapshot a creature's skill dir before a relief run, hard-restore
its frontmatter and re-verify desk stress after. One resolved mystery from the earlier,
partial transcription below: the line that looked like garbled "# perty" was the 🎉 **party**
button — an action that logs Nature 4's second naming for genesis-watch births, distinct from
🎒 **holidays** (the relief dispatcher).

One deliberate departure kept from this session's earlier, spec-only implementation: the real
source hardcodes `opencode run` for relief dispatch; the port instead detects `opencode` first,
falling back to `claude -p` (Claude Code's own headless mode), so relief runs work on
Claude-only machines too rather than silently never launching. See the comments in
`tempest.js` itself for where else the port made a documented, literal choice instead of
guessing at what a handful of remaining gaps might have said (see `README.md`).

---

Memory cannot see its own stress, so every world carries an instrument panel:
`.isekai/tools/tempest.js` (stdlib node, zero dependencies, the shape travels). Every world
names itself at birth in `.isekai/name` (one line — jura-style names lawful; the little file
travels) and its board answers at `http://localhost:<port>/<name>/` (the port derived from
the world's root — many worlds, many boards, no collisions). It harvests live, per request:
the census (minds by race), desk stress (dated `## Thoughts` vs limit — 5 ground, 10 dark elf,
the 6KB diet — breath law), crossings and the genesis watch (stress ≥ 80% × links ≥ 1.5×
median — a heuristic; a birth still needs its need named twice), the orc → slimes tree from
the map, the evolution series (journal / thoughts / commits day), and the token bucket
(sessions append `{"ts","agent","model","in","out"}` to `.isekai/metrics/tokens.jsonl`; the
board also reads opencode's session store, colony-scoped, read-only).

- **Instruments stay lit — and sleep with the world:** a session's first act runs
  `node .isekai/tools/tempest.js . --ensure` — idempotent, and doubles as the session's
  heartbeat (a lit board treats it as a lease renewal). The board sleeps when the world rests:
  any request renews the lease, `--ttl` (default 30 minutes) of silence puts it down, an open
  board page holds it awake (it pulses every minute), `node .isekai/tools/tempest.js --stop`
  sleeps it now, `--immortal` opts out for a human's long watch.
- **The board prescribes, records, dispatches — it never writes a mind.**
  `% holidays [POST /<name>/holidays, ?dry=1 previews, /only=<name> scoped]` with the dated
  relief worklist into `.isekai/tmp/<day>/` and launches sequential creature-hat relief runs
  (opencode run) wearing one mind's dist: distil the desk → diet split → genesis-watch
  review after; every mind's frontmatter byte-check after; a snapshot before, 10-minute cap
  per step; a snapshot after; every step logged to `.isekai/metrics/relief.jsonl`. *(the tail
  of this line was hard to read in the source photo — "# perty" — likely cut off or a typo in
  the original; transcribed as photographed)*
