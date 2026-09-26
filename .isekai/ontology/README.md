# Ontology

The world's relations, formal enough to check and projected to plain text for prompts
(`canon/binary.md § The ontology`).

- `schema.ttl` — the classes, bonds and shapes, one prefix (`is:`). The isekai binary carries the
  same file built in; this copy is the world's and wins when present. Change both together.
- `graph/*.ttl` — asserted facts. `graph/unsaid.ttl` is append-only: every `@U <kind> …` a Court
  reports becomes one Fact here (`isekai onto assert <body> <kind> <text>`).
- Everything else — creatures, bonds, territories, worn minds — is derived at load from
  `.isekai/{elf,orc,slime,kijin}/<name>/` and the Minds on disk. Derived triples are never written
  back; the docs stay the truth.

`isekai onto check` runs the shapes (a Slime has exactly one orc, no two Slimes overlap, a Mind is
worn by someone, a bond points at a creature with a doc). A skill with no rank prefix under
`.opencode/skill(s)` or `.claude/skills` is a shared host tool (`is:shared true`, the law's shared-skills
row), not a creature's mind: `MindWorn` skips it (`is:unless is:shared`). Ranks are data: a rank the
schema does not name becomes a derived class ⊂ `Creature`, bonded to its parent by `above`. `isekai onto project <creature>
--budget N` emits what that creature may see, nearest first, no IRIs.
