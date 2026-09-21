# Memory tiers — three memories for every agent and sub-agent

The law's own statement is `isekai.md § Memory tiers`; this file is the design behind it — the
boundary table, the three kinds of knowledge, the ranking, the instrument, and the path to a
database tier when a world outgrows plain files. Nature 5 (Memory) is the biology: a small fast buffer, consolidated
during rest into durable schemas, then cleared.

## The three memories, per creature

Every rank has the same three. What differs is *which files* answer for them.

| Memory | Sub-kind | Where it lives | Who writes it | Lifetime | Git |
|---|---|---|---|---|---|
| **Short** | context window | the running session's transcript | the host | the session | no (host's) |
| | working memory | `## Thoughts` desk on the worn Mind (~5) | the creature | until distilled | yes (inside the doc) |
| | semantic cache | `.isekai/memory/short/<creature>.jsonl` | `memory.js recall` | until `forget --short` | **no** — local, disposable |
| **Long** | episodic | `.isekai/log.md` (one memory per dated entry) | whoever lands a change | forever, append-only | yes |
| | procedural | Minds `SKILL.md`, commands, tools | creatures, `/don`, `/mint` | until Vitality retires it | yes |
| | semantic | `isekai.md`, creature docs, canon, README | creatures, Veldora | until Vitality retires it | yes |
| | *(the index over all three)* | `.isekai/memory/long/index.json` | `memory.js index` | rebuilt at will | **no** — derived |
| **Shared** | world-shared | `.isekai/memory/shared/notes.jsonl` | any body, `memory.js remember` | forever, append-only | yes — travels |
| | machine-shared | `~/.isekai/shared/notes.jsonl` | any body, `remember --machine` | forever, append-only | outside any repo |

Rank makes no difference to the shape, only to the *content*: a Slime's semantic memory is its
zone's ground truth; an Orc's is its gate verdicts and traits; the Elf's is the cross-domain map;
Rimuru's is the law itself. A Court Body has the same three but its short memory is the whole
of it — context dies with the task — which is why the law says: write what should outlive you
to shared or long memory *before* the wire report.

## The unsaid — three kinds of knowledge on the tier table

**The unsaid is your real knowledge** (`isekai.md § The unsaid`). The tiers say where memory
lives; the three kinds say what a head holds that the tiers do not yet. Each kind has a home, a
surfacer and a moment:

| Kind | Definition | Analogy | Home (tier · files) | Who surfaces it | When |
|---|---|---|---|---|---|
| **law** (institutional knowledge) | the rules, definitions and decisions the isekai runs on | how data is modelled | long · semantic — `isekai.md`, canon, creature docs (traits, territory, verdicts) | anyone who catches the world running on a rule no doc states; Rimuru escalates a law gap to Veldora (Law 6) | before a gate verdict; at a distill wave (a thought becoming a rule) |
| **colony** (tribal knowledge) | what the colony knows but rarely writes down anywhere | how queries are executed | the unwritten — desks (short · working memory), `memory/shared/notes.jsonl` (shared), and what a Court Body's context carries and loses | the Court Body itself (`@U colony …` in its wire report), the Orc that asks `+unsaid`, the worn Mind's desk | before a Court Body's context dies; before a distill wave |
| **territory** (domain context) | what the numbers and entities actually mean in your territory | metadata | long · semantic — the Slime's own doc, the zone's ground truth | the Slime that owns the zone; the Orc at the gate (check 4, doc truthful) | before a gate verdict; on any change to the zone |

The colony kind is the one the principle is really about: law has a file, territory has a doc,
but colony knowledge has only heads and notes — it is where the world's real knowledge leaks. So
the instrument makes the kinds first-class where it is cheap: `remember --kind law|colony|territory`
stamps a shared note, `recall --kind <kind>` filters shared notes by it, and `status` counts shared
notes per kind. A note without a kind is lawful — the field is a pointer, not a gate.

