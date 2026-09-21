# The agent loop — six beats a Court Body works to

The law's own statement is `isekai.md § The loop` (its text is at the end of this file); this is
the design behind it — where the loop sits in the world, the six beats, the four step classes
and the human gate, recovery, budgets, the plan format, and the protocol form an LLM-driven body
follows by hand. The instrument is `.isekai/tools/loop.js`.

The loop is **both a tool and a protocol**. `loop.js` runs scripted work — a plan of shell
steps — on its own. The same six beats are what a Court Body follows *by hand* when the work is
not scriptable: an investigation, a port, a rewrite. Tool or protocol, the shape is one, so a
dispatcher reads one journal and one wire report whichever form the body took.

## Where it fits

The loop lives at the dispatch seam — the moment a rank hands work to a Court Body and waits for
one wire report back. Three seats, one loop:

- **Rimuru dispatching a Court Body.** This is the primary seat. Rimuru commissions
  (`@ROOT · @SCOPE · @ASK`), the body runs the loop, and what comes back is a wire report.
  `loop.js run <plan> --as <body>` *is* a Court dispatch in scripted form: the journal under
  `.isekai/instruments/loop/` is the Court's transcript that survives its dying context, and
  the report is the only thing that crosses back. Rimuru in the stress zone must dispatch heavy
  work rather than run it inline (§ Instruments) — the loop is what it dispatches to, and the
  loop reads the same instrument, so a body never presses past a budget its dispatcher already
  respects.
- **The Elf routing.** A cross-domain ask becomes a plan: the Elf writes the steps down, each
  one a question (`ask`) with a command and a check, and names the body that will run it. One
  body per run (Law 2: a body stays in its rank); a plan that crosses domains is several runs,
  one per Orc territory, and the Elf is the one who orders them. The plan file is the Elf's
  routing written where a resume can read it.
- **An Orc's gate.** The `verify` beat is the gate with mechanical teeth: *traits hold* and
  *duties done* (gate checks 2 and 3) become a command whose exit code is a reading. *Right
  slime authored* (check 1) is the run's `--as`: a step in a zone runs as the Slime that owns
  it. *Doc truthful* (check 4) is a `verify` on the owning doc. Above the Orc's gate sits the
  human gate: the Orc passes writes inside the world; only Veldora passes anything outward
  (Nature 7) or destructive (Law 6). The loop asks the Orc's question with a command and
  Veldora's question with a prompt, and never confuses the two.

The loop is *not* a Keeper's rhythm (a Keeper persists across tasks; the loop is per task) and
it is *not* Rimuru's judgment (Rimuru's input duty — shaping Veldora's ask before it flows down
— stays human-facing, above any plan).

## The six beats

Every step, in this order; each beat is journaled before the next begins.

