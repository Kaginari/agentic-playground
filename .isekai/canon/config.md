# Config — the switchboard

One engine, two distributions. This file is the configuration spec for both: where the files
live, how they merge, the schema in which **every feature carries an `enabled` switch**, the
defaults, and the rules that keep the switches honest. `harness-parity.md` names the
capabilities; this file is how each one is turned on, off or tuned. The Go package that
implements it is `isekai/config` (parser in `isekai/config/yaml`); its selftest and tests are
the instrument that says whether this file and the code still agree.

## Distributions

The binary knows which distribution it is at build time, or detects it from the world directory
it finds (`.isekai/` or `.agent-zero/`), or from the name it was invoked as. The lexicon
(`agent-zero/lexicon.json`) changes words, file names and the env prefix — never a config key. A
config file written for one distribution is valid for the other, so the schema below is written
once.

| | `isekai` | `agent-zero` | lexicon key |
|---|---|---|---|
| project dir (`<dist-dir>`) | `.isekai/` | `.agent-zero/` | `dir.root` |
| global dir | `~/.config/isekai/` | `~/.config/agent-zero/` | `tool.binary` under `~/.config/` |
| data dir (sessions, undo, tool-out) | `~/.local/share/isekai/` | `~/.local/share/agent-zero/` | `tool.binary` under `~/.local/share/` |
| machine-shared memory | `~/.isekai/` | `~/.agent-zero/` | `dir.machine` |
| env prefix | `ISEKAI_` | `AGENT_ZERO_` | `tool.binary`, upper-cased, `-` → `_` |
| law document | `.isekai/isekai.md` | `.agent-zero/AGENT-ZERO.md` | `file.law` |

`XDG_CONFIG_HOME` / `XDG_DATA_HOME` are honored when set. The global dir is created on first
run; the project dir is never created by the binary — a world is founded by `/isekai`, not by
a config lookup. In config values, `[dist]` and `[dist-dir]` are substituted by the lexicon and
a leading `~/` by the home directory.

Words that differ between the vocabularies are accepted in either spelling on input and folded
to the canonical (isekai) key: office names (`analyst`/`judge`/`drafter` → `great-sage`/
`raphael`/`ciel`), rank names (`coord`/`domain`/`zone`/`service`/`auditor` → `elf`/`orc`/
`slime`/`kijin`/`dark-elf`), and rule scopes (`rank:zone` reads as `rank:slime`).

## Files and precedence

A config file is **YAML or JSON** — `config.yaml`, `config.yml` or `config.json` — with one
schema. Every file layer accepts either format; two files of different formats side by side in
one layer is a load error naming both. JSON tolerates `//` and `/* */` comments and trailing
commas. YAML is a strict subset (below).

Lowest to highest; later layers override earlier ones for scalar keys.

| Layer | Path | Tracked |
|---|---|---|
| 1 · built-in defaults | in the binary (`config.Defaults`) | — |
| 2 · global | `~/.config/<dist>/config.{yaml,yml,json}` | no |
| 3 · project | `<dist-dir>/config.{yaml,yml,json}` | yes |
| 4 · project local | `<dist-dir>/config.local.{yaml,yml,json}` | no — gitignored, machine-only |
| 5 · env, whole file | `<PREFIX>CONFIG=<path>` (a file, format by extension), `<PREFIX>CONFIG_CONTENT=<yaml or json>` | — |
| 6 · env, single keys | `<PREFIX>MODEL`, `<PREFIX>SMALL_MODEL`, `<PREFIX>MODE`, `<PREFIX>LOG_LEVEL`, `<PREFIX>PROVIDER_<NAME>_API_KEY`, `<PREFIX>DISABLE_PROJECT_CONFIG=1` | — |
| 7 · CLI flags | `--model`, `--mode`, `--approve <class>`, `--dry-run`, `--strict`, `--format`, `--max-steps`, `--budget`, `--profile`, `--set key=value`, `--no-<feature>` for any `enabled` key (`--no-hooks`, `--no-mcp`, `--no-compaction`, `--no-law.crest`) | — |

`<PREFIX>PROVIDER_<NAME>_API_KEY` does not carry a key into the config: it sets
`providers.<name>.apiKeyEnv` to its own variable name, so only the key's *location* is
recorded. A name that matches no provider is a `@?`.

**The YAML subset.** Block maps, block lists, one-line flow `{}` / `[]`, plain / `'single'` /
`"double"` scalars, `true`/`false`, ints (`0x`, `0o`, `0b` too), floats, `null`/`~`/empty, `#`
comments, `|` and `>` block scalars with `-`/`+` chomping. A single leading `---` is allowed.
Anything outside the subset — anchors `&`, aliases `*`, tags `!`, directives `%`, a second
document, `...`, complex keys `?`, merge keys, tab indentation, duplicate keys, a nested
mapping on one line (`a: b: c`) — is a load error with `file:line`, never a silent misparse.

**Merge rules.**
- Objects merge deep; scalars last-wins; `enabled` is a scalar; `null` leaves the lower value.
- Lists concatenate in layer order; lists of scalars (discovery paths, instruction files,
  `approve`, `envAllow`) also de-duplicate. `permissions.rules`, `rules` and hook lists are
  ordered by layer, lowest first.
