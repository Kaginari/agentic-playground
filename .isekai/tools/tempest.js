#!/usr/bin/env node
'use strict';
/**
 * tempest.js — the instrument board (Nature 9: Perception).
 *
 * Built from the spec in .isekai/canon/instruments-tempest.md and
 * .isekai/canon/rimuru-throne-body.md — no source implementation was ever photographed,
 * only the prose description of what it does. stdlib only, zero dependencies, so the shape
 * travels with the world. Ecosystem-neutral throughout: the board itself doesn't care whether
 * Claude Code or OpenCode is driving, and /holidays relief runs use whichever CLI (`opencode`
 * or `claude`) is actually on PATH — the source spec only ever named `opencode run`, but that
 * would silently strand Claude-only machines, so this adds the `claude -p` fallback.
 *
 * Usage:
 *   node tempest.js <target-dir> --ensure     idempotent heartbeat; starts the board if it
 *                                              isn't running, prints its URL either way
 *   node tempest.js <target-dir> --json       one-shot snapshot, no server needed
 *   node tempest.js <target-dir> --stop       sleeps a running board
 *   node tempest.js <target-dir> --ensure --immortal   opt out of the 30m idle sleep
 */

const fs = require('fs');
const path = require('path');
const http = require('http');
const crypto = require('crypto');
const { spawn, spawnSync } = require('child_process');

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--ensure') args.ensure = true;
    else if (a === '--stop') args.stop = true;
    else if (a === '--json') args.json = true;
    else if (a === '--serve') args.serve = true;
    else if (a === '--immortal') args.immortal = true;
    else if (a === '--ttl') args.ttl = Number(argv[++i]);
    else if (a === '--port') args.port = Number(argv[++i]);
    else args._.push(a);
  }
  return args;
}

const args = parseArgs(process.argv.slice(2));
const root = path.resolve(args._[0] || '.');
const isekaiDir = path.join(root, '.isekai');

function fail(msg) {
  console.error(msg);
  process.exit(1);
}

if (!fs.existsSync(isekaiDir)) {
  fail(`no .isekai/ world at ${root} — run /isekai first`);
}

const instrumentsDir = path.join(isekaiDir, 'instruments');
const pidFile = path.join(instrumentsDir, 'board.pid');
const stateFile = path.join(instrumentsDir, 'board.json');
const metricsDir = path.join(isekaiDir, 'metrics');
const tokensFile = path.join(metricsDir, 'tokens.jsonl');
const reliefFile = path.join(metricsDir, 'relief.jsonl');

// ---------------------------------------------------------------- world identity

function ensureName() {
  const nameFile = path.join(isekaiDir, 'name');
  if (fs.existsSync(nameFile)) {
    return fs.readFileSync(nameFile, 'utf8').trim();
  }
  const base = path.basename(root).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'world';
  fs.writeFileSync(nameFile, base + '\n');
  return base;
}

// stable per-root hash so many worlds get distinct, collision-free ports without coordination
function derivePort(rootPath) {
  const hash = crypto.createHash('sha1').update(rootPath).digest();
  return 30000 + (hash.readUInt16BE(0) % 10000);
}

// ---------------------------------------------------------------- state (pid + lease)

function readState() {
  try { return JSON.parse(fs.readFileSync(stateFile, 'utf8')); } catch { return null; }
}
function writeState(state) {
  fs.mkdirSync(instrumentsDir, { recursive: true });
  fs.writeFileSync(stateFile, JSON.stringify(state, null, 2));
}
function removeFile(p) {
  try { fs.unlinkSync(p); } catch {}
}
function isAlive(pid) {
  if (!pid) return false;
  try { process.kill(pid, 0); return true; } catch { return false; }
}

// ---------------------------------------------------------------- data collection
// shared by --json, the served dashboard, and the holidays worklist

const DEFAULT_THOUGHTS_LIMIT = 5;
const RACE_LIMITS = { dark_elf: 10 }; // breath law: ground races 5, dark elf 10 (rule 14)
const DIET_BYTES = 6 * 1024; // rule 14's ~6KB diet
const RACES = ['elf', 'orc', 'slime', 'kijin', 'high_elf', 'high_orc', 'dark_elf'];