| Beat | What happens | Reading, not feeling |
|---|---|---|
| **Perceive** | budgets are read: steps used, minutes elapsed, the context window | `memory.js status` → the same transcript method as `context-check.sh`, 200k budget, 180k stress zone |
| **Recall** | `memory.js recall "<ask>" --as <body> -k N` — the top hits enter the step as paths + section anchors (`$LOOP_RECALL`), never payloads (point, don't carry); then `toolbox.js brief "<ask>" --as <body> --budget N --json` — the level-1 manifest that fits the ask and the tool budget enters as `@T` lines (`$LOOP_TOOLS`): names, paths and both costs, never a body; the picks are journaled the same way (names + costs) on a `toolbox` line | a missing or stale index or registry is a `@?` from the instrument; the loop rebuilds it and asks again — once |
| **Plan** | the step's class is settled (read · write · outward · destructive) and the gate is asked if the class needs it | the classifier's reason and the plan's declaration are both journaled |
| **Act** | the command runs (shell, cwd = the world root or the step's `cwd`) | exit code, signal, duration, output tails |
| **Verify** | the step's `verify` command runs; non-zero means the *act* failed | the retry is journaled with `because: verify` |
| **Record** | `done` / `failed` / `denied` with attempts and duration; on `done`, the step's `remember` note lands in shared memory (memory flows up) | one JSON line per beat, `.isekai/instruments/loop/<run-id>.jsonl` |

**Past a budget the run checkpoints.** A step budget, a wall-clock budget or a context window in
the stress zone stops the run *before* the next step, exit 5, with the resume command in the
report. It does not finish "just this one"; it stops honestly and says where.

## The four classes and the gate

A step's class says how far its effect reaches. It is settled from two sources, and the plan's
word can only tighten:

| Class | Reach | Examples the classifier reads | Gate |
|---|---|---|---|
| `read` | none | `cat`, `ls`, `grep`, `git status`/`log`/`diff`, `memory.js recall`/`status` | never |
| `write` | inside the world — `.isekai/` and the target directory | `>`/`>>`, `tee`, `cp`, `mkdir`, `sed -i`, `git add`/`commit`/`checkout`, `memory.js remember`/`index`, any unknown command or script | only under `--strict` |
| `outward` | beyond the world (Nature 7) | `git push`/`fetch`/`pull`/`clone`, `curl`, `wget`, `ssh`, `scp`, `rsync`, `npx`, `npm install`/`publish`, `pip install`, `docker push`/`pull`, `gh`/`aws`/`kubectl`…, a URL, a path outside the root (`/home/…`, `~`, `../` climbing out; `~/.isekai` and system prefixes are inward) | **always** |
| `destructive` | irreversible (Law 6) | `rm`, `mv`, `shred`, `truncate`, `dd`, `git reset`/`clean`/`rebase`/`branch -D`/`push --force`/`checkout --`/`stash drop`, `find -delete`, and an overwrite of a record — `>` (not `>>`), `sed -i`, `tee`, `cp` onto `isekai.md`, `log.md`, a canon file or `notes.jsonl` | **always** |

The classifier is a heuristic, not a proof: it looks at every pipe/`;`/`&&` segment and at
command words inside `bash -c "…"`, and it errs toward asking — an unknown command is `write`,
never `read`. The plan may declare `class` to tighten (`echo x > f` declared `outward` is
gated). A declaration that would loosen is ignored and reported as `@?` (`git push` declared
`read` is still `outward`). The `verify` command is classified too: a verify that reaches out
gates the step.

**The gate is a real question, never a default yes.** Three ways to answer it:

1. `y/N` on a TTY — the loop prints the step, its class and the reason, and waits.
2. `--approve <id>,<id>` — an explicit pre-approval list, given by the human who read the plan
   (`loop.js explain` shows exactly which steps would ask). An id not in the list is not
   approved.
3. `--dry-run` — nothing runs, nothing is journaled; the report says `would-ask` per gated step
   and `would-run` for the rest.

Without a TTY and without `--approve`, a gated step is **denied**: journaled as such, exit 4,
and the report names the flag that would approve it. A denial stops the run at that step —
the human decides how to proceed, the loop does not skip past it. Nothing auto-approves, and
`resume` never reuses an approval for an act that was cut mid-flight (below).

## Recovery and resume

Every beat is one JSON line, appended (`O_APPEND`, atomic below `PIPE_BUF`) to
`.isekai/instruments/loop/<run-id>.jsonl`. The first line carries the run — body, flags, and
the **whole validated plan** — so a resume depends on nothing but the journal (the plan file may
be gone or edited; an edited plan is a `@?`, and the journaled plan is what resumes). The
journal is instrument state: machine-local, gitignored, overwritten never, deleted freely once
its run has been written up in `log.md`.

- **Retry with bounded backoff.** A failed act, or a failed verify (which *is* a failed act),
  retries up to `retries` times (plan step, else `--retries`, default 2), waiting
  `backoff × 2^(attempt−1)` ms (default 500, cap 10 s). Every attempt is journaled.
- **Escalate one hop, never guess past.** Retries exhausted → the step is recorded `failed`,
  the run ends `ESCALATE`, exit 3, and the wire report carries a `@?` naming the step, the
  attempts, the last exit and stderr tail, the journal, and the resume command. That `@?` is
  the escalation (Absolute Rule III): it goes to whoever dispatched the run — Rimuru, or the
  Orc — and no further. The loop never loops forever and never skips to the next step.
- **Resume from the journal.** `loop.js resume <run-id>` reads the journal back: steps
  recorded `done` are skipped (`done (before)` in the report); a step recorded `failed` or
  `denied` runs again from its recall beat with fresh retries (the human resumed on purpose —
  `--approve` may now be given); a step whose act *started* but never *recorded* was cut
  mid-flight (crash, `/clear`, kill): a `read`/`write` act simply runs again, an `outward`/
  `destructive` act is **re-gated** with a `@?` saying its side effect may already have
  happened — the first run's `--approve` does not carry for it; a fresh `--approve` on the
  resume, or a TTY answer, is required (a journaled approval still carries for a gated step
  the first run never started). Budgets restart with the resuming process. `loop.js status` lists every journal
  with `DONE` / `ESCALATE` / `DENIED` / `CHECKPOINT` / `INTERRUPTED`; `status <run-id>` shows
  the steps.

## Budgets

Read from instruments before every step, never from a feeling:

| Budget | Reading | Default | Flag |
|---|---|---|---|
| steps | steps executed by this process | plan `budget.steps`, else 50 | `--max-steps N` |
| wall clock | minutes since this process began | plan `budget.minutes`, else 30 | `--max-minutes N` |
| context | `memory.js status` → `short.contextWindow` (the session transcript, `context-check.sh`'s method) | 200,000 budget · 180,000 stress | — (the world's one method, two mouths) |
| tool manifest | `toolbox.js brief` → the resident cost of the picks that fit a step's ask | 1,500 tokens | `--tool-budget N` |
| per-step time | a single act or verify | 600 s, then `SIGKILL` and a failed attempt | `--step-timeout-s N` |

An unmeasurable context window (no transcript — a headless run, a throwaway world) is a `@?`
in the report, not a stop: silence is a finding (Nature 9), and the other two budgets still
bind.

## The plan

One JSON file — `{as?, budget?: {steps, minutes}, steps: [...]}` — or a bare array, or JSONL
with one step per line. A step:

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | `[A-Za-z0-9_.-]`, unique in the plan; what `--approve`, the journal and the report name |
| `ask` | yes | the step's question — what recall searches for and what the journal answers |
| `run` | one of run/remember | the shell command (cwd = root, or `cwd`) |
| `verify` | no | a command whose non-zero exit fails the *act*; needs `run` |
| `class` | no | `read` · `write` · `outward` · `destructive` — tightens the classifier, never loosens |
| `retries` | no | integer ≥ 0; default `--retries` (2) |
| `recall` | no | `false` to skip the recall beat, or a question that replaces `ask` for it |
| `toolbox` | no | `false` to skip the toolbox manifest for this step — no picks journaled, `$LOOP_TOOLS` empty |
| `remember` | no | `"note"` or `{text, kind?: law\|colony\|territory, tag?}` — appended to shared memory after the step passes, as the run's body |
| `cwd` | no | working directory, relative to the root; outside the root it is `outward` |

The act sees `LOOP_ROOT`, `LOOP_RUN_ID`, `LOOP_STEP`, `LOOP_ATTEMPT`, `LOOP_AS`, `LOOP_PID`,
`LOOP_JOURNAL`, `LOOP_RECALL` (a JSON array of `path#section` anchors) and `LOOP_TOOLS` (the
step's `@T` lines, one per line — empty when `toolbox: false` or nothing fits the ask and the
budget; level 2 is the act's own decision: `toolbox.js load <name>`). `loop.js explain <plan>`
validates the plan and prints every step with its settled class, whether it would ask, what it
recalls, its toolbox setting and what it remembers — read it before `--approve`.

```json
{ "as": "slime-docs", "budget": { "steps": 10, "minutes": 5 }, "steps": [
  { "id": "survey",  "ask": "which canon files mention the loop", "run": "grep -l 'loop' .isekai/canon/*.md || true" },
  { "id": "index",   "ask": "rebuild the memory index", "run": "node .isekai/tools/memory.js index --json",
    "verify": "test -s .isekai/memory/long/index.json", "recall": false },
  { "id": "note",    "ask": "what this run taught the colony", "run": "true",
    "remember": { "text": "explain renders a plan without running it", "kind": "colony", "tag": "loop" } },
  { "id": "publish", "ask": "push the branch", "run": "git push origin main", "retries": 0 }
] }
```

Exit codes: `0` DONE · `2` FAIL (usage, an invalid plan — nothing ran) · `3` ESCALATE · `4`
DENIED · `5` CHECKPOINT. The report rides the wire — `@S <status> run=<id> …`, one `@F` per step
(status, class, attempts, duration, recall hits, remembered), `@?` per hole, `@E <bytes>` —
and `--json` gives the same as one line.

## The protocol form — an LLM-driven Court Body

When the work is not a list of shell commands, the body *is* the loop. It follows the same six
beats per unit of work, by hand, and its dispatcher can read the same shape back:

1. **Perceive** — check the budget before each unit: `context-check.sh` or `memory.js status`.
   In the stress zone, write what is not yet durable (its owning doc, a shared note), report,
   and stop; do not take one more file.
2. **Recall** — `memory.js recall "<the unit's question>" --as <body>` before reading widely;
   follow the anchors it returns instead of re-reading the world.
3. **Plan** — name the unit's class before acting. `read` and `write` inside the world proceed.
   `outward` and `destructive` are put to the human — and here the gate is the **host's own
   permission prompt** (Claude Code's or OpenCode's). The body never works around it: no
   `--yes` flags, no wrapping a gated command in a script that would not prompt, no
   "the plan said so". A prompt the body cannot answer is escalated, not bypassed.
4. **Act** — do the unit.
5. **Verify** — check it with a reading (a test, a diff, an instrument), not a belief. A failed
   check is the act's failure: fix and retry within reason; past that, escalate one hop with
   `@?`, never guess past.
6. **Record** — what should outlive the body goes to its owning doc or a shared note
   (`memory.js remember --kind …`) *before* the wire report; the report carries `@S`, `@F` per
   unit, `@?` per hole, `@U` for the unsaid, `@E`.

## The law's text

```
## The loop

A Court Body works to one rhythm, six beats per step: perceive → recall → plan → act → verify
→ record — both a tool (`.isekai/tools/loop.js`, for scripted plans) and a protocol (the same
beats, followed by hand when the work is not scriptable); the shape is one.
- **Budgets are readings.** Steps, wall clock and the context window are read from instruments
  before every step (`memory.js status`, the same method as `context-check.sh`). Past any of
  them the run checkpoints and stops honestly; it never presses on.
- **Recall before, remember after.** Each step recalls by its question and carries anchors, not
  payloads; a step that learned something lands a shared note. A stale index is rebuilt, never
  routed around.
- **Four classes, one gate.** `read` · `write` (inside the world) · `outward` (Nature 7) ·
  `destructive` (Law 6). Outward and destructive always pass the human gate — a real answer on
  a TTY, an explicit pre-approval, or a dry run that only says what it would ask. Nothing is
  auto-approved; a declared class only tightens; a denial stops the run there. In the protocol
  form the gate is the host's own permission prompt, never worked around.
- **Failure escalates one hop.** A failed verify is a failed act; retries are bounded; past
  them the run ends with `@?` to its dispatcher (Absolute Rule III), never a guess, never a
  loop forever. Every beat is journaled (`.isekai/instruments/loop/`) and a cut run resumes
  from its journal — an interrupted outward act is gated again, not replayed.
```
