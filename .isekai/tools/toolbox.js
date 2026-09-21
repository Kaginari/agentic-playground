#!/usr/bin/env node
// toolbox.js — the world's toolbox instrument (isekai.md §Minds & Bodies "Loading is already
// two-tiered"; Nature 2 Symbiosis, Nature 8 The Wire, Nature 9 Perception). Stdlib Node only,
// Node 16-compatible; one file travels with the world.
//
// A big registry, a small prompt. Everything a body could pick up is indexed once — never
// hand-maintained — and a turn receives only the entries that fit the ask AND a token budget.
// The pattern is TWO LEVELS, and both are commands:
//
//   LEVEL 1 · MANIFEST  what sits in a prompt: name · one-line description · triggers · path · the
//             cost of level 2. `pick`/`brief` emit only this — the way a Mind's frontmatter
//             description already sits in context every turn — within a token budget (default 1500).
//   LEVEL 2 · LOAD      the full body, pulled only when the body decides to use the tool: `load <name>`
//             — whole text, or one section by anchor (`--map` the section index, `--sec N` one
//             section: the doc-route shape). Every load is journaled in an instrument
//             (.isekai/instruments/toolbox/loads.jsonl, local, append-only) so `status` reports what
//             was actually loaded against what was merely offered — the pattern's honest measure.
//
//   REGISTRY  .isekai/toolbox/registry.json — derived, rebuilt at will (`index`), gitignored
//             · mind      .claude/skills/*/SKILL.md, .opencode/skill(s)/*/SKILL.md    frontmatter
//             · command   .claude/commands/*.md, .opencode/command(s)/*.md             frontmatter or # title
//             · tool      .isekai/tools/*                                               leading comment block
//             · body      .claude/agents/*.md, .opencode/agent(s)/*.md                  frontmatter
//             · external  .isekai/toolbox/extra.jsonl — hand-written, tracked: binaries, MCP servers,
//                         anything outside the world a body may reach for; installed? is measured at
//                         index time (a path that exists, or a name found on PATH), never believed
//             Each entry: kind · name · path · description · triggers (frontmatter `triggers:`/`when:`,
//             else phrases the description quotes, else its first sentence; the name always) · serves
//             (whom: zone / verdict / global / shared — derived from real wearers, the law's rule) · COST:
//             resident ≈ bytes(name + description)/4, the manifest's price; full ≈ bytes(body)/4, the
//             load's price. Both are known BEFORE anything is injected.
//   PICK      rank against the ask: MEANING (BM25 over name + description + triggers — memory.js's
//             scorer, normalised 0..1 as the share of the ask's ideal match) + TRIGGER (a trigger phrase
//             present in the ask: +0.8 declared / +0.5 the name / +0.4 derived — the strong signal; a
//             trigger hit surfaces even at zero similarity) + RELATION (--as: worn by the asker, serving
//             its lane, bound to its parent or children — memory.js's bonds read off `Reports to:` /
//             `Territory:`; ≤ 0.3, never surfaces an entry alone). One word in common is not a fit: an
//             entry is offered only on a trigger hit or ≥ 2 of the ask's content words (any one word when
//             the ask has fewer than 3); --min adds a score floor. -k caps, it never fills.
//             Then the budget is filled greedily by score; an entry that fits the turn but not the budget
//             is a @?, not a silent drop.
//   INJECT    pointers, never payloads (the compass: point, don't carry). No tool body is ever pasted
//             into a prompt: a Court brief carries the @TOOLS block; Claude Code loads by name (Skill,
//             ToolSearch "select:<name>", Agent); OpenCode by its skill loader; anyone by `load <name>`.
//
// Design: .isekai/canon/toolbox.md. The registry is written atomically (temp + rename); instrument
// lines are single O_APPEND writes, atomic below PIPE_BUF. Every silence is a finding (@?), never a
// guess: no registry, an empty one, one older than its sources (mtimes), an ask with no content words,
// a pick that fits the turn but not the budget, an external that is not installed.
//
// Usage:
//   node toolbox.js [root] index    [--json]
//   node toolbox.js [root] status   [--budget <tokens>] [--json]
//   node toolbox.js [root] pick "<ask>"  [--as <body>] [--budget <tokens>] [-k N] [--min <score>] [--kind mind|command|tool|body|external[,…]] [--json]
//   node toolbox.js [root] brief "<ask>" [--as <body>] [--budget <tokens>] [-k N] [--min <score>] [--kind …] [--json]   (the @TOOLS block, ready to paste; --json: head, @T lines, picks)
//   node toolbox.js [root] load <name>   [--as <body>] [--kind <kind>] [--map | --sec N] [--json]  (level 2 — journaled)
//   node toolbox.js [root] explain <name> [--kind <kind>] [--json]
//   node toolbox.js [root] selftest   (Law 5: a throwaway world under .isekai/tmp/, cleaned up)
'use strict';
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const { execSync, spawnSync } = require('child_process');

