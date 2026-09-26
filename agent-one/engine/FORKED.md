# Forked

This engine was forked from `isekai/` at commit `5913977` of the convention repository (the
`isekai/` tree is identical through `5000f74`), module `github.com/Kaginari/agentic-playground/isekai`
→ `github.com/Kaginari/agent-one`, and renamed into the agent-one vocabulary from
`agent-one/lexicon.json` down to the Go identifiers; from here the two evolve independently.
Rename table (old → new, applied to identifiers, packages, file names, strings, config keys, JSON
keys, ontology terms, fixtures and assets): isekai → agent-one · rimuru/throne → orchestrator ·
veldora → operator · elf/orc/slime/kijin → coord/domain/zone/service (prose: coordinator, domain
owner, zone worker, service owner) · high-elf/high-orc → principal-coordinator/principal-domain-owner
· dark-elf → auditor · great-sage/raphael/ciel → analyst/judge/drafter · office → role
(`models.offices` → `models.roles`) · law → policy (`law.*` → `policy.*`; the `law` tool → `policy`)
· crest → principles · nature → principle · world → workspace (package `world` → `workspace`) ·
creature → member · race → rank · mind → skill · body → agent · court → subagent, court body →
ephemeral subagent, keeper → persistent (rank agent modes `court|keeper` → `ephemeral|persistent`) ·
colony → team · territory → ownership (`Territory:` doc key → `Owns:`) · trait → invariant · desk →
working notes (`## Thoughts` → `## Working notes`, `instruments/desk` → `instruments/working-notes`,
`@DESK` → `@NOTES`) · thought → note · canon → design · tempest → board · genesis/birth/born →
provisioning/provisioned · ascend → promote · mint/don/wear → register/load/hold · distill →
consolidate · reincarnate → onboard · the nine principle names (Vitality → Docs-as-code, …) ·
unsaid kinds `law|colony|territory` → `policy|team|domain` on the wire and on disk · lane `verdict`
→ `review` · ontology prefix `is:` → `ao:` with classes Member, Coordinator, DomainOwner,
ZoneWorker, ServiceOwner, Skill, Policy, Team, Domain, shapes ZoneTruth, ZoneOwnership, SkillHeld
and properties `holds`/`heldBy` (formerly `wears`/`wornBy`) · the orchestrator's short-memory file
`memory/short/orchestrator.jsonl`. Dropped: `cmd/isekai`, the second law and the lexicon JSON with
its distribution switch (`config.Detect`, `world.Lexicons`, the alias tables and `Dist.Words`;
`workspace.Default()` is now the one place for user-facing words), and the memory/toolbox
JS-interop tests, which compared the Go port against the JS tools of the other convention, which
this engine does not ship.
