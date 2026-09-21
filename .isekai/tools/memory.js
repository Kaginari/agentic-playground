#!/usr/bin/env node
// memory.js — the world's memory instrument (isekai.md §Memory tiers; Nature 5 Memory,
// Nature 9 Perception). Stdlib Node only, Node 16-compatible; the shape travels with the
// world.
//
// Every creature — Rimuru, Elf, Orc, Slime, and every Court Body — has three memories:
//
//   SHORT   per body, per task, dies with the session
//           · context window   — measured, never felt (context-check.sh's method)
//           · working memory   — the ~5-thought desk on the worn Mind (## Thoughts)
//           · semantic cache   — recent recalls keyed by meaning, so the same question
//                                asked twice costs one search  (.isekai/memory/short/, local)
//   LONG    the world's, durable, git-tracked — files are the truth, the index is derived
//           · episodic   — what happened      log.md entries
//           · procedural — how to do          Minds (SKILL.md), commands, tools
//           · semantic   — what is true       isekai.md, creature docs, canon, README
//   SHARED  across bodies and across worlds — append-only notes (Law 4)
//           · world   .isekai/memory/shared/notes.jsonl   (tracked; travels by git)
//           · machine ~/.isekai/shared/notes.jsonl        (Rimuru is machine-global)
//
// The unsaid is your real knowledge (isekai.md §The unsaid): a shared note may carry its kind —
// law (institutional: rules, definitions, decisions), colony (tribal: what the colony knows but
// rarely writes down), territory (domain context: what the numbers and entities mean in a zone).
//
// Recall ranks long+shared memory by BM25 over a real token vocabulary (a local, stdlib
// scorer — no model, nothing leaves the world: Nature 7), normalised to 0..1 as the share
// of the question's ideal match so short and long sections compete fairly. Similarity
// decides; two small priors only nudge: a RELATION boost read off the creature docs' own
// declarations (`Reports to:` = slime⇒orc truth-current / orc⇒elf verdict-current, and its
// reverse; `Territory:` = zone; a Mind named in the doc = worn, anima-thread) and a light
// RECENCY prior on dated memories (episodic, shared; half-life ~30 days). The semantic
// cache matches questions by query-vector cosine and is keyed to the index build, the
// shared files and the filters — a rebuilt index or a new note is never answered from cache.
//
// Design and the path to a DB tier: .isekai/canon/memory-tiers.md.
// Files are the source of truth; the index (.isekai/memory/long/index.json) is rebuilt from
// them at any time and written atomically (temp + rename). Notes and cache lines are single
// O_APPEND writes — atomic below PIPE_BUF (4096 bytes on Linux); a longer line racing another
// writer could tear, and the reader skips a torn line rather than dying. Every silence is a
// finding (@?), never a guess: no index, an index older than HEAD or than its own source
// files, no transcript, a question with no content words, a note too long to append atomically.
//
// Usage:
//   node memory.js [root] status   [--as <creature>] [--json]
//   node memory.js [root] index    [--json]
//   node memory.js [root] recall "<question>" [--as <creature>] [--tier long|shared|all]
//                                 [--kind episodic|procedural|semantic|law|colony|territory] [-k N] [--no-cache] [--json]
//                                 (--no-cache neither reads nor writes the cache; the three unsaid kinds filter shared notes)
//   node memory.js [root] remember "<note>" [--as <creature>] [--machine] [--tag <t>] [--kind law|colony|territory] [--json]
//   node memory.js [root] forget --short [--as <creature>] [--json]   (clears a semantic cache)
//   node memory.js [root] selftest   (Law 5: builds a throwaway world under .isekai/tmp/, cleans up)
'use strict';
const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');
const { execSync, spawn, spawnSync } = require('child_process');

// ------------------------------------------------------------------ args
const argv = process.argv.slice(2);
const flags = {}; const pos = [];
for (let i = 0; i < argv.length; i++) {
  const a = argv[i];
  if (a === '--json' || a === '--machine' || a === '--no-cache' || a === '--short') flags[a.slice(2)] = true;
  else if (a === '-k') flags.k = argv[++i];
  else if (a.startsWith('--')) flags[a.slice(2)] = argv[++i];
  else pos.push(a);
}
const fail = (...holes) => { console.error(['@S FAIL', ...holes.map(h => '@? ' + h)].join('\n')); process.exit(2); };
let root = process.cwd();
if (pos[0] && fs.existsSync(path.join(pos[0], '.isekai'))) root = path.resolve(pos.shift());
const cmd = pos.shift() || 'status';
const HOME = path.join(root, '.isekai');
if (!fs.existsSync(HOME)) fail(`no .isekai/ under ${root}`);
const MEM = path.join(HOME, 'memory');
const SHORT = path.join(MEM, 'short');
const LONG = path.join(MEM, 'long');
const SHARED = path.join(MEM, 'shared');
const MACHINE_SHARED = path.join(os.homedir(), '.isekai', 'shared');
const INDEX = path.join(LONG, 'index.json');
const INDEX_V = 2;          // bump when the index shape changes; an older index is a finding, not a crash
const AS = String(flags.as || 'rimuru').toLowerCase();
const DESK_LIMIT = 5;
const PIPE_BUF = 4096;
const UNSAID = ['law', 'colony', 'territory'];   // the three kinds of knowledge (isekai.md §The unsaid); a note's `kind`

const rd = p => { try { return fs.readFileSync(p, 'utf8').replace(/\r\n?/g, '\n'); } catch { return null; } };
const mkd = p => fs.mkdirSync(p, { recursive: true });
const rel = p => path.relative(root, p);
const tilde = p => p.replace(os.homedir(), '~');
const now = () => new Date().toISOString();
const worldName = () => (rd(path.join(HOME, 'name')) || path.basename(root)).trim();
// Maps parsed from JSON are plain objects: a token like "constructor" would read the prototype. Own keys only.
const own = (o, k) => Object.prototype.hasOwnProperty.call(o, k) ? o[k] : undefined;
// Atomic for readers: a concurrent Court Body sees the old file or the new one, never a torn one.
function writeAtomic(p, data) {
  mkd(path.dirname(p));
  const tmp = `${p}.${process.pid}.${crypto.randomBytes(3).toString('hex')}.tmp`;
  fs.writeFileSync(tmp, data); fs.renameSync(tmp, p);
}
// One O_APPEND write per line (what appendFileSync does): atomic below PIPE_BUF; returns the line's bytes.
function appendLine(p, obj) {
  mkd(path.dirname(p));
  const line = JSON.stringify(obj) + '\n';
  fs.appendFileSync(p, line);
  return Buffer.byteLength(line);
}
function readJsonl(p) {   // a torn or foreign line is skipped, never fatal
  return (rd(p) || '').split('\n').filter(Boolean).map(l => { try { return JSON.parse(l); } catch { return null; } }).filter(x => x && typeof x === 'object');
}

