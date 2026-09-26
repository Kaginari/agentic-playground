# Agent-One

**Agent-One** is the isekai convention in plain IT / AI-engineering vocabulary. Same rules,
same mechanics, same engine — only the names and the voice change.

- The law is [`AGENT-ONE.md`](AGENT-ONE.md): a faithful translation of `.isekai/isekai.md`.
  Every principle, absolute rule, policy, the review gate, the wire protocol, the memory tiers,
  the unsaid, the loop, the toolbox, the roles and the agent modes are there, one to one.
- The wire is shared: `@ROOT @SCOPE @ASK @CAP @DUMP @SIZE` and `@S @F @V @? @U @E` are
  identical in both vocabularies. They are the protocol, not the theme.
- One Go binary serves both. It detects the vocabulary from the workspace directory it finds
  (`.isekai/` or `.agent-one/`) — or from the name it was invoked as — and renders every
  role, path, heading and UI string through [`lexicon.json`](lexicon.json).

## What differs, what stays

| Stays identical (protocol / files) | Rendered through the lexicon |
|---|---|
| wire tags, `@T`/`@TOOLS` dialect | rank names, agent-name prefixes, role directories |
| `log.md`, `name`, `memory/`, `toolbox/`, `instruments/` layout and formats | the law's file name (`isekai.md` → `AGENT-ONE.md`) |
| step classes `read · write · outward · destructive` | the `## Thoughts` heading (→ `## Working notes`), the `Territory:` doc key (→ `Owns:`) |
| memory kinds `episodic · procedural · semantic`, tiers `short · long · shared` | the unsaid kind tokens (see below) |
| ontology property names (`truth`, `verdict`, `wears`, `owns`, `knows`, `about`) | their display names, the board's UI strings, the host commands |

**Unsaid kind tokens.** Agent-One speaks its own tokens on the wire and on the CLI —
`@U policy|team|domain`, `--kind policy|team|domain` — because a human reads `@U` lines and the
themed words would leak. On disk (`memory/shared/notes.jsonl`, and especially the machine-shared
`~/.agent-one/shared/notes.jsonl`, which may cross workspaces of both vocabularies) the engine
stores the **canonical** token (`law|colony|territory`) and accepts either spelling on input.
The lexicon records both under `kind.unsaid.*` (`token` per vocabulary, `canonical` on disk).

## Lexicon schema

`lexicon.json` is one flat object; the Go engine loads it once and looks entries up by key.

```json
{
  "schema": 1,
  "vocabularies": ["isekai", "agent-one"],
  "canonical": "isekai",
  "entries": {
    "rank.slime": {
      "isekai": "Slime", "agent-one": "Zone worker",
      "plural": { "isekai": "Slimes", "agent-one": "Zone workers" },
      "prefix": { "isekai": "slime-", "agent-one": "zone-" },
      "dir":    { "isekai": "slime",  "agent-one": "zone" }
    },
    "kind.unsaid.colony": {
      "isekai": "colony", "agent-one": "team",
      "token": { "isekai": "colony", "agent-one": "team" },
      "canonical": "colony"
    }
  }
}
```

- `schema` — integer, bumped on any incompatible change to this shape.
- `vocabularies` — the vocabulary ids; every entry carries one display string per id.
- `canonical` — the vocabulary whose tokens are stored on disk when a value must be shared
  across vocabularies (kind tokens, `serves` lanes, registry kinds, ontology properties).
- `entries` — keyed `<namespace>.<name>`, lowercase `[a-z0-9_]` segments joined by `.`.
  Keys are stable identifiers; renaming a key is a schema change.
- Each entry has, at minimum, `isekai` and `agent-one`: the display string in that
  vocabulary. `null` means the concept does not exist there (e.g. `dir.portraits` in agent-one).
- Optional facets, each an object keyed by vocabulary unless noted:
  - `plural` — the plural display string.
  - `short` — a short display form (`Court Body` → `Court`; `Ephemeral subagent` → `subagent`).
  - `prefix` — the agent-name prefix for a rank, dash included (`orc-` → `domain-`). Invariant:
    `prefix == dir + "-"` for every rank that has a directory.
  - `dir` — the directory name, relative to the workspace root or as noted.
  - `token` — the value emitted on the wire / accepted on the CLI in that vocabulary.
  - `canonical` — a single string: the value stored on disk regardless of vocabulary.
  - `note` — free text for humans; never rendered.

Namespaces: `convention.*` (general terms), `rank.*`, `unit.*` (mind/body/court/keeper),
`mode.*` (agent frontmatter), `verb.*`, `principle.N`, `section.*` (law headings), `dir.*`,
`file.*`, `tool.*`, `cmd.*` (host slash commands), `heading.*` and `field.*` (parsed doc
markers), `kind.unsaid.*`, `kind.memory.*`, `tier.*`, `toolbox.kind.*`, `lane.*`, `bond.*`,
`triad.*`, `class.*`, `gate.*`, `ui.*` (board and command strings).

