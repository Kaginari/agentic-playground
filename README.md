# Isekai

**Isekai** is a convention for turning a directory into a small, self-documenting multi-agent
"world." A directory that adopts it gets a `.isekai/` folder holding the world's law, its
change log, and the creatures (agents) that work in it — so that AI coding sessions, which
forget everything between runs, can pick up exactly where the last one left off by reading
what's on disk instead of guessing.

## The idea, briefly

- **Slimes** hold narrow ground truth about one area of the codebase.
- **Orcs** rule domains and gate every change before it lands.
- **Elves** keep the shared mind and speak back up to the human.
- **Rimuru** — the session agent itself — thinks, decides, and orders, working through the
  Elf by default. Rimuru is a *machine-global throne body*: one agent, installed once per
  machine, that rules whichever world it's currently standing in.

Documents are memory (what the world knows it wrote down); instruments are perception (what
the world can check about itself right now). Nine "nature laws" — Vitality, Symbiosis,
Evolution, Genesis, Memory, Swarm, Containment, The Wire, Perception — govern how creatures
are born, how they remember, how they talk to each other economically, and how nothing lands
without a gate check. The full text lives in [`.isekai/isekai.md`](.isekai/isekai.md).

## What's in this repo

```
.isekai/
├── isekai.md       # the Reincarnation Convention in full — the law every creature reads
├── log.md          # append-only chronicle of changes made under this convention
├── LICENSE         # this project's license
├── portraits/      # one portrait per race (rimuru, elf, orc, slime, kijin, high_elf, high_orc, dark_elf)
├── instruments/    # raw signals the world watches itself with
└── elf/ orc/ slime/ tmp/   # where creatures and scratch work live as the world grows

.claude/commands/   # /isekai, /don, /mint, /genesis — the Claude Code slash commands that operate on this convention
.opencode/commands/ # /isekai, /genesis — the OpenCode port of the same commands (/don and /mint have no OpenCode equivalent)
```

- **`/isekai [target-dir]`** reincarnates a directory: it writes `.isekai/isekai.md`,
  `.isekai/log.md`, the portrait set, and the empty creature directories, then — on a *fresh*
  reincarnation only — runs `/genesis`'s survey-and-populate flow in the same pass (offer-first:
  it shows the proposed Elf/Orc/Slime population before writing any creature). Re-running it on
  a world that already exists never overwrites what's there and never auto-populates again.
- **`/don`** (Claude Code only) brings a *Mind* (a skill) in from `.opencode/skill(s)/` into
  `.claude/skills/`.
- **`/mint`** (Claude Code only) brings a *Body* (an agent) in from `.opencode/agents/` into
  `.claude/agents/`.
- **`/genesis`** (both Claude Code and OpenCode) scans a world's code and decides on its own
  which Elves, Orcs and Slimes it needs, then births them — Nature 4 (Genesis) invoked
  directly, offer-first. `/isekai` already runs it automatically the first time a directory is
  reincarnated; call `/genesis` directly later to re-survey a living world or fill in what's
  missing. Two depth levels: `--depth code` (default) reads each zone's current source before
  classifying it; `--depth deep` additionally mines that zone's git history (`git log`/`git
  show`) for the fixes, reversals and incidents that explain *why* it looks the way it does,
  not just what it looks like now. Simpler and narrower than the unmerged canon "Phase 2b"
  populate logic described in
  [`.isekai/canon/isekai-phases.md`](.isekai/canon/isekai-phases.md) (no `AGENTS.md`, no
  Bodies offer, no instruments); kept as its own command rather than folded fully into `/isekai`.

Both Claude Code and OpenCode can run `/isekai` and `/genesis`; the world document and
population they produce are the same either way, since `.isekai/isekai.md` already treats
`.opencode/` and `.claude/` as two coexisting homes for the same Minds and Bodies.

## Minds shipped in this repo

- **`palette-audit`** (`.opencode/skills/palette-audit/`, ported to
  `.claude/skills/palette-audit/`) — audits a UI's color palette and typography for
  accessibility using computed OKLCH/CVD/contrast checks (bundles its own validator script)
  instead of eyeballing, and insists dark/light modes each get their own re-stepped palette
  rather than reusing identical hex with the background swapped. A working example of the
  `/don` cross-ecosystem mechanism: authored natively for OpenCode, ported byte-identical into
  `.claude/skills/` — proof this convention actually runs Minds in parallel across both.

## Using it elsewhere

Run `/isekai` (or `/isekai <path>`) in any directory to reincarnate it as its own world,
independent of this one. Each world keeps its own `.isekai/`; only the command definitions
and portrait assets are shared machine-wide.