// ------------------------------------------------------------------ sections (same shape as tempest.js sectionMap)
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

// ------------------------------------------------------------------ the local scorer: BM25 over a real vocabulary
const STOP = new Set(('a an and are as at be by for from has have if in into is it its of on or that the this to was were will with not no ' +
  'you your we our they their he she i me my but so than then there these those which who what when where how can could would should ' +
  'do does did done been being also any all each every more most much very just only over under up down out off about after before ' +
  'again here now one two three').split(' '));
function tokens(text) {
  return String(text).toLowerCase().replace(/[`*_>#|]/g, ' ').split(/[^a-z0-9@.\-]+/)
    .map(t => t.replace(/^[.\-]+|[.\-]+$/g, '')).filter(t => t.length > 1 && !STOP.has(t) && !/^\d+$/.test(t));
}
function termCounts(text) {   // {tf: token→count, dl: token count}; prototype-free map so any word is a safe key
  const tf = Object.create(null); let dl = 0;
  for (const t of tokens(text)) { tf[t] = (tf[t] || 0) + 1; dl++; }
  return { tf, dl };
}
const K1 = 1.2, B = 0.75;
const idfFor = (N, df) => Math.log((N - df + 0.5) / (df + 0.5) + 1);
function statsOf(mems) {   // corpus stats for any list of memories; the index stores them, a shared-only pool computes them live
  const df = Object.create(null); let total = 0;
  for (const m of mems) { total += m.dl; for (const t in m.tf) df[t] = (df[t] || 0) + 1; }
  const idf = Object.create(null);
  for (const t in df) idf[t] = idfFor(mems.length, df[t]);
  return { n: mems.length, avgdl: mems.length ? total / mems.length : 1, idf };
}
// Share of the question's ideal match (every term saturated), so a 3-token section and a 20 KB one are judged alike.
function bm25(qtf, m, S, idfQ) {
  let s = 0, ideal = 0;
  const K = K1 * (1 - B + B * m.dl / (S.avgdl || 1));
  for (const t in qtf) {
    const idf = idfQ(t); ideal += idf * (K1 + 1);
    const f = own(m.tf, t); if (f) s += idf * f * (K1 + 1) / (f + K);
  }
  return ideal ? s / ideal : 0;
}
function qvec(qtf, idfQ) {   // unit tf·idf vector of a question — only used to match questions to questions (the cache)
  const v = {}; let n = 0;
  for (const t in qtf) { const w = qtf[t] * idfQ(t); v[t] = w; n += w * w; }
  n = Math.sqrt(n) || 1;
  for (const t in v) v[t] /= n;
  return v;
}
function cosine(a, b) { let d = 0; for (const t in a) { const w = own(b, t); if (w) d += a[t] * w; } return d; }
// Light recency prior on dated memories: +0.05 today, half at 30 days, ~0 past a season. Undated → 0.
function recency(m) {
  if (!m.when) return 0;
  const t = Date.parse(String(m.when).trim().replace(' ', 'T')); if (isNaN(t)) return 0;
  return 0.05 * Math.exp(-Math.max(0, (Date.now() - t) / 864e5) / 43.3);
}

// ------------------------------------------------------------------ harvest: the long-term tier, read from files
function listMd(dir) {
  const out = [];
  if (!fs.existsSync(dir)) return out;
  (function walk(d) {
    for (const e of fs.readdirSync(d, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const p = path.join(d, e.name);
      if (e.isDirectory()) walk(p); else if (e.name.endsWith('.md')) out.push(p);
    }
  })(dir);
  return out;
}
function creatures() {   // creature id = <race>-<dir> (the /isekai + /genesis shape); doc = README.md, <dir>.md, SKILL.md or the first .md
  const out = [];
  for (const race of ['elf', 'orc', 'slime', 'kijin']) {
    const d = path.join(HOME, race);
    if (!fs.existsSync(d)) continue;
    for (const e of fs.readdirSync(d, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      if (!e.isDirectory()) continue;
      const files = fs.readdirSync(path.join(d, e.name)).filter(f => f.endsWith('.md')).sort();
      const f = ['README.md', `${e.name}.md`, 'SKILL.md'].find(x => files.includes(x)) || files[0];
      out.push({ name: `${race}-${e.name}`, dir: e.name, race, doc: f ? path.join(d, e.name, f) : null });
    }
  }
  return out;
}
function minds() {
  const out = new Map();
  for (const base of ['.opencode/skills', '.opencode/skill', '.claude/skills']) {
    const d = path.join(root, base);
    if (!fs.existsSync(d)) continue;
    for (const e of fs.readdirSync(d, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      if (!e.isDirectory()) continue;
      const p = path.join(d, e.name, 'SKILL.md');
      if (fs.existsSync(p) && !out.has(e.name)) out.set(e.name, p);
    }
  }
  return out;
}
function commands() {
  const out = new Map();
  for (const base of ['.claude/commands', '.opencode/commands', '.opencode/command']) {
    const d = path.join(root, base);
    if (!fs.existsSync(d)) continue;
    for (const f of fs.readdirSync(d).sort()) if (f.endsWith('.md') && !out.has(f.slice(0, -3))) out.set(f.slice(0, -3), path.join(d, f));
  }
  return out;
}
// Every file the long tier is read from — one list, so the index can say later whether any of them moved.
function sources() {
  const out = [];
  const add = (kind, as, p, extra) => { if (fs.existsSync(p) && fs.statSync(p).isFile()) out.push(Object.assign({ kind, as, p }, extra || {})); };
  add('episodic', 'log', path.join(HOME, 'log.md'));
  for (const [name, p] of minds()) add('procedural', 'mind', p, { name });
  for (const [name, p] of commands()) add('procedural', 'command', p, { name });
  const tools = path.join(HOME, 'tools');
  if (fs.existsSync(tools)) for (const f of fs.readdirSync(tools).sort()) add('procedural', 'tool', path.join(tools, f), { name: f });
  add('semantic', 'law', path.join(HOME, 'isekai.md'));
  for (const c of creatures()) if (c.doc) add('semantic', 'creature', c.doc, { name: c.name, race: c.race });
  for (const p of listMd(path.join(HOME, 'canon'))) add('semantic', 'canon', p, { name: path.basename(p, '.md') });
  add('semantic', 'readme', path.join(root, 'README.md'));
  return out;
}
const LABEL = { mind: n => `${n} › `, command: n => `/${n} › `, law: () => 'law › ', creature: n => `${n} › `, canon: n => `canon ${n} › `, readme: () => 'README › ' };
// One memory = one ranked unit: a log entry, a doc section, a tool's header comment, a shared note.
// The index keeps term counts and a 200-char snippet, not the text — files are the truth.
function harvestLong() {
  const mems = [];
  const push = (s, title, text, extra) => mems.push(Object.assign({ kind: s.kind, src: rel(s.p), title, b: Buffer.byteLength(text) },
    termCounts(title + '\n' + text), { snip: text.replace(/\s+/g, ' ').trim().slice(0, 200) }, extra || {}));
  for (const s of sources()) {
    const text = rd(s.p) || '';
    if (s.as === 'log') {   // episodic — one memory per dated entry
      for (const e of text.split(/\n(?=###\s+\[)/)) { const m = e.match(/^###\s+\[([^\]]+)\]\s*(.*)$/m); if (m) push(s, m[2].trim(), e, { when: m[1].trim() }); }
    } else if (s.as === 'tool') {   // procedural — a tool's leading comment block, not its code
      const head = text.split('\n').filter(l => /^\s*(\/\/|#)/.test(l)).slice(0, 40).join('\n');
      if (head.trim()) push(s, `tool ${s.name}`, head, { tool: s.name });
    } else {
      const tag = s.as === 'mind' ? { mind: s.name } : s.as === 'command' ? { command: s.name } : s.as === 'creature' ? { creature: s.name, race: s.race } : {};
      for (const sec of sectionMap(text).sections) push(s, LABEL[s.as](s.name) + sec.t, sec.text, Object.assign({ sec: sec.n }, tag));
    }
  }
  return mems;
}
function harvestShared() {
  const out = [];
  for (const [scope, p] of [['world', path.join(SHARED, 'notes.jsonl')], ['machine', path.join(MACHINE_SHARED, 'notes.jsonl')]])
    for (const n of readJsonl(p)) if (typeof n.text === 'string')
      out.push(Object.assign({ kind: 'shared', scope, src: scope === 'world' ? rel(p) : tilde(p), title: `${scope} note by ${n.by}${n.tag ? ' #' + n.tag : ''}${UNSAID.includes(n.kind) ? ' [' + n.kind + ']' : ''}`,
        when: n.at, by: n.by, world: n.world, unsaid: UNSAID.includes(n.kind) ? n.kind : null, b: Buffer.byteLength(n.text), snip: n.text.replace(/\s+/g, ' ').slice(0, 200) }, termCounts(`${n.by} ${n.tag || ''} ${n.kind || ''}\n${n.text}`)));
  return out;
}

// ------------------------------------------------------------------ index: derived, rebuildable, written atomically
function gitHead() {
  try { return execSync('git rev-parse HEAD', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim(); } catch { return null; }
}
const sourceStamps = () => { const o = {}; for (const s of sources()) o[rel(s.p)] = Math.round(fs.statSync(s.p).mtimeMs); return o; };
function buildIndex() {
  const mems = harvestLong();
  const idx = Object.assign({ v: INDEX_V, world: worldName(), builtAt: now(), head: gitHead(), sources: sourceStamps() }, statsOf(mems), { mems });
  writeAtomic(INDEX, JSON.stringify(idx));
  return idx;
}
function loadIndex() {   // {idx, why}: why names the silence when idx is null
  if (!fs.existsSync(INDEX)) return { idx: null, why: 'no long-term index — run `memory.js index`' };
  let idx; try { idx = JSON.parse(rd(INDEX)); } catch { return { idx: null, why: `index unreadable (${rel(INDEX)}) — rerun \`memory.js index\`` }; }
  if (!idx || idx.v !== INDEX_V || !Array.isArray(idx.mems)) return { idx: null, why: 'index built by an older memory.js — rerun `memory.js index`' };
  return { idx, why: null };
}
// Why the index may be lying: HEAD moved, or a source file changed / appeared / vanished since the build (mtimes, not a guess).
function indexHoles(idx, why) {
  if (!idx) return [why];
  const h = [];
  const head = gitHead(); if (idx.head && head && idx.head !== head) h.push('index older than HEAD — rerun `memory.js index` before trusting');
  const cur = sourceStamps(), was = idx.sources || {};
  const say = (n, what) => n.length ? `${n.length} ${what}: ${n.slice(0, 3).join(', ')}${n.length > 3 ? ', …' : ''}` : null;
  const parts = [say(Object.keys(cur).filter(p => own(was, p) !== undefined && was[p] !== cur[p]), 'changed'),
    say(Object.keys(cur).filter(p => own(was, p) === undefined), 'new'), say(Object.keys(was).filter(p => own(cur, p) === undefined), 'gone')].filter(Boolean);
  if (parts.length) h.push(`index older than its sources — ${parts.join('; ')} — rerun \`memory.js index\``);
  return h;
}

