// tempest — the world's instrument panel (ISEKAI nature law 8 — Instruments;
// tooling law: stdlib node only, no build chain, the shape travels with the world).
// Re-harvests per request. Many worlds run at once: each board answers at
//   http://localhost:<port>/<world-name>/
// the world names itself in .isekai/name (jura-style names lawful), the port is
// derived from the world's root so two boards never collide by default.
// Lifecycle law: the board SLEEPS WHEN THE WORLD RESTS — any request renews the
// lease (--ensure pulses it), after --ttl minutes of silence it dies; --immortal
// opts out (a human's long watch); --stop posts it to sleep now.
// 🎉 holidays also DISPATCHES sequential creature-hat relief runs (opencode run)
// — the tool plans and launches, hat-sessions write the minds; the tool never does.
// Usage: node tempest.js [worldRoot] [--port N] [--ttl minutes] [--immortal] [--json] [--ensure] [--stop]
//
// Prerequisites: plain Node for everything except reading OpenCode's own usage —
// that one path (harvestOpencodeGlobal, the global dashboard's OpenCode card) needs
// `node:sqlite`, a Node 22.5+ builtin. On an older system Node (this machine shipped
// 16.20.2, confirmed 2026-09-20) that card still renders — dbExists still gets
// checked with plain fs — but `available` comes back false with an honest `error`
// instead of a silent zero (see the comment at harvestOpencodeGlobal's definition).
// Fix: install Node 22.5+ (e.g. `nvm install 22`) and run tempest.js with that Node;
// everything else about tempest.js keeps working unmodified either way.
//
// Ported from photographs of the source (2026-09-20) — see .isekai/canon/README.md
// for how much of the original was actually visible. Two deliberate departures from
// what's shown, both kept and flagged inline where they occur:
//   1. Relief dispatch tries `opencode run` first, falls back to `claude -p` — the
//      source only ever named opencode, which would strand Claude-only machines.
//   2. The HTTP route table (GET /<name>/, POST /holidays, POST /party, GET /relief,
//      GET /pulse, POST /shutdown) was never photographed — everything past the
//      start of the --ensure probe logic is this session's own completion, built to
//      satisfy exactly what the (fully visible) client-side script calls.
// Everything else below — harvest(), render(), the relief queue/harm-fence, the
// SVG neural-layer graph, the SQLite session-store reads — is a faithful port.
//
// 2026-09-21 — PAYLOAD B of the bench-forge handoff, RE-IMPLEMENTED here (not a byte copy)
// from the handoff changelog + photographs IMG_2650–2699 of the sender's finished file, read
// against isekai.md §Minds & Bodies + Nature 5. Same-lineage lowercase-law world, so the file
// is taken and adapted, not cloned (changelog 9: propagation ≠ cloning). What changed:
//   1. Separation law — one uniform 7-lane grid, every mind lane LEFT of the rank it serves:
//      zone minds · slime · verdict minds · orc · global minds · elf · ascended (a real lane,
//      not an under-chart shelf); ROW 2 is SHARED SKILLS, full width — opencode/claude
//      commands + app skills nobody wears, named plainly, never an overload of world law.
//   2. Creature split — every race-prefixed creature renders twice: BODY in its rank lane
//      (portrait, halo, wide tint) and its worn MIND (`<name>@hat`, dashed ring, slim tint)
//      in the service lane its race picks, joined by a wear-edge; hats are first-class on
//      the graph, in zdata, in the viewer.
//   3. Adaptation loop — the dated desk (## Thoughts) is drawn on the HAT, not the head:
//      hat stress = desk/limit (an instrument reading); the body keeps kb/diet/crosslinks/
//      genesis and adapts in the same change. Viewer names the escalation rule.
//   4. Bonds typed & colored — slime⇒orc TRUTH-CURRENT (cyan, solid), orc⇒elf
//      VERDICT-CURRENT (gold, solid), body⇌mind ANIMA-THREAD (dashed, lane-tinted). Shape law
//      in both views: solid = race bond, dashed = mind bond. Adjacency (relOf/relCls) travels
//      server-side with the page — the viewer never scrapes the DOM for relations.
//   5. Minds wear brains — BRAIN_D glyph tinted per lane; the court triad GREAT-SAGE reads →
//      RAPHAEL verdicts → CIEL drafts, each mount resolved from minted bodies
//      (.claude/agents, .opencode/agents), '—' when nothing is minted (Nature 9: honest).
//   6. Docs ride the wire, lazy by anchor — GET /<world>/doc?name= answers @S:MAP (section
//      index, measured bytes); &sec=N answers @S:SEC (one section). Pipe-raw encoding was
//      considered and rejected under the anti-wire clause (tokens, not eyes): @-keyed JSON.
//   7. Instrument — net view default (vertical rows, bézier synapses), lanes view a toggle
//      away; per-layer backgrounds; one process / :7799 / /<world>/ / --ensure heartbeat /
//      .isekai/name were already here — verified, not duplicated.
//   8. Names migrate by succession: no rename wave — old ids (`n-`, `e-` classes, zdata
//      keys) keep working; new kinds get suffixes (`@hat`, `@cmd`) beside them (Law 4).
//   9. Provenance is this header; the log entry is Rimuru's, not this file's (scope).
// Unphotographed (rebuilt from the client contract, flagged @? in the wire report): the server
// doc route's MAP/SEC body; the shared-row command harvest (.claude/commands, .opencode/commands)
// and the .claude/skills merge, which the changelog names but the photos never show.

const fs = require('fs');
const path = require('path');
const http = require('http');
const os = require('os');
const { execSync, spawn, spawnSync } = require('child_process');

const args = process.argv.slice(2);
const JSON_MODE = args.includes('--json');
const JSON_GLOBAL_MODE = args.includes('--json-global');
const COLONY = path.resolve(args.find(a => !a.startsWith('--') && isNaN(+a)) || '.');
// One app for the whole machine (human order 2026-09-20: "should be 1 app in whole
// machine not multiple"), not one process + one hash-derived port per world. A fixed
// default port; --port still overrides for the rare clash.
const portIdx = args.indexOf('--port');
const PORT = portIdx > -1 ? parseInt(args[portIdx + 1], 10) : 7799;

// World home + canon resolution (the reincarnation law): the world's living
// memory rides .isekai/; an elder world's shelf may still be .convention-zero/.
// The canon doc is ISEKAI.md, else the elder CONVENTION-ZERO.md / SLIME.md.
// Computed per-root, not module-level — one process now serves every registered
// world, never just the COLONY it happened to be launched with.
const homeOf = root => fs.existsSync(path.join(root, '.isekai')) ? '.isekai' : '.convention-zero';
// Canon doc location, root-relative. The newer /isekai shape keeps it *inside* home
// (<home>/isekai.md, lowercase) — checked first; an elder world's canon doc sits at the repo
// root instead (ISEKAI.md/CONVENTION-ZERO.md/SLIME.md, the shape this function originally
// assumed exclusively, before a "canon v?" chip on a genuinely reincarnated world revealed the
// gap — human 2026-09-21: "the canon is v9"). Falls back to <home>/isekai.md as the default
// path for a world that has neither yet.
const canonOf = (root, home) => {
  const inHome = ['isekai.md', 'ISEKAI.md', 'CONVENTION-ZERO.md', 'SLIME.md'].find(f => fs.existsSync(path.join(root, home, f)));
  if (inHome) return path.join(home, inHome);
  const atRoot = ['ISEKAI.md', 'CONVENTION-ZERO.md', 'SLIME.md'].find(f => fs.existsSync(path.join(root, f)));
  return atRoot || path.join(home, 'isekai.md');
};
const journalPath = (root, home) =>
  ['log.md', 'log-git.md'].map(f => path.join(root, home, f)).find(fs.existsSync)
  || path.join(root, home, 'log.md');

