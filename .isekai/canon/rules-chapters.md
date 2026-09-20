# The E-protocols and the operational rules

Transcribed from photographs (source: `ISEKAI.md`, `ISEKAI-GUIDE.md` preview panes). See
[`README.md`](README.md) for what this file is and its known gaps.

## The E-protocols (Minds & Bodies)

- **E1 — Minds hold memory; bodies hold hands; ledgers hold the world.** Expertise, thoughts,
  and decrees live in minds. Modes, permissions, and mounts live in bodies. History lives in
  the chronicle. No mind carries another's file; no body stores memory.
- **E2 — Every creature is born a mind, and a mind only.** Genesis (nature 3) births skills.
  The default world has zero agent files.
- **E3 — Hands are minted offer-first.** A body is minted only when autonomous work names the
  need twice and the human says yes: a Keeper for housekeeping, a court rider for disposable
  heavy reads/verdicts/drafts, a creature's own body for standing duty. The body wears the
  mind's name. The convention seed ships the triad office genomes (`offices/`) — Great Sage
  reads, Raphael judges, Ciel speaks — minted per world as
  `<world>-great-sage|-raphael|-ciel`. (The pun is doctrine: the session is a slime hosting
  the skill line — slime + Raphael = Rimuru.)
- **E4 — The subagent contract.** Every dispatch carries: territory granted, read-only vs
  write, the output shape (`@ASK ... @CAP`), and the absolute laws rebound (no-push, no-secret
  echoes). Findings land through the orc gate like any other change; the task's death carries
  nothing away — findings are distilled into docs in the same change.
- **E5 — A loaded skill is a granted territory.** The session (Rimuru) wears minds as hats;
  wearing `slime-x` grants exactly its zone. Nothing else is actionable. A skill already
  injected this session is never re-injected.
- **E6 — No body without a named office.** An agent file that is neither the declared Keeper
  nor a court rider nor the named body of a mind is an ungated birth — a finding, never a
  shrug. Bodies live git-ignored; they are machine state, not doctrine.

## The operational rules

Numbers are append-only history — never renumbered. Chapters are the reading order.

### Chapter I — Creatures (1–3)

1. **One slime = one narrow zone** (`.opencode/skills/slime-<zone>/SKILL.md`). One orc = one
   domain (`orc-<domain>`), ruling related skills with a domain-level synthesis of them.
2. **Shared traits = coupling.** If a slime's work stops matching its orc's domain, they
   separate — the slime is re-homed or becomes a new domain.
3. **Slimes migrate.** A slime doing another orc's work moves there — both orcs and the map
   updated in the same change.

### Chapter II — Order (4–7)

4. **The first-action gate — load what you need.** The moment a request lands in an area —
   even read-only — load its slime before the first tool call; cross-cutting work loads the
   orc first. Unrouted area? Stop and name the gap out loud — never edit unrouted territory
   silently.
5. **The orc gate — nothing lands without its orc's pass.** Before landing, the orc verifies:
   (a) the right slime authored — nothing trespassed; (b) the domain's traits still hold; (c)
   the duties were done (tests, docs); (d) the mind's doc is still truthful within diet
   (same-change update). The verdict (pass / fail + reason) is appended to the chronicle.
6. **Elves stand above orcs — context and the voice.** The elf tracks the session (what was
   decided, what was loaded, what changed), holds the relations map, rules disputes, and
   drafts every outward message (user summaries, handoffs). New external connections are elf
   business.
7. **Orcs speak with orcs; the elf chains domains.** Slimes never bypass their orc;
   multi-domain work is ordered domain by domain by the elf.

### Chapter III — Habitat (8–9)

8. **Details live in minds, never in `AGENTS.md`.** The map stays lean: the gate law, the
   routing table, and pointers. It never duplicates a creature's content.
9. **New narrow zone → new slime; new domain → new orc; new outward duty → an elf.** Minds
   are fact-dense: paths, invariants, pitfalls, `file:line` anchors.

### Chapter IV — The membrane (10–11)

10. **Territory (absolute).** Slime → only files named in its doc. Orc → its whole domain,
    never a neighbor's (it speaks to that orc). Elf → any territory inside its world; toward
    another world it only negotiates. The session — only what its loaded skills grant. World
    border = the git repo root (or the adopted directory). Sole exception: the dark elf
    (global sight, never global hands).
11. **No-push (absolute, security).** Nobody pushes; the human pushes. Asking or urgency never
    overrides. Deleting, history rewriting, or anything irreversible needs Veldora's approval;
    `ISEKAI.md` itself changes only on Veldora's word.

### Chapter V — Tracking (12–13)

12. The skeleton travels; the living stays — versioned and *(the photographed source cuts off
    here; the rest of rule 12 and all of rule 13 were not captured — see `README.md`)*.

    …the source resumes mid-sentence, apparently near the end of rule 13: "…skill. On
    contradiction, name the divergence to the human and update the skill, mind not the plain
    skill." The content of rule 13 itself (what it establishes) is missing.

### Chapter VI — Breath & the disk (14–18)

14. **The diet (~6KB).** Every mind is a strict diet: load trigger + invariants + pitfalls.
    Deeper detail lives in `reference/` files inside the creature's own dir, pointed to by
    one-liners and read only when hit. The orc gate and the instruments both check the budget.
15. **Remote reads.** Wide exploration goes through a court subagent — its context dies with
    the task; only `file:line` condensed findings return. No re-loads, no hoarding.
16. **Scratch lives in-world, dated.** Experiments, dumps, worklists, relief snapshots:
    `.isekai/tmp/YYYY-MM-DD/` — never outside relief.
17. **The shed.** A session that grows too big writes `.isekai/tmp/<date>/checkpoint.md`
    (goal / landed / decisions / pending / next act / minds loaded) and may die cleanly; the
    next session boots from checkpoint + chronicle tail only — never transcripts. Sheds are
    journaled like gate stamps.
18. **Diaries.** The world's bodies (Keeper, court riders) keep dated append-only diaries at
    `.isekai/agents-diaries/<name>.diary.md` — the root served, what drifted, what was fixed,
    the mood line. The chronicle tells the world what happened; a diary tells a body who it
    has been. Neither is ever destroyed.

### Chapter VII — The law of laws (19–21)

19. **The law cascade — laws flow downward, never up.** Veldora's word > this canon >
    dark-elf divine rule > elf decree > kijin divine rule > high-orc divine rule > orc decree
    > slime genome. An elf may decree for all orcs; an orc may decree for its own slimes;
    slimes never decree — they [amend?] the subject's doc. A decree contradicting a higher
    law is void — reported up, never silently followed.
20. **Cooling.** A machine-proposed canon change rests as a dated journal proposal and becomes
    numbered only on a later pass, re-verified against reality. The human's decrees land
    immediately.
21. **The canon stamp.** This file carries a version comment (`canon vN — <date>`). A
    sub-world under an adopted tree carries a pointer doc: header naming the source + its
    canon version + "on divergence the source wins", and a dated local delta below that may
    exceed, never contradict, the canon. Derived pictures, if a world grows them, are
    re-stamped in the same change as the canon.

### Addendum — the domain table (elaborates Chapter III)

This turned out to be a fragment of the `AGENTS.md` colony-block template — see
[`templates.md`](templates.md) for the complete, correctly-sourced version.
