# Templates

Transcribed from photographs. See [`README.md`](README.md).

## `AGENTS.md` colony block

Written by Phase 2b step 3 (see [`isekai-phases.md`](isekai-phases.md)) — **appended** to
`AGENTS.md` (creating it if absent), never replacing existing content.

```markdown
## ISEKAI world — creature minds route the work

This repo is an ISEKAI world (see ISEKAI.md — nature laws 0–8 rule everything).
**First-action gate (absolute):** the moment a request lands in an area — even
read-only — load its slime `.opencode/skills/slime-*/SKILL.md` **before the first
tool call**; cross-cutting work loads its orc first. Unrouted area → stop and name
the gap out loud (genesis: a need named twice births a mind) — never edit unrouted
territory silently. The colony is alive: every area change updates its mind in the
same change on disk; a stale mind is misinformation. No change lands without its
orc's gate pass, journaled to `.isekai/log.md`. Nobody pushes — pushing is the
human's act.

| Orc | Domain | Slimes |
|---|---|---|
| `orc-<domain>` | <domain words> | `slime-<zone>`, … |

Above the orcs: `elf-colony` — session context, external-relations map, public
speaker. Instruments: `node .isekai/tools/tempest.js . --ensure` → board at
`localhost:<port>/<world-name>/` (the world names itself in `.isekai/name`; the
board sleeps after 30 min of silence — re-`--ensure` is a heartbeat);
sessions drop a token line into `.isekai/metrics/tokens.jsonl`; scratch lives in
`.isekai/tmp/<YYYY-MM-DD>/`.
```