## Term mapping

| Isekai | Agent-One | Key |
|---|---|---|
| Veldora | Operator (the human) | `rank.veldora` |
| Rimuru | Orchestrator (session agent, machine-wide) | `rank.rimuru` |
| Elf · `elf-` · `elf/` | Coordinator · `coord-` · `coord/` | `rank.elf` |
| Orc · `orc-` · `orc/` | Domain owner · `domain-` · `domain/` | `rank.orc` |
| Slime · `slime-` · `slime/` | Zone worker · `zone-` · `zone/` | `rank.slime` |
| Kijin · `kijin-` | Service owner · `service-` · `service/` | `rank.kijin` |
| High Elf / High Orc | Principal coordinator / Principal domain owner | `rank.high_elf`, `rank.high_orc` |
| Dark Elf | Auditor · `auditor-` · `auditor/` | `rank.dark_elf` |
| ascended | principal | `rank.ascended` |
| Mind | Skill | `unit.mind` |
| Body | Agent | `unit.body` |
| Court Body | Ephemeral subagent | `unit.court_body` |
| Keeper | Persistent agent | `unit.keeper` |
| creature / race | member / role | `convention.creature`, `convention.race` |
| world / reincarnate | workspace / onboard | `convention.world`, `convention.adopt` |
| territory | ownership (owned paths) | `convention.territory` |
| trait | invariant | `convention.trait` |
| genome | configuration | `convention.genome` |
| desk / thought / distill | working notes / note / consolidate | `convention.desk`, `convention.thought`, `convention.distill` |
| mind stress | skill overload | `convention.mind_stress` |
| sacrifice | decommission | `convention.sacrifice` |
| birth / born | provisioning / provisioned | `convention.birth` |
| ascension | promotion | `convention.ascension` |
| colony (Nature 6) / colony (the population) | cluster / team | `convention.colony_group`, `convention.colony_population` |
| elder | parent | `convention.elder` |
| proving grounds | sandbox (`tmp/`) | `convention.proving_grounds` |
| crest / breaths | core principles / one-liners | `convention.crest`, `convention.breath` |
| Nature N / Law N | Principle N / Policy N | `convention.nature`, `convention.law` |
| the gate | the review gate | `convention.gate` |
| mouth / siphon | endpoint / channel | `convention.mouth`, `convention.siphon` |
| commission / answer | request / response | `convention.commission`, `convention.answer` |
| don / wear / mint | load / hold / register | `verb.don`, `verb.wear`, `verb.mint` |
| Vitality | Docs-as-code | `principle.1` |
| Symbiosis | Single responsibility | `principle.2` |
| Evolution | Continuous improvement | `principle.3` |
| Genesis | Provisioning | `principle.4` |
| Memory | Memory consolidation | `principle.5` |
| Swarm | Clustering | `principle.6` |
| Containment | Egress control | `principle.7` |
| The Wire | Wire protocol | `principle.8` |
| Perception | Observability | `principle.9` |
| Principles (section) | Design axioms | `section.principles` |
| The world (section) | Roles | `section.world` |
| `.isekai/` · `~/.isekai/` | `.agent-one/` · `~/.agent-one/` | `dir.root`, `dir.machine` |
| `canon/` | `design/` | `dir.canon` |
| `natures/` · `portraits/` | `principles/` · (none) | `dir.natures`, `dir.portraits` |
| `isekai.md` | `AGENT-ONE.md` | `file.law` |
| `tools/tempest.js` · tempest | `tools/board.js` · observability board | `tool.tempest`, `ui.board` |
| `isekai` (binary) | `agent-one` | `tool.binary` |
| `/isekai` · `/genesis` · `/don` · `/mint` | `/agent-one` · `/provision` · `/import-skill` · `/import-agent` | `cmd.*` |
| `## Thoughts` · `Territory:` | `## Working notes` · `Owns:` | `heading.thoughts`, `field.territory` |
| `@U law` · `colony` · `territory` | `@U policy` · `team` · `domain` (disk: canonical) | `kind.unsaid.*` |
| lanes zone · verdict · global · shared | zone · review · global · shared | `lane.*` |
| truth-current · verdict-current · anima-thread | ground-truth link · verdict link · skill link | `bond.*` |
| GREAT-SAGE → RAPHAEL → CIEL | ANALYST → JUDGE → DRAFTER | `triad.*` |
| holidays · party · genesis watch · census | consolidation run · confirm need · provisioning watch · roster | `ui.*` |

## Role badges

`portraits/` holds one 512×512 badge per role — the same white rounded tile as isekai's portraits,
with a [Tabler Icons](https://tabler.io/icons) glyph (MIT, `portraits/LICENSE-tabler-icons`) in the
role's colour; principal roles carry a crown. They are generated, never hand-edited:
`presentations/src/make_role_badges.js`.
