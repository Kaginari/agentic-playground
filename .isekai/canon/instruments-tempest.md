# 8. Instruments — the world measures itself

Transcribed from photographs. This is the detailed spec behind Nature 9 (Perception) and the
`.isekai/instruments/` folder that `/isekai` already scaffolds. No source implementation was
ever photographed — only this prose. It has since been implemented at
[`../tools/tempest.js`](../tools/tempest.js) (stdlib Node, a Bootstrap 5 dashboard, `--ensure`
/`--stop`/`--json`, the `/holidays` relief endpoint). Where this spec was ambiguous (the exact
definition of "crossings"/"links" for the genesis watch, resolving which orc owns which
slime), the implementation makes a documented, literal choice rather than guessing further —
see the comments in `tempest.js` itself. See [`README.md`](README.md) for the remaining gaps
in the spec (the token-bucket section mentions reading "opencode's session store,
colony-scoped, read-only" — that integration is a stub, since no schema for it was ever
captured).

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