// from .isekai/canon/model-assignments.md — not transcribed canon, reasoned guidance mapping
// each office's actual job (as the canon itself defines it) onto model cost/capability tiers.
// Rimuru is deliberately excluded: "the mount is whatever model the human picked" (throne body).
const MODEL_GUIDE = [
  { group: 'Court triad', rank: 'Great Sage', job: 'Perceive only — read wide, report thin, no judgment', model: 'Haiku 4.5', why: 'Bulk retrieval/extraction, high dispatch volume, its own office laws forbid anything heavier.' },
  { group: 'Court triad', rank: 'Raphael', job: 'Judge — verdicts need file:line evidence, BLOCK beats a lazy PASS', model: 'Sonnet 5', why: 'Correctness-critical. Opus 5 for high-stakes domains (security, payments, migrations).' },
  { group: 'Court triad', rank: 'Ciel', job: 'Speak — durable, journal-ready, human-facing prose', model: 'Sonnet 5', why: 'Prose quality and voice consistency over raw reasoning. Fable 5.1 worth trialing, unconfirmed.' },
  { group: 'Base ranks', rank: 'Elf', job: 'Shared mind and voice — cross-domain, drafts outward messages', model: 'Sonnet 5', why: 'Closest to Rimuru in reasoning load. Opus 5 for large worlds with real cross-domain conflict.' },
  { group: 'Base ranks', rank: 'Orc', job: 'Domain ruler and the actual landing gate', model: 'Sonnet 5', why: 'Same reasoning-critical profile as Raphael, with real teeth. Opus 5 for high-stakes domains.' },
  { group: 'Base ranks', rank: 'Slime', job: 'One narrow zone, deep but narrow, frequently reloaded', model: 'Haiku 4.5', why: 'Cost efficiency at volume. Sonnet 5 per-slime when a zone’s own logic is intrinsically complex.' },
  { group: 'Ascended', rank: 'Kijin / High Orc / Dark Elf', job: 'Rare, human-declared, long-lived, archival memory', model: 'Opus 5', why: 'Scarcity makes cost moot — default to the ceiling until there’s a reason not to.' },
];

function safeRead(p) {
  try { return fs.readFileSync(p, 'utf8'); } catch { return ''; }
}

function listCreatures() {
  const creatures = [];
  for (const race of RACES) {
    const raceDir = path.join(isekaiDir, race);
    if (!fs.existsSync(raceDir)) continue;
    for (const name of fs.readdirSync(raceDir)) {
      const dir = path.join(raceDir, name);
      if (!fs.statSync(dir).isDirectory()) continue;
      creatures.push({ race, name, dir, doc: findDoc(dir, name) });
    }
  }
  return creatures;
}

function findDoc(dir, name) {
  for (const c of [`${name}.md`, 'index.md', 'SKILL.md']) {
    const p = path.join(dir, c);
    if (fs.existsSync(p)) return p;
  }
  const md = fs.readdirSync(dir).filter((f) => f.endsWith('.md'));
  return md.length ? path.join(dir, md[0]) : null;
}