- `providers.<name>`, `mcp.servers.<name>`, `tools.custom.<name>` and `ranks.<name>` merge by
  name; `{ "enabled": false }` alone is a valid entry and switches a provider, server or tool
  off without repeating its definition. A named entry that does not say `enabled` is on.
- `<PREFIX>DISABLE_PROJECT_CONFIG=1` skips layers 3 and 4 and all project-relative discovery,
  and says so as a `@?`.
- **Every value carries its origin** — `file:line`, `env:VAR`, `flag:--x` or `default` — kept
  per key. `status`, `config explain` and every error message name it.

**Validation.** An unknown key is a `@?`, not an error (a config written for a newer binary
still loads). A wrong type is an error and the binary refuses to start. The following are
refused at load, whatever layer they come from, each naming `file:line` and the key:
- `permissions.rules[]` with `action: allow` and a pattern with no literal character (`*`,
  `**`) on `bash`, `git`, `webfetch`, `websearch` or the tool wildcard `*` (a wildcard
  pre-approval is an auto-approve mode);
- `law.humanGate.enabled: false` from any layer but 2, 3 or 4 (a standing pre-approval is the
  human's written word, never an env var or a flag; `--no-human-gate` does not exist and
  `--set law.humanGate.enabled=false` is refused);
- `law.humanGate.approve` from layers 5 or 6 (a file or the `--approve` flag, never env);
- a credential in a file: `providers.<p>.apiKey` / `key` / `token` / `secret`, an
  `Authorization` / `X-Api-Key` header, a header or `env` value shaped like a key
  (`sk-…`, `Bearer …`), or an `apiKeyEnv` / `headersEnv` value that is not an environment
  variable name;
- `compaction.trigger.tokens` ≥ `law.budget.contextTokens` (a drain that never fires is a
  disabled drain — say so with `enabled: false` instead);
- a `rules[].file` that does not exist; a rule with both `text` and `file`, or with none of
  `text`, `file`, `check`; a scope that is not `all`, `rank:<race>` or `creature:<name>`;
- a custom tool whose name collides with a builtin without `override: true`, a `{{param}}`
  not in `params`, a missing `class`, both `run` and `shell`, a `{{…}}` inside `shell`;
- a builtin `class` below its floor; a model that is not `<provider>/<model-id>` or names a
  missing or disabled provider; a triad whose declared tiers break great-sage ≤ raphael ≤ ciel;
- an MCP server with an `oauth` key (later — until then a remote server authenticates by a
  header from env), an MCP tool class below `outward` on a server not marked `inward`;
- a rank hierarchy that is not a tree rooted at Rimuru, a rank with no `reportsTo`, a
  `reportsTo` that names no rank, an authoring rank with no gate-holding rank above it (Law 3),
  an ascended rank dropping a capability its base held, `ranks.<r>.model` disagreeing with
  `models.ranks.<r>`.

## The honesty rule

**A disabled law feature is a finding, not silence** (Nature 9). Every key under `law`,
`memory`, `instruments`, `toolbox`, `ontology`, `compaction`, `hooks`, `permissions`, `mcp` and
`discovery` whose live value is `enabled: false` is:
- printed by `status` as `@? off <key> — <origin>`, one line each, before the instrument board;
- written as the first beat of every loop journal (`.isekai/instruments/loop/<run>.jsonl`), so a
  later reader knows what was switched off when the work was done;
- shown once at REPL start, in human prose.

A feature the binary itself ships off (origin `default`, such as `law.doomLoop`) is not a
finding: the finding is a switch someone threw. `law.humanGate.approve` containing
`destructive` and `instruments.status.showOff: false` are listed the same way. `status` also
lists every **loosening**: each `allow` rule on `bash`, `git`, `webfetch` or `websearch`, each
standing approval, and a gate switched off — one line each with its origin.

A switched-off *tool* (`tools.<name>.enabled: false`, or a tool the profile does not offer) is
not a law finding; it appears in `status` under a plain `tools off:` line with the reason and
origin. A switched-off *import* is listed because a world set up under Claude Code or OpenCode
would silently lose its Minds and Bodies otherwise.

## Schema

Every object that represents a feature has `enabled`. Defaults are given inline; a key omitted
takes its default. Values marked `[dist]` / `[dist-dir]` are substituted by the lexicon. The
document is JSON here; the same keys in YAML are the same config.