// ------------------------------------------------------------------ args
const argv = process.argv.slice(2);
const flags = {}; const pos = [];
for (let i = 0; i < argv.length; i++) {
  const a = argv[i];
  if (a === '--json' || a === '--map') flags[a.slice(2)] = true;
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
const BOX = path.join(HOME, 'toolbox');
const REGISTRY = path.join(BOX, 'registry.json');
const EXTRA = path.join(BOX, 'extra.jsonl');
const LOADS = path.join(HOME, 'instruments', 'toolbox', 'loads.jsonl');
const REG_V = 1;               // bump when the registry shape changes; an older registry is a finding, not a crash
const AS = String(flags.as || 'rimuru').toLowerCase();
const KINDS = ['mind', 'command', 'tool', 'body', 'external'];
const BUDGET_DEFAULT = 1500;
const MIN_DEFAULT = 0;          // an optional score floor; the real fit rule is FIT_WORDS below
const FIT_WORDS = 2;            // an entry fits the turn on a trigger hit, or on ≥ 2 of the ask's content words (any one when the ask has < 3)
const PIPE_BUF = 4096;
const LANE_OF_RACE = { slime: 'zone', orc: 'verdict', elf: 'global', kijin: 'global' };
const TRIGGER_W = { frontmatter: 0.8, quoted: 0.8, name: 0.5, sentence: 0.4 };
const TOK = s => Math.ceil(Buffer.byteLength(String(s)) / 4);   // ~4 bytes/token: an estimate, not a billing figure

const rd = p => { try { return fs.readFileSync(p, 'utf8').replace(/\r\n?/g, '\n'); } catch { return null; } };
const mkd = p => fs.mkdirSync(p, { recursive: true });
const rel = p => path.isAbsolute(p) && p.startsWith(root + path.sep) ? path.relative(root, p) : p;
const now = () => new Date().toISOString();
const worldName = () => (rd(path.join(HOME, 'name')) || path.basename(root)).trim();
const own = (o, k) => Object.prototype.hasOwnProperty.call(o, k) ? o[k] : undefined;
const unquote = s => { s = String(s || '').trim(); if (/^".*"$/.test(s)) return s.slice(1, -1).replace(/\\"/g, '"'); if (/^'.*'$/.test(s)) return s.slice(1, -1); return s; };
function writeAtomic(p, data) {
  mkd(path.dirname(p));
  const tmp = `${p}.${process.pid}.${crypto.randomBytes(3).toString('hex')}.tmp`;
  fs.writeFileSync(tmp, data); fs.renameSync(tmp, p);
}
function appendLine(p, obj) { mkd(path.dirname(p)); const line = JSON.stringify(obj) + '\n'; fs.appendFileSync(p, line); return Buffer.byteLength(line); }
function readJsonl(p) {
  return (rd(p) || '').split('\n').filter(Boolean).map(l => { try { return JSON.parse(l); } catch { return null; } }).filter(x => x && typeof x === 'object');
}

// ------------------------------------------------------------------ sections (memory.js / tempest.js sectionMap shape)
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
function frontmatter(text) {   // {fm, body}: `key: value` lines, `- item` lists, indented continuations; nothing more
  const m = text.match(/^---\n([\s\S]*?)\n---(?:\n|$)/);
  const fm = {}; if (!m) return { fm, body: text };
  let key = null;
  for (const l of m[1].split('\n')) {
    const kv = l.match(/^([A-Za-z_][\w-]*):\s*(.*)$/);
    if (kv) { key = kv[1].toLowerCase(); fm[key] = kv[2].trim(); continue; }
    if (!key) continue;
    const li = l.match(/^\s*-\s+(.+)$/);
    if (li) { fm[key] = (Array.isArray(fm[key]) ? fm[key] : fm[key] ? [fm[key]] : []).concat(unquote(li[1])); continue; }
    if (/^\s+\S/.test(l) && typeof fm[key] === 'string') fm[key] = (fm[key] + ' ' + l.trim()).replace(/^[>|]\s*/, '');
  }
  return { fm, body: text.slice(m[0].length) };
}
const asList = v => v === undefined || v === null ? [] : Array.isArray(v) ? v.map(unquote) : String(v).replace(/^\[|\]$/g, '').split(',').map(unquote).filter(Boolean);
const firstSentence = s => (String(s).replace(/\s+/g, ' ').trim().match(/^.*?[.!?](?=\s+[A-Z"“(]|\s*$)/) || [String(s).replace(/\s+/g, ' ').trim()])[0];
const oneLine = (s, n = 120) => { const t = firstSentence(s); return t.length > n ? t.slice(0, n - 1).trimEnd() + '…' : t; };

// ------------------------------------------------------------------ the local scorer (memory.js's, kept identical in behaviour)
const STOP = new Set(('a an and are as at be by for from has have if in into is it its of on or that the this to was were will with not no ' +
  'you your we our they their he she i me my but so than then there these those which who what when where how can could would should ' +
  'do does did done been being also any all each every more most much very just only over under up down out off about after before ' +
  'again here now one two three').split(' '));
function tokens(text) {
  return String(text).toLowerCase().replace(/[`*_>#|]/g, ' ').split(/[^a-z0-9@.\-]+/)
    .map(t => t.replace(/^[.\-]+|[.\-]+$/g, '')).filter(t => t.length > 1 && !STOP.has(t) && !/^\d+$/.test(t));
}
function termCounts(text) {
  const tf = Object.create(null); let dl = 0;
  for (const t of tokens(text)) { tf[t] = (tf[t] || 0) + 1; dl++; }
  return { tf, dl };
}
const K1 = 1.2, B = 0.75;
const idfFor = (N, df) => Math.log((N - df + 0.5) / (df + 0.5) + 1);
function statsOf(mems) {
  const df = Object.create(null); let total = 0;
  for (const m of mems) { total += m.dl; for (const t in m.tf) df[t] = (df[t] || 0) + 1; }
  const idf = Object.create(null);
  for (const t in df) idf[t] = idfFor(mems.length, df[t]);
  return { n: mems.length, avgdl: mems.length ? total / mems.length : 1, idf };
}
function bm25(qtf, m, S, idfQ) {
  let s = 0, ideal = 0;
  const K = K1 * (1 - B + B * m.dl / (S.avgdl || 1));
  for (const t in qtf) {
    const idf = idfQ(t); ideal += idf * (K1 + 1);
    const f = own(m.tf, t); if (f) s += idf * f * (K1 + 1) / (f + K);
  }
  return ideal ? s / ideal : 0;
}
// Trigger matching only: a light stem so "render" meets "rendering" and "slide" meets "slides". BM25 stays unstemmed.
const stem = t => { const s = t.replace(/(ing|ed|es|s)$/, ''); return s.length >= 3 ? s : t; };
function trigHits(askSet, triggers) {
  return triggers.filter(x => { const ts = tokens(x.t).map(stem); return ts.length > 0 && ts.every(t => askSet.has(t)); });
}
function triggerBoost(hits) {
  if (!hits.length) return 0;
  return Math.max(...hits.map(h => TRIGGER_W[h.src] || 0.4)) + Math.min(0.15, 0.05 * (hits.length - 1));
}

// ------------------------------------------------------------------ harvest: what a body could pick up, read from files
function dirsOf(bases, pick) {   // first base wins; later bases add to `srcs` (one thing with two homes, not two things)
  const out = new Map();
  for (const base of bases) {
    const d = path.join(root, base);
    if (!fs.existsSync(d)) continue;
    for (const e of fs.readdirSync(d, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const got = pick(d, e); if (!got) continue;
      const hit = out.get(got.name);
      if (hit) hit.srcs.push(rel(got.p)); else out.set(got.name, { p: got.p, srcs: [rel(got.p)] });
    }
  }
  return out;
}
const minds = () => dirsOf(['.opencode/skills', '.opencode/skill', '.claude/skills'], (d, e) => e.isDirectory() && fs.existsSync(path.join(d, e.name, 'SKILL.md')) ? { name: e.name, p: path.join(d, e.name, 'SKILL.md') } : null);
const commands = () => dirsOf(['.claude/commands', '.opencode/commands', '.opencode/command'], (d, e) => e.isFile() && e.name.endsWith('.md') ? { name: e.name.slice(0, -3), p: path.join(d, e.name) } : null);
const bodies = () => dirsOf(['.claude/agents', '.opencode/agents', '.opencode/agent'], (d, e) => e.isFile() && e.name.endsWith('.md') ? { name: e.name.slice(0, -3), p: path.join(d, e.name) } : null);
const tools = () => dirsOf(['.isekai/tools'], (d, e) => e.isFile() && !e.name.startsWith('.') ? { name: e.name, p: path.join(d, e.name) } : null);
function creatures() {   // memory.js's shape: id = <race>-<dir>; doc = README.md, <dir>.md, SKILL.md or the first .md
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
const nameRe = n => new RegExp(`(^|[^a-z0-9-])${n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?![a-z0-9-])`, 'i');
function headerOf(text) {   // a tool's leading comment block (//, #, or /** … */), shebang skipped, markers stripped
  const lines = text.split('\n'); const out = [];
  let i = lines[0] && lines[0].startsWith('#!') ? 1 : 0;
  let block = false;
  for (; i < lines.length; i++) {
    const l = lines[i];
    if (/^\s*'use strict';?\s*$/.test(l) && !out.length) continue;
    if (block) { out.push(l.replace(/^\s*\*\s?/, '')); if (/\*\//.test(l)) { out[out.length - 1] = out[out.length - 1].replace(/\s*\*\/.*$/, ''); block = false; } continue; }
    if (/^\s*\/\*\*?/.test(l)) { block = true; out.push(l.replace(/^\s*\/\*\*?\s?/, '')); if (/\*\//.test(l)) { out[out.length - 1] = out[out.length - 1].replace(/\s*\*\/.*$/, ''); block = false; } continue; }
    const m = l.match(/^\s*(?:\/\/|#)\s?(.*)$/);
    if (m) out.push(m[1]); else if (out.length || l.trim()) break;
  }
  return out.join('\n').trim();
}
function triggersOf(name, fm, desc) {
  const out = [];
  const add = (t, src) => { t = String(t).replace(/\s+/g, ' ').trim(); if (t && tokens(t).length && !out.some(x => x.t.toLowerCase() === t.toLowerCase())) out.push({ t, src }); };
  add(name, 'name');
  add(name.replace(/\.[a-z0-9]+$/i, '').split(/[-_.]+/).filter(Boolean).join(' '), 'name');
  for (const k of ['triggers', 'trigger', 'when', 'use-when', 'use_when']) for (const t of asList(fm[k])) add(t, 'frontmatter');
  if (out.every(x => x.src === 'name')) for (const q of desc.match(/"([^"\n]{3,80})"|“([^”\n]{3,80})”/g) || []) add(q.replace(/^["“]|["”]$/g, ''), 'quoted');
  if (out.every(x => x.src === 'name')) add(firstSentence(desc), 'sentence');
  return out;
}
function externalsOf() {   // extra.jsonl lines; `installed` is measured here, the file's own claim is kept beside it
  const out = [];
  const onPath = n => (process.env.PATH || '').split(path.delimiter).some(d => d && fs.existsSync(path.join(d, n)));
  for (const x of readJsonl(EXTRA)) {
    if (typeof x.name !== 'string' || !x.name) continue;
    const p = typeof x.path === 'string' && x.path ? x.path : null;
    const installed = p ? fs.existsSync(p) : onPath(x.name);
    out.push({ name: x.name, path: p || (installed ? `$PATH/${x.name}` : null), description: String(x.description || ''), triggers: asList(x.triggers), cost: x.cost,
      serves: x.serves || 'shared', usage: typeof x.usage === 'string' ? x.usage : null, installed, claimed: typeof x.installed === 'boolean' ? x.installed : null });
  }
  return out;
}
function sources() {   // every file the registry is read from — so it can say later whether any moved
  const out = [];
  for (const [, v] of minds()) out.push(v.p); for (const [, v] of commands()) out.push(v.p);
  for (const [, v] of bodies()) out.push(v.p); for (const [, v] of tools()) out.push(v.p);
  if (fs.existsSync(EXTRA)) out.push(EXTRA);
  return out;
}
const sourceStamps = () => { const o = {}; for (const p of sources()) { try { o[rel(p)] = Math.round(fs.statSync(p).mtimeMs); } catch {} } return o; };
function harvest() {
  const entries = [];
  const race = n => Object.keys(LANE_OF_RACE).find(r => n.startsWith(r + '-'));
  const push = (kind, name, p, srcs, description, triggers, extra) => entries.push(Object.assign({ kind, name, path: rel(p), srcs: srcs || [rel(p)], description, line: oneLine(description), triggers },
    termCounts(`${name} ${name.replace(/[-_.]+/g, ' ')}\n${description}\n${triggers.map(t => t.t).join('\n')}`), extra));
  const docs = creatures().filter(c => c.doc).map(c => Object.assign({ text: rd(c.doc) || '' }, c));
  for (const [name, v] of minds()) {
    const text = rd(v.p) || ''; const { fm, body } = frontmatter(text);
    const description = unquote(fm.description) || ((body.match(/^#\s+(.+)$/m) || [])[1] || '').trim() || firstSentence(body);
    const wearers = docs.filter(c => nameRe(name).test(c.text)).map(c => c.name);
    const races = [...new Set(wearers.map(w => w.split('-')[0]))];
    const serves = race(name) ? LANE_OF_RACE[race(name)] : races.length === 1 ? LANE_OF_RACE[races[0]] : 'shared';
    push('mind', name, v.p, v.srcs, description, triggersOf(name, fm, description), { serves, wearers, cost: { resident: TOK(name + ' ' + description), full: TOK(text) }, sections: sectionMap(text).sections.length });
  }
  for (const [name, v] of commands()) {
    const text = rd(v.p) || ''; const { fm, body } = frontmatter(text);
    const description = unquote(fm.description) || ((body.match(/^#\s+(.+)$/m) || [])[1] || '').trim();
    push('command', name, v.p, v.srcs, description, triggersOf(name, fm, description), { serves: 'shared', wearers: [], cost: { resident: TOK(name + ' ' + description), full: TOK(text) }, sections: sectionMap(text).sections.length });
  }
  for (const [name, v] of tools()) {
    const text = rd(v.p) || ''; const head = headerOf(text);
    const description = (head.split(/\n\s*\n/)[0] || '').replace(/^[\w.\-]+\s+[—-]+\s+/, '').replace(/\s+/g, ' ').trim().slice(0, 500);
    push('tool', name, v.p, v.srcs, description, triggersOf(name, {}, description), { serves: 'shared', wearers: [], cost: { resident: TOK(name + ' ' + description), full: TOK(head) }, fileTokens: TOK(text), sections: 0 });
  }
  for (const [name, v] of bodies()) {
    const text = rd(v.p) || ''; const { fm, body } = frontmatter(text);
    const description = unquote(fm.description) || ((body.match(/^#\s+(.+)$/m) || [])[1] || '').trim() || firstSentence(body);
    push('body', name, v.p, v.srcs, description, triggersOf(name, fm, description), { serves: race(name) || 'shared', wearers: [], mode: fm.mode || null, model: fm.model || null, cost: { resident: TOK(name + ' ' + description), full: TOK(text) }, sections: sectionMap(text).sections.length });
  }
  for (const x of externalsOf()) {
    const c = x.cost && typeof x.cost === 'object' ? x.cost : {};
    const resident = typeof x.cost === 'number' ? x.cost : typeof c.resident === 'number' ? c.resident : TOK(x.name + ' ' + x.description);
    const full = typeof c.full === 'number' ? c.full : TOK(x.usage || x.description);
    push('external', x.name, x.path || '', null, x.description, triggersOf(x.name, { triggers: x.triggers }, x.description), { path: x.path, srcs: [rel(EXTRA)], serves: x.serves, wearers: [], installed: x.installed, claimed: x.claimed, usage: x.usage, cost: { resident, full }, sections: 0 });
  }
  return entries;
}

// ------------------------------------------------------------------ registry: derived, rebuildable, written atomically
function gitHead() { try { return execSync('git rev-parse HEAD', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim(); } catch { return null; } }
function buildRegistry(write) {
  const entries = harvest();
  const reg = Object.assign({ v: REG_V, world: worldName(), builtAt: now(), head: gitHead(), sources: sourceStamps() }, statsOf(entries), { entries });
  if (write) writeAtomic(REGISTRY, JSON.stringify(reg));
  return reg;
}
function loadRegistry() {   // {reg, why, live}: a missing registry is answered from a live harvest and named as a hole
  if (!fs.existsSync(REGISTRY)) return { reg: buildRegistry(false), live: true, why: 'no registry on disk — answered from a live harvest; run `toolbox.js index`' };
  let reg; try { reg = JSON.parse(rd(REGISTRY)); } catch { return { reg: buildRegistry(false), live: true, why: `registry unreadable (${rel(REGISTRY)}) — answered live; rerun \`toolbox.js index\`` }; }
  if (!reg || reg.v !== REG_V || !Array.isArray(reg.entries)) return { reg: buildRegistry(false), live: true, why: 'registry built by an older toolbox.js — answered live; rerun `toolbox.js index`' };
  return { reg, live: false, why: null };
}
function registryHoles(R) {
  const h = [];
  if (R.why) h.push(R.why);
  if (!R.reg.entries.length) h.push(`registry empty — nothing under .claude/skills, .opencode/skill(s), .claude/commands, .opencode/command(s), .isekai/tools, .claude/agents, .opencode/agent(s) or ${rel(EXTRA)}`);
  if (R.live) return h;
  const cur = sourceStamps(), was = R.reg.sources || {};
  const say = (n, what) => n.length ? `${n.length} ${what}: ${n.slice(0, 3).join(', ')}${n.length > 3 ? ', …' : ''}` : null;
  const parts = [say(Object.keys(cur).filter(p => own(was, p) !== undefined && was[p] !== cur[p]), 'changed'),
    say(Object.keys(cur).filter(p => own(was, p) === undefined), 'new'), say(Object.keys(was).filter(p => own(cur, p) === undefined), 'gone')].filter(Boolean);
  if (parts.length) h.push(`registry older than its sources — ${parts.join('; ')} — rerun \`toolbox.js index\``);
  return h;
}
const isStale = R => !R.live && registryHoles(R).some(h => h.startsWith('registry older'));

// ------------------------------------------------------------------ relations: whom the asking body is bound to (memory.js's reading)
function relationsOf(name) {
  const R = { self: name, race: null, lane: null, parent: null, children: [], minds: [], zone: [] };
  const all = creatures();
  const c = all.find(x => x.name === name || x.dir === name);
  if (!c || !c.doc) return R;
  const doc = rd(c.doc) || '';
  R.race = c.race; R.lane = LANE_OF_RACE[c.race] || null;
  const fieldOf = (d, k) => ((d.match(new RegExp(`^\\s*-\\s*\\*\\*${k}:\\*\\*\\s*(.+)$`, 'mi')) || [])[1] || '');
  const named = s => s.toLowerCase().match(/\b(?:elf|orc|slime|kijin)-[a-z0-9-]+/g) || [];
  const want = c.race === 'slime' ? 'orc-' : c.race === 'orc' ? 'elf-' : null;
  R.parent = named(fieldOf(doc, 'Reports to'))[0] || (want && named(doc).find(n => n.startsWith(want) && n !== c.name)) || null;
  for (const o of all) if (o.name !== c.name && o.doc && named(fieldOf(rd(o.doc) || '', 'Reports to')).includes(c.name)) R.children.push(o.name);
  for (const m of minds().keys()) if (nameRe(m).test(doc)) R.minds.push(m);
  const zoneText = fieldOf(doc, 'Territory') + ' ' + (doc.match(/`[^`\n]*\/[^`\n]*`/g) || []).join(' ');
  R.zone = [...new Set((zoneText.match(/[\w.\-]+(?:\/[\w.\-]*)+/g) || []).map(z => z.replace(/\/+$/, '').toLowerCase()))];
  return R;
}
// Small on purpose (≤ 0.3): relation nudges and breaks ties; meaning and triggers decide.
function relationBoost(e, R) {
  const has = s => { const ts = tokens(s); return ts.length > 0 && ts.every(t => own(e.tf, t)); };
  const parts = [];
  if (R.self !== 'rimuru') {
    if ((e.wearers || []).includes(R.self) || R.minds.includes(e.name)) parts.push(['worn', 0.15]);
    else if (has(R.self)) parts.push(['names-asker', 0.10]);
    if (R.lane && e.serves === R.lane) parts.push(['lane', 0.08]);
    if (e.kind === 'body' && (e.name === R.parent || R.children.includes(e.name))) parts.push(['bond', 0.10]);
    else if ((R.parent && has(R.parent)) || R.children.some(has)) parts.push(['bond', 0.05]);
    if (R.zone.some(has)) parts.push(['zone', 0.08]);
  }
  return { b: Math.min(0.3, parts.reduce((a, p) => a + p[1], 0)), why: parts.map(p => p[0]).join('+') };
}

// ------------------------------------------------------------------ the pick: rank, then fill the budget
function kindFilter() {
  if (!flags.kind) return null;
  const ks = String(flags.kind).split(',').map(s => s.trim()).filter(Boolean);
  for (const k of ks) if (!KINDS.includes(k)) fail(`--kind must be ${KINDS.join('|')}, got ${k}`);
  return ks;
}
function rank(reg, q, R, kinds, min) {
  const idfQ = t => { const v = own(reg.idf, t); return v !== undefined ? v : idfFor(reg.n || 1, 1); };
  const qtf = termCounts(q).tf, qTerms = Object.keys(qtf);
  if (!qTerms.length) return [];
  const askSet = new Set(tokens(q).map(stem));
  const need = Math.min(FIT_WORDS, qTerms.length < 3 ? 1 : FIT_WORDS);
  return reg.entries.filter(e => !kinds || kinds.includes(e.kind)).map(e => {
    const sim = bm25(qtf, e, reg, idfQ), hits = trigHits(askSet, e.triggers), trig = triggerBoost(hits), rl = relationBoost(e, R);
    const matched = qTerms.filter(t => own(e.tf, t)).length;
    const why = trig ? `trigger "${hits[0].t}"` : rl.b && rl.b >= sim ? `relation ${rl.why}` : 'meaning';
    return { kind: e.kind, name: e.name, path: e.path, srcs: e.srcs, serves: e.serves, installed: e.installed, resident: e.cost.resident, full: e.cost.full,
      score: +(sim + trig + rl.b).toFixed(4), sim: +sim.toFixed(4), matched, trig: +trig.toFixed(2), rel: +rl.b.toFixed(2), why, hits: hits.map(h => h.t), line: e.line, triggers: e.triggers.filter(t => t.src === 'frontmatter' || t.src === 'quoted').map(t => t.t) };
  }).filter(r => (r.trig > 0 || r.matched >= need) && r.score >= min).sort((a, b) => b.score - a.score || a.resident - b.resident || a.name.localeCompare(b.name));
}
function fill(ranked, K, budget) {   // greedy by score; what fits the turn but not the budget is named, never silently dropped
  const picks = [], over = []; let cost = 0;
  for (const r of ranked) {
    if (picks.length >= K) break;
    if (cost + r.resident <= budget) { picks.push(r); cost += r.resident; } else over.push(r);
  }
  return { picks, over, cost };
}
function doPick() {
  const q = pos.join(' ').trim();
  if (!q) fail(`${cmd} needs the turn's ask`);
  const K = flags.k === undefined ? 5 : parseInt(flags.k, 10);
  if (!(K > 0)) fail(`-k needs a positive integer, got ${flags.k}`);
  const budget = flags.budget === undefined ? BUDGET_DEFAULT : parseInt(flags.budget, 10);
  if (!(budget > 0)) fail(`--budget needs a positive integer, got ${flags.budget}`);
  const min = flags.min === undefined ? MIN_DEFAULT : parseFloat(flags.min);
  if (!(min >= 0)) fail(`--min needs a score ≥ 0, got ${flags.min}`);
  const kinds = kindFilter();
  const Rg = loadRegistry();
  const R = relationsOf(AS);
  const ranked = rank(Rg.reg, q, R, kinds, min);
  const { picks, over, cost } = fill(ranked, K, budget);
  const holes = [];
  if (!tokens(q).length) holes.push('ask has no content words after stop-word removal — nothing to rank');
  holes.push(...registryHoles(Rg));
  if (over.length) holes.push(`${over.length} fit the turn but not the budget: ${over.slice(0, 3).map(r => `${r.name} ${r.resident}tok`).join(', ')}${over.length > 3 ? ', …' : ''} — raise --budget or load by name`);
  for (const r of picks) if (r.kind === 'external' && r.installed === false) holes.push(`${r.name} fits the turn but is not installed on this machine`);
  if (picks.length) appendLine(LOADS, { at: now(), by: AS, ev: 'offer', cmd, ask: q.slice(0, 200), names: picks.map(r => r.name), tokens: cost, budget });
  return { q, K, budget, min, picks, over, cost, holes };
}
function pick() {
  const P = doPick();
  if (flags.json) return console.log(JSON.stringify({ '@S': 'PICK', as: AS, ask: P.q, k: P.picks.length, cost: P.cost, budget: P.budget, min: P.min, picks: P.picks, over: P.over.map(r => ({ name: r.name, resident: r.resident, score: r.score })), '@?': P.holes }));
  console.log(`@S PICK k=${P.picks.length} cost=${P.cost}/${P.budget} as=${AS}`);
  for (const r of P.picks) console.log(`@T ${r.kind} ${r.name} — ${r.path || 'not installed'} — ${r.resident}tok (load≈${r.full}) — ${r.why} ${r.score.toFixed(2)}`);
  for (const h of P.holes) console.log(`@? ${h}`);
  console.log(`@E ${Buffer.byteLength(JSON.stringify(P.picks.map(r => [r.kind, r.name, r.path, r.resident, r.full, r.line])))}`);
}
// LEVEL 1 on the wire: the manifest a Court brief carries. Descriptions, triggers, paths, level-2 costs — never a body.
function brief() {
  const P = doPick();
  const tool = rel(path.join(HOME, 'tools', 'toolbox.js'));
  const head = `@TOOLS as=${AS} k=${P.picks.length} cost=${P.cost}/${P.budget} — level 2 on decision only: node ${tool} load <name> [--map|--sec N] · Claude Code: Skill <name> / ToolSearch "select:<name>" / Agent <body> · OpenCode: load skill <name>`;
  const lines = P.picks.map(r => `@T ${r.kind} ${r.name} — ${r.path || 'not installed'} — load≈${r.full}tok — ${r.line}${r.triggers.length ? ' — ⟨' + r.triggers.slice(0, 4).join(' · ') + '⟩' : ''}`);
  // --json: the same manifest for a machine mouth (loop.js's recall beat) — head, the @T lines, and the picks as pointers with costs; still never a body
  if (flags.json) return console.log(JSON.stringify({ '@S': 'TOOLS', as: AS, ask: P.q, k: P.picks.length, cost: P.cost, budget: P.budget, head, lines,
    picks: P.picks.map(r => ({ kind: r.kind, name: r.name, path: r.path, resident: r.resident, full: r.full, why: r.why, score: r.score })), over: P.over.map(r => ({ name: r.name, resident: r.resident, score: r.score })), '@?': P.holes }));
  console.log(head);
  for (const l of lines) console.log(l);
  for (const h of P.holes) console.log(`@? ${h}`);
}

// ------------------------------------------------------------------ level 2: load — the body, whole or by section; journaled
function findEntry(reg, name, kind) {
  const n = String(name).toLowerCase().replace(/^\//, '');
  let c = reg.entries.filter(e => e.name.toLowerCase() === n || e.name.toLowerCase() === n + '.js' || e.name.toLowerCase() === n + '.sh');
  if (kind) c = c.filter(e => e.kind === kind);
  if (c.length > 1) fail(`${name} names ${c.length} entries (${c.map(e => e.kind).join(', ')}) — add --kind`);
  return c[0] || null;
}
function bodyOf(e) {   // {text, why}: what level 2 hands over per kind
  if (e.kind === 'external') return e.usage ? { text: e.usage } : { text: e.description, why: `${e.name} has no usage notes in ${rel(EXTRA)} — description only` };
  const text = rd(path.resolve(root, e.path));
  if (text === null) return { text: '', why: `${e.path} unreadable — registry older than the world? rerun \`toolbox.js index\`` };
  return e.kind === 'tool' ? { text: headerOf(text), why: null } : { text, why: null };
}
function load() {
  const name = pos.shift();
  if (!name) fail('load needs a name — `toolbox.js pick "<ask>"` lists them');
  const Rg = loadRegistry();
  const e = findEntry(Rg.reg, name, flags.kind || null);
  if (!e) fail(`no entry named ${name} in the registry${flags.kind ? ' with kind ' + flags.kind : ''} — \`toolbox.js index\` rebuilds it`);
  const { text, why } = bodyOf(e);
  const holes = [...registryHoles(Rg)]; if (why) holes.push(why);
  const map = sectionMap(text);
  if (flags.map) {   // the section index — nothing of the body crosses, so nothing is journaled
    if (!map.sections.length) holes.push(`${e.name} has no sections — load it whole`);
    if (flags.json) return console.log(JSON.stringify({ '@S': 'MAP', name: e.name, kind: e.kind, path: e.path, preambleBytes: Buffer.byteLength(map.preamble), sections: map.sections.map(s => ({ n: s.n, title: s.t, depth: s.depth, bytes: s.b, tokens: TOK(s.text) })), '@?': holes }));
    console.log(`@S MAP ${e.name} sections=${map.sections.length} preamble=${TOK(map.preamble)}tok — ${e.path}`);
    for (const s of map.sections) console.log(`@F #${s.n} — ${'#'.repeat(s.depth)} ${s.t} — ${TOK(s.text)}tok`);
    for (const h of holes) console.log(`@? ${h}`);
    return console.log(`@E ${Buffer.byteLength(JSON.stringify(map.sections.map(s => [s.n, s.t, s.b])))}`);
  }
  let out = text, sec = 'all';
  if (flags.sec !== undefined) {
    const n = parseInt(flags.sec, 10);
    if (!(n >= 0)) fail(`--sec needs a section number (0 = preamble), got ${flags.sec}`);
    if (n === 0) out = map.preamble; else { const s = map.sections[n - 1]; if (!s) fail(`${e.name} has ${map.sections.length} section${map.sections.length === 1 ? '' : 's'}, no #${n} — \`load ${e.name} --map\``); out = s.text; }
    sec = n;
  }
  const tk = TOK(out);
  appendLine(LOADS, { at: now(), by: AS, ev: 'load', name: e.name, kind: e.kind, tokens: tk, sec });
  if (flags.json) return console.log(JSON.stringify({ '@S': 'LOAD', name: e.name, kind: e.kind, path: e.path, sec, tokens: tk, text: out, '@?': holes }));
  console.log(`@S LOAD ${e.kind} ${e.name} sec=${sec} tokens=${tk} — ${e.path || 'not installed'}`);
  console.log(out);
  for (const h of holes) console.log(`@? ${h}`);
  console.log(`@E ${Buffer.byteLength(out)}`);
}

// ------------------------------------------------------------------ hygiene: index · status · explain
function index() {
  const reg = buildRegistry(true);
  const by = k => reg.entries.filter(e => e.kind === k).length;
  const holes = [];
  if (!reg.n) holes.push(`nothing to index under ${root} — no Minds, commands, tools, Bodies or ${rel(EXTRA)} entries`);
  for (const e of reg.entries) if (e.kind === 'external' && !e.installed) holes.push(`external ${e.name} is not installed on this machine${e.claimed === true ? ' (extra.jsonl claims it is)' : ''}`);
  if (flags.json) return console.log(JSON.stringify(Object.assign({ '@S': 'INDEXED', world: reg.world, n: reg.n }, KINDS.reduce((o, k) => (o[k] = by(k), o), {}), { resident: reg.entries.reduce((a, e) => a + e.cost.resident, 0), head: reg.head, src: rel(REGISTRY), '@?': holes })));
  console.log(`@S INDEXED ${reg.n} entries — ${KINDS.map(k => `${k} ${by(k)}`).join(' · ')} → ${rel(REGISTRY)}`);
  for (const h of holes) console.log(`@? ${h}`);
}
function instrument() {   // what was offered against what was actually loaded — read from the loads journal
  const ev = readJsonl(LOADS);
  const offers = ev.filter(x => x.ev === 'offer'), loads = ev.filter(x => x.ev === 'load');
  const offeredNames = new Set(); for (const o of offers) for (const n of o.names || []) offeredNames.add(n);
  return { offers: offers.length, offered: offers.reduce((a, o) => a + (o.names || []).length, 0), offeredDistinct: offeredNames.size, residentTokens: offers.reduce((a, o) => a + (o.tokens || 0), 0),
    loads: loads.length, loadedDistinct: new Set(loads.map(l => l.name)).size, loadedTokens: loads.reduce((a, l) => a + (l.tokens || 0), 0), src: rel(LOADS) };
}
function status() {
  const budget = flags.budget === undefined ? BUDGET_DEFAULT : parseInt(flags.budget, 10);
  const Rg = loadRegistry(), reg = Rg.reg;
  const holes = registryHoles(Rg);
  const by = k => reg.entries.filter(e => e.kind === k).length;
  const resident = reg.entries.reduce((a, e) => a + e.cost.resident, 0), full = reg.entries.reduce((a, e) => a + e.cost.full, 0);
  const ext = reg.entries.filter(e => e.kind === 'external'), missing = ext.filter(e => !e.installed).map(e => e.name);
  const ins = instrument();
  if (!fs.existsSync(LOADS)) holes.push(`no loads journal yet (${rel(LOADS)}) — nothing offered or loaded through the toolbox so far`);
  const report = { world: worldName(), at: now(), registry: { present: !Rg.live, builtAt: reg.builtAt, head: reg.head, stale: isStale(Rg), n: reg.n, byKind: KINDS.reduce((o, k) => (o[k] = by(k), o), {}), src: rel(REGISTRY) },
    cost: { residentIfAllInjected: resident, budget, budgetShare: budget ? +(resident / budget).toFixed(2) : null, fullIfAllLoaded: full }, externals: { installed: ext.length - missing.length, missing }, instrument: ins, '@?': holes };
  if (flags.json) return console.log(JSON.stringify(report));
  console.log(`toolbox — ${report.world}`);
  console.log(`REGISTRY ${reg.n} entries — ${KINDS.map(k => `${k} ${by(k)}`).join(' · ')}${Rg.live ? '  (live harvest)' : `  (built ${reg.builtAt}${reg.head ? ' @ ' + reg.head.slice(0, 7) : ''})${report.registry.stale ? ' — STALE' : ''}`}`);
  console.log(`COST     resident if all injected ≈ ${resident.toLocaleString()} tok vs budget ${budget.toLocaleString()} (${Math.round(resident / budget * 100)}%) · full if all loaded ≈ ${full.toLocaleString()} tok`);
  console.log(`EXTERNAL ${ext.length ? `${ext.length - missing.length} installed${missing.length ? ' · missing: ' + missing.join(', ') : ''}` : `none — ${rel(EXTRA)} is empty or absent`}`);
  console.log(`LOADS    offered ${ins.offered} (${ins.offeredDistinct} distinct, ${ins.offers} pick${ins.offers === 1 ? '' : 's'}) · loaded ${ins.loads} (${ins.loadedDistinct} distinct) · resident cost ${ins.residentTokens.toLocaleString()} tok · loaded cost ${ins.loadedTokens.toLocaleString()} tok  (${ins.src})`);
  for (const h of holes) console.log(`@? ${h}`);
}
function explain() {
  const name = pos.shift();
  if (!name) fail('explain needs a name');
  const Rg = loadRegistry();
  const e = findEntry(Rg.reg, name, flags.kind || null);
  if (!e) fail(`no entry named ${name} in the registry — \`toolbox.js index\` rebuilds it`);
  const holes = registryHoles(Rg);
  if (flags.json) return console.log(JSON.stringify(Object.assign({ '@S': 'EXPLAIN' }, e, { tf: undefined, dl: undefined, '@?': holes })));
  console.log(`@S EXPLAIN ${e.kind} ${e.name}`);
  console.log(`@F path — ${e.path || 'not installed'}${e.srcs && e.srcs.length > 1 ? ' (also ' + e.srcs.slice(1).join(', ') + ')' : ''}`);
  console.log(`@F cost — resident ${e.cost.resident}tok (level 1: name + description) · load ${e.cost.full}tok (level 2${e.kind === 'tool' ? ': header; file ' + e.fileTokens + 'tok' : e.sections ? ': ' + e.sections + ' sections, --sec N' : ''})`);
  console.log(`@F serves — ${e.serves}${e.wearers && e.wearers.length ? ' · worn by ' + e.wearers.join(', ') : ''}${e.mode ? ' · mode ' + e.mode : ''}${e.model ? ' · model ' + e.model : ''}${e.kind === 'external' ? ' · installed ' + e.installed : ''}`);
  console.log(`@F triggers — ${e.triggers.map(t => `"${t.t}" (${t.src})`).join(' · ')}`);
  console.log(`@F description — ${e.description.replace(/\s+/g, ' ')}`);
  for (const h of holes) console.log(`@? ${h}`);
  console.log(`@E ${Buffer.byteLength(e.description)}`);
}

// ------------------------------------------------------------------ selftest: a throwaway world under .isekai/tmp/ (Law 5)
function selftest() {
  const T = path.join(HOME, 'tmp', 'toolbox-selftest'), W = path.join(T, 'world');
  const fails = []; let checks = 0;
  const ok = (cond, what) => { checks++; if (!cond) fails.push(what); };
  const w = (p, s) => { mkd(path.dirname(p)); fs.writeFileSync(p, s); };
  const parse = s => { try { return JSON.parse(s.trim().split('\n').pop()); } catch { return null; } };
  const run = (...a) => { const r = spawnSync(process.execPath, [__filename, W, ...a], { encoding: 'utf8', env: Object.assign({}, process.env, { PATH: path.join(W, 'bin') }) }); return { code: r.status, out: (r.stdout || '') + (r.stderr || ''), j: parse(r.stdout || '') }; };
  try {
    fs.rmSync(T, { recursive: true, force: true });
    w(path.join(W, '.isekai', 'name'), 'selftest-world\n');
    w(path.join(W, '.isekai', 'isekai.md'), '# Law\n\n## Laws\nVeldora\'s word is law.\n');
    w(path.join(W, '.isekai', 'slime', 'auth', 'README.md'), '# slime-auth\n\n- **Rank:** Slime\n- **Territory:** `src/auth/`\n- **Reports to:** orc-api\n- **Purpose:** ground truth of the login zone; wears the wire mind\n\n## Thoughts\n- 2026-09-22 born\n');
    w(path.join(W, '.isekai', 'orc', 'api', 'README.md'), '# orc-api\n\n- **Rank:** Orc\n- **Territory:** `src/api/`\n- **Reports to:** elf-core\n- **Purpose:** rules the api domain\n');
    // minds: wire (worn by slime-auth) · deploy-ship (declared triggers, unrelated words) · review (dense meaning) · dup (in both homes) · big (costly) · cheap
    w(path.join(W, '.claude', 'skills', 'wire', 'SKILL.md'), '---\nname: wire\ndescription: How to speak the wire between machine mouths.\n---\n# Wire\n\n## Report\nA court body reports back over the wire. BODYSENTINEL-WIRE\n\n## Hygiene\nNo greetings.\n');
    w(path.join(W, '.claude', 'skills', 'deploy-ship', 'SKILL.md'), '---\nname: deploy-ship\ndescription: Push a release to the production cluster and watch the rollout.\ntriggers:\n  - ship it\n  - rollout\n---\n# Deploy\n\nBODYSENTINEL-DEPLOY\n');
    w(path.join(W, '.claude', 'skills', 'standards-check', 'SKILL.md'), '---\nname: standards-check\ndescription: "Review the branch changes against the repo coding standards and report findings. Use when asked to \\"review since X\\"."\n---\n# Review\n\n## Standards\nBODYSENTINEL-REVIEW standards section.\n\n## Spec\nSpec section text.\n\n## Report\nReport section text.\n');
    w(path.join(W, '.opencode', 'skills', 'dup', 'SKILL.md'), '---\nname: dup\ndescription: Lives in both homes; one entry, two sources.\n---\n# Dup\n');
    w(path.join(W, '.claude', 'skills', 'dup', 'SKILL.md'), '---\nname: dup\ndescription: Lives in both homes; one entry, two sources.\n---\n# Dup\n');
    const heavy = 'Render slides and decks to pdf; check a slide; export the deck. ' + 'Slides decks pdf render export slide deck check. '.repeat(40);
    w(path.join(W, '.claude', 'skills', 'big', 'SKILL.md'), `---\nname: big\ndescription: "${heavy}"\n---\n# Big\n`);
    w(path.join(W, '.claude', 'skills', 'cheap', 'SKILL.md'), '---\nname: cheap\ndescription: Render a deck.\n---\n# Cheap\n');
    w(path.join(W, '.claude', 'skills', 'mid', 'SKILL.md'), '---\nname: mid\ndescription: Render slides and decks to pdf and check every slide of the deck before export.\n---\n# Mid\n');
    // commands · tools · body · externals (one present on the throwaway PATH, one missing)
    w(path.join(W, '.claude', 'commands', 'genesis.md'), '---\ndescription: Genesis — birth Elves, Orcs and Slimes from observed need\n---\n# /genesis\n');
    w(path.join(W, '.opencode', 'commands', 'isekai.md'), '---\ndescription: Reincarnate a directory as a living world\n---\n# /isekai\n');
    w(path.join(W, '.isekai', 'tools', 'x.sh'), '#!/bin/bash\n# x.sh — proving-grounds helper: runs the world\'s tests under .isekai/tmp/.\n#\n# Usage: x.sh\necho hi\n');
    w(path.join(W, '.isekai', 'tools', 'memo.js'), '#!/usr/bin/env node\n// memo.js — the memory instrument: remember a note, recall by meaning.\n// Second header line.\n\nconsole.log(1)\n');
    w(path.join(W, '.claude', 'agents', 'orc-api.md'), '---\nname: orc-api\ndescription: Orc of the api domain — holds the gate for slime-auth.\nmode: subagent\nmodel: sonnet\n---\n# orc-api\n\nBODYSENTINEL-ORC\n');
    w(path.join(W, 'bin', 'soffice'), '#!/bin/sh\necho fake\n'); fs.chmodSync(path.join(W, 'bin', 'soffice'), 0o755);
    w(path.join(W, '.isekai', 'toolbox', 'extra.jsonl'), JSON.stringify({ kind: 'external', name: 'soffice', description: 'LibreOffice headless: converts a pptx deck to pdf.', triggers: ['pdf', 'libreoffice'], cost: { resident: 30, full: 60 }, usage: 'soffice --headless --convert-to pdf <file>' }) + '\n'
      + JSON.stringify({ kind: 'external', name: 'ollama', path: path.join(W, 'nowhere', 'ollama'), description: 'Local embedding model server.', triggers: ['embedding'], installed: true }) + '\n' + '{not json}\n');
    // index: counts per kind, dup merged, external installed measured not believed
    let r = run('index', '--json');
    ok(r.code === 0 && r.j && r.j['@S'] === 'INDEXED' && r.j.mind === 7 && r.j.command === 2 && r.j.tool === 2 && r.j.body === 1 && r.j.external === 2 && r.j.n === 14, `index counts: ${r.out.slice(0, 200)}`);
    ok(r.j && r.j['@?'].some(h => /ollama is not installed.*claims/.test(h)), 'index: an external that is not installed is a @?, against its own claim');
    ok(fs.existsSync(path.join(W, '.isekai', 'toolbox', 'registry.json')) && !fs.readdirSync(path.join(W, '.isekai', 'toolbox')).some(f => f.endsWith('.tmp')), 'index: registry present, no temp left');
    r = run('explain', 'dup', '--json');
    ok(r.j && r.j.srcs.length === 2 && r.j.cost.resident > 0 && r.j.cost.full >= r.j.cost.resident, `explain dup: two sources, costs known, got ${r.out.slice(0, 160)}`);
    r = run('explain', 'wire', '--json');
    ok(r.j && r.j.serves === 'zone' && r.j.wearers.includes('slime-auth') && r.j.triggers.some(t => t.t === 'wire' && t.src === 'name') && r.j.sections === 3, `explain wire: zone lane from its wearer, name trigger, 3 sections — got ${r.out.slice(0, 200)}`);
    r = run('explain', 'standards-check', '--json');
    ok(r.j && r.j.triggers.some(t => t.t === 'review since X' && t.src === 'quoted'), `explain standards-check: a quoted phrase is a derived trigger — got ${r.j && JSON.stringify(r.j.triggers)}`);
    ok(run('explain', 'x', '--json').j.triggers.some(t => t.src === 'sentence') && run('explain', 'deploy-ship', '--json').j.triggers.some(t => t.t === 'ship it' && t.src === 'frontmatter'), 'explain: first-sentence and frontmatter triggers');
    ok(run('explain', 'nope').code === 2, 'explain: an unknown name is a FAIL');
    // pick by trigger beats pick by meaning
    r = run('pick', 'review the branch changes against the coding standards, then ship it', '--json');
    ok(r.j && r.j.picks[0] && r.j.picks[0].name === 'deploy-ship' && /^trigger "ship it"/.test(r.j.picks[0].why) && r.j.picks[1] && r.j.picks[1].name === 'standards-check' && r.j.picks[1].why === 'meaning' && r.j.picks[1].sim > 0.4,
      `trigger beats meaning: ${r.j && JSON.stringify(r.j.picks.slice(0, 2).map(p => [p.name, p.why, p.sim, p.trig]))}`);
    ok(r.j && r.j.picks.every(p => p.sim >= 0 && p.sim <= 1 && !isNaN(p.score)), 'pick: sims in [0,1], no NaN');
    // level 1 is descriptions only: no body text crosses in pick or brief
    const pickText = run('pick', 'review the branch changes', '-k', '3').out;
    ok(/^@S PICK k=/.test(pickText) && /@T mind standards-check — \.claude\/skills\/standards-check\/SKILL\.md — \d+tok \(load≈\d+\)/.test(pickText) && /@E \d+/.test(pickText) && !/BODYSENTINEL/.test(pickText), `pick wire: ${pickText.slice(0, 200)}`);
    const briefText = run('brief', 'speak on the wire, then ship it', '--as', 'slime-auth').out;
    ok(/^@TOOLS as=slime-auth k=\d+ cost=\d+\/1500 — level 2 on decision only: node .isekai\/tools\/toolbox\.js load <name>/.test(briefText) && /Skill <name>/.test(briefText) && /@T mind wire — .*load≈\d+tok — How to speak the wire between machine mouths\.$/m.test(briefText) && /@T mind deploy-ship — .* — ⟨ship it · rollout⟩$/m.test(briefText) && !/BODYSENTINEL/.test(briefText), `brief: ${briefText.slice(0, 300)}`);
    r = run('brief', 'speak on the wire, then ship it', '--as', 'slime-auth', '--json');
    ok(r.j && r.j['@S'] === 'TOOLS' && r.j.as === 'slime-auth' && r.j.k === r.j.picks.length && r.j.lines.length === r.j.k && r.j.lines.every(l => /^@T /.test(l)) && /^@TOOLS as=slime-auth/.test(r.j.head) && r.j.picks.every(p => typeof p.resident === 'number' && typeof p.full === 'number' && !('text' in p)) && !/BODYSENTINEL/.test(r.out), `brief --json: head, @T lines, priced picks, no body — ${r.out.slice(0, 200)}`);
    // budget: the cut drops the lowest-SCORING picks, not the cheapest; the dropped ones are a @?
    const all = run('pick', 'render the deck to pdf and check slide 8', '--budget', '100000', '-k', '10', '--json').j;
    const names = all.picks.map(p => p.name);
    ok(names[0] === 'soffice' && names.indexOf('cheap') > names.indexOf('big') && names.indexOf('cheap') > names.indexOf('mid'), `budget: unbudgeted order (declared trigger first, the cheapest-cost entry behind the dearer ones) is ${names.join(',')}`);
    const cheap = all.picks.find(p => p.name === 'cheap'), top2 = all.picks[0].resident + all.picks[1].resident;
    ok(cheap && cheap.resident < Math.min(...all.picks.filter(p => p.name !== 'cheap').map(p => p.resident)), `budget: cheap is the cheapest (${cheap && cheap.resident}tok) yet scores below big and mid`);
    r = run('pick', 'render the deck to pdf and check slide 8', '--budget', String(top2 + 1), '-k', '10', '--json');
    ok(r.j && r.j.picks.map(p => p.name).join(',') === names.slice(0, 2).join(',') && r.j.cost <= top2 + 1 && !r.j.picks.some(p => p.name === 'cheap'),
      `budget cut keeps the two best scores, drops the two lowest — not the cheapest-cost: ${r.j && JSON.stringify(r.j.picks.map(p => p.name))}`);
    ok(r.j && r.j['@?'].some(h => /\d fit the turn but not the budget/.test(h) && /cheap/.test(h)), `budget: the over-budget picks are a @?, got ${r.j && JSON.stringify(r.j['@?'])}`);
    ok(run('pick', 'render the deck', '--budget', '0').code === 2 && run('pick', 'render', '-k', '0').code === 2, 'pick: --budget 0 and -k 0 are FAILs');
    // --as relation boost: the mind slime-auth wears is pulled closer; its lane too
    r = run('pick', 'speak on the wire', '--as', 'slime-auth', '--json');
    const wire = r.j && r.j.picks.find(p => p.name === 'wire');
    ok(wire && wire.rel >= 0.23 && r.j.picks[0].name === 'wire', `--as slime-auth: wire worn+lane ≥ 0.23, got ${wire && wire.rel}`);
    ok(run('pick', 'speak on the wire', '--json').j.picks.find(p => p.name === 'wire').rel === 0, '--as rimuru: no relation boost');
    r = run('pick', 'the api gate', '--as', 'slime-auth', '--kind', 'body', '--json');
    ok(r.j && r.j.picks.length === 1 && r.j.picks[0].name === 'orc-api' && r.j.picks[0].rel >= 0.1 && /bond/.test(r.j.picks[0].why + ' bond'), `--kind body + parent bond: ${r.out.slice(0, 160)}`);
    ok(run('pick', 'x', '--kind', 'hat').code === 2 && run('pick', 'x', '--min', '-1').code === 2, 'pick: --kind hat and --min -1 are FAILs');
    // the floor: one incidental word in common is not a fit; --min 0 shows it, the default does not
    const one = run('pick', 'render the deck to pdf, check slide 8 and leave a note', '-k', '20', '--json').j.picks, two = run('pick', 'leave a note', '--json').j.picks;
    ok(!one.some(p => p.name === 'memo.js') && two.length === 1 && two[0].name === 'memo.js' && two[0].matched === 1, `fit rule: one word ("note") of seven is no fit; one of two is — got ${JSON.stringify([one.map(p => p.name), two.map(p => p.name)])}`);
    ok(run('pick', 'render the deck to pdf', '--min', '0.9', '--json').j.picks.every(p => p.score >= 0.9), '--min: a score floor');
    // holes: empty ask, a missing external
    r = run('pick', 'the of and', '--json');
    ok(r.code === 0 && r.j && r.j.picks.length === 0 && r.j['@?'].some(h => /no content words/.test(h)), `empty ask: ${r.out.slice(0, 160)}`);
    r = run('pick', 'embedding server', '--json');
    ok(r.j && r.j.picks[0].name === 'ollama' && r.j.picks[0].installed === false && r.j['@?'].some(h => /ollama.*not installed/.test(h)), `missing external is picked but named a hole: ${r.out.slice(0, 200)}`);
    // level 2: load whole, load a section, map; every load is one journal line; a map is not a load
    const journal = () => readJsonl(path.join(W, '.isekai', 'instruments', 'toolbox', 'loads.jsonl'));
    const before = journal().filter(x => x.ev === 'load').length;
    r = run('load', 'standards-check', '--json');
    ok(r.j && r.j['@S'] === 'LOAD' && r.j.sec === 'all' && /BODYSENTINEL-REVIEW/.test(r.j.text) && r.j.text.startsWith('---\nname: standards-check') && r.j.tokens === TOK(r.j.text), `load whole: ${r.out.slice(0, 160)}`);
    r = run('load', 'standards-check', '--sec', '3', '--as', 'orc-api', '--json');
    ok(r.j && r.j.sec === 3 && r.j.text === '## Spec\nSpec section text.\n' && !/BODYSENTINEL/.test(r.j.text), `load --sec 3 is exactly that section, got ${r.j && JSON.stringify(r.j.text)}`);
    ok(run('load', 'standards-check', '--sec', '0', '--json').j.text.startsWith('---\nname: standards-check'), 'load --sec 0 is the preamble (frontmatter)');
    ok(run('load', 'standards-check', '--sec', '9').code === 2, 'load --sec past the end is a FAIL that points at --map');
    r = run('load', 'standards-check', '--map', '--json');
    ok(r.j && r.j['@S'] === 'MAP' && r.j.sections.length === 4 && r.j.sections[2].title === 'Spec', `load --map: ${r.out.slice(0, 160)}`);
    ok(/^@S MAP standards-check sections=4/.test(run('load', 'standards-check', '--map').out), 'load --map wire form');
    const loadText = run('load', 'wire').out;
    ok(/^@S LOAD mind wire sec=all tokens=\d+ — \.claude\/skills\/wire\/SKILL\.md\n---\nname: wire/.test(loadText) && /BODYSENTINEL-WIRE/.test(loadText) && /\n@E \d+\n$/.test(loadText), `load wire form: ${loadText.slice(0, 120)}`);
    ok(/x\.sh — proving-grounds helper/.test(run('load', 'x.sh', '--json').j.text) && !/echo hi/.test(run('load', 'x.sh', '--json').j.text), 'load tool: the header, not the code');
    ok(run('load', 'soffice', '--json').j.text === 'soffice --headless --convert-to pdf <file>', 'load external: its usage notes');
    ok(run('load', 'ollama', '--json').j['@?'].some(h => /no usage notes/.test(h)), 'load external without usage: description + @?');
    ok(run('load', 'BODYSENTINEL').code === 2 && run('load').code === 2, 'load: unknown name / no name are FAILs');
    const loads = journal().filter(x => x.ev === 'load');
    ok(loads.length === before + 8 && loads.every(l => l.at && l.by && l.name && typeof l.tokens === 'number' && l.sec !== undefined) && loads.some(l => l.by === 'orc-api' && l.sec === 3), `journal: one line per load (${loads.length - before}), map not counted, {at, by, name, tokens, sec}`);
    // status: offered vs loaded, from the journal
    r = run('status', '--json');
    ok(r.j && r.j.registry.present && !r.j.registry.stale && r.j.registry.n === 14 && r.j.cost.residentIfAllInjected > 0 && r.j.cost.fullIfAllLoaded > r.j.cost.residentIfAllInjected, `status: ${r.out.slice(0, 200)}`);
    ok(r.j && r.j.instrument.loads === 8 && r.j.instrument.offers >= 8 && r.j.instrument.offered > r.j.instrument.loads && r.j.instrument.loadedTokens > 0 && r.j.instrument.residentTokens > 0, `status instrument: ${r.j && JSON.stringify(r.j.instrument)}`);
    ok(r.j && r.j.externals.installed === 1 && r.j.externals.missing.join() === 'ollama', 'status: externals measured');
    ok(/LOADS    offered \d+ \(\d+ distinct, \d+ picks\) · loaded 8 \(\d+ distinct\) · resident cost [\d,]+ tok · loaded cost [\d,]+ tok/.test(run('status').out), 'status text: offered vs loaded line');
    // stale: a source changed after the build → @? everywhere; rebuild clears it
    fs.utimesSync(path.join(W, '.claude', 'skills', 'wire', 'SKILL.md'), new Date(), new Date(Date.now() + 5000));
    r = run('status', '--json');
    ok(r.j && r.j.registry.stale && r.j['@?'].some(h => /older than its sources.*wire/.test(h)), `status stale: ${r.j && JSON.stringify(r.j['@?'])}`);
    ok(run('pick', 'wire', '--json').j['@?'].some(h => /older than its sources/.test(h)) && run('load', 'wire', '--json').j['@?'].some(h => /older than its sources/.test(h)), 'pick and load: a stale registry is a @?');
    ok(run('index', '--json').j.n === 14 && !run('status', '--json').j.registry.stale, 'index: rebuild clears stale');
    // no registry: answered live, named as a hole; empty world: every silence is a finding
    fs.unlinkSync(path.join(W, '.isekai', 'toolbox', 'registry.json'));
    r = run('pick', 'speak on the wire', '--json');
    ok(r.j && r.j.picks.length && r.j['@?'].some(h => /no registry on disk/.test(h)), 'no registry: live harvest + @?');
    const E = path.join(T, 'empty'); mkd(path.join(E, '.isekai'));
    const runE = (...a) => { const x = spawnSync(process.execPath, [__filename, E, ...a], { encoding: 'utf8' }); return { code: x.status, j: parse(x.stdout || '') }; };
    r = runE('index', '--json');
    ok(r.code === 0 && r.j.n === 0 && r.j['@?'].length === 1, 'empty world: index of nothing is a @?');
    r = runE('pick', 'anything', '--json');
    ok(r.code === 0 && r.j.picks.length === 0 && r.j['@?'].some(h => /registry empty/.test(h)), 'empty world: pick is a @?');
    ok(runE('status', '--json').code === 0, 'empty world: status parses');
  } finally {
    fs.rmSync(T, { recursive: true, force: true });
  }
  ok(!fs.existsSync(T), 'throwaway world removed');
  console.log(fails.length ? '@S FAIL' : `@S PASS ${checks} checks · node ${process.version}`);
  for (const f of fails) console.log(`@F selftest — ${f}`);
  if (fails.length) process.exit(1);
}

const run = { index, status, pick, brief, load, explain, selftest }[cmd];
if (!run) fail(`unknown command ${cmd} — index | status | pick | brief | load | explain | selftest`);
try { run(); } catch (e) { fail(`${cmd} crashed: ${e && e.message}`); }