function countThoughts(text) {
  const m = text.match(/^##\s+Thoughts\s*$/m);
  if (!m) return 0;
  const rest = text.slice(m.index + m[0].length);
  const next = rest.search(/^##\s+/m);
  const section = next === -1 ? rest : rest.slice(0, next);
  const entries = section.match(/^-\s+\d{4}-\d{2}-\d{2}|^###\s+\d{4}-\d{2}-\d{2}/gm);
  return entries ? entries.length : 0;
}

function census() {
  const creatures = listCreatures();
  const byRace = {};
  for (const c of creatures) byRace[c.race] = (byRace[c.race] || 0) + 1;
  return { total: creatures.length, byRace };
}

function deskStress() {
  return listCreatures().map((c) => {
    const text = c.doc ? safeRead(c.doc) : '';
    const thoughts = countThoughts(text);
    const limit = RACE_LIMITS[c.race] || DEFAULT_THOUGHTS_LIMIT;
    const bytes = Buffer.byteLength(text, 'utf8');
    return {
      race: c.race,
      name: c.name,
      thoughts,
      limit,
      stress: limit ? thoughts / limit : 0,
      bytes,
      overDiet: bytes > DIET_BYTES,
    };
  });
}

// "links" = how often a creature's id is cross-referenced from every OTHER creature's doc
// and from the chronicle — a stand-in for the "crossings" the spec names but never defines
function linkCounts(creatures) {
  const logText = safeRead(path.join(isekaiDir, 'log.md'));
  const counts = {};
  for (const c of creatures) {
    const id = `${c.race}-${c.name}`;
    const pattern = new RegExp(id.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g');
    let n = (logText.match(pattern) || []).length;
    for (const other of creatures) {
      if (other === c) continue;
      n += (safeRead(other.doc || '').match(pattern) || []).length;
    }
    counts[id] = n;
  }
  return counts;
}

function genesisWatch() {
  const creatures = listCreatures();
  const stressRows = deskStress();
  const links = linkCounts(creatures);
  const values = Object.values(links);
  const sorted = [...values].sort((a, b) => a - b);
  const median = sorted.length ? sorted[Math.floor(sorted.length / 2)] : 0;
  const flagged = [];
  for (const row of stressRows) {
    const id = `${row.race}-${row.name}`;
    const l = links[id] || 0;
    if (row.stress >= 0.8 && l >= 1.5 * median) flagged.push({ id, stress: row.stress, links: l });
  }
  return { median, flagged };
}

function listDirs(dir) {
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir).filter((name) => fs.statSync(path.join(dir, name)).isDirectory());
}

function domainTree() {
  const tree = {};
  for (const orc of listDirs(path.join(isekaiDir, 'orc'))) tree[orc] = [];
  for (const slime of listDirs(path.join(isekaiDir, 'slime'))) {
    // no routing table to resolve orc ownership from yet — left unassigned rather than guessed
    (tree.__unassigned = tree.__unassigned || []).push(slime);
  }
  return tree;
}

function evolutionSeries() {
  const byDay = {};
  const dates = safeRead(path.join(isekaiDir, 'log.md')).match(/^### \[(\d{4}-\d{2}-\d{2})/gm) || [];
  for (const d of dates) {
    const day = d.match(/\d{4}-\d{2}-\d{2}/)[0];
    (byDay[day] = byDay[day] || { journal: 0, commits: 0 }).journal++;
  }
  const out = spawnSync('git', ['-C', root, 'log', '--format=%ad', '--date=short'], { encoding: 'utf8' });
  if (out.status === 0) {
    for (const line of out.stdout.split('\n').filter(Boolean)) {
      (byDay[line] = byDay[line] || { journal: 0, commits: 0 }).commits++;
    }
  }
  return byDay;
}

// opencode's own session store format isn't documented anywhere in the transcribed canon,
// so this stays a stub rather than a fragile guess at an undocumented schema
function tokenBucket() {
  const lines = safeRead(tokensFile).split('\n').filter(Boolean);
  const totals = { entries: lines.length, in: 0, out: 0, byAgent: {}, byModel: {} };
  for (const line of lines) {
    try {
      const row = JSON.parse(line);
      totals.in += row.in || 0;
      totals.out += row.out || 0;
      if (row.agent) totals.byAgent[row.agent] = (totals.byAgent[row.agent] || 0) + (row.in || 0) + (row.out || 0);
      if (row.model) totals.byModel[row.model] = (totals.byModel[row.model] || 0) + (row.in || 0) + (row.out || 0);
    } catch {}
  }
  return totals;
}

function snapshot() {
  return {
    world: ensureName(),
    root,
    generatedAt: new Date().toISOString(),
    census: census(),
    deskStress: deskStress(),
    genesisWatch: genesisWatch(),
    domainTree: domainTree(),
    evolution: evolutionSeries(),
    tokens: tokenBucket(),
    modelGuide: MODEL_GUIDE,
  };
}

// ---------------------------------------------------------------- holidays / relief runs

function runHolidays({ dry, only }) {
  const day = new Date().toISOString().slice(0, 10);
  const worklist = deskStress().filter((r) => r.stress >= 1 && (!only || r.name === only));
  const plan = { day, dry, only: only || null, worklist };

  if (dry) return plan;

  const dayDir = path.join(isekaiDir, 'tmp', day);
  fs.mkdirSync(dayDir, { recursive: true });
  const lines = worklist.map((r) => `- ${r.race}-${r.name}: stress ${Math.round(r.stress * 100)}% (${r.thoughts}/${r.limit} thoughts)`);
  fs.writeFileSync(path.join(dayDir, 'holidays.md'), `# Relief worklist — ${day}\n\n${lines.join('\n') || '(nothing over the diet)'}\n`);

  fs.mkdirSync(metricsDir, { recursive: true });
  const cli = detectReliefCli(); // { bin: 'opencode'|'claude', args: (prompt) => [...] } or null
  plan.launched = !!cli;
  plan.cli = cli ? cli.bin : null;
  if (!cli) plan.note = 'neither opencode nor claude found on PATH — worklist written, relief runs not launched';

  for (const row of worklist) {
    const entry = { ts: new Date().toISOString(), creature: `${row.race}-${row.name}`, launched: !!cli, cli: cli ? cli.bin : null };
    fs.appendFileSync(reliefFile, JSON.stringify(entry) + '\n');
    if (cli) {
      const prompt = `relief for ${row.race}-${row.name}: distil its Thoughts, split by diet, review under genesis-watch`;
      // sequential, one 10-minute cap per step, matching the shed's own budget (rule 17)
      spawnSync(cli.bin, cli.args(prompt), { cwd: root, timeout: 10 * 60 * 1000 });
    }
  }
  return plan;
}

// prefer opencode (the CLI the source spec was written against: "opencode run"); fall back to
// Claude Code's own headless mode (`claude -p`) so relief runs work on Claude-only machines too
function detectReliefCli() {
  const which = process.platform === 'win32' ? 'where' : 'which';
  if (spawnSync(which, ['opencode']).status === 0) {
    return { bin: 'opencode', args: (prompt) => ['run', prompt] };
  }
  if (spawnSync(which, ['claude']).status === 0) {
    return { bin: 'claude', args: (prompt) => ['-p', prompt] };
  }
  return null;
}

// ---------------------------------------------------------------- dashboard (Bootstrap 5)

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function stressBadge(stress) {
  const pct = Math.round(stress * 100);
  const cls = stress >= 1 ? 'text-bg-danger' : stress >= 0.6 ? 'text-bg-warning' : 'text-bg-success';
  return `<span class="badge ${cls}">${pct}%</span>`;
}

function renderHtml(data) {
  const raceSummary = Object.entries(data.census.byRace).map(([r, n]) => `${esc(r)}: ${n}`).join(' · ') || 'none yet';

  const rows = data.deskStress.length
    ? data.deskStress
        .map(
          (r) => `<tr>
            <td>${esc(r.race)}</td>
            <td>${esc(r.name)}</td>
            <td>${r.thoughts}/${r.limit}</td>
            <td>${stressBadge(r.stress)}</td>
            <td>${r.bytes.toLocaleString()}B ${r.overDiet ? '<span class="badge text-bg-danger ms-1">over diet</span>' : ''}</td>
          </tr>`
        )
        .join('\n')
    : `<tr><td colspan="5" class="text-muted text-center py-3">no creatures yet</td></tr>`;

  const flaggedItems = data.genesisWatch.flagged.length
    ? data.genesisWatch.flagged
        .map(
          (f) => `<li class="list-group-item d-flex justify-content-between align-items-center">
            <span>${esc(f.id)}</span>
            <span class="text-muted small">${Math.round(f.stress * 100)}% stress · ${f.links} links</span>
          </li>`
        )
        .join('\n')
    : `<li class="list-group-item text-muted">nothing flagged</li>`;

  const orcNames = Object.keys(data.domainTree).filter((k) => k !== '__unassigned');
  const domainHtml = orcNames.length
    ? `<ul class="list-unstyled mb-0">${orcNames.map((o) => `<li>🐗 <code>${esc(o)}</code></li>`).join('')}</ul>`
    : `<p class="text-muted mb-0">no orcs yet</p>`;
  const unassigned = data.domainTree.__unassigned || [];

  const evolutionRows = Object.entries(data.evolution).sort(([a], [b]) => (a < b ? 1 : -1));
  const evolutionHtml = evolutionRows.length
    ? `<div class="table-responsive"><table class="table table-sm mb-0">
        <thead><tr><th>Day</th><th>Journal entries</th><th>Commits</th></tr></thead>
        <tbody>${evolutionRows.map(([day, v]) => `<tr><td>${day}</td><td>${v.journal}</td><td>${v.commits}</td></tr>`).join('')}</tbody>
      </table></div>`
    : `<p class="text-muted mb-0">no history yet</p>`;

  const overDietCount = data.deskStress.filter((r) => r.overDiet).length;
  const overDietClass = overDietCount ? 'text-danger' : '';
  const flaggedClass = data.genesisWatch.flagged.length ? 'text-danger' : '';

  const modelBadge = (m) => {
    const cls = m.startsWith('Opus') ? 'text-bg-danger' : m.startsWith('Sonnet') ? 'text-bg-primary' : m.startsWith('Fable') ? 'text-bg-info' : 'text-bg-success';
    return `<span class="badge ${cls}">${esc(m)}</span>`;
  };
  const modelRows = data.modelGuide
    .map(
      (m, i) => `<tr>
        ${i === 0 || data.modelGuide[i - 1].group !== m.group ? `<td rowspan="${data.modelGuide.filter((x) => x.group === m.group).length}" class="text-muted small text-uppercase align-middle">${esc(m.group)}</td>` : ''}
        <td><strong>${esc(m.rank)}</strong></td>
        <td class="small">${esc(m.job)}</td>
        <td>${modelBadge(m.model)}</td>
        <td class="small text-muted">${esc(m.why)}</td>
      </tr>`
    )
    .join('\n');

  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(data.world)} — tempest board</title>
<link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css" rel="stylesheet">
<style>
  :root { color-scheme: light dark; }
  body { background: #f5f6f8; }
  .navbar-brand { font-weight: 600; letter-spacing: .01em; }
  .stat-card .display-6 { font-weight: 700; }
  footer { color: #8a8f98; font-size: .85rem; }
  @media (prefers-color-scheme: dark) {
    body { background: #14161a; color: #e6e6e6; }
    .card { background: #1c1f26; color: #e6e6e6; border-color: #2a2e37; }
    .table { color: inherit; }
    .list-group-item { background: transparent; color: inherit; border-color: #2a2e37; }
    code { color: #9ecbff; }
  }
</style>
</head>
<body>
<nav class="navbar navbar-dark bg-dark mb-4">
  <div class="container">
    <span class="navbar-brand">🌍 ${esc(data.world)}</span>
    <span class="navbar-text small text-white-50 d-none d-md-inline">${esc(data.root)} · generated ${esc(data.generatedAt)}</span>
  </div>
</nav>

<div class="container pb-5">

  <div class="row g-3 mb-4">
    <div class="col-6 col-lg-3">
      <div class="card stat-card shadow-sm h-100"><div class="card-body">
        <div class="text-muted small text-uppercase">Creatures</div>
        <div class="display-6">${data.census.total}</div>
        <div class="small text-muted">${raceSummary}</div>
      </div></div>
    </div>
    <div class="col-6 col-lg-3">
      <div class="card stat-card shadow-sm h-100"><div class="card-body">
        <div class="text-muted small text-uppercase">Genesis watch</div>
        <div class="display-6 ${flaggedClass}">${data.genesisWatch.flagged.length}</div>
        <div class="small text-muted">median links ${data.genesisWatch.median}</div>
      </div></div>
    </div>
    <div class="col-6 col-lg-3">
      <div class="card stat-card shadow-sm h-100"><div class="card-body">
        <div class="text-muted small text-uppercase">Tokens logged</div>
        <div class="display-6">${data.tokens.entries}</div>
        <div class="small text-muted">${data.tokens.in.toLocaleString()} in / ${data.tokens.out.toLocaleString()} out</div>
      </div></div>
    </div>
    <div class="col-6 col-lg-3">
      <div class="card stat-card shadow-sm h-100"><div class="card-body">
        <div class="text-muted small text-uppercase">Over diet</div>
        <div class="display-6 ${overDietClass}">${overDietCount}</div>
        <div class="small text-muted">6KB limit (rule 14)</div>
      </div></div>
    </div>
  </div>

  <div class="row g-3">
    <div class="col-lg-8">
      <div class="card shadow-sm mb-3">
        <div class="card-header fw-semibold">Desk stress</div>
        <div class="table-responsive">
          <table class="table table-sm mb-0 align-middle">
            <thead><tr><th>Race</th><th>Name</th><th>Thoughts</th><th>Stress</th><th>Diet</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </div>

      <div class="card shadow-sm">
        <div class="card-header fw-semibold">Evolution series</div>
        <div class="card-body">${evolutionHtml}</div>
      </div>
    </div>

    <div class="col-lg-4">
      <div class="card shadow-sm mb-3">
        <div class="card-header fw-semibold">Genesis watch</div>
        <ul class="list-group list-group-flush">${flaggedItems}</ul>
      </div>

      <div class="card shadow-sm">
        <div class="card-header fw-semibold">Domain tree</div>
        <div class="card-body small">
          ${domainHtml}
          ${unassigned.length ? `<hr><div class="text-muted mb-1">unassigned slimes</div><ul class="list-unstyled mb-0">${unassigned.map((s) => `<li>🟡 <code>${esc(s)}</code></li>`).join('')}</ul>` : ''}
        </div>
      </div>
    </div>
  </div>

  <div class="card shadow-sm mt-3">
    <div class="card-header fw-semibold">Suggested models per rank/office</div>
    <div class="table-responsive">
      <table class="table table-sm mb-0 align-middle">
        <thead><tr><th>Group</th><th>Rank</th><th>Job</th><th>Model</th><th>Why</th></tr></thead>
        <tbody>${modelRows}</tbody>
      </table>
    </div>
    <div class="card-body small text-muted py-2">
      Rimuru excluded on purpose — the throne's mount is whatever model the human picked, never
      hardcoded. Reasoned guidance, not transcribed canon — see
      <code>.isekai/canon/model-assignments.md</code>.
    </div>
  </div>

  <footer class="text-center mt-4">tempest.js · this page pulses every 60s to keep the board's lease alive</footer>
</div>

<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"></script>
<script>setInterval(() => { fetch(location.pathname).catch(() => {}); }, 60000);</script>
</body>
</html>`;
}

// ---------------------------------------------------------------- CLI commands

function boardUrl(name, port) {
  return `http://localhost:${port}/${name}/`;
}

function cmdEnsure() {
  const name = ensureName();
  const port = args.port || derivePort(root);
  fs.mkdirSync(instrumentsDir, { recursive: true });
  const state = readState();

  if (state && isAlive(state.pid)) {
    writeState({ ...state, lastSeen: Date.now() }); // renew the lease
    console.log(boardUrl(name, state.port || port));
    return;
  }

  const serveArgs = [__filename, root, '--serve', '--port', String(port)];
  if (args.immortal) serveArgs.push('--immortal');
  if (args.ttl) serveArgs.push('--ttl', String(args.ttl));
  const child = spawn(process.execPath, serveArgs, { detached: true, stdio: 'ignore' });
  child.unref();
  writeState({ pid: child.pid, port, startedAt: Date.now(), lastSeen: Date.now(), immortal: !!args.immortal });
  console.log(boardUrl(name, port));
}

function cmdStop() {
  const state = readState();
  if (!state || !isAlive(state.pid)) {
    console.log('no board running');
    return;
  }
  try { process.kill(state.pid); } catch {}
  removeFile(pidFile);
  writeState({ ...state, pid: null, stoppedAt: Date.now() });
  console.log('board stopped');
}

function cmdJson() {
  console.log(JSON.stringify(snapshot(), null, 2));
}

function startServer() {
  const name = ensureName();
  const port = args.port || derivePort(root);
  const immortal = !!args.immortal;
  const ttlMinutes = args.ttl || 30;
  let lastSeen = Date.now();

  const server = http.createServer((req, res) => {
    lastSeen = Date.now();
    writeState({ pid: process.pid, port, lastSeen, immortal });

    const url = new URL(req.url, `http://127.0.0.1:${port}`);
    const base = `/${name}/`;

    if (req.method === 'GET' && (url.pathname === base || url.pathname === base.slice(0, -1))) {
      const data = snapshot();
      if (url.searchParams.get('json') === '1') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(data, null, 2));
      } else {
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end(renderHtml(data));
      }
      return;
    }

    if (req.method === 'POST' && url.pathname === `${base}holidays`) {
      const result = runHolidays({ dry: url.searchParams.get('dry') === '1', only: url.searchParams.get('only') });
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(result, null, 2));
      return;
    }

    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('not found');
  });

  server.listen(port, '127.0.0.1', () => {
    writeState({ pid: process.pid, port, startedAt: lastSeen, lastSeen, immortal });
  });

  if (!immortal) {
    setInterval(() => {
      if ((Date.now() - lastSeen) / 60000 >= ttlMinutes) {
        removeFile(pidFile);
        process.exit(0);
      }
    }, 60 * 1000).unref();
  }
}

// ---------------------------------------------------------------- entry point

if (args.serve) startServer();
else if (args.stop) cmdStop();
else if (args.json) cmdJson();
else cmdEnsure(); // bare invocation behaves like --ensure — the form rimuru's first act always uses