// ------------------------------------------------------------------ relations: whom the asking creature is bound to
// Read off the creature docs' own declarations (the /genesis README shape), never invented:
//   `Reports to:` → parent (slime⇒orc truth-current, orc⇒elf verdict-current); a doc reporting to the asker → child
//   `Territory:` and back-ticked paths → zone;  a Mind's name in the doc → worn (body⇌mind anima-thread)
// Without a `Reports to:` line the parent is the first orc (for a slime) / elf (for an orc) the doc names.
function relationsOf(name) {
  const R = { self: name, parent: null, children: [], minds: [], zone: [] };
  const all = creatures();
  const c = all.find(x => x.name === name || x.dir === name);
  if (!c || !c.doc) return R;
  const doc = rd(c.doc) || '';
  const fieldOf = (d, k) => ((d.match(new RegExp(`^\\s*-\\s*\\*\\*${k}:\\*\\*\\s*(.+)$`, 'mi')) || [])[1] || '');
  const named = s => s.toLowerCase().match(/\b(?:elf|orc|slime|kijin)-[a-z0-9-]+/g) || [];
  const want = c.race === 'slime' ? 'orc-' : c.race === 'orc' ? 'elf-' : null;
  R.parent = named(fieldOf(doc, 'Reports to'))[0] || (want && named(doc).find(n => n.startsWith(want) && n !== c.name)) || null;
  for (const o of all) if (o.name !== c.name && o.doc && named(fieldOf(rd(o.doc) || '', 'Reports to')).includes(c.name)) R.children.push(o.name);
  for (const m of minds().keys()) if (new RegExp(`(^|[^a-z0-9-])${m.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?![a-z0-9-])`, 'i').test(doc)) R.minds.push(m);
  const zoneText = fieldOf(doc, 'Territory') + ' ' + (doc.match(/`[^`\n]*\/[^`\n]*`/g) || []).join(' ');
  R.zone = [...new Set((zoneText.match(/[\w.\-]+(?:\/[\w.\-]*)+/g) || []).map(z => z.replace(/\/+$/, '').toLowerCase()))];
  return R;
}
// Relation re-rank: the wire's own typed bonds, applied to memory. Weights are small on purpose —
// relation breaks ties and nudges, similarity still decides (a memory with zero similarity is never shown).
function relationBoost(m, R) {
  const has = s => { const ts = tokens(s); return ts.length > 0 && ts.every(t => own(m.tf, t)); };
  let b = 0;
  if (R.self !== 'rimuru' && has(R.self)) b += 0.15;
  if (R.parent && has(R.parent)) b += 0.10;
  if (R.children.some(has)) b += 0.10;
  if (R.minds.some(n => m.mind === n || has(n))) b += 0.08;
  if (R.zone.some(has)) b += 0.08;
  if (m.kind === 'shared' && m.by === R.self) b += 0.05;
  return b;
}