```jsonc
{
  "$schema": "https://<dist>/config.json",          // optional; editors only

  // ---------------------------------------------------------------- models
  "models": {
    "default": "anthropic/claude-sonnet-4-5",         // the mount: the human's choice, never routed
    "offices":   {},                                  // great-sage | raphael | ciel  (analyst | judge | drafter)
    "ranks":     {},                                  // elf | orc | slime | kijin | high-elf | high-orc | dark-elf | any configured rank
    "creatures": {},                                  // <name>
    "tasks":     {}                                   // drain | gate | log | bench
  },
  // every entry is "provider/model-id" or { "model", "effort", "fallback" }; `model` at the top
  // level is accepted as an alias of models.default (it is what --model and <PREFIX>MODEL set).
  "smallModel": "",                                   // for titles, unsaid pass; "" = same as models.default
  "mode": "build",                                    // build | plan

  "providers": {
    "anthropic":  { "enabled": true,  "type": "anthropic", "apiKeyEnv": "ANTHROPIC_API_KEY", "baseURL": "https://api.anthropic.com", "maxOutputTokens": 8192, "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "openai":     { "enabled": true,  "type": "openai", "apiKeyEnv": "OPENAI_API_KEY", "baseURL": "https://api.openai.com/v1", "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "openrouter": { "enabled": false, "type": "openai", "apiKeyEnv": "OPENROUTER_API_KEY", "baseURL": "https://openrouter.ai/api/v1" },
    "ollama":     { "enabled": false, "type": "openai", "apiKeyEnv": "", "baseURL": "http://127.0.0.1:11434/v1" },
    "mock":       { "enabled": false, "type": "mock", "script": "" }     // scripted replies; tests and dry runs
  },
  // providers.<name>: { enabled, type: anthropic | openai | mock, baseURL, apiKeyEnv, headers,
  //   models, timeout ("10m" or seconds), tls: { insecureSkipVerify, caFile }, maxOutputTokens,
  //   toolCalls: native | text, guidedDecoding, contextWindow: auto | <n> }.
  // providers.<name>.models.<id>: { id, tier, contextWindow, maxOutput, reasoning,
  //   price: { input, output, cacheRead, cacheWrite } }  — price in USD per 1M tokens; a model
  //   with no price is unpriced: usage shows tokens, a cost is never guessed. `tier` orders the
  //   triad. Keys come from env only; a literal key in a config file is refused at load.

  // ---------------------------------------------------------------- tools
  "tools": {
    "profile":   "max",                               // max | anthropic | openai | minimal
    "read":      { "enabled": true, "maxLines": 2000, "maxBytes": 51200 },
    "ls":        { "enabled": true, "limit": 500 },
    "glob":      { "enabled": true, "limit": 100 },
    "grep":      { "enabled": true, "limit": 100 },
    "write":     { "enabled": true },
    "edit":      { "enabled": true },
    "multiedit": { "enabled": true },
    "patch":     { "enabled": true },
    "bash":      { "enabled": true, "shell": "", "timeout": "2m", "maxTimeout": "10m", "background": true, "jobsDir": "[dist-dir]/tmp/jobs", "sandbox": "bwrap", "envAllow": [] },
    "git":       { "enabled": true, "timeout": "2m" },   // classified per subcommand, like bash
    "webfetch":  { "enabled": true, "maxBytes": 5242880, "timeout": "1m" },
    "websearch": { "enabled": true, "timeout": "1m" },
    "ask":       { "enabled": true },                 // `question` is accepted as an alias
    "dispatch":  { "enabled": true, "maxDepth": 1, "background": false },
    "recall":    { "enabled": true },
    "remember":  { "enabled": true },
    "toolbox":   { "enabled": true },
    "onto":      { "enabled": true },
    "skill":     { "enabled": true },                 // level-2 load of a Mind
    "desk":      { "enabled": true, "limit": 5 },     // working memory; the todo list's seat
    "custom":    {},                                  // tools.custom.<name>, below
    "output":    { "maxLines": 2000, "maxBytes": 51200, "dumpDir": "[dist-dir]/tmp/tool-out", "keepDays": 7 },
    "missing":   { "doomLoopRepeats": 2, "proposeOnSecondNaming": true, "liveReload": true }
  },
  // every builtin takes { enabled, description (override), class (tighten-only), timeout };
  // `timeout` is a duration ("30s", "2m") or a number of seconds; `timeoutMs` (milliseconds)
  // is accepted anywhere as an alias, and setting both in one block is a load error.
  // bash also { sandbox: bwrap | none, envAllow: [...], background }.

  // ---------------------------------------------------------------- the law, as code paths
  "law": {
    "crest":     { "enabled": true },                 // the nine breaths in every system prompt; off = a bare harness
    "humanGate": { "enabled": true, "strict": false,  // strict: writes ask too
                   "approve": [], "dryRun": false },  // standing pre-approvals by class: ["outward"] — logged on every use
    "gate":      { "enabled": true,                   // the Orc's four checks at turn end
                   "rightAuthor": true, "traitsHold": true, "dutiesDone": true, "docTruthful": true },
    "vitality":  { "enabled": true },                 // alias of gate.docTruthful for worlds without orcs
    "territory": { "enabled": true },                 // a Slime's write outside its globs is refused
    "wire":      { "enabled": true, "cap": 2048, "requireUnsaid": true, "raw": false },
    "log":       { "enabled": true, "path": "[dist-dir]/log.md" },   // append-only; off = verdicts are printed, not recorded
    "escalation":{ "enabled": true, "maxRetries": 2 },// a failed verify retries, then @? one hop up
    "budget":    { "contextTokens": 200000, "stressTokens": 180000, "steps": 40, "minutes": 60 },
    "doomLoop":  { "enabled": false, "threshold": 3 } // LATER
  },

  // ---------------------------------------------------------------- memory tiers
  "memory": {
    "short":  { "enabled": true, "cacheDir": "[dist-dir]/memory/short" },
    "long":   { "enabled": true, "index": "[dist-dir]/memory/long/index.json", "rebuildOnStale": true },
    "shared": { "world":   { "enabled": true, "path": "[dist-dir]/memory/shared/notes.jsonl" },
                "machine": { "enabled": true, "path": "~/.[dist]/shared/notes.jsonl" } },
    "recall": { "k": 5, "relationBoost": true }
  },

  // ---------------------------------------------------------------- toolbox and ontology
  "toolbox":  { "enabled": true, "budgetTokens": 1500, "registry": "[dist-dir]/toolbox/registry.json", "extra": "[dist-dir]/toolbox/extra.jsonl" },
  "ontology": { "enabled": true, "schema": "[dist-dir]/ontology/schema.ttl", "graphDir": "[dist-dir]/ontology/graph", "projectBudgetTokens": 800, "validate": true },

  // ---------------------------------------------------------------- instruments
  "instruments": {
    "context": { "enabled": true },                   // occupancy from provider usage
    "loop":    { "enabled": true, "journalDir": "[dist-dir]/instruments/loop" },
    "toolbox": { "enabled": true, "journalDir": "[dist-dir]/instruments/toolbox" },
    "status":  { "showOff": true }                    // the honesty rule; false is itself shown as off
  },

  // ---------------------------------------------------------------- permissions
  "permissions": {
    "enabled": true,                                  // off = only the class floor applies
    "rules": [
      // { "match": "bash:git push*", "action": "allow" }   — pre-approval of one outward pattern
      // { "match": "edit:**/*.lock", "action": "deny" }
      // { "match": "read:**/.env*",  "action": "ask" }
    ],
    "import": {
      "claudeCode": { "enabled": true },              // .claude/settings.json + settings.local.json → rules
      "opencode":   { "enabled": true }               // opencode.json(c) permission block → rules
    }
  },

  // ---------------------------------------------------------------- injected rules
  "rules": [
    // { "id": "no-force", "text": "Never force-push.", "scope": "all" }
    // { "file": "rules/slime.md", "scope": "rank:slime" }
    // { "id": "vet", "check": "go vet ./...", "scope": "all", "timeout": "60s" }
  ],

  // ---------------------------------------------------------------- hooks (shell)
  "hooks": {
    "enabled": true,
    "timeout": "10s",
    "preTool":      [ /* { "match": "bash|write|edit", "command": "…" } */ ],
    "postTool":     [],
    "sessionStart": [],
    "preCompact":   [],
    "stop":         []
  },

  // ---------------------------------------------------------------- MCP
  "mcp": {
    "enabled": true,
    "timeout": "5s",
    "import": { "claudeCode": { "enabled": true, "path": ".mcp.json" }, "opencode": { "enabled": true } },
    "servers": {
      // "<name>": { "enabled": true, "type": "stdio", "command": ["npx", "-y", "@x/server"], "args": [], "env": {}, "envAllow": [], "cwd": "", "network": false, "sandbox": "inherit", "inward": false, "tools": {}, "timeout": "5s" }
      // "<name>": { "enabled": true, "type": "http", "url": "https://…", "headers": {}, "headersEnv": { "Authorization": "MY_TOKEN" }, "inward": false, "tools": {} }
    }
  },
  // mcp.servers.<s>.tools.<t>: { enabled, class, description } — class tightens only, unless the
  // server is inward. `oauth` is reserved and refused as "later".

  // ---------------------------------------------------------------- discovery
  "discovery": {
    "instructions": { "enabled": true,
      "files": ["AGENTS.md", "CLAUDE.md"],
      "global": ["~/.config/[dist]/AGENTS.md", "~/.config/opencode/AGENTS.md", "~/.claude/CLAUDE.md"],
      "walkUp": true },
    "skills":   { "enabled": true,
      "paths": ["[dist-dir]/skills", ".claude/skills", ".opencode/skills", ".opencode/skill", ".agents/skills",
                "~/.config/[dist]/skills", "~/.claude/skills", "~/.config/opencode/skills", "~/.config/opencode/skill"] },
    "commands": { "enabled": true,
      "paths": ["[dist-dir]/commands", ".claude/commands", ".opencode/commands", ".opencode/command",
                "~/.config/[dist]/commands", "~/.claude/commands", "~/.config/opencode/commands", "~/.config/opencode/command"] },
    "agents":   { "enabled": true,
      "paths": ["[dist-dir]/{elf,orc,slime}", ".claude/agents", ".opencode/agents", ".opencode/agent",
                "~/.config/[dist]/agents", "~/.claude/agents", "~/.config/opencode/agents", "~/.config/opencode/agent"] }
  },

  // ---------------------------------------------------------------- sessions
  "sessions": {
    "enabled": true,                                  // off = nothing persisted; --continue is an error
    "dir": "~/.local/share/[dist]/sessions",          // <dir>/<world-hash>/<id>.jsonl
    "keepDays": 30,
    "title": true                                     // one small-model call per session for a title
  },

  // ---------------------------------------------------------------- compaction — the drain
  "compaction": {
    "enabled": true,
    "strategy": "drain",                              // drain | summary
    "trigger": { "tokens": 180000, "fraction": 0.85 },// fires at the lower of the two; never on a provider overflow error
    "keepRecentTurns": 4,
    "passes": {
      "pointerize": { "enabled": true },              // file reads, grep, glob → path:range · digest
      "trimSpent":  { "enabled": true },              // finished tool outputs → their journal line
      "unsaid":     { "enabled": true },              // one model call: @ASK findings +unsaid; @U lines go home
      "desk":       { "enabled": true },              // same call writes the ~5-thought desk
      "episode":    { "enabled": true },              // log.md entry for landed changes, now
      "verify":     { "enabled": true }               // ask verbatim, @? kept, pointers resolve, tokens shrank — else abort
    },
    "journal": true
  },

  // ---------------------------------------------------------------- modes, output, undo, budgets, ui
  "plan":    { "enabled": true, "dir": "[dist-dir]/tmp/plans" },   // --mode plan: every write denied but <dir>/*.md
  "undo":    { "enabled": true, "dir": "~/.local/share/[dist]/undo", "keepDays": 7 },
  "output":  { "format": "text", "stream": true, "thinking": false, "color": "auto" },
  "budgets": { "session": { "tokens": 0, "usd": 0 }, "court": { "tokens": 0, "usd": 0 } },   // 0 = off; shown in status
  "ui":      { "statusLine": true, "announceCourts": true },
  "logLevel": "WARN",

  // ---------------------------------------------------------------- ranks
  "rankSet": "extend",                                // extend | replace
  "ranks": {}                                         // ranks.<name>, below
}
```