## Files are the truth; everything else is derived

The hybrid rule — files first, a database only when needed:

1. **Only in files** — creature docs and their desks, canon, `SKILL.md`, `isekai.md`, `log.md`,
   shared notes. Agents keep reading and writing these directly; sessions reconnect through
   `CLAUDE.md → isekai.md → log.md`; the wire points at them by path.
2. **Only in the index / database** — section vectors, term statistics, the relation graph,
   a distilled-thought archive if a world wants one. Rebuildable from (1) at any time.
3. **Mirrored** — nothing, by design. A fresh clone with no index works, degraded but lawful:
   `grep`, paths, the wire all answer; `memory.js recall` answers `@? no index` until
   `memory.js index` has run. The one future exception is a *pending-landing ledger* (below).
4. **When the database tier is reached for** — an instrument reading, never a feeling:
   - concurrency seen (`git worktree list` > 1, or a non-blocking lock on `log.md` fails);
   - a question the file tools cannot answer (nearest neighbours, "which mind serves this orc");
   - a desk over its ~5 limit (tempest and `memory.js status` both measure it);
   - the index's recorded `HEAD` ≠ `git rev-parse HEAD` → rebuild before trusting.

## Recall: meaning first, relation second

`memory.js recall "<question>" --as <creature>` ranks long + shared memories:

- **Meaning** — a local, model-free scorer: BM25 over a real token vocabulary (no hashing,
  no collisions), normalised to 0..1 as the share of the question's ideal match so a short
  section and a long one compete fairly. No model to install, nothing leaves the world
  (Nature 7). It is deliberately the *smallest* thing that ranks by meaning; it is not the
  last word (see below). A memory with zero similarity is never surfaced by relation alone.
- **Relation** — the same typed bonds the colony diagram draws, applied to memory, read off
  the creature docs' own declarations: `Reports to:` gives the parent (slime⇒orc
  truth-current, orc⇒elf verdict-current) and, reversed, the children; `Territory:` and
  back-ticked paths give the zone; a Mind named in the doc is worn (body⇌mind anima-thread).
  A memory that names the asker, its parent or child, a worn mind, or a path in its zone is
  pulled closer by a small fixed boost. Similarity still decides; relation breaks ties and
  nudges — the weights are small on purpose. Creature ids are `<race>-<dir>`, the shape
  `/isekai` and `/genesis` birth.
- **Recency** — a light prior on dated memories only (episodic, shared): +0.05 today, half at
  30 days, ~0 past a season. Undated memories (law, minds, canon) get none.
- **Short first** — the asker's semantic cache is checked before any search; a near-identical
  question (query-vector cosine ≥ 0.92) is a `@S HIT` and costs nothing. A cache entry is keyed
  to the index build, the shared files and the filters, so a rebuilt index, a new note or a
  different `-k` is never answered from cache. Hits are counted so `status` can report whether
  the cache earns its keep.

Output rides the wire: `@S HIT|MISS`, one `@F path#section — score — kind — title` per result,
`@?` for every hole (no index, stale index), `@E bytes`. `--json` for machines.

## Growing the engine without moving the boundary

When a world outgrows BM25 over plain tokens (thousands of sections, several writers at once, a need for
true semantic neighbours), the tier grows an engine; the boundary above does not move.

- **Engine step 1 — real embeddings, still inside the world.** A local model (Ollama +
  `nomic-embed-text`, ~274 MB) replaces the BM25 term match as the vector; one consented pull, then
  offline forever. A hosted embedding API would ship every doc out on every run and is not
  recommended under Nature 7.
- **Engine step 2 — SQLite via `node:sqlite`** (Node ≥ 22.5; this machine has 22.23.2 under
  nvm). WAL, real multi-statement transactions, one file at `.isekai/memory/long/mind.db`,
  gitignored like the index it replaces. Vectors as BLOBs with brute-force cosine (a world of
  ~1k sections × 384 dims ranks in milliseconds); a `bond` table + recursive CTE for relation
  ranking; `sqlite-vec` only if scale ever demands it. The tooling law bends exactly one notch —
  "Node ≥ 22" — which `tempest.js`'s header already asks for.