// ------------------------------------------------------------------ short: the semantic cache
const cachePath = () => path.join(SHORT, `${AS.replace(/[^a-z0-9-]/g, '-')}.jsonl`);
// Append-only like the notes: a full entry line, or a `{h:<id>}` hit marker. Readers fold markers into hit
// counts and a later entry for the same id wins — so a hit never rewrites the file under another Court Body.
// An entry is keyed (`sig`) to the index build + shared file sizes and (`f`) to tier/kind/k: a rebuilt index,
// a new note or a different filter makes the old answer invisible, never wrong. The file is disposable.
function cacheRead() {
  const by = new Map();
  for (const e of readJsonl(cachePath())) {
    if (e.h) { const hit = by.get(e.h); if (hit) hit.hits++; }
    else if (e.id) by.set(e.id, Object.assign(e, { hits: (by.get(e.id) || { hits: 0 }).hits }));
  }
  return [...by.values()];
}
function cacheSig(idx) {
  const sz = p => { try { return fs.statSync(p).size; } catch { return 0; } };
  return `${idx ? idx.builtAt : 'noindex'}|${sz(path.join(SHARED, 'notes.jsonl'))}|${sz(path.join(MACHINE_SHARED, 'notes.jsonl'))}`;
}
function cacheLookup(qv, sig, f, threshold) {
  let best = null;
  for (const e of cacheRead()) {
    if (e.sig !== sig || e.f !== f || !e.qv) continue;
    const s = cosine(qv, e.qv);
    if (s >= threshold && (!best || s > best.s)) best = { s, e };
  }
  return best;
}