### Notes on specific keys

- **`law.humanGate.approve`** is the config form of `--approve <class>`. It is honored from
  layers 2–4 and from the flag, never from env, and each act it silences is logged as
  `gate: pre-approved by config (<origin>)`. `["destructive"]` is accepted but `status` prints it
  in the off-list even though the gate is nominally on — a standing approval of the irreversible
  is a switched-off instrument in everything but name.
- **`law.humanGate.enabled: false`** is honored only from a config file and is always the first
  line of the off-list and of the loosening list. With the gate off, `Decide` answers `allow`
  for everything except a matching `deny` rule.
- **`mcp.servers.<n>.inward`** declares the server never leaves the machine (a local database, a
  filesystem server). Calls to an inward server are class `write` at most and config may lower a
  tool to `read`; every other server's calls are `outward`, and a tool's class may only tighten
  that. The flag is the human's claim and is shown in `status` beside the server.
- **`discovery.agents.paths`** — the native creature dirs are read as bodies from their docs
  (race from the dir, territory and traits from the doc); CC and OC agent files get `mode:
  subagent` unless their frontmatter says otherwise. A name found twice is one body with two
  sources (Nature 2), the native one winning on conflict.
- **`compaction.trigger`** — `tokens` is against the law budget; `fraction` is of the model's
  real window as the provider reports it; the lower wins. `summary` strategy runs one
  model-written summary the way both other harnesses do, keeps `keepRecentTurns`, and skips every
  pass; it is the fallback when a provider cannot answer the wire or when a person asks for it.
