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
worn by someone, a bond points at a creature with a doc). `isekai onto project <creature>
--budget N` emits what that creature may see, nearest first, no IRIs.