const RACES = ['slime', 'orc', 'elf', 'highorc', 'darkelf', 'kijin'];
const DIET_KB = 6;                    // rule 14 breath law
const DESK_LIMIT = n => n.startsWith('darkelf') ? 10 : 5; // archive law vs ground races
const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const rd = f => { try { return fs.readFileSync(f, 'utf8'); } catch { return ''; } };
// A YAML frontmatter `description: "..."` captures its quotes literally with a bare .+
// regex — strip one matching pair so the extracted text reads the same whether the source
// quoted it or not.
const unquote = s => (s || '').replace(/^"(.*)"$/, '$1').replace(/^'(.*)'$/, '$1');
// Section map of a markdown doc (the doc route's @S:MAP answer, 2026-09-21): every ATX
// heading outside a fenced code block opens a section; `preamble` is whatever precedes the
// first heading (frontmatter included). Bytes are measured, never estimated (Nature 9).
function sectionMap(text) {
  const lines = text.split('\n');
  const sections = [];
  let pre = [], cur = null, fence = false;
  for (const l of lines) {
    if (/^\s*(```|~~~)/.test(l)) fence = !fence;
    const h = !fence && l.match(/^(#{1,6})\s+(.+?)\s*#*\s*$/);
    if (h) { cur = { n: sections.length + 1, t: h[2].trim(), depth: h[1].length, lines: [l] }; sections.push(cur); continue; }
    if (cur) cur.lines.push(l); else pre.push(l);
  }
  for (const s of sections) { s.text = s.lines.join('\n'); s.b = Buffer.byteLength(s.text); delete s.lines; }
  return { preamble: pre.join('\n'), sections };
}

// The world name (nature law 8 naming): the home's `name` file holds one line —
// jura-style names lawful, chosen at birth/populate; it travels (rule 12 re-include).
// Also per-root now, for the same reason home/canon are.
const nameOf = (root, home) => {
  const raw = rd(path.join(root, home, 'name')) || path.basename(root);
  return String(raw).trim().toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+|-+$/g, '') || 'world';
};

// ------------ the registry: which worlds this one app knows about ------------
// Machine-global, like opencode's own session store — not any one world's .isekai/,
// since the whole point is one app spanning every world. { path, name, lastSeen }[].
const REGISTRY_DIR = path.join(os.homedir(), '.local', 'share', 'tempest');
const REGISTRY_FILE = path.join(REGISTRY_DIR, 'registry.json');
function readRegistry() {
  try { return JSON.parse(rd(REGISTRY_FILE) || '[]'); } catch { return []; }
}
function writeRegistry(list) {
  fs.mkdirSync(REGISTRY_DIR, { recursive: true });
  fs.writeFileSync(REGISTRY_FILE, JSON.stringify(list, null, 2));
}
// Upsert root into the registry (by path) and prune any entry whose .isekai/ (or
// .convention-zero/) is gone — self-healing when a world is moved or deleted.
function registerWorld(root) {
  const home = homeOf(root);
  const name = nameOf(root, home);
  const list = readRegistry().filter(w => fs.existsSync(path.join(w.path, w.home || '.isekai')) || fs.existsSync(path.join(w.path, '.convention-zero')));
  const i = list.findIndex(w => w.path === root);
  const entry = { path: root, name, home, lastSeen: new Date().toISOString() };
  if (i > -1) list[i] = entry; else list.push(entry);
  writeRegistry(list);
  return entry;
}
function prunedRegistry() {
  const list = readRegistry().filter(w => fs.existsSync(path.join(w.path, w.home || homeOf(w.path))));
  writeRegistry(list);
  return list;
}
function worldRootForName(name) {
  const w = prunedRegistry().find(w => w.name === name);
  return w ? w.path : null;
}

// Lifecycle law: the board sleeps when the world rests. Any request (for any
// registered world) renews the one shared lease; --ttl minutes of silence → sleep.
// --immortal opts out.
const ttlIdx = args.indexOf('--ttl');
const TTL_MIN = ttlIdx > -1 ? parseFloat(args[ttlIdx + 1]) : 30;
const TTL_MS = TTL_MIN * 60000;
const IMMORTAL = args.includes('--immortal');
let lastTouch = Date.now();

// ------------ Claude Code usage (no opencode.db — read its JSONL transcripts instead) ------------
// Claude Code has no SQLite session store; each session is one JSONL transcript at
// ~/.claude/projects/<slugified-cwd>/<sessionId>.jsonl. Assistant turns carry
// message.model, message.usage.{input,output}_tokens and a timestamp; d.cwd is matched
// against root with the same LIKE-prefix semantics as the opencode query below.
// Best-effort only: unlike opencode's `agent` column (a precise mounted-body name),
// Claude's transcripts don't cleanly name which minted sub-agent handled a turn, so agent
// identity here is coarse — d.isSidechain: 'subagent' vs 'main session' — not per-body.
// Read beats inferring (see /genesis): this is honestly labeled as coarser, not silently
// passed off as equally precise.
function harvestClaudeUsage(root) {
  const out = { live: { rows: 0, totalIn: 0, totalOut: 0, perDay: {}, perModel: {}, series: {} },
    agentUse: {} };
  const projectsDir = path.join(os.homedir(), '.claude', 'projects');
  if (!fs.existsSync(projectsDir)) return out;
  const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;
  let slugDirs;
  try { slugDirs = fs.readdirSync(projectsDir, { withFileTypes: true }).filter(e => e.isDirectory()).map(e => e.name); }
  catch { return out; }
  for (const slug of slugDirs) {
    let files;
    try { files = fs.readdirSync(path.join(projectsDir, slug)).filter(f => f.endsWith('.jsonl')); }
    catch { continue; }
    for (const f of files) {
      let text;
      try { text = fs.readFileSync(path.join(projectsDir, slug, f), 'utf8'); }
      catch { continue; }
      for (const line of text.split('\n')) {
        if (!line) continue;
        let d;
        try { d = JSON.parse(line); } catch { continue; }
        if (d.type !== 'assistant') continue;
        const cwd = d.cwd || '';
        if (cwd !== root && !cwd.startsWith(rootPrefix)) continue;
        const msg = d.message || {};
        const usage = msg.usage || {};
        const mid = msg.model || 'unknown';
        // usage.input_tokens alone is only the *uncached* sliver of a turn's real input —
        // found 2026-09-20 after a human asked why "in" showed 0.0M next to a 1.2M "out":
        // with prompt caching (the normal case for any multi-turn Claude Code session),
        // almost all real input is cache_read_input_tokens (context reused from earlier
        // turns) or cache_creation_input_tokens (context newly cached this turn) — both
        // silently excluded before. On this world alone that was a ~177M-token undercount,
        // not a rounding error.
        const tin = (usage.input_tokens || 0) + (usage.cache_read_input_tokens || 0) + (usage.cache_creation_input_tokens || 0);
        const tout = usage.output_tokens || 0;
        const day = String(d.timestamp || '').slice(0, 10) || 'undated';
        const agentKey = d.isSidechain ? 'subagent' : 'main session';

        out.live.rows++; out.live.totalIn += tin; out.live.totalOut += tout;
        const pd = out.live.perDay[day] ||= { in: 0, out: 0 };
        pd.in += tin; pd.out += tout;
        const pm = out.live.perModel[mid] ||= { in: 0, out: 0, rows: 0, agents: [] };
        pm.in += tin; pm.out += tout; pm.rows++;
        if (!pm.agents.includes(agentKey)) pm.agents.push(agentKey);
        const sd = (out.live.series[mid] ||= {})[day] ||= [0, 0, 0];
        sd[0] += tin; sd[1] += tout; sd[2]++;

        const row = out.agentUse[agentKey] ||= { sessions: 0, totIn: 0, totOut: 0, models: [], lastDay: '' };
        row.sessions++; row.totIn += tin; row.totOut += tout;
        if (!row.models.includes(mid)) row.models.push(mid);
        if (day > row.lastDay) row.lastDay = day;
      }
    }
  }
  return out;
}
// Mind usage: how many times each Mind (a Claude Code Skill tool_use, `input.skill`) was
// actually invoked in this world's own transcripts — not just whether it exists. Human order
// 2026-09-20: "show on tempest the skills and how much they are used". Same cwd-prefix
// scoping as harvestClaudeUsage(root); a skill invoked from a subagent still counts (isSidechain
// doesn't change which Mind ran).
function harvestSkillUsage(root) {
  const counts = {};
  const projectsDir = path.join(os.homedir(), '.claude', 'projects');
  if (!fs.existsSync(projectsDir)) return counts;
  const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;
  let slugDirs;
  try { slugDirs = fs.readdirSync(projectsDir, { withFileTypes: true }).filter(e => e.isDirectory()).map(e => e.name); }
  catch { return counts; }
  for (const slug of slugDirs) {
    let files;
    try { files = fs.readdirSync(path.join(projectsDir, slug)).filter(f => f.endsWith('.jsonl')); }
    catch { continue; }
    for (const f of files) {
      let text;
      try { text = fs.readFileSync(path.join(projectsDir, slug, f), 'utf8'); }
      catch { continue; }
      for (const line of text.split('\n')) {
        if (!line) continue;
        let d;
        try { d = JSON.parse(line); } catch { continue; }
        if (d.type !== 'assistant') continue;
        const cwd = d.cwd || '';
        if (cwd !== root && !cwd.startsWith(rootPrefix)) continue;
        const content = (d.message || {}).content;
        if (!Array.isArray(content)) continue;
        for (const block of content) {
          if (block && block.type === 'tool_use' && block.name === 'Skill' && block.input && block.input.skill) {
            const name = block.input.skill.split(':').pop(); // strip plugin/dir-scope prefix
            counts[name] = (counts[name] || 0) + 1;
          }
        }
      }
    }
  }
  return counts;
}
// Context-window stress — the dashboard's own read of the same instrument
// `.isekai/tools/context-check.sh` reports from a terminal (Nature 9: Rimuru's own stress is
// a measured reading, not a feeling). Same method, same 200k/180k defaults: the *most recent*
// assistant turn's own usage (input + cache_read + cache_creation) across every session
// matching this root is the live context-window occupancy — a cumulative sum across turns
// would answer a different question (total spend, see harvestClaudeUsage). Human 2026-09-21:
// "context window in tempest dash at the beginning, with color, so it's seeable."
function harvestContextStress(root) {
  const LIMIT = 200000, STRESS = 180000;
  const projectsDir = path.join(os.homedir(), '.claude', 'projects');
  if (!fs.existsSync(projectsDir)) return null;
  const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;
  let slugDirs;
  try { slugDirs = fs.readdirSync(projectsDir, { withFileTypes: true }).filter(e => e.isDirectory()).map(e => e.name); }
  catch { return null; }
  let best = null; // { ts, tin } — the latest-by-timestamp usage-bearing turn seen
  for (const slug of slugDirs) {
    let files;
    try { files = fs.readdirSync(path.join(projectsDir, slug)).filter(f => f.endsWith('.jsonl')); }
    catch { continue; }
    for (const f of files) {
      let text;
      try { text = fs.readFileSync(path.join(projectsDir, slug, f), 'utf8'); }
      catch { continue; }
      for (const line of text.split('\n')) {
        if (!line) continue;
        let d;
        try { d = JSON.parse(line); } catch { continue; }
        if (d.type !== 'assistant') continue;
        const cwd = d.cwd || '';
        if (cwd !== root && !cwd.startsWith(rootPrefix)) continue;
        const usage = (d.message || {}).usage;
        if (!usage) continue;
        const ts = d.timestamp || '';
        if (!best || ts > best.ts) {
          const tin = (usage.input_tokens || 0) + (usage.cache_read_input_tokens || 0) + (usage.cache_creation_input_tokens || 0);
          best = { ts, tin };
        }
      }
    }
  }
  if (!best) return null;
  const pct = Math.round(100 * best.tin / LIMIT);
  const band = best.tin >= STRESS ? 'hot' : best.tin >= STRESS * 0.75 ? 'warn' : 'cool';
  return { tin: best.tin, limit: LIMIT, pct, band, ts: best.ts };
}
// ------------ machine-wide activity (for the global / index — not scoped to one world) ------------
// Human order 2026-09-20: "how much consumption in total if opencode session ... maybe
// information if session is active or not". Unlike harvestClaudeUsage(root)/the opencode.db
// query in harvest(root), these are NOT filtered to one colony — every session on this
// machine, whether or not its directory is a reincarnated world.
const ACTIVE_WINDOW_MS = 5 * 60000; // "active" = touched in the last 5 minutes
function harvestClaudeGlobal() {
  const out = { sessions: 0, active: 0, totalIn: 0, totalOut: 0, perModel: {} };
  const projectsDir = path.join(os.homedir(), '.claude', 'projects');
  if (!fs.existsSync(projectsDir)) return out;
  const now = Date.now();
  let slugDirs;
  try { slugDirs = fs.readdirSync(projectsDir, { withFileTypes: true }).filter(e => e.isDirectory()).map(e => e.name); }
  catch { return out; }
  for (const slug of slugDirs) {
    let files;
    try { files = fs.readdirSync(path.join(projectsDir, slug)).filter(f => f.endsWith('.jsonl')); }
    catch { continue; }
    for (const f of files) {
      const fp = path.join(projectsDir, slug, f);
      let stat; try { stat = fs.statSync(fp); } catch { continue; }
      out.sessions++;
      if (now - stat.mtimeMs <= ACTIVE_WINDOW_MS) out.active++;
      let text; try { text = fs.readFileSync(fp, 'utf8'); } catch { continue; }
      for (const line of text.split('\n')) {
        if (!line) continue;
        let d; try { d = JSON.parse(line); } catch { continue; }
        if (d.type !== 'assistant') continue;
        const usage = (d.message || {}).usage || {};
        const mid = (d.message || {}).model || 'unknown';
        // See harvestClaudeUsage(root)'s comment: input_tokens alone excludes cache
        // reads/writes, which is almost all of it under prompt caching.
        const tin = (usage.input_tokens || 0) + (usage.cache_read_input_tokens || 0) + (usage.cache_creation_input_tokens || 0);
        const tout = usage.output_tokens || 0;
        out.totalIn += tin; out.totalOut += tout;
        const pm = out.perModel[mid] ||= { in: 0, out: 0 };
        pm.in += tin; pm.out += tout;
      }
    }
  }
  return out;
}
// OpenCode: only the columns already proven against a real store elsewhere in this file
// (model, tokens_input, tokens_output, time_created) — no session-id column is used
// anywhere else here, so "sessions" isn't claimed, only rows and recent-row activity.
function harvestOpencodeGlobal() {
  // Nature 9: an instrument that's gone silent is itself a finding, not a thing to route
  // around. `dbExists` (the store is there) and `available` (we could actually read it) are
  // deliberately separate — found the hard way 2026-09-20: node:sqlite is a Node 22.5+
  // built-in, so on any older system Node (this machine ships 16.20.2), `require('node:sqlite')`
  // throws and a REAL opencode.db with real sessions in it was silently reported as "OpenCode
  // not in use here" — indistinguishable from the file genuinely not existing. That's exactly
  // the failure mode this Nature warns against.
  const out = { rows: 0, activeRows: 0, totalIn: 0, totalOut: 0, perModel: {}, dbExists: false, available: false, error: null };
  const dbp = path.join(os.homedir(), '.local', 'share', 'opencode', 'opencode.db');
  out.dbExists = fs.existsSync(dbp);
  if (!out.dbExists) return out;
  try {
    const { DatabaseSync } = require('node:sqlite');
    const db = new DatabaseSync(dbp, { readOnly: true });
    out.available = true;
    try {
      const sinceMs = Date.now() - ACTIVE_WINDOW_MS;
      // Same undercount class as harvestClaudeUsage/harvestClaudeGlobal above: tokens_input
      // alone is only the uncached sliver of a turn's real input. Found 2026-09-20 by the
      // deterministic test in test-opencode-integration.js — tokens_cache_read/_write must
      // be summed in too, or a session showing 11,008 real cache-read tokens in `opencode
      // export` reports as if it cost 51.
      for (const r of db.prepare(
        `SELECT COALESCE(model,'') m, COUNT(*) runs,
                COALESCE(SUM(tokens_input + tokens_cache_read + tokens_cache_write),0) tin,
                COALESCE(SUM(tokens_output),0) tout,
                SUM(CASE WHEN time_created >= ? THEN 1 ELSE 0 END) recent
         FROM session GROUP BY m`).all(sinceMs)) {
        let mid = 'unknown';
        try { mid = JSON.parse(r.m).id || 'unknown'; } catch { mid = String(r.m) || 'unknown'; }
        out.rows += r.runs; out.activeRows += r.recent || 0;
        out.totalIn += r.tin; out.totalOut += r.tout;
        const pm = out.perModel[mid] ||= { in: 0, out: 0 };
        pm.in += r.tin; pm.out += r.tout;
      }
    } finally { db.close(); }
  } catch (e) { out.error = e.message; }
  return out;
}
function mergeLive(into, from) {
  into.rows += from.rows; into.totalIn += from.totalIn; into.totalOut += from.totalOut;
  for (const [day, v] of Object.entries(from.perDay)) {
    const pd = into.perDay[day] ||= { in: 0, out: 0 };
    pd.in += v.in; pd.out += v.out;
  }
  for (const [mid, v] of Object.entries(from.perModel)) {
    const pm = into.perModel[mid] ||= { in: 0, out: 0, rows: 0, agents: [] };
    pm.in += v.in; pm.out += v.out; pm.rows += v.rows;
    for (const a of v.agents) if (!pm.agents.includes(a)) pm.agents.push(a);
  }
  for (const [mid, days] of Object.entries(from.series || {})) {
    const s = into.series[mid] ||= {};
    for (const [day, arr] of Object.entries(days)) {
      const sd = s[day] ||= [0, 0, 0];
      sd[0] += arr[0]; sd[1] += arr[1]; sd[2] += arr[2];
    }
  }
}
function mergeAgentUse(into, from) {
  for (const [agent, v] of Object.entries(from)) {
    const row = into[agent] ||= { sessions: 0, totIn: 0, totOut: 0, models: [], lastDay: '' };
    row.sessions += v.sessions; row.totIn += v.totIn; row.totOut += v.totOut;
    for (const m of v.models) if (!row.models.includes(m)) row.models.push(m);
    if (v.lastDay > row.lastDay) row.lastDay = v.lastDay;
  }
}

// ------------ harvest ------------
function harvest(root) {
  const home = homeOf(root), canonFile = canonOf(root, home), worldName = nameOf(root, home);
  const skillsDir = path.join(root, '.opencode', 'skills');
  const docOf = d => path.join(skillsDir, d, 'SKILL.md');
  const dirs = fs.existsSync(skillsDir)
    ? fs.readdirSync(skillsDir, { withFileTypes: true }).filter(e => e.isDirectory()).map(e => e.name).sort()
    : [];
  // A .opencode/skills/<name>/ entry is a creature only if its name carries a race
  // prefix (orc-, slime-, ...) — the "real source" shape where creatures live here.
  // Anything else is a genuine Mind (reusable know-how, not a creature): e.g.
  // palette-audit. Harmony rule (human order 2026-09-20): a Mind links to whichever
  // creatures its own doc names, or whose doc names it back — never assigned by hand.
  const creatures = [];
  const minds = [];
  // 2026-09-21 (payload B): the same scan also covers .claude/skills/<name>/SKILL.md — a
  // `/don --project` install, or a Claude-only world with no .opencode/ at all. Merged by
  // name AFTER .opencode/skills/ so a Mind installed in both places is one node, not two
  // (Nature 2: no duplication); `src` remembers where it was actually read from.
  const claudeSkillsDir = path.join(root, '.claude', 'skills');
  const claudeDirs = fs.existsSync(claudeSkillsDir)
    ? fs.readdirSync(claudeSkillsDir, { withFileTypes: true }).filter(e => e.isDirectory()).map(e => e.name).sort()
    : [];
  const skillScan = [...dirs.map(name => ({ name, docPath: docOf(name), src: '.opencode/skills' })),
    ...claudeDirs.map(name => ({ name, docPath: path.join(claudeSkillsDir, name, 'SKILL.md'), src: '.claude/skills' }))];
  for (const { name, docPath, src } of skillScan) {
    if (creatures.some(c => c.name === name) || minds.some(m => m.name === name)) continue;
    const doc = rd(docPath);
    if (!doc && src === '.claude/skills') continue; // an empty dir there is not a Mind
    const race = RACES.find(r => name.startsWith(r + '-'));
    const bytes = Buffer.byteLength(doc);
    const desc = unquote((doc.match(/^description:\s*(.+)$/m) || [])[1] || '');
    // A Mind's context cost isn't its file size — only its `description` sits in context on
    // every turn by default (see the writing-for-agents Mind and isekai.md's own "Minds"
    // section); the full body (`kb`) only loads when actually donned. ~4 bytes/token, the same
    // rough estimate context-check.sh already uses and names as an estimate, not a billing
    // figure. Human 2026-09-21: "add how much it costs [a Mind] in context tokens too."
    if (!race) { minds.push({ name, kb: +(bytes / 1024).toFixed(1), descTok: Math.round(Buffer.byteLength(desc) / 4), desc, doc, links: [], docPath, src }); continue; }
    // thoughts: section-scoped dated bullets (>- ## Thoughts until next ## or EOF)
    const m = doc.match(/##\s*Thoughts([\s\S]*?)(?=\n##\s|\n#\s|$)/i);
    const tBody = m ? m[1] : '';
    const thoughtLines = tBody.split('\n').filter(l => /^\s*(-|###)/.test(l) && /\d{4}-\d{2}-\d{2}/.test(l));
    const thoughtDates = thoughtLines.map(l => (l.match(/\d{4}-\d{2}-\d{2}/) || [])[0]).filter(Boolean);
    creatures.push({ name, race, kb: +(bytes / 1024).toFixed(1), thoughts: thoughtLines.length,
      thoughtDates, limit: DESK_LIMIT(name), desc, doc, docPath, src });
  }
  // ROW 2 — SHARED SKILLS (separation law, 2026-09-21): opencode commands & app-provided
  // skills that are NOT isekai minds. Veldora: the row does not "overload" the world's law
  // with the host repo's tools; it names them plainly for what they are. Sourced from
  // .opencode/commands/*.md and .claude/commands/*.md, merged by name (the same command
  // ported to both hosts is one thing with two sources, not two things).
  const commands = [];
  for (const src of ['.opencode/commands', '.claude/commands']) {
    const dir = path.join(root, ...src.split('/'));
    if (!fs.existsSync(dir)) continue;
    for (const f of fs.readdirSync(dir).filter(f => f.endsWith('.md')).sort()) {
      const name = path.basename(f, '.md');
      const hit = commands.find(c => c.name === name);
      if (hit) { hit.srcs.push(src); continue; }
      const doc = rd(path.join(dir, f));
      const desc = unquote((doc.match(/^description:\s*(.+)$/m) || [])[1] || '')
        || ((doc.match(/^#\s+(.+)$/m) || [])[1] || '').trim();
      commands.push({ name, srcs: [src], kb: +(Buffer.byteLength(doc) / 1024).toFixed(1), desc, docPath: path.join(dir, f) });
    }
  }

  // operative /isekai + /genesis shape: .isekai/{elf,orc,slime}/<name>/*.md — no
  // .opencode/skills/ or AGENTS.md required. Merged in alongside any skills-shaped
  // creatures above (not a replacement), so either shape, or both at once, renders.
  const isekaiOrcCommands = {}; // orc name -> [slime names], read off its own "Commands:" line
  for (const race of ['elf', 'orc', 'slime']) {
    const raceDir = path.join(root, home, race);
    if (!fs.existsSync(raceDir)) continue;
    for (const zone of fs.readdirSync(raceDir, { withFileTypes: true })
      .filter(e => e.isDirectory()).map(e => e.name).sort()) {
      const name = `${race}-${zone}`;
      if (creatures.some(c => c.name === name)) continue; // skills-shaped already has it
      const zoneDir = path.join(raceDir, zone);
      const mdFile = ['README.md', ...fs.readdirSync(zoneDir).filter(f => f.endsWith('.md'))]
        .find(f => fs.existsSync(path.join(zoneDir, f)));
      if (!mdFile) continue;
      const doc = rd(path.join(zoneDir, mdFile));
      const bytes = Buffer.byteLength(doc);
      const m = doc.match(/##\s*Thoughts([\s\S]*?)(?=\n##\s|\n#\s|$)/i);
      const tBody = m ? m[1] : '';
      const thoughtLines = tBody.split('\n').filter(l => /^\s*(-|###)/.test(l) && /\d{4}-\d{2}-\d{2}/.test(l));
      const thoughtDates = thoughtLines.map(l => (l.match(/\d{4}-\d{2}-\d{2}/) || [])[0]).filter(Boolean);
      const desc = unquote((doc.match(/^description:\s*(.+)$/m) || [])[1]
        || (doc.match(/^-\s*\*\*Purpose:\*\*\s*(.+)$/m) || [])[1] || '');
      if (race === 'orc') {
        const cmds = (doc.match(/^-\s*\*\*Commands:\*\*\s*(.+)$/m) || [])[1] || '';
        isekaiOrcCommands[name] = cmds.split(',').map(s => s.trim()).filter(Boolean);
      }
      creatures.push({ name, race, kb: +(bytes / 1024).toFixed(1), thoughts: thoughtLines.length,
        thoughtDates, limit: DESK_LIMIT(name), desc, doc, docPath: path.join(zoneDir, mdFile) });
    }
  }
  // crosslink index: mentions of other creature names (dir name or map alias) inside a doc
  const aliases = {}; // dir -> [names it answers to]
  const cz = rd(path.join(root, canonFile));
  const ag = rd(path.join(root, 'AGENTS.md'));
  for (const c of creatures) aliases[c.name] = [c.name];
  for (const hit of (cz + ag).matchAll(/`([a-z0-9][a-z0-9-]+)`/g)) {
    const bare = hit[1];
    for (const c of creatures)
      if (bare.endsWith(c.name) && bare !== c.name) (aliases[c.name] ||= []).push(bare); // ui-design ↔ benchforge-ui-design
  }
  // Mind ↔ creature harmony links, computed here while c.doc/mind.doc still exist:
  // bidirectional name-mention, exactly the same "does the text actually say so"
  // rule crosslinks use between creatures — never a hand-assigned pairing.
  for (const mind of minds) {
    mind.links = creatures.filter(c => mind.doc.includes(c.name) || c.doc.includes(mind.name)).map(c => c.name);
  }
  const skillUses = harvestSkillUsage(root);
  for (const mind of minds) mind.uses = skillUses[mind.name] || 0;
  for (const c of creatures) {
    c.crosslinks = creatures.filter(o => o.name !== c.name)
      .reduce((n, o) => n + aliases[o.name].reduce((k, al) =>
        k + (c.doc.split(al).length - 1), 0), 0);
    delete c.doc;
    c.stressPct = Math.round(100 * c.thoughts / c.limit);
    c.dietPct = Math.round(100 * c.kb / DIET_KB);
  }
  const median = xs => xs.length ? xs.slice().sort((a, b) => a - b)[Math.floor(xs.length / 2)] : 0;
  const linkMed = median(creatures.map(c => c.crosslinks));
  for (const c of creatures)
    c.genesisSignal = c.stressPct >= 80 && c.crosslinks >= Math.max(2, linkMed * 1.5); // heuristic, labeled in UI

  // architecture tree from the map's routing table
  const orcs = [];
  for (const l of ag.split('\n')) {
    const m = l.match(/\|\s*`?(orc-[^`|]+)`?\s*\|([^|]*)\|([^|]*)\|/);
    if (m) orcs.push({ orc: m[1], domain: m[2].trim(),
      slimes: [...m[3].matchAll(/`([^`]+)`/g)].map(x => x[1]).filter(s => s !== '-') });
  }
  // same tree, read straight off each isekai-shaped orc's own "Commands:" line —
  // no AGENTS.md required for the operative /isekai + /genesis shape.
  for (const [orc, slimes] of Object.entries(isekaiOrcCommands)) {
    if (!orcs.some(o => o.orc === orc)) orcs.push({ orc, domain: '', slimes });
  }
  // canon stamps
  const canonV = (cz.match(/canon v(\d+)/) || [])[1] || '?';
  const chartDebt = rd(path.join(root, home, 'assets', path.basename(canonFile).replace(/\.md$/, '.drawio')))
    .match(/canon v(\d+)/);
  const chartV = chartDebt ? chartDebt[1] : null;

  // evolution series per day
  const days = {};
  const bump = (d, k) => { if (d) (days[d] ||= { journal: 0, commits: 0, thoughts: 0 })[k]++; };
  for (const l of rd(journalPath(root, home)).split('\n')) {
    // entry headers: isekai `### [YYYY-MM-DD HH:MM] being - title` and elder `# Log — date`
    const m = l.match(/^#{1,3}\s+(?:\[?(\d{4}-\d{2}-\d{2})|Log\s.*?\[?(\d{4}-\d{2}-\d{2}))/);
    if (m) bump(m[1] || m[2], 'journal');
  }
  for (const c of creatures) for (const d of c.thoughtDates) bump(d, 'thoughts');
  try {
    for (const l of execSync('git log --date=format:%Y-%m-%d --format=%ad', { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim().split('\n'))
      bump(l, 'commits');
  } catch { /* not a git repo — series simply lacks commits */ }
  for (const c of creatures) { c.lastThought = c.thoughtDates.length ? c.thoughtDates.slice().sort().pop() : null; delete c.thoughtDates; }

  // model mounts (colony genome pins) + ledger sightings
  const models = {};
  // Bodies live natively at .opencode/agents/ (OpenCode) or .claude/agents/ (Claude Code,
  // via /mint) — both scanned and merged, since a world can have either or both.
  const agentDirs = [path.join(root, '.opencode', 'agents'), path.join(root, '.claude', 'agents')];
  const agentFiles = agentDirs.flatMap(d => fs.existsSync(d)
    ? fs.readdirSync(d).filter(f => f.endsWith('.md')).map(f => path.join(d, f)) : []);
  for (const f of agentFiles) {
    const mm = rd(f).match(/^model:\s*(\S+)/m);
    if (mm) (models[mm[1]] ||= { mounted: [], mentions: 0 }).mounted.push(path.basename(f, '.md'));
  }
  const ledger = rd(journalPath(root, home)) + '\n' + ag + '\n' + cz;
  for (const id of Object.keys(models)) models[id].mentions = ledger.split(id).length - 1;

  // session token ledger (rule: sessions append JSONL lines; the board reads).
  // A line may carry "model" (law extended 2026-09-15 — "usage token per model");
  // absent, the session's agent joins through the mount pins above; unmounted = alone.
  const agentModel = {};
  for (const [mid, mo] of Object.entries(models)) for (const a of mo.mounted) agentModel[a] = mid;
  const tokens = { rows: 0, totalIn: 0, totalOut: 0, perDay: {}, perModel: {} };
  for (const l of rd(path.join(root, home, 'metrics', 'tokens.jsonl')).split('\n').filter(Boolean)) {
    try {
      const j = JSON.parse(l); tokens.rows++;
      tokens.totalIn += +j.in || 0; tokens.totalOut += +j.out || 0;
      const day = String(j.ts || '').slice(0, 10) || 'undated';
      const pd = tokens.perDay[day] ||= { in: 0, out: 0 };
      pd.in += +j.in || 0; pd.out += +j.out || 0;
      const pmid = String(j.model || '') || agentModel[String(j.agent || '')] || 'unmounted';
      const pm = tokens.perModel[pmid] ||= { in: 0, out: 0, rows: 0, agents: [] };
      pm.in += +j.in || 0; pm.out += +j.out || 0; pm.rows++;
      const ag2 = String(j.agent || ''); if (ag2 && !pm.agents.includes(ag2)) pm.agents.push(ag2);
    } catch { /* a malformed line is noise, never a crash */ }
  }

  // live usage from opencode's own session store (human 2026-09-15: "i still
  // dont see the token usage" — the manual ledger was a law nobody fed). Truth
  // source: session.agent/model/tokens_*, colony-scoped by directory, opened
  // read-only. Absent db / unreadable schema → the manual ledger above stands.
  const live = { rows: 0, totalIn: 0, totalOut: 0, perDay: {}, perModel: {}, series: {} };
  try {
    const dbp = path.join(os.homedir(), '.local', 'share', 'opencode', 'opencode.db');
    if (fs.existsSync(dbp)) {
      const { DatabaseSync } = require('node:sqlite');
      const db = new DatabaseSync(dbp, { readOnly: true });
      try {
        for (const r of db.prepare(
          `SELECT COALESCE(model,'') m, COALESCE(agent,'main session') a, COUNT(*) runs,
                  COALESCE(SUM(tokens_input),0) tin, COALESCE(SUM(tokens_output),0) tout,
                  date(time_created/1000,'unixepoch') day
           FROM session WHERE directory LIKE ? GROUP BY m, a, day`).all(root + '%')) {
          let mid = 'unknown';
          try { mid = JSON.parse(r.m).id || 'unknown'; } catch { mid = String(r.m) || 'unknown'; }
          live.rows += r.runs; live.totalIn += r.tin; live.totalOut += r.tout;
          const pd = live.perDay[r.day || 'undated'] ||= { in: 0, out: 0 };
          pd.in += r.tin; pd.out += r.tout;
          const pm = live.perModel[mid] ||= { in: 0, out: 0, rows: 0, agents: [] };
          pm.in += r.tin; pm.out += r.tout; pm.rows += r.runs;
          if (!pm.agents.includes(r.a)) pm.agents.push(r.a);
          const sd = (live.series[mid] ||= {})[r.day || 'undated'] ||= [0, 0, 0];
          sd[0] += r.tin; sd[1] += r.tout; sd[2] += r.runs;
        }
      } finally { db.close(); }
    }
  } catch { /* an unreadable store is noise — the manual ledger still stands */ }
  // Claude Code's own session transcripts — merged additively, not a replacement, so a
  // machine running both ecosystems on the same world sees combined numbers.
  const claudeUsage = harvestClaudeUsage(root);
  mergeLive(live, claudeUsage.live);

  // ==== agents: minted bodies on disk + per-session breath (the agents ledger) ====
  const bodies = [];
  for (const f of agentFiles) {
    const t = rd(f);
    const claudeBody = f.includes(`${path.sep}.claude${path.sep}agents${path.sep}`);
    bodies.push({ name: path.basename(f, '.md'),
      model: (t.match(/^model:\s*(\S+)/m) || [null, '(inherits session)'])[1],
      // Claude Code sub-agents carry no `mode:` field (no primary/all distinction the way
      // OpenCode has) — every .claude/agents/ file is Court-shaped (Agent-tool invoked), so
      // default to 'subagent' there instead of an unhelpful '?'.
      mode: (t.match(/^mode:\s*(\S+)/m) || [null, claudeBody ? 'subagent' : '?'])[1],
      born: fs.statSync(f).birthtime.toISOString().slice(0, 10),
      kb: +(Buffer.byteLength(t) / 1024).toFixed(1) });
  }
  // per-session agent breath — opencode's store, read-only; avg context = the mean
  // token draw (in+out) per session answering as that agent.
  const agentUse = {};
  try {
    const dbp2 = path.join(os.homedir(), '.local', 'share', 'opencode', 'opencode.db');
    if (fs.existsSync(dbp2)) {
      const { DatabaseSync } = require('node:sqlite');
      const db = new DatabaseSync(dbp2, { readOnly: true });
      try {
        for (const r of db.prepare(
          `SELECT COALESCE(agent,'?') a, COALESCE(model,'') m, COUNT(*) n,
                  COALESCE(SUM(tokens_input),0) tin, COALESCE(SUM(tokens_output),0) tout,
                  MAX(date(time_created/1000,'unixepoch')) lastDay
           FROM session WHERE directory LIKE ? GROUP BY a, m`).all(root + '%')) {
          let mid = 'unknown';
          try { mid = JSON.parse(r.m).id || 'unknown'; } catch { mid = String(r.m) || 'unknown'; }
          const row = agentUse[r.a] ||= { sessions: 0, totIn: 0, totOut: 0, models: [], lastDay: '' };
          row.sessions += r.n; row.totIn += r.tin || 0; row.totOut += r.tout || 0;
          if (!row.models.includes(mid)) row.models.push(mid);
          if (r.lastDay > row.lastDay) row.lastDay = r.lastDay;
        }
      } finally { db.close(); }
    }
  } catch { /* store absent/unreadable — the ledger simply lacks usage */ }
  mergeAgentUse(agentUse, claudeUsage.agentUse);
  for (const u of Object.values(agentUse))
    u.avgCtx = u.sessions ? Math.round((u.totIn + u.totOut) / u.sessions) : 0;

  // the agents ledger — a metrics artifact regenerated when reality moves
  // (instruments write metrics, never minds — nature law 8). Populated by working.
  try {
    const fmtN = v => v >= 1e6 ? (v / 1e6).toFixed(1) + 'M' : v >= 1e3 ? (v / 1e3).toFixed(1) + 'k' : String(v);
    const ledgerMd = [
      `# Agents ledger — ${worldName} (${path.basename(root)})`,
      `Regenerated live by the instruments when reality moves. Bodies live on disk; breath lives in opencode's session store.`,
      ``,
      `## Minted bodies (.opencode/agents/)`,
      `| Agent | Mode | Mount | Born | KB |`, `|---|---|---|---|---|`,
      ...(bodies.length
        ? bodies.map(b => `| \`${b.name}\` | ${b.mode} | ${b.model} | ${b.born} | ${b.kb} |`)
        : ['| – | – | minds-only world (embodiment E2) | – | – |']),
      ``,
      `## Session breath per agent (avg context = mean tokens in+out per session)`,
      `| Agent | Sessions | Avg context | Total in | Total out | Mounts seen | Last day |`,
      `|---|---|---|---|---|---|---|`,
      ...(Object.keys(agentUse).length
        ? Object.entries(agentUse).sort((a, b) => (b[1].totIn + b[1].totOut) - (a[1].totIn + a[1].totOut))
          .map(([a, u]) => `| ${a} | ${u.sessions} | ${fmtN(u.avgCtx)} | ${fmtN(u.totIn)} | ${fmtN(u.totOut)} | ${u.models.join(', ') || '–'} | ${u.lastDay || '–'} |`)
        : ['| – | 0 | – | – | – | no sessions recorded in this world yet | – |']),
    ].join('\n');
    const mdir = path.join(root, home, 'metrics'); fs.mkdirSync(mdir, { recursive: true });
    const fp = path.join(mdir, 'agents-usage.md');
    if (rd(fp) !== ledgerMd) fs.writeFileSync(fp, ledgerMd);
  } catch { /* metrics unwritable — the panel still renders */ }

  for (const m of minds) delete m.doc;

  const census = {};
  for (const c of creatures) census[c.race] = (census[c.race] || 0) + 1;
  const stressed = creatures.filter(c => c.thoughts > c.limit || c.dietPct > 100)
    .map(c => `${c.name} (thoughts ${c.thoughts}/${c.limit}, ${c.kb}KB/${DIET_KB}KB)`);
  return { root, world: worldName, home, canonFile, port: PORT, bodies, agentUse, when: new Date().toISOString(), canonV, chartV: chartV ? +chartV : null,
    chartDebt: chartV !== null && +chartV !== +canonV, census, orcs, creatures, minds, commands, days, models, tokens, live,
    genesisWatch: creatures.filter(c => c.genesisSignal).map(c => c.name),
    ctxStress: harvestContextStress(root),
    health: stressed.length ? stressed.join(' · ') : 'all minds within budget' };
}

// ------------ render ------------
// The constellation render (human 2026-09-14: "futuristic, worthy of an AI").
// Pure SVG + CSS — no scripts, no CDN, no fonts; the shape travels (rule 23).
// Layout deterministic: elf at the heart, orcs in orbit, slimes outer ring
// clustered by their orc, the dark elf burns apart. Node size = doc weight,
// aura = stress; a hot node breathes. Colors honor the colony's night themes.
// CSS custom properties, not raw hex — lets light/dark carry their own validated
// steps (see the :root / body.light palette comment) instead of one hex per race
// baked into server-rendered SVG regardless of theme.
const RCOL = { slime: 'var(--r-slime)', orc: 'var(--r-orc)', elf: 'var(--r-elf)', darkelf: 'var(--r-darkelf)', highorc: 'var(--r-highorc)', kijin: 'var(--r-kijin)', plain: 'var(--r-plain)', mind: 'var(--r-mind)' };
const aura = c => c.stressPct >= 100 ? 'hot' : c.stressPct >= 60 || c.dietPct >= 100 ? 'warn' : 'cool';
// Race portrait files, human 2026-09-21 — served only by this fixed map (never a raw
// filename off the URL) from <root>/<home>/portraits/. 'plain' and 'mind' have none —
// they keep their flat color-dot look, which is honest: they're not a named race/likeness.
const PORTRAIT_FILE = { slime: 'slime.png', orc: 'orc.png', elf: 'elf.png', darkelf: 'dark_elf.png', highorc: 'high_orc.png', kijin: 'kijin.png' };
// Word 19 (Veldora 2026-09-21): minds wear BRAINS, colored by their lane's reasoning.
// Hand-drawn glyph (two lobes + crease); unit height ~12.4 → scale ≈ r*0.13 at render.
// Lane colors reuse the ANIMA tints so a brain's hue answers "whose work does this mind do?"
// The path is lifted verbatim from the photographed source; the tints ride CSS custom
// properties (--l-*) instead of the source's raw hex, so light mode gets its own re-stepped
// values (see the :root / body.light palette block) — same doctrine as RCOL above.
const BRAIN_D = 'M0 -6 C-3.2 -6 -5.6 -4.4 -5.6 -1.6 C-7.2 -0.6 -7.2 1.8 -5.4 2.8 C-6 4.6 -4.2 6.2 -2.2 6.2 C-1.1 6.2 -0.4 5.4 0 4.4 C0.4 5.4 1.1 6.2 2.2 6.2 C4.2 6.2 6 4.6 5.4 2.8 C7.2 1.8 7.2 -0.6 5.6 -1.6 C5.6 -4.4 3.2 -6 0 -6 Z';
const LANECOL = { smind: 'var(--l-smind)', omind: 'var(--l-omind)', gmind: 'var(--l-gmind)', shared: 'var(--l-shared)' };
const LANE_NAME = { smind: 'zone', omind: 'verdict', gmind: 'global', shared: 'shared' };
const brainIcon = (x, y, r, col) => `<g transform="translate(${x.toFixed(1)} ${y.toFixed(1)}) scale(${(r * 0.13).toFixed(2)})"><path d="${BRAIN_D}" fill="${col}"/><path d="M0 -6 L0 4.4" stroke="var(--nodefill)" stroke-width="1.1" fill="none"/></g>`;
// The court triad: who reads, who verdicts, who drafts — and WHICH MODEL rides each brain.
// Mounts come from the minted bodies (.claude/agents / .opencode/agents frontmatter, the
// same `bodies` harvest() already builds), matched by name family, never hand-typed. The
// source's families (hayai / mon / kata) are its own body names; anchored here to a name
// segment so a `slime-monitoring` body is never mistaken for RAPHAEL's mount.
const TRIAD = [
  { mind: 'GREAT-SAGE', role: 'reads', re: /(^|-)(great-sage|hayai)(-|$)/i },
  { mind: 'RAPHAEL', role: 'verdicts', re: /(^|-)(raphael|mon)(-|$)/i },
  { mind: 'CIEL', role: 'drafts', re: /(^|-)(ciel|kata)(-|$)/i },
];
const MODEL_COL = [{ re: /qwen/i, col: 'var(--r-slime)' }, { re: /glm/i, col: 'var(--r-orc)' }, { re: /kimi/i, col: 'var(--r-elf)' }, { re: /claude|opus|sonnet|haiku/i, col: 'var(--gd)' }];

const PAGE_STYLE = `<style>
:root{--bg:#03060c;--ink:#c9d4e3;--dim:#616b79;--cy:#2695bd;--vi:#7c5cd6;--em:#d6402a;--gd:#bd8c24;--nodefill:#05070d;
--r-slime:#2695bd;--r-orc:#7c5cd6;--r-elf:#bd8c24;--r-darkelf:#d6402a;--r-highorc:#8a7300;--r-kijin:#c53d34;--r-plain:#616b79;--r-mind:#9fb0c3;
--l-smind:#4db8dd;--l-omind:#9a86e8;--l-gmind:#d4af37;--l-shared:#9fb0c3}
/* --l-* are the ANIMA lane tints (word 15, 2026-09-21): zone / verdict / global / shared — one
   per mind lane, lighter siblings of the race hue each lane serves, used for wear-edges and
   the brain glyphs alike. Light mode re-steps them below. */
/* Palette re-tuned 2026-09-20 against the dataviz skill's validate_palette.js (OKLCH
   lightness band, CVD/normal-vision Delta E, WCAG contrast) — see .isekai/tmp/palette-check/.
   Was: identical hex reused for both themes, several near the lightness ceiling for a
   dark surface, and the core slime/orc/elf trio sat below the CVD normal-vision floor
   (worst pair Delta E 7.8, need >=15). Now: core trio passes every hard gate in both
   modes; darkelf/highorc/kijin (rare "born in time" races) keep a softer separation —
   a known-hard 3-way warm-hue constraint the skill's own reference palette hits past
   3-4 slots too — mitigated by the mandatory node label + tooltip every creature
   already carries (the skill's required secondary encoding for a WARN-band pair). */
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);
font:13px/1.55 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;overflow-x:hidden;padding:34px 28px 60px}
body::before{content:"";position:fixed;inset:0;pointer-events:none;z-index:0;
background:radial-gradient(900px 480px at 78% -8%,rgba(89,214,255,.09),transparent 62%),
           radial-gradient(760px 420px at 6% 10%,rgba(167,139,250,.08),transparent 60%),
           radial-gradient(1100px 700px at 50% 118%,rgba(255,122,89,.06),transparent 62%)}
body::after{content:"";position:fixed;inset:0;pointer-events:none;z-index:0;opacity:.35;
background:repeating-linear-gradient(0deg,transparent 0 3px,rgba(0,0,0,.14) 3px 4px)}
main{position:relative;z-index:1;max-width:1220px;margin:0 auto}
h1{font-size:15px;letter-spacing:.34em;text-transform:uppercase;margin:0;font-weight:600;color:#eaf2ff;
text-shadow:0 0 18px rgba(89,214,255,.5)}
h1 .sigil{color:var(--cy)}
.allworlds{display:inline-block;color:var(--dim);text-decoration:none;font-size:11px;letter-spacing:.14em;
text-transform:uppercase;margin-bottom:14px;border:1px solid #1a2436;border-radius:999px;padding:4px 14px;
background:rgba(13,20,32,.6)}
.allworlds:hover{color:var(--cy);border-color:var(--cy)}
body.light .allworlds{background:rgba(255,255,255,.8);border-color:#cfdae9}
.wcards{display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:14px;margin-top:14px}
.wcard{display:block;text-decoration:none;color:inherit;border:1px solid #121c2e;border-radius:12px;
background:rgba(10,16,27,.5);padding:16px 18px;transition:border-color .2s,transform .2s}
.wcard:hover{border-color:var(--cy);transform:translateY(-2px)}
.wcard h3{margin:0 0 8px;font-size:13px;letter-spacing:.08em;color:#eaf2ff}
.wcard .wstat{font-size:11px;color:var(--dim);margin-top:4px}
.wcard .wcensus span{margin-right:10px}
body.light .wcard{background:rgba(255,255,255,.74);border-color:#dbe2ee}
body.light .wcard h3{color:#0b1424}
h2{font-size:11px;letter-spacing:.3em;text-transform:uppercase;color:var(--dim);margin:42px 0 10px;
border-bottom:1px solid #101826;padding-bottom:6px}
.meta{color:var(--dim);margin-top:8px}.chip{display:inline-block;border:1px solid #1a2436;border-radius:999px;
padding:2px 12px;margin-right:8px;background:rgba(13,20,32,.6)}
.ok{color:#3fb950}.debt{color:var(--em);text-shadow:0 0 10px rgba(255,122,89,.6)}
.mood{border:1px solid #1a2436;border-left:3px solid var(--em);background:rgba(16,24,38,.55);
padding:12px 16px;border-radius:0 8px 8px 0;letter-spacing:.02em}
.panel{display:block;width:100%;background:rgba(10,16,27,.5);border:1px solid #121c2e;border-radius:12px}
.node text{fill:var(--dim);font-size:10px;text-anchor:middle;letter-spacing:.06em}
.node .halo{opacity:.12}.node.warn .halo{opacity:.22}.node.hot .halo{opacity:.45}
.node.hot .core{fill:var(--em)}
.node.gs{filter:drop-shadow(0 0 6px rgba(242,214,117,.8))}
.node:hover .halo{opacity:.5}
/* Human 2026-09-21: "links are kind of invisible" — the old stroke (#22304a, width .7) was
   hardcoded for a dark surface and barely readable there either; light mode's own override
   (#c6cfe0) was even paler against a now-default light background. var(--dim) is already
   re-stepped per theme (see the :root / body.light block) for exactly this legibility job —
   reusing it fixes both themes at once instead of hand-tuning a third hex. */
line{stroke:var(--dim);stroke-width:1.3;opacity:.65}
/* 2026-09-21, Veldora: "add different color for each neural link" — the edge color now carries
   its lane. Word 15: the bonds are TYPED — TRUTH-CURRENT (slime⇒orc) cyan, VERDICT-CURRENT
   (orc⇒elf) gold, both SOLID; ANIMA-THREAD (body⇌worn mind) one DASHED shape tinted by the
   lane it serves (zone / verdict / global / shared). Shape law, both views: solid = race bond,
   dashed = mind bond. The .lit rule still wins when a node's attention field is up. */
line.sgedge{stroke:var(--r-slime);stroke-width:1.2;opacity:.55}
line.elfedge{stroke:var(--r-elf);stroke-width:1.3;opacity:.6}
line.mindedge{stroke:var(--r-mind);stroke-width:1.2;stroke-dasharray:2 4;opacity:.75}
line.mindedge.lan-smind,.arc.lan-smind{stroke:var(--l-smind)}
line.mindedge.lan-omind,.arc.lan-omind{stroke:var(--l-omind)}
line.mindedge.lan-gmind,.arc.lan-gmind{stroke:var(--l-gmind)}
g.node{cursor:pointer;transition:transform .25s ease,opacity .25s ease;transform-box:fill-box;transform-origin:center}
body.focused g.node{opacity:.16}body.focused g.node.focus{opacity:1;transform:scale(1.6)}
/* attention halo: the focused node's 1-hop neighborhood holds at half-light instead of dimming
   away — the path the signal actually travels stays visible, not just the queried node. */
body.focused g.node.near{opacity:.6}
body.focused line{opacity:.1}line.lit{opacity:1;stroke:var(--cy);stroke-width:1.4}
/* Word 15/16 (Veldora 2026-09-21): "a button to switch display — a vertical neural net between
   layers." Two views, ONE truth (same nodes, same edges, same rels): #laneView is the
   interleaved grid; #netView stands the stack on end — rows root→crown, bonds drawn as
   vertical bézier arcs, a pulse traveling each arc so truth is SEEN rising. The toggle never
   forks data: both SVGs render from the same structures, page-load once. Net view is default. */
#netView{display:none}
body.netview #laneView{display:none}
body.netview #netView{display:block}
@keyframes synapse{0%{stroke-dashoffset:24}100%{stroke-dashoffset:-24}}
.arc{stroke-dasharray:5 12;stroke-width:1.3;opacity:.6;fill:none}
body.netview .arc{animation:synapse 1.15s linear infinite}
/* Word 16: "relations clignote when selected" — the focused node's bonds never just stay lit,
   they BLINK, so the eye finds the web, not the dot. Applies in both views. */
@keyframes litblink{0%,100%{opacity:1}50%{opacity:.22}}
body.focused line.lit,body.focused path.arc.lit,body.focused path.bond.lit{opacity:1;animation:litblink .85s ease-in-out infinite}
body.focused .arc,body.focused .bond{opacity:.1}
.bond{fill:none;stroke-width:1.6;opacity:.8}
.bond.sgedge{stroke:var(--r-slime)}.bond.elfedge{stroke:var(--r-elf)}
.arc.sgedge{stroke:var(--r-slime)}.arc.elfedge{stroke:var(--r-elf)}.arc.mindedge{stroke:var(--r-mind)}
/* Word 18: every net layer gets its OWN background — body rows in their race family's tint,
   mind rows echoing their lane's ANIMA color, ascended LIGHT RED (the divines burn apart even
   in paint). The class keeps the stroke; fill travels per-row inline. */
.netlayer{stroke:#101826}body.light .netlayer{stroke:#dbe2ee}
@media (prefers-reduced-motion:reduce){body.netview .arc{animation:none}body.focused line.lit,body.focused path.arc.lit,body.focused path.bond.lit{animation:none;opacity:1}}
.castface{width:20px;height:20px;border-radius:50%;object-fit:cover;vertical-align:middle;margin-right:2px}
.focusface{width:56px;height:56px;border-radius:50%;object-fit:cover;float:left;margin:0 12px 6px 0;
border:1.6px solid var(--dim)}
#focusPanel{padding:12px 16px;min-height:64px;letter-spacing:.02em;display:flow-root}
#focusPanel b{color:#eaf2ff}#focusPanel .dim{color:var(--dim)}
.docbox{max-height:360px;overflow:auto;white-space:pre-wrap;word-break:break-word;
background:rgba(13,20,32,.5);border:1px solid #1a2436;border-radius:6px;padding:10px 14px;
font-size:12px;line-height:1.5;margin:0;letter-spacing:normal;cursor:auto}
body.light .docbox{background:rgba(255,255,255,.6);border-color:#cfdae9}
.modal-backdrop{position:fixed;inset:0;background:rgba(1,3,7,.72);z-index:50;
display:flex;align-items:center;justify-content:center;padding:28px}
.modal-box{position:relative;background:#0a0f18;border:1px solid #1a2436;border-radius:12px;
padding:22px 26px;max-width:860px;width:100%;max-height:82vh;overflow:auto;
box-shadow:0 20px 60px rgba(0,0,0,.5)}
.modal-close{position:absolute;top:10px;right:12px;background:transparent;border:none;
color:var(--dim);font-size:22px;line-height:1;cursor:pointer;padding:4px 8px}
.modal-close:hover{color:var(--ink)}
.modal-box .docbox{max-height:none}
body.light .modal-box{background:#fff;box-shadow:0 20px 60px rgba(20,30,50,.25)}
/* Word 14 (Veldora 2026-09-21): clicking a creature opens the VIEWER — a tabbed inspector,
   not a doc dump. Metrics first (relations, attention, desks), relations second, doc last.
   Right-docked so the graph + halo stay visible behind it — inspection is reading WITH the
   world in view, not instead of it. Replaces the centered modal; same open/close gestures. */
.viewer-backdrop{position:fixed;inset:0;z-index:50;display:none}
.viewer{position:absolute;top:58px;right:22px;width:430px;max-width:90vw;background:#0a0f18;
border:1px solid #1a2436;border-radius:12px;padding:16px 18px;max-height:84vh;overflow:auto;
box-shadow:0 24px 60px rgba(0,0,0,.55)}
body.light .viewer{background:#fff;border-color:#dbe2ee;box-shadow:0 20px 60px rgba(20,30,50,.22)}
.vtabs{display:flex;gap:6px;margin:10px 0 12px}
.vtabs button{background:transparent;border:1px solid #1a2436;color:var(--dim);font:inherit;
font-size:10px;letter-spacing:.16em;text-transform:uppercase;padding:5px 12px;border-radius:999px;cursor:pointer}
.vtabs button.active{color:var(--cy);border-color:var(--cy);box-shadow:0 0 10px rgba(89,214,255,.35)}
.vgrid{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-bottom:10px}
.vstat{border:1px solid #1a2436;border-radius:8px;padding:8px 10px;background:rgba(13,20,32,.4)}
.vstat b{font-size:16px;color:var(--ink)}.vstat .lbl{font-size:9px;letter-spacing:.16em;color:var(--dim);text-transform:uppercase}
body.light .vstat{background:rgba(255,255,255,.6);border-color:#cfdae9}
.vrel a{color:var(--cy);cursor:pointer;text-decoration:none;display:block;padding:3px 0}
.vrel a:hover{text-decoration:underline}
.vlane{font-size:9px;letter-spacing:.14em;text-transform:uppercase;color:var(--dim);margin:8px 0 2px}
.vsec{display:block;padding:4px 0;color:var(--cy);cursor:pointer;text-decoration:none}
.vsec:hover{text-decoration:underline}
tr[data-name]:hover{background:rgba(89,214,255,.06)}
.tbtns{float:right}.tbtns button{background:rgba(13,20,32,.6);border:1px solid #1a2436;color:var(--ink);
font:inherit;font-size:10px;letter-spacing:.18em;text-transform:uppercase;padding:6px 14px;border-radius:999px;
cursor:pointer;margin-left:8px}
.tbtns button.active{border-color:var(--em);color:var(--em);box-shadow:0 0 10px rgba(255,122,89,.35)}
.tbtns button.rng.active{border-color:var(--cy);color:var(--cy);box-shadow:0 0 10px rgba(89,214,255,.35)}
@keyframes blink{50%{opacity:.12}}
body.stressmode .node.hot{animation:blink 1.05s ease-in-out infinite}
body.stressmode .node.warn{animation:blink 1.9s ease-in-out infinite}
body.light{--bg:#edf1f7;--ink:#1c2634;--dim:#4b5568;--nodefill:#ffffff;
--cy:#1a7fa3;--vi:#5c3fc9;--em:#b8341f;--gd:#8a6600;
--r-slime:#1a7fa3;--r-orc:#5c3fc9;--r-elf:#8a6600;--r-darkelf:#b8341f;--r-highorc:#6b7400;--r-kijin:#a12e26;--r-plain:#4b5568;--r-mind:#546578;
--l-smind:#1a7fa3;--l-omind:#5c3fc9;--l-gmind:#8a6600;--l-shared:#546578}
/* Light steps are their own re-stepped values, not the dark hexes with the background
   flipped — the same ramps, validated separately against the light surface. */
body.light::before{background:radial-gradient(900px 480px at 78% -8%,rgba(30,140,200,.10),transparent 62%),
    radial-gradient(760px 420px at 6% 10%,rgba(120,90,220,.09),transparent 60%),
    radial-gradient(1100px 700px at 50% 118%,rgba(230,110,70,.08),transparent 62%)}
body.light::after{background:repeating-linear-gradient(0deg,transparent 0 3px,rgba(28,38,52,.045) 3px 4px);opacity:.5}
body.light .panel{background:rgba(255,255,255,.74);border-color:#dbe2ee}
body.light h1{color:#0b1424;text-shadow:none}
body.light .mood{background:rgba(255,255,255,.74);border-color:#dbe2ee}
body.light .chip{background:rgba(255,255,255,.74);border-color:#dbe2ee}
body.light td,body.light th{border-color:#e6ebf4}
body.light .tbtns button{background:rgba(255,255,255,.8);border-color:#cfdae9;color:#1c2634}
.rib{filter:drop-shadow(0 0 5px currentColor)}.ax{fill:var(--dim);font-size:10px;letter-spacing:.12em}
.mindedgelabel{font-size:9px;fill:var(--dim);stroke:var(--nodefill);stroke-width:3px;paint-order:stroke fill;font-variant-numeric:tabular-nums}
table{border-collapse:collapse;width:100%}td,th{padding:5px 12px;border-bottom:1px solid #0e1624;text-align:left;
font-size:12px}th{color:var(--dim);font-weight:400;letter-spacing:.18em;font-size:10px;text-transform:uppercase}
.num{text-align:right;font-variant-numeric:tabular-nums}.dim{color:var(--dim)}.gs{color:var(--gd)}
.pill{border:1px solid;border-radius:999px;padding:1px 10px;font-size:10px;letter-spacing:.14em}
.pill.cool{color:var(--cy);border-color:var(--cy);background:color-mix(in srgb,var(--cy) 16%,transparent)}
.pill.warn{color:var(--gd);border-color:var(--gd);background:color-mix(in srgb,var(--gd) 16%,transparent)}
.pill.hot{color:var(--em);border-color:var(--em);background:color-mix(in srgb,var(--em) 18%,transparent);text-shadow:0 0 8px rgba(255,122,89,.7)}
td.desc{color:var(--dim);font-size:11px}td.desc b{color:var(--ink)}
body.calm .halo{transition:opacity 1.2s;opacity:.04!important}
@keyframes fest{30%{filter:saturate(1.9) hue-rotate(30deg) brightness(1.3)}}
body.fest main{animation:fest 2.6s ease}
@media (prefers-reduced-motion:reduce){*{animation:none!important}}
</style>`;

function render(d) {
  // --- neural-layer geometry ---
  // The colony as a neural net: slimes = input layer, orcs = hidden layer, elf = output, the
  // ascended burn beyond. Radius ∝ √(KB / 6KB law): an over-diet mind GROWS INTO its neighbors
  // — collisions aren't layout bugs, they're the noise made visible (human doctrine 2026-09-14).
  const W = 1240; // H is no longer a constant — derived after the pack (below).
  const byName = {};
  for (const c of d.creatures) byName[c.name] = c;
  const resolve = n => byName[n] || byName[Object.keys(byName).find(k => n.endsWith(k) || k.endsWith(n)) || ''];
  const rad = c => Math.round(100 * 11 * Math.sqrt(Math.max(c.kb, 0.5) / 6)) / 100;
  // Veldora 2026-09-21 — the day's words on layout, each superseding the last, kept dated for
  // the record: (1) slime → orc → minds → elf; (2) slime → minds → orc → elf; (3) three service
  // lanes by service; (4) canonical interleaved order; (5) uniform grid; (6) two framed regions
  // — JUDGED OFF; (7) "keep old formal minds between races, use a global grid system, and the
  // parts are the NEXT ROW"; (8–12) stacked rows → strips → hats → adaptation loop → ascended
  // lane; (13) "we should switch places of slime and zone minds — same for orc/elf": minds LEFT
  // of the rank they serve. Tools before hands: zone minds | slime | verdict minds | orc |
  // global minds | elf | ascended — each rank preceded by the hats it wears.
  // Word 16b: "latest col colliding — use % css like bootstrap": percentage centers at
  // (i+.5)/7 of W, so the grid re-balances itself at ANY lane count or width change.
  const gx = i => Math.round((i + 0.5) / 7 * W);
  const X = { smind: gx(0), slime: gx(1), omind: gx(2), orc: gx(3), gmind: gx(4), elf: gx(5), asc: gx(6) };
  // One label rule for every node: centered, below, clearing the halo (r*1.7 + 8) — human
  // 2026-09-21: "always load text below cercle so no collision happens for every thing."
  const below = (x, y, r) => ({ anch: 'middle', lx: x, ly: y + r * 1.7 + 8 });
  const nodes = [], edges = [];
  // Server-side adjacency — the relation truth travels WITH the page (the viewer reads ONE
  // structure, never a DOM-scrape shadow of the graph). relCls = bond type per unordered pair,
  // which the net view tints its arcs by (word 15).
  const relOf = {};
  const relCls = {};
  const relAdd = (a, b, cls) => { relCls[a < b ? a + '|' + b : b + '|' + a] = cls;
    for (const [x, y] of [[a, b], [b, a]]) { const l = relOf[x] = relOf[x] || []; if (!l.includes(y)) l.push(y); } };
  // 2026-09-21 collision fix (Veldora: "the text are colliding with slimes"): the old uniform
  // spread gave every row an EQUAL pitch inside a fixed H=700 band, blind to radii. Columns
  // now pack top-down from actual radii (Δy ≥ (rA+rB)*1.7 + 26 ⇒ a label band always clears
  // the next halo), and the canvas GROWS in H to fit instead of squeezing creatures into each
  // other. The 2026-09-14 growth doctrine stands for node art; it never licensed label
  // collisions. layerBottom, sharedDivider and H itself are computed right after the pack.
  const pack = items => { let y = 74, prev = null;
    return items.map(it => { if (prev) y += Math.ceil((prev.r + it.r) * 1.7 + 26); prev = it; return { ...it, y }; }); };
  const slimeSeq = [];
  for (const o of d.orcs) for (const s of o.slimes) slimeSeq.push({ s, orc: o.orc });
  const slimeNodes = pack(slimeSeq.map(({ s, orc }) => {
    const c = resolve(s) || { kb: 6, stressPct: 0, race: 'slime', thoughts: 0, limit: 5, crosslinks: 0 };
    return { name: s, c, x: X.slime, r: rad(c), orc };
  })).map(n => ({ ...n, ...below(n.x, n.y, n.r) }));
  const orcNodes = pack(d.orcs.map(o => {
    const c = resolve(o.orc) || { kb: 6, stressPct: 0, race: 'orc', thoughts: 0, limit: 5, crosslinks: 0 };
    return { name: o.orc, c, x: X.orc, r: rad(c) };
  })).map(n => ({ ...n, ...below(n.x, n.y, n.r) }));
  const orcByName = {}; for (const n of orcNodes) orcByName[n.name] = n;
  // TRUTH-CURRENT (slime⇒orc): ground facts flow up. Solid, slime-cyan (word 15).
  for (const sn of slimeNodes) { const oc = orcByName[sn.orc];
    if (oc) { edges.push(`<line class="sgedge e-${esc(oc.name)} e-${esc(sn.name)}" x1="${sn.x.toFixed(1)}" y1="${sn.y.toFixed(1)}" x2="${oc.x.toFixed(1)}" y2="${oc.y.toFixed(1)}"/>`); relAdd(oc.name, sn.name, 'sgedge'); } }
  nodes.push(...slimeNodes, ...orcNodes);
  // Minds (skills) — THREE service lanes (zone / verdict / global) plus the shared row. Not
  // ranks — lanes of service; a mind still links to whichever creature nodes the harmony pass
  // found a textual reason to connect (see harvest()'s "Mind ↔ creature harmony links"), never
  // hand-assigned, and draws zero edges when nothing names it — honest reading, not bug.
  // Lane assignment is DATA, no table to maintain (isekai.md: "a mind's place is whom it
  // serves, derived from real wearers/links"): any linked wearer who is an elf or an ascended →
  // GLOBAL lane (the voice's and divines' tools); uniform slime wearers → ZONE; uniform orc
  // wearers → VERDICT; MIXED base-race wearers → SHARED row; zero wearers → race-prefix
  // fallback (slime-* zone, orc-* verdict, elf-/darkelf-/… global), default shared — "a mind
  // nobody wears is nobody's private tool."
  const laneOf = m => {
    const races = new Set();
    for (const ln of m.links) { const c = resolve(ln); if (c && c.race) races.add(c.race); }
    if ([...races].some(r => r === 'elf' || r === 'darkelf' || r === 'highorc' || r === 'kijin')) return 'gmind';
    const base = [...races].filter(r => r === 'slime' || r === 'orc');
    if (base.length === 1) return base[0] === 'slime' ? 'smind' : 'omind';
    if (base.length > 1) return 'shared';
    return /^slime-/.test(m.name) ? 'smind' : /^orc-/.test(m.name) ? 'omind'
      : /^(elf|darkelf|highorc|kijin)-/.test(m.name) ? 'gmind' : 'shared';
  };
  // Creature = BODY + MIND (word 10: "break current creature into body and mind"). Every
  // race-prefixed skill is BOTH: its body renders in its rank lane (above), and its worn hat
  // renders here as a dashed mind in its service lane — slime's hat in ZONE, orc's hat in
  // VERDICT, elf's + ascended divines' hats in GLOBAL. The hat carries exactly one harmony
  // link (its body): the ANIMA-THREAD wear-edge, the two halves of one creature drawn apart,
  // linked. This is also why the lanes were empty before: the know-how was counted only as
  // body, never drawn as hat.
  // Adaptation loop (word 11, isekai.md §Minds & Bodies): THE DESK LIVES IN THE HAT. The dated
  // ## Thoughts count, its ~5 limit and the stress it implies are drawn on the mind that was
  // worn when they were earned; the body keeps kb / diet / crosslinks / genesis — the durable
  // facts that must adapt in the same change once the hat reads stressed.
  const hatLane = race => race === 'slime' ? 'smind' : race === 'orc' ? 'omind' : 'gmind';
  const hatOf = c => { const lane = hatLane(c.race);
    const mc = { race: 'mind', kb: c.kb, descTok: Math.round((c.desc || '').length / 4), stressPct: c.stressPct, dietPct: 0,
      thoughts: c.thoughts, limit: c.limit, crosslinks: 1, genesisSignal: false, desc: c.desc || '', uses: 0, lastThought: c.lastThought || null };
    return { name: c.name + '@hat', base: c.name, label: c.name.replace(/^(slime|orc|elf|darkelf|highorc|kijin)-/, ''),
      c: mc, lane, r: Math.max(9, Math.min(18, rad(mc) * 0.55)), links: [c.name] }; };
  const hats = d.creatures.map(hatOf);
  const mindBody = (m, lane) => { const mc = { race: 'mind', kb: m.kb, descTok: m.descTok, stressPct: 0, dietPct: 0, thoughts: 0, limit: 1,
    crosslinks: m.links.length, genesisSignal: false, desc: m.desc, uses: m.uses, src: m.src };
    return { name: m.name, c: mc, lane, r: Math.max(9, Math.min(18, rad(mc) * 0.55 + Math.min(m.uses, 10) * 0.4)), links: m.links }; };
  // One pack per lane, hats + free minds together — two packs in one lane would re-collide.
  const laneNodes = lane => pack([...hats.filter(h => h.lane === lane),
    ...(d.minds || []).filter(m => laneOf(m) === lane).map(m => mindBody(m, lane))]
    .map(n => ({ ...n, x: X[lane] }))).map(n => ({ ...n, ...below(n.x, n.y, n.r) }));
  const smindNodes = laneNodes('smind');
  const omindNodes = laneNodes('omind');
  const gmindNodes = laneNodes('gmind');
  // SHARED skills are not a lane column — they own ROW 2, a full-width band of their own
  // (word 7/8: stacked under the lanes row, never side-by-side with it; cells collided with
  // row 1's lane x-slices). Two kinds ride it: Minds nobody wears (laneOf → 'shared') and the
  // host commands (.opencode/commands, .claude/commands — `<name>@cmd`, no harmony links: a
  // command is a procedure, not know-how a creature dons). Their harmony links, if any, still
  // draw up into row 1 — wearing crosses rows.
  const sharedList = (d.minds || []).filter(m => laneOf(m) === 'shared');
  const cmdNode = k => ({ name: k.name + '@cmd', label: '/' + k.name, kind: 'command', lane: 'shared', links: [],
    c: { race: 'mind', kb: k.kb, descTok: 0, stressPct: 0, dietPct: 0, thoughts: 0, limit: 1, crosslinks: 0, genesisSignal: false, desc: k.desc || '', uses: 0, srcs: k.srcs },
    r: Math.max(9, Math.min(14, rad(k) * 0.5)) });
  const sharedSeq = [...sharedList.map(m => mindBody(m, 'shared')), ...(d.commands || []).map(cmdNode)];
  const sharedX = i => 100 + (W - 200) * (i + 1) / (sharedSeq.length + 1);
  const sharedNodes = sharedSeq.map((n, i) => ({ ...n, x: sharedX(i) })); // y set after geometry is derived below
  const mindNodes = [...smindNodes, ...omindNodes, ...gmindNodes, ...sharedNodes];
  nodes.push(...mindNodes);
  // Elf(s): found generically by race, never by an assumed literal name — a world's Elf is not
  // always named "elf-colony" (that was one demo world's own name, not a schema). Packed like
  // orcs/slimes if a world ever has more than one (Nature 6 — an elf-colony forming from 2+
  // elves thinking alike is a real possibility, not the default).
  const elfNodes = pack(d.creatures.filter(c => c.race === 'elf').map(c => ({ name: c.name, c, x: X.elf, r: rad(c) })))
    .map(n => ({ ...n, ...below(n.x, n.y, n.r) }));
  nodes.push(...elfNodes);
  // VERDICT-CURRENT (orc⇒elf): rulings rise, wisdom descends. Solid, elf-gold (word 15).
  for (const en of elfNodes) for (const oc of orcNodes) {
    edges.push(`<line class="elfedge e-${esc(en.name)} e-${esc(oc.name)}" x1="${oc.x.toFixed(1)}" y1="${oc.y.toFixed(1)}" x2="${en.x.toFixed(1)}" y2="${en.y.toFixed(1)}"/>`); relAdd(en.name, oc.name, 'elfedge'); }
  // Ascended — a full lane now (word 13, X.asc above), not an under-chart shelf: highorc +
  // darkelf + kijin pack vertically in the seventh column like every other lane. Portrait,
  // halo, label-below — bodies' dress ("the ascended are bodies of bodies"). Computed BEFORE
  // the geometry so the ascended lane counts toward the deepest column. Drawn at zero
  // population too: the lane tint + caption keep the place visible and named.
  const ascended = d.creatures.filter(c => c.race === 'highorc' || c.race === 'darkelf' || c.race === 'kijin');
  const ascendedNodes = pack(ascended.map(c => ({ name: c.name, c, x: X.asc, r: rad(c) })))
    .map(n => ({ ...n, ...below(n.x, n.y, n.r) }));
  nodes.push(...ascendedNodes);
  // Derived geometry — once all seven packed columns exist: the lanes row closes below the
  // deepest column's label band; ONE remainder row (shared skills) hangs under it; H follows.
  // Fixed-H uniform spread is the collision bug this family of fixes replaced — never return.
  const deepest = Math.max(260, ...[slimeNodes, smindNodes, omindNodes, orcNodes, gmindNodes, elfNodes, ascendedNodes]
    .map(col => col.length ? col[col.length - 1].y + col[col.length - 1].r * 1.7 + 26 : 74));
  const layerBottom = Math.ceil(deepest), sharedDivider = layerBottom + 12,
    sharedShelfY = sharedDivider + 65, H = sharedDivider + 132;
  // Row 2 packs horizontally, so a crowded row (this world: 10 shared skills in 1240px) would
  // stack ten labels on one baseline — zig-zag alternate labels one line lower when the pitch
  // is tighter than a label is wide, and clip long names the way the net view already does.
  const sharedPitch = sharedNodes.length > 1 ? (W - 200) / (sharedNodes.length + 1) : W;
  sharedNodes.forEach((n, i) => { n.y = sharedShelfY; Object.assign(n, below(n.x, n.y, n.r));
    if (sharedPitch < 140) { if (i % 2) n.ly += 12; n.label = (n.label || n.name).slice(0, 16); } });
  // Mind-edge linking happens last, once every node type (including elf and the ascended lane)
  // actually exists in `nodes` — a Mind can link to a creature of *any* race (never
  // race-restricted, see above), so building this lookup before elf/ascended existed would
  // have silently dropped any edge pointing at one of them. ANIMA-THREAD: dashed, lane-tinted.
  const nodeByName = {}; for (const n of nodes) nodeByName[n.name] = n;
  for (const mn of mindNodes) for (const linkName of mn.links) {
    const t = nodeByName[linkName];
    if (!t) continue;
    const cls = 'mindedge lan-' + (mn.lane || 'shared');
    edges.push(`<line class="${cls} e-${esc(mn.name)} e-${esc(linkName)}" x1="${mn.x.toFixed(1)}" y1="${mn.y.toFixed(1)}" x2="${t.x.toFixed(1)}" y2="${t.y.toFixed(1)}"/>`);
    relAdd(mn.name, linkName, cls);
    // Human 2026-09-21: "if slime used skill it will link to it with a variable of how much
    // time it used." Honest limit, named rather than guessed past: Claude Code's own
    // transcripts don't attribute a Skill invocation to which specific creature/subagent
    // triggered it — only coarsely 'subagent' vs 'main session' (see harvestClaudeUsage's own
    // comment on this same ceiling). So this is the mind's total use count across the whole
    // world, drawn once per edge it has — not a true per-pair count, which the data can't
    // currently support. Skipped entirely at 0 uses rather than printing a hollow "0×".
    if (mn.c.uses) {
      const mx = (mn.x + t.x) / 2, my = (mn.y + t.y) / 2;
      edges.push(`<text class="mindedgelabel" x="${mx.toFixed(1)}" y="${my.toFixed(1)}" text-anchor="middle">${mn.c.uses}×</text>`);
    }
  }
  const zoneRects = [
    // One global grid, seven lanes (word 13 ordering). Bodies tint wide, mind lanes tint slim.
    { x: X.smind, race: 'mind', half: 72 }, { x: X.slime, race: 'slime', half: 95 },
    { x: X.omind, race: 'mind', half: 72 }, { x: X.orc, race: 'orc', half: 90 },
    { x: X.gmind, race: 'mind', half: 72 }, { x: X.elf, race: 'elf', half: 95 },
    { x: X.asc, race: 'darkelf', half: 82 }, // ascended tint = LIGHT RED in lanes too (word 18)
  ].map(({ x, race, half }) =>
    // lane tints stop 8px short of the row-2 divider (a fixed `layerBottom - 8` height read
    // off the photographs overshot it by 28px once the divider moved to layerBottom + 12).
    `<rect x="${(x - half).toFixed(1)}" y="48" width="${half * 2}" height="${(sharedDivider - 56).toFixed(1)}" rx="14" fill="${RCOL[race]}" fill-opacity="0.05"/>`
  ).join('') +
    // The one remainder row (shared skills) — strips per node, the column style rotated into
    // the row (word 9); a faint blanket keeps the place named + present when empty (shelf law).
    sharedNodes.map(n =>
      `<rect x="${(n.x - 62).toFixed(1)}" y="${(sharedDivider + 34).toFixed(1)}" width="124" height="${(H - 20 - (sharedDivider + 34)).toFixed(1)}" rx="12" fill="${RCOL.mind}" fill-opacity="0.05"/>`
    ).join('') +
    (sharedNodes.length ? '' : `<rect x="40" y="${sharedDivider}" width="${W - 80}" height="${(H - 20 - sharedDivider).toFixed(1)}" rx="14" fill="${RCOL.mind}" fill-opacity="0.05"/>`);
  const layerTags = `<text class="ax" x="${X.smind}" y="36" text-anchor="middle">ZONE MINDS</text>` +
    `<text class="ax" x="${X.slime}" y="36" text-anchor="middle">INPUT — SLIMES</text>` +
    `<text class="ax" x="${X.omind}" y="36" text-anchor="middle">VERDICT MINDS</text>` +
    `<text class="ax" x="${X.orc}" y="36" text-anchor="middle">ORCS — GATE</text>` +
    `<text class="ax" x="${X.gmind}" y="36" text-anchor="middle">GLOBAL MINDS</text>` +
    `<text class="ax" x="${X.elf}" y="36" text-anchor="middle">OUTPUT — ELF · ABOVE</text>` +
    `<text class="ax" x="${X.asc}" y="36" text-anchor="middle">⋄ ASCENDED</text>` +
    (ascended.length ? '' : `<text class="ax" x="${X.asc}" y="${Math.round(layerBottom / 2 + 24)}" text-anchor="middle" opacity="0.7">none born yet</text><text class="ax" x="${X.asc}" y="${Math.round(layerBottom / 2 + 38)}" text-anchor="middle" opacity="0.7">place reserved</text>`) +
    `<line x1="40" y1="${sharedDivider}" x2="${W - 40}" y2="${sharedDivider}" stroke="#2a3a52" stroke-width="1" stroke-dasharray="7 5"/>` +
    `<text class="ax" x="${W / 2}" y="${sharedDivider + 18}" text-anchor="middle">ROW 2 ⋄ SHARED SKILLS — OPENCODE COMMANDS &amp; APP SKILLS, NOT ISEKAI MINDS${sharedNodes.length ? '' : ' (none yet — place reserved)'}</text>`;
  const nodeSvg = layerTags + nodes.map(({ name, c, x, y, r, lx, ly, anch, label, base, lane, kind }) => {
    const col = RCOL[c.race] || RCOL.plain, au = aura(c);
    const tip = c.race === 'mind'
      ? (base ? `${esc(base)} — its worn MIND · the hat, not the head · desk ${c.thoughts}/${c.limit} (stress ${c.stressPct}%) · ${c.kb}KB (≈${c.descTok} tok resident in its wearer's context)`
        : kind === 'command' ? `/${esc(name.replace(/@cmd$/, ''))} — host command (${esc((c.srcs || []).join(', '))}) · ${c.kb}KB · shared skill, not an isekai mind${c.desc ? ' — ' + esc(c.desc) : ''}`
        : `${esc(name)} — Mind · ${LANE_NAME[lane] || 'shared'} lane · ${c.kb}KB (≈${c.descTok} tok resident) · ${c.uses} use${c.uses === 1 ? '' : 's'} · worn by ${c.crosslinks} creature${c.crosslinks === 1 ? '' : 's'}${c.desc ? ' — ' + esc(c.desc) : ''}`)
      : `${esc(name)} — ${esc(c.race)} · ${c.kb}KB · desk ${c.thoughts}/${c.limit} (on its hat) · links ${c.crosslinks ?? '–'}${c.genesisSignal ? ' · ⋄ genesis watch' : ''}`;
    const portrait = PORTRAIT_FILE[c.race];
    // A portrait fills the node's face at ~0.85r (leaving the ring's own stroke visible) in
    // place of the old flat color dot; minds take the lane-colored brain glyph (word 19);
    // anything else keeps the dot — honest, it's not a named likeness. Radius still ∝ √(KB)
    // either way, so a face grows into its neighbors exactly like the plain dot did.
    const pr = r * 0.85;
    const core = portrait
      ? `<clipPath id="clip-${esc(name)}"><circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${pr.toFixed(1)}"/></clipPath>
      <image href="portrait/${c.race}" x="${(x - pr).toFixed(1)}" y="${(y - pr).toFixed(1)}" width="${(pr * 2).toFixed(1)}" height="${(pr * 2).toFixed(1)}"
        preserveAspectRatio="xMidYMid slice" clip-path="url(#clip-${esc(name)})" onerror="this.remove()"/>`
      : (c.race === 'mind' ? brainIcon(x, y, r, LANECOL[lane] || LANECOL.shared)
        : `<circle class="core" cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${(r * 0.45).toFixed(1)}" fill="${col}"/>`);
    return `<g id="n-${esc(name)}" data-name="${esc(name)}" class="node ${au}${c.race === 'mind' ? ' mind' : ''}${c.genesisSignal ? ' gs' : ''}">
      <circle class="halo" cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${(r * 1.7).toFixed(1)}" fill="${col}"/>
      <circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${r.toFixed(1)}" fill="var(--nodefill)" stroke="${col}" stroke-width="1.6" stroke-dasharray="${c.race === 'mind' ? '3 2' : 'none'}"/>
      ${core}
      <text text-anchor="${anch}" x="${lx.toFixed(1)}" y="${ly.toFixed(1)}">${esc(label || name.replace(/^(slime|orc|elf|darkelf|highorc|kijin)-/, ''))}</text>
      <title>${tip}</title></g>`;
  }).join('');

  // ---------- WORD 15/16: THE NET VIEW — vertical neural net between layers ----------
  // Same truth, re-rooted geometry: lanes become ROWS stacked signal-root→crown (zone minds /
  // slimes at root, elf at crown, ascended above all); bonds become vertical bézier synapses
  // tinted per bond law (.arc + sgedge/elfedge/lan-*); each carries e- classes so focus lights
  // and BLINKS the selected creature's web in this view too (Veldora: "relations clignote
  // when selected"). Word 18: every row paints its own background — race family for bodies,
  // ANIMA tint for mind rows, light red for the ascended.
  const NETROWS = [
    { label: '⋄ ASCENDED — born in time', nodes: ascendedNodes, fill: 'rgba(214,64,42,0.09)' },
    { label: 'CROWN — ELF OUTPUT', nodes: elfNodes, fill: 'rgba(189,140,36,0.08)' },
    { label: 'GLOBAL MINDS', nodes: gmindNodes, fill: 'rgba(212,175,55,0.06)' },
    { label: 'ORCS — THE GATE', nodes: orcNodes, fill: 'rgba(124,92,214,0.08)' },
    { label: 'VERDICT MINDS', nodes: omindNodes, fill: 'rgba(154,134,232,0.06)' },
    { label: 'INPUT — SLIMES', nodes: slimeNodes, fill: 'rgba(38,149,189,0.08)' },
    { label: 'ZONE MINDS', nodes: smindNodes, fill: 'rgba(77,184,221,0.06)' },
    { label: '⋄ SHARED SKILLS — opencode commands & app skills, not isekai minds', nodes: sharedNodes, fill: 'rgba(159,176,195,0.06)' },
  ];
  const NETM = 96, NETR = 118, netTop = 44;
  const netPos = {};
  let netLayerSvg = '';
  NETROWS.forEach((row, i) => {
    const y = netTop + i * NETR, cy = y + NETR / 2;
    netLayerSvg += `<rect class="netlayer" x="40" y="${y}" width="${W - 80}" height="${NETR - 14}" rx="14" fill="${row.fill}"/>` +
      `<text class="ax" x="58" y="${y + 18}">${esc(row.label)}${row.nodes.length ? ' · ' + row.nodes.length : ' — place reserved'}</text>`;
    row.nodes.forEach((n, j) => { netPos[n.name] = { x: NETM + (W - 2 * NETM) * (j + 0.5) / row.nodes.length, y: cy, n }; });
  });
  const netH = netTop + NETROWS.length * NETR + 8;
  let netArcSvg = '';
  for (const key of Object.keys(relCls)) {
    const [a, b] = key.split('|'), A = netPos[a], B = netPos[b];
    if (!A || !B) continue;
    const cls = relCls[key];
    // Word 17–17b (Veldora 2026-09-21): "continue lines" meant CONTINUOUS stroke, not straight
    // — and the curved arcs won. Every bond is a bézier between layers; race bonds carry
    // class .bond (solid, unbroken current), mind bonds carry .arc (dashed, pulsing thread).
    const mx = (A.x + B.x) / 2;
    const pth = `M${A.x.toFixed(1)} ${A.y.toFixed(1)} C ${mx.toFixed(1)} ${A.y.toFixed(1)} ${mx.toFixed(1)} ${B.y.toFixed(1)} ${B.x.toFixed(1)} ${B.y.toFixed(1)}`;
    netArcSvg += (cls === 'sgedge' || cls === 'elfedge')
      ? `<path class="bond ${cls} e-${esc(a)} e-${esc(b)}" d="${pth}"/>`
      : `<path class="arc ${cls} e-${esc(a)} e-${esc(b)}" d="${pth}"/>`;
  }
  const netNodeSvg = Object.values(netPos).map(({ x, y, n }) => {
    const c = n.c, col = RCOL[c.race] || RCOL.plain;
    const pr = n.r * 0.8;
    // Word 19: portraits clip via a real clipPath per NET node (nclip- prefix — the style=
    // circle() trick is unreliable inside SVG); minds take the lane-colored brain glyph.
    const core = PORTRAIT_FILE[c.race]
      ? `<clipPath id="nclip-${esc(n.name)}"><circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${pr.toFixed(1)}"/></clipPath>
        <image href="portrait/${c.race}" x="${(x - pr).toFixed(1)}" y="${(y - pr).toFixed(1)}" width="${(pr * 2).toFixed(1)}" height="${(pr * 2).toFixed(1)}"
          preserveAspectRatio="xMidYMid slice" clip-path="url(#nclip-${esc(n.name)})" onerror="this.remove()"/>`
      : (c.race === 'mind' ? brainIcon(x, y, n.r, LANECOL[n.lane] || LANECOL.shared)
        : `<circle class="core" cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${(n.r * 0.45).toFixed(1)}" fill="${col}"/>`);
    return `<g id="nn-${esc(n.name)}" data-name="${esc(n.name)}" class="node ${aura(c)}${c.race === 'mind' ? ' mind' : ''}">
      <circle class="halo" cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${(n.r * 1.7).toFixed(1)}" fill="${col}"/>
      <circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="${n.r.toFixed(1)}" fill="var(--nodefill)" stroke="${col}" stroke-width="1.6" stroke-dasharray="${c.race === 'mind' ? '3 2' : 'none'}"/>
      ${core}
      <text text-anchor="middle" x="${x.toFixed(1)}" y="${(y + n.r * 1.7 + 8).toFixed(1)}" font-size="9">${esc((n.label || n.name).replace(/^(slime|orc|elf|darkelf|highorc|kijin)-/, '').slice(0, 16))}</text>
    </g>`;
  }).join('');
  const netSvg = `<svg viewBox="0 0 ${W} ${netH}" class="panel">${netLayerSvg}${netArcSvg}${netNodeSvg}</svg>`;

  // Word 19 continued — the triad with its actual mounts (bodies d.bodies carry .model from
  // .claude/agents / .opencode/agents frontmatter): each reasoning role is answered by naming
  // its mind AND its horse. Direction fixed: reads → verdicts → drafts. No body minted for a
  // role → '—', drawn honestly (Nature 9), never a placeholder model name.
  const brainMini = col => `<svg width="13" height="13" viewBox="-7 -7 14 14" style="vertical-align:-2px"><g transform="scale(0.86)"><path d="${BRAIN_D}" fill="${col}"/></g></svg>`;
  const triad = TRIAD.map(t => {
    const b = (d.bodies || []).find(b => t.re.test(b.name));
    const model = b ? b.model : null;
    const col = model ? ((MODEL_COL.find(m => m.re.test(model)) || {}).col || 'var(--ink)') : 'var(--dim)';
    return { mind: t.mind, role: t.role, body: b ? b.name : null, model, col };
  });
  const triadHtml = triad.map(t =>
    `<span style="color:${t.col}">${brainMini(t.col)} <b>${t.mind}</b> ${t.role}</span><span class="dim"> · ${t.model ? esc(t.body) + ' on ' + esc(t.model) : '—'}</span>`
  ).join(' <span class="dim">→</span> ');

  // --- breath timeline: unified scale, real axes (human 2026-09-15 —
  // "xaxis should be in bottom of chart not there": the dates used to be
  // crammed into the top caption; now a true bottom axis with ticks/grid) ---
  const dates = Object.keys(d.days).sort();
  const breath = (() => {
    if (!dates.length) return '<div class="dim panel" style="padding:14px 16px">no days on record yet.</div>';
    const keys = [['journal', '#59d6ff'], ['thoughts', '#a78bfa'], ['commits', '#ff7a59']];
    const gMax = Math.max(1, ...dates.flatMap(x => keys.map(([k]) => d.days[x][k] || 0)));
    const H2 = 250, ml = 52, mr = 20, mt = 26, mb = 44, pw = W - ml - mr, ph = H2 - mt - mb;
    const sx = i => ml + (dates.length === 1 ? pw / 2 : i * pw / (dates.length - 1));
    const sy = v => mt + ph * (1 - v / gMax);
    let s = '';
    for (let t = 0; t <= 4; t++) { const v = Math.round(gMax * t / 4), yy = sy(v).toFixed(1);
      s += `<line x1="${ml}" y1="${yy}" x2="${W - mr}" y2="${yy}" stroke="#22304a" stroke-width="0.6" opacity="${t ? 0.45 : 1}"/>`
        + `<text x="${ml - 8}" y="${+yy + 3}" class="ax" text-anchor="end">${v}</text>`; }
    const step = Math.ceil(dates.length / 12);
    dates.forEach((x, i) => { if (i % step && i !== dates.length - 1) return; const xx = sx(i).toFixed(1);
      s += `<line x1="${xx}" y1="${H2 - mb}" x2="${xx}" y2="${H2 - mb + 5}" stroke="#22304a"/>`
        + `<text x="${xx}" y="${H2 - mb + 17}" class="ax" text-anchor="middle">${esc(x.slice(5))}</text>`; });
    for (const [k, col] of keys) {
      const pts = dates.map((x, i) => `${sx(i).toFixed(1)},${sy(d.days[x][k] || 0).toFixed(1)}`);
      s += `<polyline points="${pts.join(' ')}" fill="none" stroke="${col}" stroke-width="1.8" class="rib"/>`; }
    s += `<text x="${ml}" y="14" class="ax">events / day — unified scale (max ${gMax})</text>`
      + `<text x="${W - mr}" y="14" class="ax" text-anchor="end">${keys.map(([k, col]) => `<tspan fill="${col}">■ ${k}</tspan>`).join(' · ')}</text>`
      + `<text x="${W - mr}" y="${H2 - 4}" class="ax" text-anchor="end">day →</text>`;
    return `<svg viewBox="0 0 ${W} ${H2}" class="panel">${s}</svg>`;
  })();

  // Cast and manifest used to be two separate tables (same creatures, split columns) —
  // merged into one at the human's request, with an explicit numeric stress score
  // (not just the color pill) alongside the description.
  const brief = s => { let b = (s || '').split(/ [—-] /)[0].trim();
    if (b.length > 118) b = b.slice(0, 115).replace(/\s\S*$/, '') + '…'; return b; };
  const castRows = d.creatures.map(c => `<tr data-name="${esc(c.name)}" style="cursor:pointer">
      <td>${PORTRAIT_FILE[c.race] ? `<img class="castface" src="portrait/${c.race}" alt="" onerror="this.remove()">` : `<span style="color:${RCOL[c.race] || RCOL.plain}">●</span>`} <b>${esc(c.name)}</b>${c.genesisSignal ? ' <span class="gs">⋄</span>' : ''}</td>
      <td class="dim">${esc(c.race)}</td>
      <td class="num">${c.kb}</td><td class="num">${c.thoughts}/${c.limit}</td><td class="num">${c.crosslinks}</td>
      <td><span class="pill ${aura(c)}">${c.stressPct}%</span></td>
      <td class="desc" title="${esc(c.desc || '')}">${c.desc ? esc(brief(c.desc)) : '–'}</td></tr>`).join('');
  const modelRows = Object.entries(d.models || {})
    .sort((a, b) => b[1].mentions - a[1].mentions)
    .map(([id, v]) => `<tr><td><b>${esc(id)}</b></td><td class="dim">${esc(v.mounted.join(' · ') || '–')}</td><td class="num">${v.mentions}</td></tr>`).join('');
  const mindRows = (d.minds || []).slice().sort((a, b) => b.uses - a.uses)
    .map(m => `<tr data-name="${esc(m.name)}" style="cursor:pointer"><td><b>${esc(m.name)}</b></td><td class="dim" style="color:${LANECOL[laneOf(m)]}">${LANE_NAME[laneOf(m)]}</td><td class="num">${m.uses}</td><td class="num">${m.kb}</td>
      <td class="num" title="estimate — the description alone, the part that sits in context every turn; ~4 bytes/token">≈${m.descTok}</td>
      <td class="dim">${m.links.length ? esc(m.links.join(' · ')) : '–'}</td>
      <td class="desc" title="${esc(m.desc || '')}">${m.desc ? esc(brief(m.desc)) : '–'}</td></tr>`).join('');

  const fmtN = v => v >= 1e6 ? (v / 1e6).toFixed(1) + 'M' : v >= 1e3 ? (v / 1e3).toFixed(1) + 'k' : String(v);
  const bRows = (d.bodies || []).map(b => `<tr><td><b>${esc(b.name)}</b></td><td class="dim">${esc(b.mode)}</td><td>${esc(b.model)}</td><td class="dim">${b.born}</td><td class="num">${b.kb}</td></tr>`).join('');
  const uRows = Object.entries(d.agentUse || {}).sort((a, b) => (b[1].totIn + b[1].totOut) - (a[1].totIn + a[1].totOut))
    .map(([a, u]) => `<tr><td><b>${esc(a)}</b></td><td class="dim">${esc(u.models.join(' · ') || '–')}</td><td class="num">${u.sessions}</td><td class="num">${fmtN(u.avgCtx)}</td><td class="num">${fmtN(u.totIn)} / ${fmtN(u.totOut)}</td><td class="dim">${u.lastDay || '–'}</td></tr>`).join('');

  return `<!doctype html><meta charset="utf-8"><title>tempest ⋄ ${esc(d.world)} — ${esc(path.basename(d.root))}</title>
${PAGE_STYLE}
<body class="light netview">
<main>
<a href="/" class="allworlds">← all worlds</a>
<h1><span class="sigil">⋄</span> TEMPEST <span style="letter-spacing:.1em;color:var(--dim);font-size:11px"> ${d.world.toUpperCase()} — THE WORLD, OBSERVED · /${d.world}/ · :${PORT}</span></h1>
<div class="meta"><span class="tbtns"><button id="themeBtn" title="light/dark">◑ dark</button><button id="stressBtn" title="the suffering blink, while you watch">⚡ stress detect</button>
<button class="rng" data-r="1" title="last 24h — today (day-bucket)">24h</button><button class="rng" data-r="7" title="last 7 days">7d</button><button class="rng active" data-r="30" title="last 30 days">30d</button><button class="rng" data-r="0" title="all time">all</button></span>${d.ctxStress ? `<span class="pill ${d.ctxStress.band}" title="most recent turn's context occupancy (estimate, Nature 9 instrument), ${d.ctxStress.pct}% of the ${fmtN(d.ctxStress.limit)} budget">⋄ context ${fmtN(d.ctxStress.tin)} tok</span>` : ''}<span class="chip">${esc(path.basename(d.root))}</span><span class="chip">canon v${d.canonV}</span>
<span class="chip">${d.chartV === null ? 'chart: none' : d.chartDebt ? `<span class="debt">chart v${d.chartV} × debt 22/22a</span>` : `<span class="ok">chart v${d.chartV} ✓</span>`}</span>
<span class="chip">${esc(d.when.replace('T', ' ').slice(0, 19))}</span>
<span class="chip">genesis watch: ${d.genesisWatch.length ? esc(d.genesisWatch.join(' · ')) : '<span class="ok">none</span>'}</span></div>

<h2>⋄ cast — who holds what</h2>
<table><tr><th>creature (body — its hat rides the graph as <i>@hat</i>)</th><th>race</th><th>KB</th><th>desk (on its hat)</th><th>links</th><th>stress</th><th>expertise (brief — full text on hover)</th></tr>${castRows || '<tr><td colspan="7" class="dim">no bodies yet — zero population is drawn as zero, not guessed (Nature 9); /genesis births from observed need</td></tr>'}</table>

<h2>⋄ minds — worn, not raced</h2>
<table><tr><th>mind</th><th>lane (whom it serves)</th><th>uses</th><th>KB</th><th>≈ ctx tok</th><th>worn by</th><th>purpose (brief — full text on hover)</th></tr>${mindRows || '<tr><td colspan="7" class="dim">no Minds in this world yet — /don brings one in from .opencode/skills/ or .claude/skills/</td></tr>'}</table>
<div class="dim">lane = derived from real wearers (elf/ascended → global · slimes → zone · orcs → verdict · mixed or nobody → shared row) — never hand-assigned. Shared row also carries the host commands (${(d.commands || []).length ? (d.commands || []).map(k => '/' + esc(k.name)).join(' · ') : 'none'}) — procedures, not know-how a creature dons.</div>
<div class="dim">uses = Skill tool_use invocations counted from this world's own Claude Code transcripts (~/.claude/projects/) — a Mind that exists but reads 0 has never actually been invoked here, only referenced. ≈ ctx tok = estimated tokens the <b>description alone</b> costs every turn it's installed (~4 bytes/token) — not the KB column, which is the full body, loaded only when actually donned.</div>

<h2>⋄ models — mounted &amp; mentioned</h2>
<table><tr><th>model</th><th>mounted on (colony genomes)</th><th>ledger sightings</th></tr>${modelRows || '<tr><td class="dim">no model pins found</td></tr>'}</table>
<div class="dim">mounts = <b>model:</b> pins in .opencode/agents/*.md; sightings = name-drops across the colony ledger + maps (traces, not wires — real call frequency lives in the gateway's telemetry).</div>

<h2>⋄ agents — minted bodies &amp; session breath</h2>
${(() => {
    return `<table><tr><th>minted body</th><th>mode</th><th>mount</th><th>born</th><th>KB</th></tr>${bRows || '<tr><td colspan="5" class="dim">minds-only world — no bodies minted (embodiment E2)</td></tr>'}</table>
<div class="dim" style="margin:10px 0 4px">per-session breath — avg context = mean tokens (in+out) per session answering as that agent; the living ledger: <b>${d.home}/metrics/agents-usage.md</b> (rewritten when reality moves)</div>
<table><tr><th>agent</th><th>mounts seen</th><th>sessions</th><th>avg ctx</th><th>in / out</th><th>last day</th></tr>${uRows || '<tr><td colspan="6" class="dim">no sessions recorded for this world yet — the table fills itself as sessions work</td></tr>'}</table>`;
  })()}

<h2>⋄ system mood</h2><div class="mood">${esc(d.health)}</div>

<h2>⋄ remedies</h2>
<div class="panel" style="padding:14px 16px">
  <span class="tbtns" style="float:none"><button id="holiBtn" title="write the dated relief worklist from live metrics AND launch sequential creature-hat relief runs">🎒 holidays</button><button id="partyBtn" title="celebrate + name the genesis-watch births (naming #1)">🎉 party</button></span>
  <span id="actOut" class="dim"></span>
  <div class="dim" style="margin-top:8px">the board prescribes, records, dispatches (law 2026-09-15): <b>🎒 holidays</b> writes the relief worklist into the day's scratch <b>and launches sequential relief</b> — one creature-hat <span style="font-family:inherit">opencode run</span> per stressed mind (distil the desk · diet split · ⋄ review the SPLIT — every creature snapshotted into the day's scratch <b>before</b> its run, frontmatter byte-restored if drifted <b>after</b>), progress streamed above, steps logged to <span style="font-family:inherit">${d.home}/metrics/relief.jsonl</span>; the tool itself never writes a mind.
  <b>🎉 party</b> logs a celebration to <span style="font-family:inherit">${d.home}/metrics/celebrations.jsonl</span> — and any ⋄ names it carries become naming #1 of the genesis law (a birth still needs its need named twice, by a mind that means it).</div>
</div>

<h2>⋄ tokens — session breath</h2>${(() => { const U = d.live.rows ? d.live : d.tokens;
    if (!U.rows) return `<div class="dim">no usage yet — the board reads opencode's session store (<span style="font-family:inherit">~/.local/share/opencode/opencode.db</span>) live, and the manual law stands beside it: a session may append one JSONL line to <b>${d.home}/metrics/tokens.jsonl</b> (<span style="font-family:inherit">{"ts","agent","model","in","out"}</span>). USD truth lives in the gateway's telemetry, not here.</div>`;
    const withSrc = d.live.rows ? 'opencode session store (read-only, colony-scoped by directory)' : 'manual ledger';
    return `<div class="cards">
  <div class="card"><b>${(U.totalIn / 1e6).toFixed(1)}M</b> tokens in</div>
  <div class="card"><b>${(U.totalOut / 1e6).toFixed(1)}M</b> tokens out</div>
  <div class="card"><b>${U.rows}</b> ${d.live.rows ? 'sessions counted' : 'ledger rows'}</div></div>
<div class="dim" style="margin:10px 0 4px">per model — who the breath cost</div>
<table id="tokModelTable"><tr><th>model</th><th>sessions (agents)</th><th>runs</th><th>in</th><th>out</th><th>share</th></tr>
${Object.entries(U.perModel).sort((a, b) => (b[1].in + b[1].out) - (a[1].in + a[1].out)).map(([mid, v]) => { const all = (U.totalIn + U.totalOut) || 1; return `<tr><td>${esc(mid)}</td><td class="dim">${esc(v.agents.join(', ') || '–')}</td><td class="num">${v.rows}</td><td class="num">${v.in}</td><td class="num">${v.out}</td><td class="num">${(100 * (v.in + v.out) / all).toFixed(1)}%</td></tr>`; }).join('')}</table>
<div class="dim" style="margin:10px 0 4px">per day</div>
<table id="tokDayTable"><tr><th>day</th><th>in</th><th>out</th></tr>
${Object.entries(U.perDay).sort().map(([k, v]) => `<tr><td>${esc(k)}</td><td class="num">${v.in}</td><td class="num">${v.out}</td></tr>`).join('')}</table>
<div class="dim" style="margin:14px 0 4px">consumption — pick a model · solid in · dashed out · range obeys the toolbar (24h = today, day-bucket)</div>
<div id="tokSel" style="margin-bottom:6px"></div>
<div id="tokChart"></div>
<script type="application/json" id="tokData">${esc(JSON.stringify(d.live.series || {}))}</script>
<script type="application/json" id="tokAgents">${esc(JSON.stringify(Object.fromEntries(Object.entries(d.live.perModel).map(([k, v]) => [k, v.agents]))))}</script>
<div class="dim" style="margin-top:6px">source: ${withSrc}${d.live.rows && d.tokens.rows ? ` + manual ledger (${d.tokens.rows} rows beside it)` : ''} — USD still lives in gateway telemetry.</div>`; })()}

<h2>⋄ the colony — size is weight, glow is stress <span class="tbtns" style="float:none;margin-left:10px"><button id="netBtn" class="active" title="switch diagram: vertical neural net (default) ⇄ lanes grid — same truth, both views">▤ lane view</button></span></h2>
<div id="laneView"><svg viewBox="0 0 ${W} ${H}" class="panel">${zoneRects}${edges.join('')}${nodeSvg}</svg></div>
<div id="netView">${netSvg}</div>
<div class="dim" style="margin:8px 2px 0">
how to read it — one grid, two rows, seven lanes: <b>ROW 1</b> — <b>zone minds</b> · <b>input slimes</b> (ground truth) · <b>verdict minds</b> · <b>orcs</b> (the gate) · <b>global minds</b> · <b>output elf</b> (the voice) · <b>ascended</b>; each rank preceded by the minds that serve it — tools before hands. <b>ROW 2</b> — shared skills (opencode commands &amp; app skills — not isekai minds), full width.
Two planes read by dress: bodies carry portraits, halos, wide tints; minds are dashed rings on slim tints — worn, never raced. A race-prefixed creature is drawn twice on purpose: its <b>body</b> in its rank lane, its worn <b>hat</b> (<i>@hat</i>) in the lane its race is served by — the desk of dated thoughts lives on the hat.
<br><b>The bonds</b> (each relation has its own color):
<span style="color:var(--r-slime)">— TRUTH-CURRENT</span> slime⇒orc — ground facts flow up ·
<span style="color:var(--r-elf)">— VERDICT-CURRENT</span> orc⇒elf — rulings rise, wisdom descends ·
<span style="color:var(--l-smind)">╌ ANIMA-THREAD · zone</span>, <span style="color:var(--l-omind)">╌ verdict</span>, <span style="color:var(--l-gmind)">╌ global</span>, <span style="color:var(--l-shared)">╌ shared</span> — body⇌its worn mind: one dashed shape, tinted by the lane it serves. A thread with no creature at either end would be a lie; none exists.
<br><b>The brains</b> — colored by their lane's reasoning: <span style="color:var(--l-smind)">● zone</span> gathers facts · <span style="color:var(--l-omind)">● verdict</span> weighs rulings · <span style="color:var(--l-gmind)">● global</span> arms the voice · <span style="color:var(--l-shared)">● shared</span> app/toolbox.
<br><b>The triad</b> — one reasoning chain, three minds, three mounts: ${triadHtml}. End to end: reads → verdicts → drafts — a draft that skipped the read's eye would be writing blind. Mounts resolve from minted bodies (${(d.bodies || []).length ? (d.bodies || []).length + ' minted' : 'none minted here — all three ride nothing yet'}).
<br><b>Attention:</b> click any creature — it steps forward, its edges light and blink, its 1-hop neighborhood holds at half-light, and the inspector opens: metrics first, relations with jump links, doc last (lazy by section — the wire carries a map, never a dump). Click again, or the void, to release.
</div>
<div id="focusPanel" class="panel" style="margin-top:10px"></div>
<div id="docModal" class="viewer-backdrop" style="display:none">
  <div id="docModalBox" class="viewer">
    <button id="docModalClose" class="modal-close" aria-label="close" title="close (Esc)">×</button>
    <div id="docModalHead" style="margin-bottom:4px"></div>
    <div id="docModalTabs" class="vtabs"><button data-t="metrics" class="active">metrics</button><button data-t="relations">relations</button><button data-t="doc">doc</button></div>
    <div id="docModalBody"></div>
  </div>
</div>
<script type="application/json" id="zdata">${esc(JSON.stringify({
    creatures: Object.fromEntries(d.creatures.map(c => [c.name, { race: c.race, kb: c.kb, dietPct: c.dietPct, thoughts: c.thoughts, limit: c.limit, stressPct: c.stressPct, links: c.crosslinks, g: !!c.genesisSignal, last: c.lastThought || null, desc: c.desc || '', src: c.src || '' }])),
    minds: Object.fromEntries((d.minds || []).map(m => [m.name, { race: 'mind', kb: m.kb, descTok: m.descTok, uses: m.uses, links: m.links.length, wearers: m.links, lane: laneOf(m), desc: m.desc || '', src: m.src || '' }])),
    hats: Object.fromEntries(hats.map(h => [h.name, { race: 'mind', hatOf: h.base, lane: h.lane, kb: h.c.kb, descTok: h.c.descTok, thoughts: h.c.thoughts, limit: h.c.limit, stressPct: h.c.stressPct, last: h.c.lastThought, uses: 0, links: 1, desc: h.c.desc || '' }])),
    commands: Object.fromEntries((d.commands || []).map(k => [k.name + '@cmd', { race: 'mind', kind: 'command', lane: 'shared', kb: k.kb, srcs: k.srcs, desc: k.desc || '' }])),
    orcOf: Object.fromEntries(d.orcs.flatMap(o => o.slimes.map(s => { const c = (byName[s] || byName[Object.keys(byName).find(k => s.endsWith(k) || k.endsWith(s))] || ''); return c ? [c.name, o.orc] : null; }).filter(Boolean))),
    rels: relOf, relCls,
    triad: triad.map(t => ({ mind: t.mind, role: t.role, body: t.body, model: t.model })),
  }))}</script>
<script>
(function(){
var Z=JSON.parse(document.getElementById('zdata').textContent);
var BRAIN_D_CLIENT='${BRAIN_D}';
var LANE_NAME={smind:'zone',omind:'verdict',gmind:'global',shared:'shared'};
var LANE_VAR={smind:'var(--l-smind)',omind:'var(--l-omind)',gmind:'var(--l-gmind)',shared:'var(--l-shared)'};
var P=document.getElementById('focusPanel');
P.addEventListener('click',function(ev){ev.stopPropagation();}); // reading/selecting the loaded doc must not self-clear
function esc2(s){return String(s==null?'':s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
// The viewer (word 14, 2026-09-21): right-docked tabbed inspector — metrics / relations / doc —
// opens on the same click that focuses a node or cast row; closes on ×, backdrop click, or Escape.
// Lineage: the centered doc modal (human 2026-09-21: "load like on screen ... like a popup").
var DM=document.getElementById('docModal'), DMbox=document.getElementById('docModalBox'),
    DMhead=document.getElementById('docModalHead'), DMbody=document.getElementById('docModalBody'),
    DMclose=document.getElementById('docModalClose');
DMbox.addEventListener('click',function(ev){ev.stopPropagation();});
function closeDocModal(){DM.style.display='none';window._viewer=null;}
DMclose.addEventListener('click',function(ev){ev.stopPropagation();closeDocModal();});
DM.addEventListener('click',closeDocModal);
document.addEventListener('keydown',function(ev){if(ev.key==='Escape')closeDocModal();});
function clearAll(){document.body.classList.remove('focused');closeDocModal();
  document.querySelectorAll('.node.focus,.node.near,.lit').forEach(function(e){e.classList.remove('focus','near','lit');});
  P.innerHTML='<span class="dim">click a creature — node or cast row — and it steps forward; click again (or the void) to release. Nothing moves on its own: calm is ambient, attention is yours.</span>';}
var DOCCACHE={}; // name -> {map} | {error}; sections cached inside as .secData[n]
// Lazy by anchor (Veldora 2026-09-21): the doc tab fetches only the SECTION MAP (@S:'MAP',
// @F titles+bytes), renders it as an index, and a section's body loads when — and only when —
// its anchor is clicked. Whole-file dumps stay forbidden on the wire; the map is the surface.
function loadDoc(name,mount){
 if(DOCCACHE[name]){paintDoc(DOCCACHE[name],mount,name);return;}
 mount.innerHTML='<span class="dim">loading section map…</span>';
 fetch('doc?name='+encodeURIComponent(name)).then(function(r){return r.json();}).then(function(j){
  DOCCACHE[name]=j;paintDoc(j,mount,name);
 }).catch(function(){paintDoc({error:'failed to load — is the board alive?'},mount,name);});}
function paintDoc(j,mount,name){
 if(j['@S']!=='MAP'){mount.innerHTML='<span class="dim">'+esc2(j.error||j['@?']||'no doc file for this creature.')+'</span>';return;}
 var html='<div class="dim" style="margin-bottom:4px">'+esc2(j['@P']||'')+' · '+j['@E']+'B · '+(j['@F']?j['@F'].length:0)+' sections — click a section to load its body</div>';
 if(j.preamble&&j.preamble.trim())html+='<pre class="docbox" style="max-height:120px">'+esc2(j.preamble.trim())+'</pre>';
 html+=(j['@F']||[]).map(function(s){
  return '<a class="vsec" data-sec="'+s.n+'">▸ '+esc2(s.t)+' <span class="dim">· '+s.b+'B</span></a><div class="vsecbody" id="vs-'+esc2(name)+'-'+s.n+'" style="display:none"></div>';}).join('');
 mount.innerHTML=html;
 mount.querySelectorAll('a.vsec').forEach(function(a){
  a.addEventListener('click',function(ev){ev.stopPropagation();
   var n=a.getAttribute('data-sec'),box=document.getElementById('vs-'+name+'-'+n);
   if(!box)return;
   if(box.style.display!=='none'){box.style.display='none';a.firstChild.textContent='▸ ';return;}
   var cached=DOCCACHE[name].secData&&DOCCACHE[name].secData[n];
   if(cached){box.innerHTML='<pre class="docbox">'+esc2(cached.text)+'</pre>';box.style.display='block';a.firstChild.textContent='▾ ';return;}
   box.innerHTML='<span class="dim">loading §'+n+'…</span>';box.style.display='block';
   fetch('doc?name='+encodeURIComponent(name)+'&sec='+n).then(function(r){return r.json();}).then(function(s){
    (DOCCACHE[name].secData=DOCCACHE[name].secData||{})[n]=s;
    if(s['@S']==='SEC'){box.innerHTML='<pre class="docbox">'+esc2(s.text)+'</pre><div class="dim">§'+esc2(s['@T']||'')+' · '+s['@E']+'B</div>';a.firstChild.textContent='▾ ';}
    else box.innerHTML='<span class="dim">'+esc2(s.error||s['@?']||'section missing')+'</span>';
   }).catch(function(){box.innerHTML='<span class="dim">failed to load section</span>';});});});}
var PORTRAIT_RACES={slime:1,orc:1,elf:1,darkelf:1,highorc:1,kijin:1};
function kindOf(name){return name.slice(-4)==='@hat'?'hat':name.slice(-4)==='@cmd'?'command':Z.creatures[name]?'creature':Z.minds[name]?'mind':null;}
function lookup(name){var k=kindOf(name);return k==='hat'?Z.hats[name]:k==='command'?Z.commands[name]:k==='creature'?Z.creatures[name]:k==='mind'?Z.minds[name]:null;}
function neighborsOf(name){
 // Adjacency arrives WITH the page (server-side relOf), not scraped from the DOM at click
 // time — the DOM-scrape version answered "no relations" for orc/elf because sgedges point
 // creature→creature with e- classes, but the hat-pass edges carry @hat names; reading one
 // true structure (Z.rels) ends the shadow-structure bug for good.
 return ((Z.rels&&Z.rels[name])||[]).slice();}
function brainSvg(col){return '<svg width="22" height="22" viewBox="-7 -7 14 14" style="vertical-align:middle;margin-right:8px"><g transform="scale(1.4)"><path d="'+BRAIN_D_CLIENT+'" fill="'+col+'"/></g></svg>';}
function openDocModal(name,c){
 var k=kindOf(name),isHat=k==='hat',isCmd=k==='command';
 var shown=isHat||isCmd?name.slice(0,-4):name;
 DMhead.innerHTML=(c.race==='mind'
  ?'<span style="display:inline-block;vertical-align:middle">'+brainSvg(LANE_VAR[c.lane]||LANE_VAR.shared)+'</span>'
  :(PORTRAIT_RACES[c.race]?'<img class="focusface" src="portrait/'+c.race+'" alt="" onerror="this.remove()">':''))
  +'<b>'+esc2(isCmd?'/'+shown:shown)+'</b> <span class="dim">'+(isHat?'worn mind — the hat speaks for itself':isCmd?'host command — shared skill, not an isekai mind':esc2(c.race))+'</span>';
 DM.style.display='block';
 window._viewer={name:name,c:c,tab:'metrics'};
 renderViewerTab('metrics');}
function renderViewerTab(tab){
 var v=window._viewer;if(!v)return;v.tab=tab;
 document.querySelectorAll('#docModalTabs button').forEach(function(b){b.classList.toggle('active',b.getAttribute('data-t')===tab);});
 var c=v.c,name=v.name,neis=neighborsOf(name),k=kindOf(name),html='';
 if(tab==='metrics'){
  var laneOfNode=c.race==='slime'?'zone':c.race==='orc'?'verdict':c.race==='elf'?'global':c.race==='mind'?(LANE_NAME[c.lane]||'shared'):'ascended';
  var hat=k==='creature'?Z.hats[name+'@hat']:null;
  html='<div class="vgrid">'
   +'<div class="vstat"><b>'+(c.kb!=null?c.kb:'–')+'</b><div class="lbl">KB docweight</div></div>'
   +(c.dietPct!=null?'<div class="vstat"><b>'+c.dietPct+'%</b><div class="lbl">diet · of 6KB law</div></div>':'')
   +(c.stressPct!=null?'<div class="vstat"><b>'+c.stressPct+'%</b><div class="lbl">stress aura</div></div>':'')
   +(c.thoughts!=null?'<div class="vstat"><b>'+c.thoughts+'/'+c.limit+'</b><div class="lbl">desk thoughts'+(k==='creature'?' (on its hat)':'')+'</div></div>':'')
   +(c.uses!=null?'<div class="vstat"><b>'+c.uses+'</b><div class="lbl">real uses counted</div></div>':'')
   +(c.descTok!=null?'<div class="vstat"><b>≈'+c.descTok+'</b><div class="lbl">tok resident (desc)</div></div>':'')
   +'<div class="vstat"><b>'+neis.length+'</b><div class="lbl">relations · 1-hop</div></div>'
   +'<div class="vstat"><b>'+(c.links!=null?c.links:'–')+'</b><div class="lbl">doc links</div></div>'
   +'</div>'
   +'<div class="vlane">state</div><div class="dim">'
   +(c.race==='mind'
     ?(k==='hat'?'the worn mind of <b>'+esc2(c.hatOf)+'</b> · lane: '+laneOfNode+' · desk '+c.thoughts+'/'+c.limit+(c.thoughts>c.limit?' — <span class="pill hot">STRESSED</span> an instrument reading: distill (rule / trait / ascension / wrap-up) AND adapt the body\\'s doc in the same change; the same stress twice = adaptation failed → escalate the form':c.thoughts>=c.limit?' — at the limit: next thought is stress':'')
      :k==='command'?'host command · sources: '+esc2((c.srcs||[]).join(', '))+' · row 2, shared — a procedure, not know-how a creature dons'
      :'worn by '+c.links+' creature'+(c.links===1?'':'s')+(c.wearers&&c.wearers.length?' ('+esc2(c.wearers.join(', '))+')':' — nobody\\'s private tool')+' · lane: '+laneOfNode+(c.src?' · '+esc2(c.src):''))
     :'rank '+esc2(c.race)+(Z.orcOf[name]?' (under '+esc2(Z.orcOf[name])+')':'')+' · lane: '+laneOfNode+(hat?' · its worn mind is drawn @hat — desk '+hat.thoughts+'/'+hat.limit+(hat.thoughts>hat.limit?' <span class="pill hot">STRESSED</span> → this body\\'s doc must adapt in the same change':''):' · no hat drawn (unprefixed)'))
   +(c.g?' · <span class="gs">⋄ genesis watch</span>':'')+'</div>'
   +(c.last?'<div class="vlane">last thought</div><div class="dim">'+esc2(c.last)+'</div>':'')
   +(c.desc?'<div class="vlane">purpose</div><div class="dim">'+esc2(c.desc)+'</div>':'');
 }else if(tab==='relations'){
  if(!neis.length)html='<span class="dim">no relations drawn — an isolated node is itself a finding (Nature 9).</span>';
  else html='<div class="dim" style="margin-bottom:6px">'+neis.length+' direct relation'+(neis.length===1?'':'s')+'. Click to jump.</div><div class="vrel">'
   +neis.map(function(n){var key=name<n?name+'|'+n:n+'|'+name,cls=(Z.relCls&&Z.relCls[key])||'';
    var kind=cls==='sgedge'?'TRUTH-CURRENT':cls==='elfedge'?'VERDICT-CURRENT':cls.indexOf('mindedge')===0?'ANIMA-THREAD · '+(LANE_NAME[cls.replace('mindedge lan-','')]||'shared'):'';
    return '<a data-jump="'+esc2(n)+'">'+esc2(n)+(n.slice(-4)==='@hat'?' <span class="dim">· worn mind</span>':'')+(kind?' <span class="dim">· '+kind+'</span>':'')+'</a>';}).join('')+'</div>';
 }else{
  // hats have no doc of their own — the mind's SKILL.md IS the body's doc desk; open the base
  loadDoc(k==='hat'?name.replace(/@hat$/,''):name,DMbody);return;}
 DMbody.innerHTML=html;
 DMbody.querySelectorAll('a[data-jump]').forEach(function(a){a.addEventListener('click',function(ev){ev.stopPropagation();focus(a.getAttribute('data-jump'));});});}
document.getElementById('docModalTabs').addEventListener('click',function(ev){var t=ev.target.getAttribute&&ev.target.getAttribute('data-t');if(t){ev.stopPropagation();renderViewerTab(t);}});
function focus(name){
  var c=lookup(name),k=kindOf(name),isMind=c&&c.race==='mind';
  if(!c)return;clearAll();
  // word 19+: a clicked HAT stays lit as itself — hats are first-class nodes now (Z.hats).
  // Its anima-thread edge carries e-<hat>, so the wear-link lights; the halo covers both views.
  ['n-','nn-'].forEach(function(p){var g=document.getElementById(p+name);
   if(g){document.body.classList.add('focused');g.classList.add('focus');}});
  // cross-view: lanes-view g#n-<name> AND net-view g#nn-<name> both carry data-name; edge
  // .lit travels via the shared e- classes, which is why arcs blink in the net view too.
  document.querySelectorAll('.e-'+CSS.escape(name)).forEach(function(e){e.classList.add('lit');});
  // Attention halo (Veldora 2026-09-21 — "is there an attention mechanism we can add"):
  // every edge carries its endpoints as e-<name> classes, so the focused creature's 1-hop
  // neighborhood is *computable from the graph itself*, not hand-listed. Lit edges give the
  // signal path style; their other endpoints hold at half-light via the CSS .near rule — the
  // shape of "what does this node touch" stays visible while the rest of the field dims.
  document.querySelectorAll('.e-'+CSS.escape(name)).forEach(function(e){
   (e.getAttribute('class')||'').split(/\\s+/).forEach(function(kk){
    if(kk.indexOf('e-')!==0)return;var other=kk.slice(2);if(other===name)return;
    ['n-','nn-'].forEach(function(p){var og=document.getElementById(p+other);
     if(og&&!og.classList.contains('focus'))og.classList.add('near');});});});
  if(isMind){
   var shown=(k==='hat'||k==='command')?name.slice(0,-4):name;
   P.innerHTML='<b>'+esc2(k==='command'?'/'+shown:shown)+'</b> <span class="dim">'+(k==='hat'?'worn mind of '+esc2(c.hatOf)+' · desk '+c.thoughts+'/'+c.limit+' (stress '+c.stressPct+'%)':k==='command'?'host command · '+esc2((c.srcs||[]).join(', '))+' · shared row':'mind · '+(LANE_NAME[c.lane]||'shared')+' lane · worn by '+c.links+' creature'+(c.links===1?'':'s'))+'</span><br>'
    +(c.desc?'<span class="focusdesc">'+esc2(c.desc)+'</span><br>':'')
    +c.kb+'KB'+(c.descTok!=null?' <span class="dim">(≈'+c.descTok+' tok resident — description only, always in context)</span>':'')+(c.uses!=null?' · '+c.uses+' use'+(c.uses===1?'':'s'):'');
  }else{
   P.innerHTML=(PORTRAIT_RACES[c.race]?'<img class="focusface" src="portrait/'+c.race+'" alt="" onerror="this.remove()">':'')
    +'<b>'+esc2(name)+'</b> <span class="dim">'+esc2(c.race)+(Z.orcOf[name]?' under '+esc2(Z.orcOf[name]):'')+'</span><br>'
    +(c.desc?'<span class="focusdesc">'+esc2(c.desc)+'</span><br>':'')
    +'doc '+c.kb+'KB <span class="dim">(diet '+c.dietPct+'% of the 6KB law)</span> · desk '+c.thoughts+'/'+c.limit
    +' <span class="dim">(stress '+c.stressPct+'% — read on its hat)</span> · crosslinks '+c.links
    +(c.g?' · <span class="gs">⋄ genesis watch</span>':'')
    +'<br><span class="dim">last thought written: '+esc2(c.last||'none yet')+'</span>';
  }
  openDocModal(name,c);}
document.querySelectorAll('g.node,tr[data-name]').forEach(function(el){
  el.addEventListener('click',function(ev){ev.stopPropagation();var n=el.getAttribute('data-name');
   var g=document.getElementById('n-'+n);
   if(g&&g.classList.contains('focus'))clearAll();else focus(n);});});
document.body.addEventListener('click',clearAll);
var tb=document.getElementById('themeBtn');
if(tb)tb.addEventListener('click',function(ev){ev.stopPropagation();
 var l=document.body.classList.toggle('light');tb.textContent=l?'◑ dark':'◐ light';});
var nb=document.getElementById('netBtn');
if(nb)nb.addEventListener('click',function(ev){ev.stopPropagation();
 var on=document.body.classList.toggle('netview');nb.textContent=on?'▤ lane view':'⧉ net view';nb.classList.toggle('active',on);});
var sb=document.getElementById('stressBtn');
if(sb)sb.addEventListener('click',function(ev){ev.stopPropagation();
 var s=document.body.classList.toggle('stressmode');sb.classList.toggle('active',s);
 sb.textContent=s?'⚡ stress live':'⚡ stress detect';});
function act(url){var o=document.getElementById('actOut');o.textContent='…';
 fetch(url,{method:'POST'}).then(function(r){return r.json();}).then(function(j){
  if(url==='holidays'){
   if(!j.stressed){o.textContent='all minds within budget — nothing to relieve.';return;}
   o.textContent='worklist → '+j.file+' ('+j.stressed+' stressed)';
   if(j.relief&&j.relief.refused)o.textContent+=' — run already in flight: ';
   if(j.relief&&(j.relief.launched||j.relief.refused))pollRelief(o);
  }else{
   o.textContent='logged — births named: '+(j.births&&j.births.length?j.births.join(', '):'none tonight');
  }
 }).catch(function(){o.textContent='failed — is the board alive?';});}
function pollRelief(o){fetch('relief').then(function(r){return r.json();}).then(function(j){
 if(!j.total&&!j.active)return;
 if(j.active){
  o.textContent='relieving '+j.current+' ('+(j.done.length+j.failed.length+1)+'/'+j.total+') · done '+j.done.length+' · failed '+j.failed.length+' · next: '+(j.remaining.slice(0,3).join(', ')||'–');
  setTimeout(function(){pollRelief(o);},5000);
 }else{
  o.textContent='relief finished — relieved '+j.done.length+' ('+j.done.join(', ')+')'+(j.failed.length?', failed '+j.failed.length+' ('+j.failed.join(', ')+')':'')+' — re-reading minds…';
  setTimeout(function(){location.reload();},3000);}}).catch(function(){});}
var hb=document.getElementById('holiBtn');
if(hb)hb.addEventListener('click',function(ev){ev.stopPropagation();act('holidays');
 document.body.classList.add('calm');setTimeout(function(){document.body.classList.remove('calm');},3000);});
var pb=document.getElementById('partyBtn');
if(pb)pb.addEventListener('click',function(ev){ev.stopPropagation();act('party');
 document.body.classList.add('fest');setTimeout(function(){document.body.classList.remove('fest');},2600);});
clearAll();
// an open page holds the board awake (lifecycle law — silence is what sleeps it)
setInterval(function(){fetch('pulse').catch(function(){});},60000);
})();
</script>
<div class="meta">Size = doc weight (radius ∝ √(KB/6KB) — an over-fed mind <i>leans on its neighbors</i>: that crowding is the disharmony, drawn true). Glow + ember core = stress — on a body, its diet; on a hat, its desk. Click a node to focus; <b>⚡ stress detect</b> arms the clignotement. ⋄ = genesis watch
(stress ≥80% ∧ crosslinks ≥1.5× median — heuristic; a birth still needs its need named twice).</div>

<h2>⋄ evolution — the breath of days</h2><div id="breathBox">${breath}</div><script type="application/json" id="breathData">${esc(JSON.stringify(d.days))}</script>

<h2>⋄ provenance</h2><div class="meta">harvested live at request time from .opencode/skills/* + .claude/skills/* · ${d.home}/{elf,orc,slime}/* · .opencode/commands + .claude/commands (row 2) · .opencode/agents + .claude/agents (mounts) · ${d.canonFile} stamps · AGENTS.md routing table · the ${d.home} journal · git log — nothing stored. stdlib node, zero scripts, zero CDN; the tooling law (nature law 8). <span class="dim">port ${PORT}</span></div>
<script>
// consolidated client runtime: range filter drives BOTH charts + token tables
// (human order 2026-09-15). No backticks or interpolation markers allowed in
// here — this text rides inside the server template literal.
// Ranges are day-bucketed: 1d = today.
(function(){
  var RANGE=0;
  var $=function(id){return document.getElementById(id);};
  function fmt(n){if(n>=1e9)return (n/1e9).toFixed(1)+'B';if(n>=1e6)return (n/1e6).toFixed(1)+'M';if(n>=1e3)return (n/1e3).toFixed(1)+'k';return String(Math.round(n));}
  function cutoff(){if(!RANGE)return null;var t=new Date(Date.now()-(RANGE-1)*864e5);var p=function(x){return String(x).length<2?'0'+x:''+x;};return t.getFullYear()+'-'+p(t.getMonth()+1)+'-'+p(t.getDate());}
  function inR(day){var c=cutoff();return !c||day>=c;}
  function hj(s){return String(s).replace(/</g,'&lt;');}
  // ---- evolution (server paints the all-range first frame; we redraw) ----
  var BD={};var btag=$('breathData');if(btag){try{BD=JSON.parse(btag.textContent);}catch(e){}}
  function drawBreath(){var box=$('breathBox');if(!box)return;
   var dates=Object.keys(BD).filter(inR).sort();
   if(!dates.length){box.innerHTML='<div class="dim panel" style="padding:14px 16px">no days in this range.</div>';return;}
   var keys=[['journal','#59d6ff'],['thoughts','#a78bfa'],['commits','#ff7a59']];
   var gM=1;dates.forEach(function(x){keys.forEach(function(kv){var v=BD[x][kv[0]]||0;if(v>gM)gM=v;});});
   var W=1180,H=250,ml=52,mr=20,mt=26,mb=44,pw=W-ml-mr,ph=H-mt-mb;
   var sx=function(i){return ml+(dates.length===1?pw/2:i*pw/(dates.length-1));};
   var sy=function(v){return mt+ph*(1-v/gM);};
   var s='',t,i,yy,xx;
   for(t=0;t<=4;t++){var v=Math.round(gM*t/4);yy=sy(v).toFixed(1);
    s+='<line x1="'+ml+'" y1="'+yy+'" x2="'+(W-mr)+'" y2="'+yy+'" stroke="#22304a" stroke-width="0.6" opacity="'+(t?0.45:1)+'"/>';
    s+='<text x="'+(ml-8)+'" y="'+(+yy+3)+'" class="ax" text-anchor="end">'+v+'</text>';}
   var step=Math.ceil(dates.length/12);
   for(i=0;i<dates.length;i++){if(i%step&&i!==dates.length-1)continue;xx=sx(i).toFixed(1);
    s+='<line x1="'+xx+'" y1="'+(H-mb)+'" x2="'+xx+'" y2="'+(H-mb+5)+'" stroke="#22304a"/>';
    s+='<text x="'+xx+'" y="'+(H-mb+17)+'" class="ax" text-anchor="middle">'+dates[i].slice(5)+'</text>';}
   keys.forEach(function(kv){var pts=[];for(i=0;i<dates.length;i++)pts.push(sx(i).toFixed(1)+','+sy(BD[dates[i]][kv[0]]||0).toFixed(1));
    s+='<polyline points="'+pts.join(' ')+'" fill="none" stroke="'+kv[1]+'" stroke-width="1.8"/>';});
   s+='<text x="'+ml+'" y="14" class="ax">events / day — unified scale (max '+gM+')</text>';
   var lg='';keys.forEach(function(kv){lg+=(lg?' · ':'')+'<tspan fill="'+kv[1]+'">■ '+kv[0]+'</tspan>';});
   s+='<text x="'+(W-mr)+'" y="14" class="ax" text-anchor="end">'+lg+'</text>';
   s+='<text x="'+(W-mr)+'" y="'+(H-4)+'" class="ax" text-anchor="end">day →</text>';
   box.innerHTML='<svg viewBox="0 0 '+W+' '+H+'" class="panel">'+s+'</svg>';}
  // ---- tokens ----
  var TD={},TAG={};var ttag=$('tokData'),atag=$('tokAgents');
  if(ttag){try{TD=JSON.parse(ttag.textContent);}catch(e){}}
  if(atag){try{TAG=JSON.parse(atag.textContent);}catch(e){}}
  var mids=Object.keys(TD);var cur=null;
  // Fixed categorical order, never cycled — the same already-validated race hues (re-tuned
  // 2026-09-20 against validate_palette.js, see the :root comment above) reused for model
  // identity rather than a second palette invented and re-validated from scratch. Last slot
  // (--r-plain, the existing muted/untyped color) is reserved for the overflow "other" bucket.
  var CAT=['var(--r-slime)','var(--r-orc)','var(--r-elf)','var(--r-darkelf)','var(--r-highorc)','var(--r-kijin)','var(--r-mind)','var(--r-plain)'];
  var COLOR={},SERIES=[]; // built by initTok once mids are sorted by consumption
  function totR(m){var t=0,dm=TD[m];for(var k in dm)if(inR(k))t+=dm[k][0]+dm[k][1];return t;}
  // rounded-top / square-baseline bar path (mark spec: 4px data-end, square at baseline)
  function barPath(x,yTop,w,h,rr){if(h<=0.5)return'';rr=Math.min(rr,w/2,h);
   return'M'+x+','+(yTop+h)+'L'+x+','+(yTop+rr)+'Q'+x+','+yTop+' '+(x+rr)+','+yTop+
    'L'+(x+w-rr)+','+yTop+'Q'+(x+w)+','+yTop+' '+(x+w)+','+(yTop+rr)+'L'+(x+w)+','+(yTop+h)+'Z';}
  function drawTok(){var box=$('tokChart');if(!box||!cur)return;
   if(cur==='__all__'){drawTokAll(box);return;}
   var days=Object.keys(TD[cur]||{}).filter(inR).sort(),n=days.length,i,t;
   if(!n){box.innerHTML='<div class="dim" style="padding:10px 0">'+hj(cur)+' — nothing in this range.</div>';return;}
   var W=1180,H=300,ml=64,mr=18,mt=22,mb=42,pw=W-ml-mr,ph=H-mt-mb,mx=1;
   for(i=0;i<n;i++){var v=TD[cur][days[i]];if(v[0]>mx)mx=v[0];if(v[1]>mx)mx=v[1];}
   var slot=pw/n,gap=2,bw=Math.max(2,Math.min(24,(slot-gap-6)/2)),baseline=mt+ph;
   var gx=function(q){return ml+slot*(q+0.5);}; // group (day) center
   var sy=function(q){return mt+ph*(1-q/mx);};
   var ic=COLOR[cur]||'var(--cy)'; // same hue as this model's selector swatch — in solid, out at half opacity
   var s='<text x="'+ml+'" y="14" class="ax">tokens / day — '+hj(cur)+'</text>';
   s+='<text x="'+(W-mr)+'" y="14" class="ax" text-anchor="end"><tspan fill="'+ic+'">■ in</tspan> · <tspan fill="'+ic+'" fill-opacity="0.45">■ out</tspan></text>';
   for(t=0;t<=4;t++){var tv=Math.round(mx*t/4);yy=sy(tv).toFixed(1);
    s+='<line x1="'+ml+'" y1="'+yy+'" x2="'+(W-mr)+'" y2="'+yy+'" stroke="#22304a" stroke-width="0.6" opacity="'+(t?0.45:1)+'"/>';
    s+='<text x="'+(ml-8)+'" y="'+(+yy+3)+'" class="ax" text-anchor="end">'+fmt(tv)+'</text>';}
   var step=Math.ceil(n/12);
   for(i=0;i<n;i++){var cx=gx(i).toFixed(1),vv=TD[cur][days[i]];
    var inTop=sy(vv[0]),outTop=sy(vv[1]);
    s+='<path d="'+barPath(gx(i)-gap/2-bw,inTop,bw,baseline-inTop,4)+'" fill="'+ic+'"><title>'+hj(days[i])+' in '+fmt(vv[0])+'</title></path>';
    s+='<path d="'+barPath(gx(i)+gap/2,outTop,bw,baseline-outTop,4)+'" fill="'+ic+'" fill-opacity="0.45"><title>'+hj(days[i])+' out '+fmt(vv[1])+'</title></path>';
    if(i%step&&i!==n-1)continue;
    s+='<line x1="'+cx+'" y1="'+baseline+'" x2="'+cx+'" y2="'+(baseline+5)+'" stroke="#22304a"/>';
    s+='<text x="'+cx+'" y="'+(baseline+17)+'" class="ax" text-anchor="middle">'+days[i].slice(5)+'</text>';}
   s+='<line x1="'+ml+'" y1="'+baseline+'" x2="'+(W-mr)+'" y2="'+baseline+'" stroke="#22304a" stroke-width="0.6"/>';
   s+='<text x="'+(W-mr)+'" y="'+(H-4)+'" class="ax" text-anchor="end">day →</text>';
   box.innerHTML='<svg viewBox="0 0 '+W+' '+H+'" class="panel">'+s+'</svg>';}
  // merged view (human order 2026-09-21: "one selector that merges all consumption,
  // separated by colors") — one bar per model per day (in+out combined), grouped by day,
  // each model its own fixed CAT hue; overflow beyond CAT's slots folds into one muted
  // "other" series rather than generating a 9th hue (dataviz skill non-negotiable).
  function drawTokAll(box){
   var days=[],seen={};SERIES.forEach(function(sr){sr.members.forEach(function(m){for(var k in TD[m]||{})if(inR(k)&&!seen[k]){seen[k]=1;days.push(k);}});});
   days.sort();var n=days.length,i,t,j;
   if(!n){box.innerHTML='<div class="dim" style="padding:10px 0">no consumption data in this range.</div>';return;}
   var W=1180,H=300,ml=64,mr=18,mt=22,mb=42,pw=W-ml-mr,ph=H-mt-mb,mx=1;
   var totals={};
   SERIES.forEach(function(sr){var byDay={};sr.members.forEach(function(m){var dm=TD[m]||{};for(var k in dm)if(inR(k)){var v=dm[k];byDay[k]=(byDay[k]||0)+v[0]+v[1];}});
    totals[sr.id]=byDay;for(var k in byDay)if(byDay[k]>mx)mx=byDay[k];});
   var k2=SERIES.length,slot=pw/n,gap=2,bw=Math.max(1.5,Math.min(22,(slot-gap*(k2-1)-6)/k2)),baseline=mt+ph;
   var gx=function(q){return ml+slot*(q+0.5);};
   // Log scale: models here differ by orders of magnitude (a main session vs. an occasional
   // subagent build) — on a linear axis the small series round to sub-pixel and vanish, which
   // reads as "broken," not "small." log10(v+1) keeps every nonzero bar visibly tall while
   // still ordering correctly; ticks are spaced evenly in log-space (the only correct way to
   // grid a log axis) and labeled with the real token value they represent.
   var logMax=Math.log10(mx+1)||1;
   var sy=function(q){return mt+ph*(1-Math.log10(q+1)/logMax);};
   var s='<text x="'+ml+'" y="14" class="ax">tokens / day — all models (log scale)</text>';
   var lg='';SERIES.forEach(function(sr){lg+=(lg?' · ':'')+'<tspan fill="'+sr.color+'">■ '+hj(sr.label)+'</tspan>';});
   s+='<text x="'+(W-mr)+'" y="14" class="ax" text-anchor="end">'+lg+'</text>';
   for(t=0;t<=4;t++){var frac=t/4,tv=Math.round(Math.pow(10,logMax*frac))-1;yy=(mt+ph*(1-frac)).toFixed(1);
    s+='<line x1="'+ml+'" y1="'+yy+'" x2="'+(W-mr)+'" y2="'+yy+'" stroke="#22304a" stroke-width="0.6" opacity="'+(t?0.45:1)+'"/>';
    s+='<text x="'+(ml-8)+'" y="'+(+yy+3)+'" class="ax" text-anchor="end">'+fmt(Math.max(0,tv))+'</text>';}
   var step=Math.ceil(n/12);
   for(i=0;i<n;i++){var cx=gx(i).toFixed(1),groupStart=gx(i)-(k2*bw+(k2-1)*gap)/2;
    for(j=0;j<k2;j++){var sr=SERIES[j],v=totals[sr.id][days[i]]||0;if(!v)continue;
     var top=sy(v),x=groupStart+j*(bw+gap);
     s+='<path d="'+barPath(x,top,bw,baseline-top,3)+'" fill="'+sr.color+'"><title>'+hj(days[i])+' '+hj(sr.label)+' '+fmt(v)+'</title></path>';}
    if(i%step&&i!==n-1)continue;
    s+='<line x1="'+cx+'" y1="'+baseline+'" x2="'+cx+'" y2="'+(baseline+5)+'" stroke="#22304a"/>';
    s+='<text x="'+cx+'" y="'+(baseline+17)+'" class="ax" text-anchor="middle">'+days[i].slice(5)+'</text>';}
   s+='<line x1="'+ml+'" y1="'+baseline+'" x2="'+(W-mr)+'" y2="'+baseline+'" stroke="#22304a" stroke-width="0.6"/>';
   s+='<text x="'+(W-mr)+'" y="'+(H-4)+'" class="ax" text-anchor="end">day →</text>';
   box.innerHTML='<svg viewBox="0 0 '+W+' '+H+'" class="panel">'+s+'</svg>';}
  function drawTokTables(){var mt=$('tokModelTable'),dt=$('tokDayTable');var tot=1;mids.forEach(function(m){tot+=totR(m);});tot=tot-1||1;
   if(mt){var rows='<tr><th>model</th><th>sessions (agents)</th><th>runs</th><th>in</th><th>out</th><th>share</th></tr>';
    mids.slice().sort(function(a,b){return totR(b)-totR(a);}).forEach(function(m){
     var ti=0,to=0,rn=0,dm=TD[m];for(var k in dm)if(inR(k)){ti+=dm[k][0];to+=dm[k][1];rn+=dm[k][2]||0;}
     rows+='<tr><td>'+hj(m)+'</td><td class="dim">'+hj((TAG[m]||[]).join(', ')||'–')+'</td><td class="num">'+rn+'</td><td class="num">'+ti+'</td><td class="num">'+to+'</td><td class="num">'+((100*(ti+to))/tot).toFixed(1)+'%</td></tr>';});
    mt.innerHTML=rows;}
   if(dt){var all={};mids.forEach(function(m){var dm=TD[m];for(var k in dm){if(inR(k)){var e=all[k]||[0,0];e[0]+=dm[k][0];e[1]+=dm[k][1];all[k]=e;}}});
    var rr='<tr><th>day</th><th>in</th><th>out</th></tr>';Object.keys(all).sort().forEach(function(kd){
     rr+='<tr><td>'+kd+'</td><td class="num">'+all[kd][0]+'</td><td class="num">'+all[kd][1]+'</td></tr>';});
    dt.innerHTML=rr;}}
  function initTok(){var sel=$('tokSel');if(!sel)return;
   if(!mids.length){sel.innerHTML='<span class="dim">no consumption data</span>';return;}
   mids.sort(function(a,b){return totR(b)-totR(a);});
   SERIES=[];COLOR={};
   var capIndiv=mids.length>CAT.length?CAT.length-1:mids.length,i;
   for(i=0;i<capIndiv;i++){COLOR[mids[i]]=CAT[i];SERIES.push({id:mids[i],label:mids[i],color:CAT[i],members:[mids[i]]});}
   if(mids.length>capIndiv){var rest=mids.slice(capIndiv),oc=CAT[CAT.length-1];rest.forEach(function(m){COLOR[m]=oc;});
    SERIES.push({id:'__other__',label:'other ('+rest.length+')',color:oc,members:rest});}
   cur='__all__';sel.innerHTML='';
   function mkBtn(label,color){var b=document.createElement('button');b.textContent=label;
    b.style.cssText='margin:0 6px 6px 0;padding:3px 10px;border-radius:3px;cursor:pointer;background:transparent;border:1px solid '+color+';color:'+color;
    return b;}
   var btns={};
   var allBtn=mkBtn('◆ all','var(--ink)');allBtn.onclick=function(){cur='__all__';paint();};sel.appendChild(allBtn);
   mids.forEach(function(m){var b=mkBtn(m,COLOR[m]);b.onclick=function(){cur=m;paint();};btns[m]=b;sel.appendChild(b);});
   function paint(){allBtn.style.boxShadow='';mids.forEach(function(m){btns[m].style.boxShadow='';});
    if(cur==='__all__')allBtn.style.boxShadow='0 0 8px var(--ink)';else btns[cur].style.boxShadow='0 0 8px '+COLOR[cur];
    drawTok();}
   paint();}
  var btns=document.querySelectorAll('.rng');
  function apply(r){RANGE=r;for(var k=0;k<btns.length;k++){btns[k].classList.toggle('active',+btns[k].getAttribute('data-r')===r);}drawBreath();drawTokTables();drawTok();}
  for(var bi=0;bi<btns.length;bi++){(function(b){b.onclick=function(){apply(+b.getAttribute('data-r'));};})(btns[bi]);}
  drawBreath();initTok();drawTokTables();
})();
</script>
</main>`;
}

// ------------ global index — every world this one app knows about ------------
// Human order 2026-09-20: "1 app in whole machine not multiple ... global dashboard
// give overall overview of all worlds". Harvests every registered world fresh on
// each request (same "nothing stored, harvested live" law as a single world's page)
// and renders a card per world, linking to /<name>/.
function renderIndex(worlds) {
  const fmtN = v => v >= 1e6 ? (v / 1e6).toFixed(1) + 'M' : v >= 1e3 ? (v / 1e3).toFixed(1) + 'k' : String(v);
  const worldData = worlds.map(w => {
    let d, err = null;
    try { d = harvest(w.path); } catch (e) { err = e.message; }
    return { w, d, err };
  });

  // ---- machine-wide activity: every session on this machine, not filtered to a
  // registered world (human order 2026-09-20: "how much consumption in total if
  // opencode session etc ... information if session is active or not") ----
  const claudeG = harvestClaudeGlobal();
  const openG = harvestOpencodeGlobal();
  const claudeModelRows = Object.entries(claudeG.perModel).sort((a, b) => (b[1].in + b[1].out) - (a[1].in + a[1].out))
    .map(([m, v]) => `<tr><td><b>${esc(m)}</b></td><td class="num">${fmtN(v.in)}</td><td class="num">${fmtN(v.out)}</td></tr>`).join('');
  const openModelRows = Object.entries(openG.perModel).sort((a, b) => (b[1].in + b[1].out) - (a[1].in + a[1].out))
    .map(([m, v]) => `<tr><td><b>${esc(m)}</b></td><td class="num">${fmtN(v.in)}</td><td class="num">${fmtN(v.out)}</td></tr>`).join('');

  const claudePanel = `<div class="panel actpanel">
    <h3>⋄ Claude Code <span class="pill ${claudeG.active ? 'hot' : 'cool'}">${claudeG.active ? claudeG.active + ' active now' : 'idle'}</span></h3>
    <div class="wstat">${claudeG.sessions} session${claudeG.sessions === 1 ? '' : 's'} on this machine (any directory, not just registered worlds)</div>
    <div class="wstat">${fmtN(claudeG.totalIn)} in / ${fmtN(claudeG.totalOut)} out — all-time, every transcript under <span style="font-family:inherit">~/.claude/projects/</span></div>
    ${claudeModelRows ? `<table><tr><th>model</th><th>in</th><th>out</th></tr>${claudeModelRows}</table>` : '<div class="dim">no usage recorded yet</div>'}
  </div>`;
  const openPanel = !openG.dbExists ? `<div class="panel actpanel">
    <h3>⋄ OpenCode</h3><div class="dim">no <span style="font-family:inherit">~/.local/share/opencode/opencode.db</span> on this machine — OpenCode not in use here, or never run.</div>
  </div>` : !openG.available ? `<div class="panel actpanel">
    <h3>⋄ OpenCode <span class="pill hot">unreadable</span></h3>
    <div class="wstat">The database exists — real sessions may be sitting in it — but this board's own Node runtime can't read it: <span style="font-family:inherit">${esc(openG.error || 'unknown error')}</span>.</div>
    <div class="wstat dim"><span style="font-family:inherit">node:sqlite</span> needs Node 22.5+; this process is running on ${esc(process.version)}. Nature 9: a silent instrument is itself a finding — this panel says so instead of quietly reporting zero.</div>
  </div>` : `<div class="panel actpanel">
    <h3>⋄ OpenCode <span class="pill ${openG.activeRows ? 'hot' : 'cool'}">${openG.activeRows ? openG.activeRows + ' recent run' + (openG.activeRows === 1 ? '' : 's') : 'idle'}</span></h3>
    <div class="wstat">${openG.rows} run${openG.rows === 1 ? '' : 's'} recorded, all-time, machine-wide</div>
    <div class="wstat">${fmtN(openG.totalIn)} in / ${fmtN(openG.totalOut)} out</div>
    ${openModelRows ? `<table><tr><th>model</th><th>in</th><th>out</th></tr>${openModelRows}</table>` : '<div class="dim">no usage recorded yet</div>'}
    <div class="wstat dim">"recent" = a row touched in the last 5 minutes — opencode's schema carries no session-id here, so this is a run count, not a precise open-session count.</div>
  </div>`;

  const cards = worldData.length ? worldData.map(({ w, d, err }) => {
    const censusLine = d ? Object.entries(d.census).map(([race, n]) => `<span>${esc(race)} ${n}</span>`).join('') : '';
    const health = d ? esc(d.health) : `unreadable: ${esc(err || 'unknown error')}`;
    const stressedN = d ? d.creatures.filter(c => c.stressPct >= 60).length : 0;
    return `<a class="wcard" href="/${esc(w.name)}/">
      <h3>⋄ ${esc(w.name)}${stressedN ? ` <span class="pill warn">${stressedN} stressed</span>` : ''}</h3>
      <div class="wstat wcensus">${censusLine || '<span class="dim">no population yet</span>'}</div>
      <div class="wstat">${health}</div>
      <div class="wstat">last seen ${esc((w.lastSeen || '').slice(0, 10) || '–')} · ${esc(w.path)}</div>
    </a>`;
  }).join('') : `<div class="dim panel" style="padding:14px 16px;grid-column:1/-1">
      no worlds registered yet. Run <b>/isekai</b> or <b>node tempest.js &lt;dir&gt; --ensure</b> in a world to add it here.</div>`;

  const totalCreatures = worldData.reduce((n, { d }) => n + (d ? d.creatures.length : 0), 0);

  return `<!doctype html><meta charset="utf-8"><title>tempest ⋄ all worlds</title>
${PAGE_STYLE}
<style>.actpanel{padding:16px 18px}.actpanel h3{margin:0 0 8px;font-size:13px;letter-spacing:.06em}
.actgrid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:14px;margin-top:14px}</style>
<body class="light">
<main>
<h1><span class="sigil">⋄</span> TEMPEST <span style="letter-spacing:.1em;color:var(--dim);font-size:11px"> ALL WORLDS, OBSERVED · :${PORT}</span></h1>
<div class="meta">${worlds.length} world${worlds.length === 1 ? '' : 's'} registered · ${totalCreatures} creature${totalCreatures === 1 ? '' : 's'} total ·
one app, one process, one port — each world lives at its own <span style="font-family:inherit">/&lt;name&gt;/</span> sub-path.</div>

<h2>⋄ machine — total breath across every session, every world</h2>
<div class="actgrid">${claudePanel}${openPanel}</div>

<h2>⋄ worlds</h2>
<div class="wcards">${cards}</div>
</main>`;
}

// ------------ relief dispatch (law amended 2026-09-15, human order "do on dashboard") ------------
// The board prescribes, records, and DISPATCHES: it never writes a mind itself.
// Each relief step spawns a sequential creature-hat session (`opencode run`, cwd =
// colony root → AGENTS.md + colony law load with it) which performs the distillation.
// Steps log to the world home's metrics/relief.jsonl; one run in flight at a time
// PER WORLD — reliefByWorld is keyed by world name since one app now serves every
// world at once, and a relief run in world A must never touch world B's queue.
//
// Departure from source: the source hardcodes `opencode run`. On a Claude-only
// machine that would silently strand relief runs, so this ports the fallback this
// session already built and tested: prefer opencode, fall back to `claude -p`.
const reliefByWorld = new Map(); // name -> {active, queue, done, failed, current, total, startedAt, finishedAt}

const reliefLog = (root, home, rec) => {
  const f = path.join(root, home, 'metrics', 'relief.jsonl');
  fs.mkdirSync(path.dirname(f), { recursive: true });
  fs.appendFileSync(f, JSON.stringify(rec) + '\n');
};
// desk-overflow first (worst ratio first), then diet-only by weight descending.
const stressedOf = d => d.creatures.filter(c => c.thoughts > c.limit || c.dietPct > 100)
  .sort((a, b) => {
    const ao = a.thoughts > a.limit, bo = b.thoughts > b.limit;
    if (ao !== bo) return bo - ao;
    if (ao && bo) return (b.thoughts / b.limit) - (a.thoughts / a.limit) || b.kb - a.kb;
    return b.kb - a.kb;
  });
function reliefBrief(c, home) {
  return [
    `Relief duty, tempest dispatch (the instruments law: the board dispatches, creature-hat sessions rewrite minds).`,
    `You ARE the creature "${c.name}" of this colony for this session only. Territory: ONLY .opencode/skills/${c.name}/ (its SKILL.md and any reference/ beside it). Touch no other creature's doc, no other file.`,
    `State: desk ${c.thoughts}/${c.limit} dated Thoughts entries; doc ${c.kb}KB against the ${DIET_KB}KB diet law (the breath law).`,
    `Act, in order:`,
    `1. Read .opencode/skills/${c.name}/SKILL.md in full, its ## Thoughts desk included.`,
    `2. Distil the desk: fold durable rules/pitfalls into the doc's own sections; bulky detail goes to reference/ files beside the doc. A lesson shared with sibling creatures is left as a one-line pointer to its orc instead.`,
    `3. Rewrite the ## Thoughts desk to at most ${c.limit} dated (YYYY-MM-DD), first-person entries. Lessons now living in the body are cleared, never restated there.`,
    c.kb > DIET_KB ? `4. Diet breach: move reference-grade detail into reference/ until SKILL.md sits near ${DIET_KB}KB. The frontmatter description block stays byte-identical (it is the load trigger).` : '',
    c.genesisSignal ? `5. Genesis-watch ⋄: your stress+crosslinks hold the signal and naming #1 is already logged for you (metrics/celebrations.jsonl) — review the SPLIT and record the verdict (need stands named a second time / rejected, with one line of why) as a dated entry in the rewritten ## Thoughts — naming #1 rides ${home}/metrics/celebrations.jsonl.` : '',
    `Absolute: no git verbs at all (no commit, no push), no network, no files outside your territory. Reply with exactly one line: creature, thoughts kept, final KB.`,
  ].filter(Boolean).join('\n');
}
function reliefCheck(root, home, c, snap) {
  // post-step harm check (human 2026-09-15: "it doesn't cause harm, right?")
  const rec = { ts: new Date().toISOString(), kind: 'relief-check', creature: c.name };
  try {
    const docP = path.join(root, '.opencode', 'skills', c.name, 'SKILL.md');
    const fm = s => (s.match(/^---\n[\s\S]*?\n---\n/) || [''])[0];
    const was = rd(path.join(snap, 'SKILL.md'));
    let now = rd(docP);
    if (was && now && fm(now) !== fm(was)) {
      // frontmatter is the load trigger — drift is hard-restored from the snapshot
      fs.writeFileSync(docP, now.replace(/^---\n[\s\S]*?\n---\n/, fm(was)));
      now = rd(docP);
      rec.frontmatter = 'restored';
    } else rec.frontmatter = 'ok';
    const m = now.match(/##\s*Thoughts([\s\S]*?)(?=\n##\s|\n#\s|$)/i);
    const desk = m ? m[1].split('\n').filter(l => /^\s*(-|###)/.test(l) && /\d{4}-\d{2}-\d{2}/.test(l)).length : 0;
    rec.desk = `${desk}/${c.limit}`; if (desk > c.limit) rec.breach = 'desk still over limit';
    rec.kbFrom = c.kb; rec.kbTo = +(fs.statSync(docP).size / 1024).toFixed(1);
  } catch (e) { rec.checkError = e.message; }
  reliefLog(root, home, rec);
}
function reliefStep(root, home, worldName) {
  const relief = reliefByWorld.get(worldName);
  const c = relief.queue.shift();
  if (!c) {
    relief.active = false; relief.current = null; relief.finishedAt = new Date().toISOString();
    reliefLog(root, home, { ts: relief.finishedAt, kind: 'relief-run', status: 'finished', done: relief.done, failed: relief.failed });
    return;
  }
  relief.current = c.name;
  // harm fence 1: snapshot the creature BEFORE its hat-session runs — minds
  // live outside git (the tracking law), so this scratch copy is the only undo in town
  const snap = path.join(root, home, 'tmp', relief.startedAt.slice(0, 10), 'relief-snapshot', c.name);
  try { fs.cpSync(path.join(root, '.opencode', 'skills', c.name), snap, { recursive: true }); }
  catch (e) { reliefLog(root, home, { ts: new Date().toISOString(), kind: 'relief-step', creature: c.name, status: 'snapshot-failed', what: e.message }); }
  reliefLog(root, home, { ts: new Date().toISOString(), kind: 'relief-step', creature: c.name, status: 'start', desk: `${c.thoughts}/${c.limit}`, kb: c.kb });
  const cli = detectReliefCli();
  if (!cli) { relief.failed.push(c.name); reliefLog(root, home, { ts: new Date().toISOString(), kind: 'relief-step', creature: c.name, status: 'spawn-failed', what: 'no relief CLI on PATH' }); return reliefStep(root, home, worldName); }
  let child;
  try { child = spawn(cli.bin, cli.args(reliefBrief(c, home)), { cwd: root, stdio: 'ignore' }); }
  catch (e) { relief.failed.push(c.name); reliefLog(root, home, { ts: new Date().toISOString(), kind: 'relief-step', creature: c.name, status: 'spawn-failed', what: e.message }); return reliefStep(root, home, worldName); }
  let timed = false, settled = false; // 'error' AND 'close' both fire on failed spawns — settle once
  const killer = setTimeout(() => { timed = true; child.kill('SIGKILL'); }, 10 * 60 * 1000); // 10 min per creature, then the next
  const settle = (status, extra) => {
    if (settled) return; settled = true; clearTimeout(killer);
    (status === 'done' ? relief.done : relief.failed).push(c.name);
    reliefLog(root, home, Object.assign({ ts: new Date().toISOString(), kind: 'relief-step', creature: c.name, status }, extra));
    if (status === 'done') reliefCheck(root, home, c, snap); // harm fence 2: frontmatter byte-check + desk re-count
    reliefStep(root, home, worldName);
  };
  child.on('error', e => settle('error', { what: e.message }));
  child.on('close', code => settle(timed ? 'timeout' : code === 0 ? 'done' : 'failed', { code }));
}
// prefer opencode (the CLI the source spec was written against: "opencode run"); fall
// back to Claude Code's own headless mode (`claude -p`) so relief runs work on Claude-only machines too
function detectReliefCli() {
  const which = process.platform === 'win32' ? 'where' : 'which';
  if (spawnSync(which, ['opencode']).status === 0) return { bin: 'opencode', args: prompt => ['run', prompt] };
  if (spawnSync(which, ['claude']).status === 0) return { bin: 'claude', args: prompt => ['-p', prompt] };
  return null;
}

// ------------ serve / dump ------------
// --ensure: instruments stay lit (nature law 8 — while a session thinks, the
// board is up). Idempotent: up → register this world and say so; down → spawn the
// ONE global daemon detached and forget. Any session, for any world, may run this —
// human order 2026-09-20: "1 app in whole machine not multiple ... use sub path".
const ENSURE = args.includes('--ensure');
const STOP = args.includes('--stop');
if (STOP) {
  // post the resident (global) board to sleep — affects every registered world at
  // once, which is the accepted tradeoff of one app for the whole machine.
  const rq = http.request({ host: '127.0.0.1', port: PORT, path: '/shutdown', method: 'POST', timeout: 1500 },
    r => { r.resume(); process.stdout.write(`tempest told to sleep :${PORT}\n`); });
  rq.on('timeout', () => { rq.destroy(); process.stdout.write(`tempest unresponsive on :${PORT}\n`); });
  rq.on('error', () => process.stdout.write(`tempest not up on :${PORT}\n`));
  rq.end();
} else if (ENSURE) {
  if (!fs.existsSync(path.join(COLONY, '.isekai')) && !fs.existsSync(path.join(COLONY, '.convention-zero'))) {
    process.stdout.write(`no .isekai/ world at ${COLONY} — run /isekai first\n`); process.exit(1);
  }
  const entry = registerWorld(COLONY);
  const probe = http.createServer();
  probe.once('error', () => { probe.close(); process.stdout.write(`tempest ${entry.name} already lit — http://localhost:${PORT}/${entry.name}/\n`); });
  probe.once('listening', () => {
    probe.close(() => {
      // No COLONY arg for the daemon — it serves every registered world, not just
      // the one that happened to light it; each request resolves its own root.
      const child = spawn(process.execPath, [__filename, '--port', String(PORT)].concat(IMMORTAL ? ['--immortal'] : []).concat(ttlIdx > -1 ? ['--ttl', String(TTL_MIN)] : []),
        { detached: true, stdio: 'ignore' });
      child.unref();
      process.stdout.write(`tempest ${entry.name} lit — http://localhost:${PORT}/${entry.name}/\n`);
    });
  });
  probe.listen(PORT, '127.0.0.1');
} else if (JSON_MODE) {
  // One line, parse-clean (payload B definition of done, 2026-09-21): a machine mouth reads
  // this — the wire's economy, not an eye's — so no pretty-print indentation.
  process.stdout.write(JSON.stringify(harvest(COLONY)) + '\n');
} else if (JSON_GLOBAL_MODE) {
  // One-shot, no daemon needed — for scripts/tests to verify the global panel's own
  // numbers directly instead of scraping rendered HTML (Nature 9: compute it, check it).
  process.stdout.write(JSON.stringify({ claude: harvestClaudeGlobal(), opencode: harvestOpencodeGlobal() }, null, 2) + '\n');
} else {
  // default: run the ONE global board in the foreground (the docker-compose shape —
  // a long-lived process, not a spawn-and-forget heartbeat). Every world it knows
  // about (the registry) is served from this single process, one sub-path each.
  const server = http.createServer((req, res) => {
    lastTouch = Date.now();
    const url = new URL(req.url, `http://127.0.0.1:${PORT}`);
    const parts = url.pathname.split('/').filter(Boolean); // [] | [name] | [name, action]

    if (req.method === 'POST' && url.pathname === '/shutdown') {
      res.writeHead(200); res.end('sleeping'); server.close(() => process.exit(0)); return;
    }
    if (url.pathname === '/pulse' || (parts.length === 2 && parts[1] === 'pulse')) { res.writeHead(204); res.end(); return; }

    if (req.method === 'GET' && parts.length === 0) {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(renderIndex(prunedRegistry())); return;
    }

    const worldName = parts[0], action = parts[1] || '';
    const root = worldRootForName(worldName);
    if (!root) { res.writeHead(404, { 'Content-Type': 'text/plain' }); res.end(`unknown world: ${worldName} — is it registered? run /isekai or tempest --ensure in it first`); return; }
    const home = homeOf(root);

    // race portrait, e.g. GET /<world>/portrait/slime — `race` is a lookup key into
    // PORTRAIT_FILE, never a raw filename, so this can't be asked to read anything
    // outside <root>/<home>/portraits/.
    if (req.method === 'GET' && action === 'portrait') {
      const race = parts[2] || '';
      const file = PORTRAIT_FILE[race];
      if (!file) { res.writeHead(404, { 'Content-Type': 'text/plain' }); res.end('no portrait for that race'); return; }
      const p = path.join(root, home, 'portraits', file);
      fs.readFile(p, (err, buf) => {
        if (err) { res.writeHead(404, { 'Content-Type': 'text/plain' }); res.end('portrait file missing'); return; }
        res.writeHead(200, { 'Content-Type': 'image/png', 'Cache-Control': 'public, max-age=3600' });
        res.end(buf);
      });
      return;
    }
    if (req.method === 'GET' && !action) {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(render(harvest(root))); return;
    }
    if (req.method === 'POST' && action === 'holidays') {
      const d = harvest(root);
      const relief = reliefByWorld.get(worldName);
      if (relief && relief.active) { res.writeHead(200, { 'Content-Type': 'application/json' }); res.end(JSON.stringify({ stressed: stressedOf(d).length, relief: { refused: true, launched: true } })); return; }
      const worklist = stressedOf(d);
      const day = new Date().toISOString().slice(0, 10);
      const dayDir = path.join(root, home, 'tmp', day); fs.mkdirSync(dayDir, { recursive: true });
      const file = path.join(dayDir, 'holidays.md');
      fs.writeFileSync(file, `# Relief worklist — ${day}\n\n` + (worklist.map(c => `- ${c.name}: thoughts ${c.thoughts}/${c.limit}, ${c.kb}KB/${DIET_KB}KB`).join('\n') || '(nothing over the diet)') + '\n');
      if (!worklist.length) { res.writeHead(200, { 'Content-Type': 'application/json' }); res.end(JSON.stringify({ stressed: 0 })); return; }
      reliefByWorld.set(worldName, { active: true, queue: worklist.slice(), done: [], failed: [], current: null, total: worklist.length, startedAt: new Date().toISOString() });
      reliefStep(root, home, worldName);
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ stressed: worklist.length, file, relief: { launched: true } })); return;
    }
    if (req.method === 'POST' && action === 'party') {
      const d = harvest(root);
      const cf = path.join(root, home, 'metrics', 'celebrations.jsonl');
      fs.mkdirSync(path.dirname(cf), { recursive: true });
      fs.appendFileSync(cf, JSON.stringify({ ts: new Date().toISOString(), kind: 'party', births: d.genesisWatch }) + '\n');
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ births: d.genesisWatch })); return;
    }
    if (req.method === 'GET' && action === 'relief') {
      const relief = reliefByWorld.get(worldName);
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(relief ? Object.assign({}, relief, { total: relief.total, remaining: relief.queue.map(c => c.name) }) : { total: 0, active: false })); return;
    }
    // the cast row's "load the md file" ask (human 2026-09-21): the panel only ever showed the
    // brief `desc`, never the creature's actual doc. Re-harvests fresh (never cached) and only
    // ever reads the docPath harvest() itself resolved for a name in *this* world's own
    // creature/mind list — the query string is a lookup key into that trusted list, never a
    // path. Extended to Minds ("do same for skills") — same SKILL.md read, same safety.
    //
    // 2026-09-21 (payload B, changelog 6): DOCS RIDE THE WIRE, LAZY BY ANCHOR. The route no
    // longer dumps the whole file. Its default answer is the SECTION MAP — the wire's own
    // envelope as JSON keys: `@S:'MAP'`, `@P` path, `@E` measured bytes, `@F` one entry per
    // heading `{n, t, b}` (index, title, bytes) — the compass's "point, don't carry". `&sec=N`
    // answers ONE section body: `@S:'SEC'`, `@T` title, `@E` bytes, `text`. Failure is
    // `@S:'FAIL'` + `@?` (a hole, named). The pipe-dense raw form (`S|MAP|…`) was considered
    // and REJECTED for this route under the anti-wire clause: JSON.parse is the client's
    // native decoder, a bespoke pipe grammar would cost more to decode than it saves in bytes
    // — tokens, not eyes. Names: `<creature>`, `<mind>`, `<creature>@hat` (the hat's doc IS its
    // body's doc), `<command>@cmd` (row 2's host commands). The sender's server-side body for
    // this route was never photographed; this is rebuilt from the (fully visible) client
    // contract — loadDoc/paintDoc in render() — and the changelog.
    if (req.method === 'GET' && action === 'doc') {
      const nm = url.searchParams.get('name') || '';
      const secQ = url.searchParams.get('sec');
      const d = harvest(root);
      const base = nm.replace(/@hat$/, '');
      const c = nm.endsWith('@cmd')
        ? (d.commands || []).find(x => x.name === nm.slice(0, -4))
        : d.creatures.find(x => x.name === base) || d.minds.find(x => x.name === base);
      const wire = (code, obj) => { res.writeHead(code, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(obj)); };
      if (!c || !c.docPath) return wire(404, { '@S': 'FAIL', '@?': 'unknown creature or no doc file' });
      let text;
      try { text = fs.readFileSync(c.docPath, 'utf8'); }
      catch (e) { return wire(404, { '@S': 'FAIL', '@?': 'doc file unreadable: ' + e.message }); }
      const map = sectionMap(text);
      if (secQ !== null) {
        const n = parseInt(secQ, 10);
        const s = map.sections[n - 1];
        if (!s) return wire(404, { '@S': 'FAIL', '@?': `no section ${esc(secQ)} — the map has ${map.sections.length}` });
        return wire(200, { '@S': 'SEC', '@N': s.n, '@T': s.t, '@E': s.b, text: s.text });
      }
      return wire(200, { '@S': 'MAP', '@P': path.relative(root, c.docPath), '@E': Buffer.byteLength(text),
        '@F': map.sections.map(s => ({ n: s.n, t: s.t, b: s.b })), preamble: map.preamble });
    }
    res.writeHead(404, { 'Content-Type': 'text/plain' }); res.end('not found');
  });
  server.listen(PORT, '127.0.0.1', () => process.stdout.write(`tempest — http://localhost:${PORT}/\n`));
  if (!IMMORTAL) {
    setInterval(() => { if ((Date.now() - lastTouch) >= TTL_MS) { process.stdout.write(`tempest sleeping (${TTL_MIN}m silence)\n`); process.exit(0); } }, 60000).unref();
  }
}