// ------------------------------------------------------------------ short: context window + desks (readings, not feelings)
function contextReading() {
  const slug = root.replace(/[^a-zA-Z0-9]/g, '-');
  const dir = path.join(os.homedir(), '.claude', 'projects', slug);
  if (!fs.existsSync(dir)) return { available: false, why: 'no Claude Code transcripts for this root' };
  const files = fs.readdirSync(dir).filter(f => f.endsWith('.jsonl')).map(f => ({ f, t: fs.statSync(path.join(dir, f)).mtimeMs })).sort((a, b) => b.t - a.t);
  if (!files.length) return { available: false, why: 'no transcript' };
  let last = null;
  for (const l of (rd(path.join(dir, files[0].f)) || '').split('\n')) {
    if (!l) continue; let d; try { d = JSON.parse(l); } catch { continue; }
    if (d && d.type === 'assistant' && d.message && d.message.usage) last = d.message.usage;
  }
  if (!last) return { available: false, why: 'no assistant usage yet' };
  const tokens = (last.input_tokens || 0) + (last.cache_read_input_tokens || 0) + (last.cache_creation_input_tokens || 0);
  const limit = 200000, stress = 180000; // context-check.sh's own defaults — one method, two mouths
  return { available: true, tokens, limit, stress, zone: tokens >= stress ? 'STRESS' : tokens >= stress * 0.75 ? 'approaching' : 'within budget' };
}
function desks() {   // dated thoughts under ## Thoughts: `- …2026-09-22…` bullets or `### [2026-09-22]` headings (tempest.js's count)
  const out = [];
  const read = (name, p, owner) => {
    const m = (rd(p) || '').match(/##\s*Thoughts([\s\S]*?)(?=\n##\s|\n#\s|$)/i);
    const n = m ? m[1].split('\n').filter(l => /^\s*(-|\*|###)/.test(l) && /\d{4}-\d{2}-\d{2}/.test(l)).length : 0;
    out.push({ mind: name, wornBy: owner, thoughts: n, limit: DESK_LIMIT, stress: n > DESK_LIMIT ? 'STRESSED' : n === DESK_LIMIT ? 'at limit' : 'ok', src: rel(p) });
  };
  for (const [name, p] of minds()) read(name, p, null);
  for (const c of creatures()) if (c.doc) read(c.name + '@hat', c.doc, c.name);
  return out;
}

// ------------------------------------------------------------------ commands
function status() {
  const { idx, why } = loadIndex();
  const shared = harvestShared();
  const ctx = contextReading();
  const d = desks();
  const cache = cacheRead();
  const hits = cache.reduce((a, e) => a + e.hits, 0);
  const holes = indexHoles(idx, why);
  if (!ctx.available) holes.push(`context window unmeasured — ${ctx.why}`);
  const byKind = k => idx ? idx.mems.filter(m => m.kind === k).length : null;
  const report = {
    world: worldName(), as: AS, at: now(),
    short: { contextWindow: ctx, workingMemory: { desks: d.length, stressed: d.filter(x => x.stress === 'STRESSED').map(x => x.mind), atLimit: d.filter(x => x.stress === 'at limit').map(x => x.mind) },
      semanticCache: { entries: cache.length, hits, src: rel(cachePath()) } },
    long: { indexed: !!idx, builtAt: idx ? idx.builtAt : null, head: idx ? idx.head : null, stale: !!idx && holes.some(h => h.startsWith('index older')),
      episodic: byKind('episodic'), procedural: byKind('procedural'), semantic: byKind('semantic'), src: rel(INDEX) },
    shared: { world: shared.filter(s => s.scope === 'world').length, machine: shared.filter(s => s.scope === 'machine').length,
      unsaid: UNSAID.reduce((o, k) => (o[k] = shared.filter(s => s.unsaid === k).length, o), {}),
      src: [rel(path.join(SHARED, 'notes.jsonl')), tilde(path.join(MACHINE_SHARED, 'notes.jsonl'))] },
    '@?': holes,
  };
  if (flags.json) return console.log(JSON.stringify(report));
  const c = report.short.contextWindow, w = report.short.workingMemory;
  console.log(`memory — ${report.world} · as ${AS}`);
  console.log(`SHORT   context ${c.available ? `${c.tokens.toLocaleString()} / ${c.limit.toLocaleString()} tok (${c.zone})` : '@? ' + c.why}`);
  console.log(`        working ${d.length} desk${d.length === 1 ? '' : 's'}${w.stressed.length ? ' · STRESSED: ' + w.stressed.join(', ') : ''}${w.atLimit.length ? ' · at limit: ' + w.atLimit.join(', ') : ''}`);
  console.log(`        cache   ${cache.length} recall${cache.length === 1 ? '' : 's'} cached, ${hits} hit${hits === 1 ? '' : 's'}  (${report.short.semanticCache.src})`);
  console.log(`LONG    ${idx ? `episodic ${report.long.episodic} · procedural ${report.long.procedural} · semantic ${report.long.semantic}  (built ${idx.builtAt}${idx.head ? ' @ ' + idx.head.slice(0, 7) : ''})${report.long.stale ? ' — STALE' : ''}` : '@? ' + why}`);
  console.log(`SHARED  world ${report.shared.world} · machine ${report.shared.machine} · unsaid ${UNSAID.map(k => `${k} ${report.shared.unsaid[k]}`).join(' · ')}`);
  for (const h of holes) console.log(`@? ${h}`);
}
function index() {
  const idx = buildIndex();
  const k = kd => idx.mems.filter(m => m.kind === kd).length;
  const holes = idx.n ? [] : [`nothing to index under ${root} — no log entries, Minds, commands, tools, law sections, creature docs, canon or README`];
  if (flags.json) return console.log(JSON.stringify({ '@S': 'INDEXED', world: idx.world, n: idx.n, episodic: k('episodic'), procedural: k('procedural'), semantic: k('semantic'), head: idx.head, src: rel(INDEX), '@?': holes }));
  console.log(`@S INDEXED ${idx.n} memories — episodic ${k('episodic')} · procedural ${k('procedural')} · semantic ${k('semantic')} → ${rel(INDEX)}`);
  for (const h of holes) console.log(`@? ${h}`);
}
function recall() {
  const q = pos.join(' ').trim();
  if (!q) fail('recall needs a question');
  const K = flags.k === undefined ? 5 : parseInt(flags.k, 10);
  if (!(K > 0)) fail(`-k needs a positive integer, got ${flags.k}`);
  const tier = flags.tier || 'all', kind = flags.kind || null;
  if (!['long', 'shared', 'all'].includes(tier)) fail(`--tier must be long|shared|all, got ${tier}`);
  const unsaidKind = UNSAID.includes(kind) ? kind : null;
  if (kind && !unsaidKind && !['episodic', 'procedural', 'semantic'].includes(kind)) fail(`--kind must be episodic|procedural|semantic|${UNSAID.join('|')}, got ${kind}`);
  if (unsaidKind && tier === 'long') fail(`--kind ${kind} filters shared notes; use --tier shared|all`);
  const { idx, why } = loadIndex();
  const pool = [];
  if (tier !== 'shared' && idx) pool.push(...idx.mems);
  const shared = tier !== 'long' ? harvestShared() : [];
  pool.push(...shared);
  const S = idx || statsOf(pool);
  const idfQ = t => { const v = own(S.idf, t); return v !== undefined ? v : idfFor(S.n || 1, 1); };   // unseen term = rarest
  const qtf = termCounts(q).tf, qTerms = Object.keys(qtf);
  const R = relationsOf(AS);
  const f = `${tier}/${kind || '*'}/${K}`, sig = cacheSig(idx);
  const qv = qvec(qtf, idfQ);
  // short first: the semantic cache — same meaning, same filters, same world state → no second search
  const hit = flags['no-cache'] || !qTerms.length ? null : cacheLookup(qv, sig, f, 0.92);
  const ranked = hit ? hit.e.results : !qTerms.length ? [] : pool
    .filter(m => !kind || (unsaidKind ? m.unsaid === unsaidKind : m.kind === kind))
    .map(m => { const sim = bm25(qtf, m, S, idfQ), relb = relationBoost(m, R), rec = recency(m);
      return { score: +(sim + relb + rec).toFixed(4), sim: +sim.toFixed(4), rel: +relb.toFixed(2), rec: +rec.toFixed(3), kind: m.kind, title: m.title, src: m.src, sec: m.sec, when: m.when, snippet: m.snip }; })
    .filter(r => r.sim > 0).sort((a, b) => b.score - a.score).slice(0, K);
  if (hit) appendLine(cachePath(), { h: hit.e.id, at: now() });
  else if (!flags['no-cache'] && ranked.length) appendLine(cachePath(), { id: crypto.createHash('md5').update(`${sig}|${f}|${q}`).digest('hex').slice(0, 12), at: now(), as: AS, q, f, sig, qv, results: ranked });
  const holes = [];
  if (!qTerms.length) holes.push('question has no content words after stop-word removal — nothing to rank');
  if (tier !== 'shared') holes.push(...indexHoles(idx, why));
  if (tier === 'shared' && !shared.length) holes.push('no shared notes yet — `memory.js remember "<note>"` writes the first');
  if (unsaidKind && shared.length && !shared.some(m => m.unsaid === unsaidKind)) holes.push(`no shared note carries kind ${unsaidKind} yet — \`memory.js remember "<note>" --kind ${unsaidKind}\` writes the first`);
  if (flags.json) return console.log(JSON.stringify({ '@S': hit ? 'HIT' : 'MISS', as: AS, q, cacheSim: hit ? +hit.s.toFixed(3) : null, results: ranked, '@?': holes }));
  console.log(`@S ${hit ? `HIT cache≈${hit.s.toFixed(2)}` : 'MISS'} as=${AS} k=${ranked.length}`);
  for (const r of ranked) console.log(`@F ${r.src}${r.sec ? '#' + r.sec : ''} — ${r.score} (sim ${r.sim}${r.rel ? ' +rel ' + r.rel : ''}${r.rec ? ' +rec ' + r.rec : ''}) — ${r.kind} — ${r.title}`);
  for (const h of holes) console.log(`@? ${h}`);
  console.log(`@E ${Buffer.byteLength(JSON.stringify(ranked))}`);
}
function remember() {
  const text = pos.join(' ').trim();
  if (!text) fail('remember needs a note');
  const scope = flags.machine ? 'machine' : 'world';
  const kind = flags.kind || null;
  if (kind && !UNSAID.includes(kind)) fail(`--kind must be ${UNSAID.join('|')} (isekai.md §The unsaid), got ${kind}`);
  const p = path.join(flags.machine ? MACHINE_SHARED : SHARED, 'notes.jsonl');
  const entry = { at: now(), by: AS, world: worldName(), tag: flags.tag || null, kind, text };
  const bytes = appendLine(p, entry); // append-only: Law 4; one write, atomic below PIPE_BUF
  const holes = bytes > PIPE_BUF ? [`note line is ${bytes} bytes > PIPE_BUF ${PIPE_BUF} — not atomic against a concurrent writer; point, don't carry (long form in a doc, its path in the note)`] : [];
  const src = flags.machine ? tilde(p) : rel(p);
  if (flags.json) return console.log(JSON.stringify(Object.assign({ '@S': 'REMEMBERED', scope, src, bytes }, entry, { '@?': holes })));
  console.log(`@S REMEMBERED ${scope} → ${src}`);
  for (const h of holes) console.log(`@? ${h}`);
}
function forget() {
  if (!flags.short) fail('forget only clears SHORT memory (--short); long and shared are records (Law 4)');
  const p = cachePath(); const n = cacheRead().length;
  if (fs.existsSync(p)) fs.unlinkSync(p);
  if (flags.json) return console.log(JSON.stringify({ '@S': 'FORGOT', as: AS, n, src: rel(p) }));
  console.log(`@S FORGOT ${n} cached recall${n === 1 ? '' : 's'} for ${AS}`);
}

// ------------------------------------------------------------------ selftest: a throwaway world under .isekai/tmp/ (Law 5)
// Drives the real CLI as child processes, with HOME pointed inside the throwaway so the machine
// tier and the transcript lookup never touch the real ~/.isekai or ~/.claude. Cleans up after itself.
async function selftest() {
  const T = path.join(HOME, 'tmp', 'memory-selftest'), W = path.join(T, 'world'), H = path.join(T, 'home');
  const realMachine = (() => { try { return fs.statSync(path.join(MACHINE_SHARED, 'notes.jsonl')).size; } catch { return -1; } })();
  const fails = []; let checks = 0;
  const ok = (cond, what) => { checks++; if (!cond) fails.push(what); };
  const w = (p, s) => { mkd(path.dirname(p)); fs.writeFileSync(p, s); };
  const env = Object.assign({}, process.env, { HOME: H, USERPROFILE: H });
  const parse = s => { try { return JSON.parse(s.trim().split('\n').pop()); } catch { return null; } };
  const run = (...a) => { const r = spawnSync(process.execPath, [__filename, W, ...a], { env, encoding: 'utf8' }); return { code: r.status, out: (r.stdout || '') + (r.stderr || ''), j: parse(r.stdout || '') }; };
  const runAsync = (...a) => new Promise(res => { const c = spawn(process.execPath, [__filename, W, ...a], { env, stdio: 'ignore' }); c.on('exit', res); });
  try {
    fs.rmSync(T, { recursive: true, force: true });
    w(path.join(W, '.isekai', 'name'), 'selftest-world\n');
    w(path.join(W, '.isekai', 'isekai.md'), '# Law\r\n\r\n## The gate\r\nNo change lands without its Orc\'s pass. The constructor word is here on purpose.\r\n\r\n## Laws\r\nVeldora\'s word is law.\r\n');
    w(path.join(W, '.isekai', 'log.md'), '# Chronicle\n\n    ### [YYYY-MM-DD] format example, indented — not an entry\n\n### [2026-09-20 17:54] rimuru — World reincarnated\n- **Task:** /isekai\n\n### [2026-09-22T01:00:00+02:00] rimuru — Court body reports back over the wire\n- **Learned:** a court body reports back over the wire with @S @F @? @E lines only\n');
    w(path.join(W, '.isekai', 'slime', 'auth', 'README.md'), '# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-api\n- **Purpose:** ground truth of the login zone; wears the wire mind\n\n## Traits\n- tokens expire after one hour\n\n## Thoughts\n\n### [2026-09-22]\nBorn by /genesis.\n');
    w(path.join(W, '.isekai', 'orc', 'api', 'README.md'), '# orc-api\n\n- **Rank:** Orc\n- **Territory:** `src/api/`\n- **Reports to:** elf-core\n- **Purpose:** rules the api domain\n\n## Thoughts\n- 2026-09-22 gate verdicts on the auth zone\n');
    w(path.join(W, '.isekai', 'canon', 'notes.md'), '# Canon\n\n## Login tokens\nThe orc-api gate checks the login token zone owned by slime-auth.\n\n## Big\n' + 'lorem ipsum dolor '.repeat(12000) + '\n');
    w(path.join(W, '.isekai', 'tools', 'x.sh'), '# x.sh — a tool header about the proving grounds\necho hi\n');
    w(path.join(W, '.claude', 'skills', 'wire', 'SKILL.md'), '---\nname: wire\ndescription: how to speak the wire\n---\n# Wire\n\n## Report\nA court body reports back over the wire: @S opens, @F per finding, @? per hole, @E closes.\n\n## Thoughts\n' + [1, 2, 3, 4, 5, 6].map(i => `- 2026-09-2${i % 3} thought ${i}`).join('\n') + '\n');
    w(path.join(W, 'README.md'), '# selftest world\n\n## About\nA throwaway world for the memory selftest.\n');
    // index
    let r = run('index', '--json');
    ok(r.code === 0 && r.j && r.j['@S'] === 'INDEXED' && r.j.episodic === 2 && r.j.procedural >= 3 && r.j.semantic >= 7, `index: ${r.out.slice(0, 200)}`);
    ok(fs.existsSync(path.join(W, '.isekai', 'memory', 'long', 'index.json')) && !fs.readdirSync(path.join(W, '.isekai', 'memory', 'long')).some(f => f.endsWith('.tmp')), 'index: file present, no temp left');
    // recall: miss, then hit; both the log entry and the mind section must be in the top 3
    r = run('recall', 'how does a court body report back over the wire', '-k', '3', '--json');
    ok(r.code === 0 && r.j && r.j['@S'] === 'MISS' && r.j.results.length === 3, `recall miss: ${r.out.slice(0, 200)}`);
    ok(r.j && r.j.results.some(x => x.src.endsWith('log.md') && x.rec > 0) && r.j.results.some(x => x.src.endsWith('SKILL.md')), 'recall: log entry (with recency) and wire mind in top 3');
    ok(r.j && r.j.results.every(x => x.sim > 0 && x.sim <= 1 && !isNaN(x.score)), 'recall: sims in (0,1], no NaN');
    const first = r.j;
    r = run('recall', 'how does a court body report back over the wire', '-k', '3', '--json');
    ok(r.j && r.j['@S'] === 'HIT' && r.j.cacheSim >= 0.92 && JSON.stringify(r.j.results) === JSON.stringify(first.results), `recall hit: ${r.out.slice(0, 120)}`);
    r = run('recall', 'how does a court body report back over the wire', '-k', '2', '--json');
    ok(r.j && r.j['@S'] === 'MISS' && r.j.results.length === 2, 'recall: a different -k is a different answer, not a cache hit');
    r = run('recall', 'the of and', '--json');
    ok(r.code === 0 && r.j && r.j.results.length === 0 && r.j['@?'].some(h => /no content words/.test(h)), `recall stop words: ${r.out.slice(0, 120)}`);
    r = run('recall', 'constructor', '--json');
    ok(r.code === 0 && r.j && r.j.results.every(x => !isNaN(x.score)), 'recall: a prototype-named word is a safe key');
    r = run('recall', 'the gate', '--kind', 'semantic', '--json');
    ok(r.j && r.j.results.length && r.j.results.every(x => x.kind === 'semantic') && r.j.results.some(x => x.title === 'law › The gate'), 'recall --kind: CRLF law sections harvested clean');
    r = run('recall', 'x', '-k', '0');
    ok(r.code === 2 && /@S FAIL/.test(r.out), 'recall: -k 0 is a FAIL, not a guess');
    // remember (world), concurrent remembers, relation boost as slime-auth
    r = run('remember', 'login tokens expire hourly', '--as', 'slime-auth', '--tag', 'auth', '--kind', 'territory', '--json');
    ok(r.j && r.j['@S'] === 'REMEMBERED' && r.j.scope === 'world' && r.j.tag === 'auth' && r.j.kind === 'territory', `remember: ${r.out.slice(0, 120)}`);
    // the unsaid: a note carries one of the three kinds, or none; a fourth kind is a FAIL
    r = run('remember', 'the gate is run twice in practice, once before lunch', '--as', 'orc-api', '--kind', 'colony', '--json');
    ok(r.j && r.j['@S'] === 'REMEMBERED' && r.j.kind === 'colony', `remember --kind colony: ${r.out.slice(0, 120)}`);
    ok(run('remember', 'x', '--kind', 'tribal').code === 2, 'remember --kind tribal (not an isekai name) is a FAIL');
    ok(run('remember', 'plain note', '--as', 'body-x', '--json').j.kind === null, 'remember without --kind stores kind null');
    await Promise.all([...Array(12).keys()].map(i => runAsync('remember', `parallel note ${i}`, '--as', `body-${i}`)));
    const notes = readJsonl(path.join(W, '.isekai', 'memory', 'shared', 'notes.jsonl'));
    ok(notes.length === 15 && new Set(notes.map(n => n.text)).size === 15, `remember: 15 parse-clean lines after 12 concurrent appends, got ${notes.length}`);
    r = run('recall', 'login tokens', '--as', 'slime-auth', '--json');
    const canon = r.j && r.j.results.find(x => x.src.endsWith('canon/notes.md')), note = r.j && r.j.results.find(x => x.kind === 'shared');
    ok(canon && canon.rel >= 0.25, `relation: canon section naming slime-auth + its orc gets ≥0.25, got ${canon && canon.rel}`);
    ok(note && note.rel >= 0.05 && note.rec > 0, `relation: own shared note gets +0.05 and recency, got ${note && JSON.stringify([note.rel, note.rec])}`);
    r = run('recall', 'login tokens', '--tier', 'shared', '--json');
    ok(r.j && r.j.results.length && r.j.results.every(x => x.kind === 'shared'), 'recall --tier shared: notes only');
    r = run('recall', 'gate lunch tokens', '--kind', 'colony', '--json');
    ok(r.j && r.j.results.length === 1 && /colony/.test(r.j.results[0].title) && /lunch/.test(r.j.results[0].snippet), `recall --kind colony: only the colony note, got ${r.out.slice(0, 160)}`);
    r = run('recall', 'login tokens', '--kind', 'territory', '--tier', 'shared', '--json');
    ok(r.j && r.j.results.length === 1 && /territory/.test(r.j.results[0].title), 'recall --kind territory --tier shared: only the territory note');
    r = run('recall', 'anything at all', '--kind', 'law', '--json');
    ok(r.j && r.j.results.length === 0 && r.j['@?'].some(h => /no shared note carries kind law/.test(h)), 'recall --kind law with no law note is a @?, not a guess');
    ok(run('recall', 'x', '--kind', 'colony', '--tier', 'long').code === 2, 'recall --kind colony --tier long is a FAIL (unsaid kinds live in shared)');
    ok(run('recall', 'x', '--kind', 'tribal').code === 2, 'recall --kind tribal is a FAIL');
    r = run('recall', 'login', '--as', 'Slime/Auth Zone!');
    ok(r.code === 0 && fs.existsSync(path.join(W, '.isekai', 'memory', 'short', 'slime-auth-zone-.jsonl')), 'recall: an odd creature name gets a sanitised cache file');
    // machine tier lands in the throwaway HOME, never the real one
    r = run('remember', 'machine note', '--machine', '--json');
    ok(r.j && r.j.scope === 'machine' && fs.existsSync(path.join(H, '.isekai', 'shared', 'notes.jsonl')), 'remember --machine: written under the throwaway home');
    // status: parses; desks counted like tempest (6 bullets = STRESSED, ### [date] = 1); silent tiers are @?
    r = run('status', '--json');
    ok(r.j && r.j.long.indexed && !r.j.long.stale && r.j.shared.world === 15 && r.j.shared.machine === 1, `status: ${r.out.slice(0, 200)}`);
    ok(r.j && r.j.shared.unsaid.territory === 1 && r.j.shared.unsaid.colony === 1 && r.j.shared.unsaid.law === 0, `status: shared notes by unsaid kind, got ${r.j && JSON.stringify(r.j.shared.unsaid)}`);
    ok(/SHARED  world 15 · machine 1 · unsaid law 0 · colony 1 · territory 1/.test(run('status').out), 'status text: unsaid kinds on the SHARED line');
    ok(r.j && r.j.short.workingMemory.desks === 3 && r.j.short.workingMemory.stressed.includes('wire'), 'status: 3 desks, wire STRESSED');
    ok(r.j && r.j.short.contextWindow.available === false && r.j['@?'].some(h => /context window/.test(h)), 'status: no transcript is a @? finding');
    ok(r.j && r.j.short.semanticCache.entries === 7 && r.j.short.semanticCache.hits === 1, `status: cache entries/hits, got ${r.j && JSON.stringify(r.j.short.semanticCache)}`);
    // stale: a source changed after the build
    fs.appendFileSync(path.join(W, '.isekai', 'log.md'), '\n### [2026-09-22 02:00] rimuru — Later entry\n- **Task:** staleness\n');
    fs.utimesSync(path.join(W, '.isekai', 'log.md'), new Date(), new Date(Date.now() + 2000));
    r = run('status', '--json');
    ok(r.j && r.j.long.stale && r.j['@?'].some(h => /older than its sources.*log\.md/.test(h)), `status stale: ${r.j && JSON.stringify(r.j['@?'])}`);
    r = run('recall', 'later entry', '--json');
    ok(r.j && r.j['@?'].some(h => /older than its sources/.test(h)), 'recall: stale index is a @?');
    ok(run('index', '--json').j.episodic === 3 && !run('status', '--json').j.long.stale, 'index: rebuild clears stale');
    // forget
    r = run('forget', '--short', '--json');
    ok(r.j && r.j["@S"] === "FORGOT" && r.j.n === 7 && !fs.existsSync(path.join(W, '.isekai', 'memory', 'short', 'rimuru.jsonl')), `forget: ${r.out.slice(0, 120)}`);
    ok(run('forget').code === 2, 'forget without --short is a FAIL');
    // empty world: every silence is a finding, nothing crashes
    const E = path.join(T, 'empty'); mkd(path.join(E, '.isekai'));
    const runE = (...a) => { const x = spawnSync(process.execPath, [__filename, E, ...a], { env, encoding: 'utf8' }); return { code: x.status, j: parse(x.stdout || '') }; };
    ok(runE('recall', 'anything', '--json').j['@?'].some(h => /no long-term index/.test(h)), 'empty world: recall without index is a @?');
    r = runE('index', '--json');
    ok(r.code === 0 && r.j.n === 0 && r.j['@?'].length === 1, 'empty world: index of nothing is a @?');
    r = runE('status', '--json');
    ok(r.code === 0 && r.j.long.indexed && r.j.long.episodic === 0 && r.j.short.workingMemory.desks === 0, 'empty world: status parses');
  } finally {
    fs.rmSync(T, { recursive: true, force: true });
  }
  const machineAfter = (() => { try { return fs.statSync(path.join(MACHINE_SHARED, 'notes.jsonl')).size; } catch { return -1; } })();
  ok(machineAfter === realMachine, 'the real ~/.isekai/shared was not touched');
  ok(!fs.existsSync(T), 'throwaway world removed');
  console.log(fails.length ? '@S FAIL' : `@S PASS ${checks} checks · node ${process.version}`);
  for (const f of fails) console.log(`@F selftest — ${f}`);
  if (fails.length) process.exit(1);
}

const run = { status, index, recall, remember, forget, selftest }[cmd];
if (!run) fail(`unknown command ${cmd} — status | index | recall | remember | forget | selftest`);
Promise.resolve(run()).catch(e => fail(`${cmd} crashed: ${e && e.message}`));