- **Transactional landing (needs beyond memory).** With the DB present, the gate can land a
  multi-file change + verdict + log append as one unit: a Court Body works in a `git worktree`;
  a pre-commit hook rejects a territory change without its doc (Vitality enforced
  mechanically); a `land.js` writes verdict + log text to the DB as `pending`, then `flock`
  appends to `log.md` and `git merge --ff-only`, then marks `landed`. A crash between the two
  leaves a `pending` row — an instrument reading, reconciled on the next run. Honest caveat:
  git and SQLite are two systems, so this is two-phase, not one commit. `.isekai/log.md
  merge=union` in `.gitattributes` lets two machines both append without conflict.
- **Engines considered and set aside:**
  - *Oracle DBFS* — a FUSE-mounted DBFS is **not transactional** for a POSIX writer: Oracle's
    own docs describe NFS-style close-to-open consistency; each flush is its own commit, so
    writing `code.js` then `doc.md` through the mount is two commits, exactly like ext4.
    Multi-file atomicity exists only through the PL/SQL `DBMS_DBFS_*` API in one session — at
    which point it is Oracle-as-database, not DBFS-as-filesystem. `dbfs_client` is not in
    Instant Client; Oracle Free caps at 2 cores / 2 GB / 12 GB and the image is ~10 GB.
    What Oracle *does* offer — AI Vector Search + property graphs in one transaction domain,
    free of licence in Free — is real, and is the path if Oracle is wanted for its own sake.
  - *PostgreSQL + pgvector (+ Apache AGE)* — the strongest engine, but a daemon and an npm
    client for more writers than this world will have.
  - *btrfs / ZFS snapshots, FUSE overlays* — root is ext4; whole-tree atomicity, no
    concurrency semantics, no search. Rejected.
  - *Git alone* — a commit already *is* an atomic multi-file landing and is the cross-machine
    text channel the log asked for twice; it has no vector search, so it is the durable layer
    under every option, not the whole answer.

## The instrument

`node .isekai/tools/memory.js [root] <command>` — stdlib Node, Node 16-compatible, one file.

| Command | Reads / writes | Answer |
|---|---|---|
| `status [--as X] [--json]` | all three tiers | context tokens + zone, desks + stress, cache size/hits, index counts + staleness, shared counts (total and per unsaid kind) |
| `index` | files → `memory/long/index.json` | `@S INDEXED n` |
| `recall "<q>" [--as X] [--tier long\|shared\|all] [--kind episodic\|procedural\|semantic\|law\|colony\|territory] [-k N] [--no-cache]` | index + shared notes; cache | wire: `@S HIT\|MISS` · `@F …` · `@?` · `@E` — the three unsaid kinds filter shared notes only |
| `remember "<note>" [--as X] [--machine] [--tag t] [--kind law\|colony\|territory]` | appends one JSON line | `@S REMEMBERED world\|machine → path` |
| `forget --short [--as X]` | deletes the asker's cache | `@S FORGOT n` — only short memory is ever forgotten; long and shared are records (Law 4) |
| `selftest` | a throwaway world under `.isekai/tmp/memory-selftest/` (Law 5), cleaned up after | `@S PASS n checks` on Node 16 and 22 |

`status` is a Perception instrument: a tier that cannot answer (no transcript, no index, HEAD
moved or a source file changed since the index was built, a note too long to append
atomically) is reported as `@?` and left visible — never routed around. Concurrency: the index
is written atomically (temp + rename); notes and cache lines are single `O_APPEND` writes,
atomic below `PIPE_BUF` (4096 bytes) — several Court Bodies and Rimuru may write at once.