- **`--no-<feature>`** on the CLI maps to `<feature>.enabled: false` for one run (kebab segments
  become camelCase: `--no-law.human-gate` would be `law.humanGate` and is refused) and is subject
  to the honesty rule like any layer; `--no-human-gate` does not exist.
- **`budgets`** are honest stop lines: a session or a Court that reaches its token or USD line
  checkpoints, reports and stops, never mid-write. A cost is computed only for priced models.

## Permission rules

A rule is `{ "match": "<tool>:<glob>", "action": "allow" | "ask" | "deny" }`. The tool is a
builtin name, a custom tool, `mcp__<server>__<tool>`, or `*`. The glob is matched against the
act's subject: the command for `bash` and `git`, the world-relative path for `read`, `ls`,
`glob`, `grep`, `write`, `edit`, `multiedit`, `patch`, the URL for `webfetch`, the body name for
`dispatch`, the Mind name for `skill`. `*` matches any run of characters (for path tools it
stops at `/`), `**` matches across `/`, `?` matches one character; a leading `**/` also matches
at the root, so `**/*.lock` matches `go.lock`. The older spelling `{ "tool", "pattern" }` is
folded to `match` on load; Claude Code's `Bash(git push:*)` translates to `bash:git push*`,
OpenCode's `{ "bash": { "git push*": "ask" } }` to one entry per pattern.

**Decision.** `Decide(tool, input, class)` returns the action and the rule that decided it:
1. Rules that match are ranked by **specificity**: a rule naming the tool beats `*`; more
   literal (non-wildcard) characters beat fewer; fewer `*`/`**` runs beat more; fewer `?` beat
   more. So `bash:git push --force*` beats `bash:git push*`, which beats `bash:git *`, which
   beats `bash:*`, and any `bash:…` beats any `*:…`.
2. On equal specificity **deny beats ask beats allow**; on the same action the later rule (the
   higher layer) wins.
3. No match → the class default: `read` and `write` allow (the gate applies `strict` itself
   when no rule decided), `outward` and `destructive` ask.

A rule loosens a single pattern without turning the gate off; every `allow` on `bash`, `git`,
`webfetch`, `websearch` or `*` is a loosening `status` lists. `permissions.enabled: false`
leaves only the class floor. A `deny` never widens; a wildcard `allow` on an outward-capable
tool is refused at load.

## Injected rules

`rules[]` are laws written in config: `{ id?, text | file, scope, check?, timeout? }`.
- `text` (or the contents of `file`) is returned by `PromptRules(rank, creature)` for the system
  prompt, placed after the crest, and by `LawFacts(rank, creature)` as ontology `Law` facts (a
  plain struct — `Fact{ID, Kind: "law", Text, Scope, Origin}` — so the ontology package can
  assert them without config importing it).
- `check` is a shell command `Checks(rank, creature)` hands to the end-of-turn gate; `RunChecks`
  runs them with `sh -c` in the world root under `timeout` (default 60s) and a non-zero exit
  fails the turn.
- `scope` is `all`, `rank:<race>` or `creature:<name>`; an `id` defaults to `rule<n>` by position
  in the merged list and must be unique.
- `file` paths resolve relative to the config file that declared them (the world root for env
  content); a missing file is a load error naming the key.

## Providers and models

`models.default` is the mount — what the session runs on. Every other call is routed by
`ResolveModel(creature, race, office, task)`, most specific first: `models.creatures.<name>` →
`models.offices.<office>` → `ranks.<race>.model` / `models.ranks.<race>` (one slot; two values
that disagree are a load error) → `models.tasks.<task>` → `models.default`. The result carries
the provider entry and the origin of the choice; `status` shows the resolved model for every
office, rank and task with that origin.

A model is `<provider>/<model-id>`, split on the first `/` — model ids may contain `/`
(`vllm/meta-llama/Llama-3.1-70B-Instruct`). The provider must exist under `providers` and be
enabled. `type` is inferred for the well-known names (`anthropic`; `openai`, `openrouter`,
`ollama`, `vllm`, `groq`, `together`, `mistral`, `deepseek`, `sglang`, `lmstudio` → `openai`;
`mock`) and required otherwise. A hosted vLLM, SGLang or Ollama is `type: openai` with its
`baseURL`; `toolCalls: text` asks for calls as tagged text where native calling is broken;
`guidedDecoding` constrains a Court's report to the wire's schema where the server supports it;
`contextWindow: auto` reads the served maximum so budgets scale to a small model.

**The lineage.** `great-sage` → `raphael` → `ciel` is one escalating chain: with `tier`
declared per model under `providers.<p>.models`, a config whose office tiers descend is refused
at load; an office model with no declared tier is a `@?`, never a guess. When all three offices
resolve to one model the chain holds without tiers.

## Ranks

Ranks are data; the law's table (`isekai.md` §The world) is the default: `elf` → rimuru (routes,
the voice; neither authors nor gates), `orc` → elf (holds the gate, speaks sideways orc ⇄ orc),
`slime` → orc (authors), `kijin` → rimuru (keeper; authors and gates its own subsystem),
`high-elf` / `high-orc` ascend from elf / orc and inherit everything, `dark-elf` → rimuru (the
auditor: read-only tools, never authors). `ranks.<name>` is `{ reportsTo, job, authors,
holdsGate, body: court | keeper, office, tools: [globs], model, dir, prefix, ascendsFrom,
sideways }`; with `rankSet: extend` (the default) a field given overrides the built-in field and
a new name adds a rank; with `rankSet: replace` only the ranks listed exist and the built-in
table is not loaded. Rimuru is the root and is never configured. The law's shape is checked at
load (see Validation). `Ranks()` returns the resolved hierarchy with the origin of every field
(`law`, `ascends:<base>` or `file:line`); `Deviations()` lists every field and rank that departs
from the law; `status` prints the rank set mode and the tree.

## Custom tools

`tools.custom.<name>` is `{ description, params, run | shell, class, timeout, cwd, sandbox:
inherit | bwrap | none, enabled, override, profiles }`. `params` is a flat JSON-schema object:
`<name>: { type: string | integer | number | boolean, description, required, default, enum }`.
- `run` is an argv; each `{{param}}` fills exactly one element (or part of one, as in
  `-n={{ns}}`) and is never shell-interpolated — a value of `; rm -rf /` stays one argument.
- `shell` is a template run by the shell with the parameters as environment variables
  `P_<name>`; `{{…}}` inside a shell template is refused, so no value is ever spliced into
  shell text.
- `class` is the declared floor and is required; the classifier on the resolved command may
  only tighten it. `profiles` names the profiles under which the tool is offered (`max` offers
  everything). A custom tool joins the toolset on the same terms as a builtin, permission rules
  and the gate included.

`tools.profile` selects the set and shape: `max` (every tool, native shapes per provider type),
`anthropic` (Claude's own bash and text-editor tools), `openai` (custom-schema equivalents),
`minimal` (bash, read, write, edit, ask, dispatch, recall, remember, skill, desk; flat schemas).
`tools.missing` is the policy when the model calls a tool that is off or absent: the same
missing call `doomLoopRepeats` times stops the turn; the second naming of a need proposes a
config patch; an accepted patch reloads live.

## Patches and reload

Config is the human's written word, so the binary never writes it on its own. A proposed change
is `Patch(file, ops)`: a unified diff of the file as it would be, produced without writing.
YAML files are edited in place — the line of a scalar is replaced, a missing path is inserted
under its deepest existing ancestor, a one-line flow container is re-rendered as a block —
so comments and order survive; a JSON file is re-emitted. The patched file is trial-loaded
through every layer first, so a patch the law refuses is refused before the human sees it.
`ApplyPatch` writes it on the human's yes; `Reload()` loads again with the same options and
returns the new config with every changed key, before and after, and the origin of the new
value — or, when the new files do not load, the old config untouched and the error.

## The API

`config.Load(dist)` (or `LoadWith(Options{Dist, Root, Cwd, Home, Env, Flags, Overlay})`) returns
`*Config`: the typed schema plus `Dist`, `Root`, `Layers`, `Origins` and `Holes`. On it:
`Where(key)` / `Origin(key)`; `Explain()` (every live value with its origin, the rank tree, the
model table, the off-list); `Show(yaml)`; `StatusLines()`, `Off()`, `Loosenings()`,
`ToolsOff()`, `BudgetLines()`; `Decide(tool, input, class)`; `RulesFor`, `PromptRules`,
`LawFacts`, `Checks` and `RunChecks`; `ResolveModel`, `Mount`, `ModelTable`, `PriceFor`;
`Ranks`, `Rank`, `Deviations`, `RankTree`; `ToolEnabled`, `EnabledTools`, `ToolClass`,
`Builtin`, `MCPClass`, and on a `CustomTool` `Argv`, `ShellEnv`, `Schema`; `Patch`,
`ApplyPatch`, `ProposeEnable`, `Reload`. `ParseFlags(args)` reads the config flags out of an
argv. `Selftest()` builds a world in a temp dir with a fake environment and checks all of it;
`CLI(args, stdout, stderr)` is `isekai config show [--yaml] | explain | check | path | patch
[--apply] [file] key=value…` (`check` answers on the wire: `@S PASS` with `@F layer` lines, the
off-list and the loosenings, or `@S FAIL` with the error as `@?`).

## Defaults per distribution

The defaults are one table, not two: every value above is the default for both distributions,
with `[dist]` and `[dist-dir]` substituted. What differs is only what the lexicon names —
paths, the env prefix, and the words `status` and the wire use for ranks, minds, bodies and the
law document. `agent-zero` ships with the same law switches on; a person who wants a bare
harness turns them off and sees the off-list, which is the point.

## Example — a full project file

`.isekai/config.yaml` for a world that mounts Sonnet, routes the court triad by tier, keeps a
hosted vLLM for benchmarks and Ollama for cheap passes, pre-approves `git push` (the same word
this repo's `.claude/settings.local.json` already gives), declares two custom tools, injects
three rules, runs one stdio and one http MCP server, adds a rank, and drains at 150k tokens.
The same document in JSON is `isekai/config/testdata/example.json`; the loader's tests prove the
two are one config.

```yaml
models:
  default: anthropic/claude-sonnet-5            # the mount: the human's choice, never routed
  offices:                                      # the court triad; tiers must not descend
    great-sage: anthropic/claude-haiku-4-5      # agent-zero may write analyst:
    raphael: anthropic/claude-opus-5            # judge:
    ciel: {model: anthropic/claude-fable-5-1, effort: high, fallback: anthropic/claude-opus-5}  # drafter:
  ranks:
    slime: anthropic/claude-sonnet-5
    kijin: anthropic/claude-opus-5
  creatures:
    slime-changelog: anthropic/claude-haiku-4-5  # a genuinely mechanical zone
  tasks:
    log: anthropic/claude-haiku-4-5
    bench: vllm/meta-llama/Llama-3.1-70B-Instruct
smallModel: ollama/qwen2.5:7b

providers:
  anthropic:
    models:                                     # price: USD per 1M tokens; absent = unpriced
      claude-haiku-4-5: {tier: 1, contextWindow: 200000, price: {input: 1, output: 5}}
      claude-sonnet-5: {tier: 2, contextWindow: 200000, price: {input: 2, output: 10}}
      claude-opus-5: {tier: 3, contextWindow: 200000, price: {input: 5, output: 25}}
      claude-fable-5-1: {tier: 3, contextWindow: 200000, price: {input: 10, output: 50}}
  ollama: {enabled: true}
  vllm:                                         # a hosted vLLM is an openai-compatible endpoint
    type: openai
    baseURL: http://gpu-box:8000/v1
    apiKeyEnv: VLLM_API_KEY                     # the key lives in the environment, never here
    headers: {X-Team: platform}
    timeout: 5m
    tls: {insecureSkipVerify: false, caFile: /etc/ssl/internal-ca.pem}
    toolCalls: native                           # needs --enable-auto-tool-choice on the server
    guidedDecoding: true
    contextWindow: auto                         # read from /v1/models max_model_len
    models:
      meta-llama/Llama-3.1-70B-Instruct: {tier: 2}

tools:
  profile: max
  bash: {timeout: 3m, sandbox: bwrap, envAllow: [GOFLAGS, PATH]}
  webfetch: {enabled: false}
  websearch: {class: outward}
  custom:
    lsl:                                        # an `ls` wrapper: argv, one element per placeholder
      description: long listing of one directory
      class: read
      params:
        path: {type: string, description: directory, required: true}
      run: [ls, -la, "{{path}}"]
    kget:                                       # `kubectl get` as a read tool
      description: kubectl get <kind> in a namespace
      class: read
      params:
        kind: {type: string, required: true, enum: [pods, svc, deploy]}
        ns: {type: string, default: default}
      run: [kubectl, get, "{{kind}}", -n, "{{ns}}", -o, wide]
      timeout: 30s
      sandbox: none
      profiles: [anthropic, openai]
  missing: {doomLoopRepeats: 2, proposeOnSecondNaming: true, liveReload: true}

law:
  humanGate: {strict: false, approve: []}
  wire: {cap: 4096}
  budget: {steps: 60}

permissions:
  rules:
    - {match: "bash:git push*", action: allow}          # one outward pattern, pre-approved
    - {match: "bash:git push --force*", action: deny}   # more specific: deny wins
    - {match: "edit:**/*.lock", action: deny}
    - {match: "read:**/.env*", action: ask}

rules:
  - id: no-force
    text: |
      Never force-push a shared branch.
    scope: all
  - file: rules/slime-zone.md                           # relative to this file's directory
    scope: rank:slime
  - id: vet
    check: go vet ./...
    scope: all
    timeout: 60s

hooks:
  postTool:
    - {match: "write|edit", command: "gofmt -l -w \"$FILE\" >/dev/null 2>&1 || true"}

mcp:
  servers:
    files:                                      # a stdio server, sandboxed like bash
      type: stdio
      command: [npx, -y, "@modelcontextprotocol/server-filesystem", "."]
      env: {LOG_LEVEL: warn}
      envAllow: [PATH]
      network: false
      inward: true                              # never leaves the machine: class write at most
      tools:
        read_file: {class: read}
    docs:                                       # a remote streamable-http server
      type: http
      url: https://mcp.example.com/docs
      headersEnv: {Authorization: DOCS_MCP_TOKEN}   # header value from the env var, never inline
      timeout: 20s
      tools:
        search: {description: search the docs}

budgets:
  session: {tokens: 2000000, usd: 20}           # honest stop lines; 0 = off
  court: {tokens: 400000, usd: 4}
ui: {statusLine: true, announceCourts: true}

ranks:
  orc: {model: anthropic/claude-opus-5}           # same slot as models.ranks.orc
  scribe:                                         # a new rank, added to the law's table
    reportsTo: orc                                # an authoring rank needs a gate above it
    job: keeps the world's changelog
    authors: true
    body: court
    office: ciel
    tools: [read, write, edit, git]

compaction:
  trigger: {tokens: 150000, fraction: 0.8}
  keepRecentTurns: 6

sessions: {keepDays: 90}
output: {format: text, thinking: false}
```

With this file, `isekai status` opens with no off-list (nothing under the honesty rule is
disabled), one loosening line (`allow bash:git push* — .isekai/config.yaml:<line>`), a
`tools off: webfetch — disabled by tools.webfetch.enabled in .isekai/config.yaml:<line>` line,
the rank tree with `scribe` and two deviations, then the instrument board. Add
`ontology: {enabled: false}` and the first line of `status` becomes
`@? off ontology.enabled — .isekai/config.yaml:<line>`.

## Hook contract

A hook is a shell command. It receives one JSON object on stdin — `{ "event", "session", "tool",
"input", "class", "paths", "output"? }` — and the same fields as `HOOK_*` env vars (`FILE` is the
first path). Exit 0 continues; exit 2 blocks the act and stderr is the reason the model sees; any
other exit is logged and ignored. A `preTool` hook cannot approve: it can only block or pass —
the gate is still asked afterwards. Stdout is read as the wire if it starts with `@S`; `@F` and
`@?` lines are folded into the loop journal, nothing else reaches the prompt.

## Session file

One JSONL file per session, one event per line: `{ "t", "kind": "user|assistant|tool|gate|hook|
compaction|verdict", … }`. Usage is recorded on every assistant line so the context instrument can
be rebuilt from the file alone. `session list` reads the directory; `run --continue` reads the
newest file for this world; `run --session <id>` reads one. A session is never rewritten:
compaction appends a `compaction` event that points at the rebuilt context, and the drained lines
stay in the file.
