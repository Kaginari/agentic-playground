#!/usr/bin/env node
// loop.js — the world's agent loop (isekai.md §Minds & Bodies "Court" mode; Absolute Rules II
// the wire and III escalation; Nature 7 Containment; Nature 9 Perception; Laws 5 and 6).
// Stdlib Node only, Node 16-compatible; the shape travels with the world.
//
// A Court Body is dispatched, works, and is gone; only what it wrote to .isekai/ survives it.
// This is the rhythm it works to — six beats per step, on its own, bounded by readings taken
// from instruments, never by feelings:
//
//   PERCEIVE  read the budgets: steps left, minutes left, and the context window as memory.js
//             status measures it (context-check.sh's method — 200k budget, 180k stress zone).
//             Past the stress zone the run checkpoints and stops honestly; it never presses on.
//   RECALL    memory.js recall "<the step's ask>" --as <body> — the top hits ride into the step
//             as paths + section anchors, never payloads (point, don't carry: $LOOP_RECALL). A
//             missing or stale index is rebuilt first, never routed around. Then the toolbox:
//             toolbox.js brief "<the step's ask>" --as <body> --budget <tool budget> — the level-1
//             manifest that fits the ask and the budget rides in as @T lines ($LOOP_TOOLS): names,
//             paths and both costs, never a body; the picks are journaled the same way (names +
//             costs). A missing or stale registry is rebuilt first. A step may set toolbox: false.
//   PLAN      the step's class is settled — read · write · outward · destructive. A heuristic
//             classifier reads the command; the plan may declare a class; a declaration can only
//             tighten, never loosen. outward (Nature 7) and destructive (Law 6) ALWAYS pass the
//             human gate — a y/N on a TTY, an explicit --approve <id>, or --dry-run, which only
//             says what it would ask. Nothing is auto-approved; a denial stops the run there.
//   ACT       the command runs (shell, cwd = the world root); a failure retries with bounded
//             backoff (default 2 retries, 500 ms doubling, 10 s cap).
//   VERIFY    the step's verify command runs; a failing verify is a failure OF THE ACT — the act
//             is retried, and when retries are exhausted the run escalates ONE hop up (Absolute
//             Rule III): a wire report carrying @?, exit 3. Never a guess past a failure, never
//             a loop forever.
//   RECORD    every beat is one JSON line in .isekai/instruments/loop/<run-id>.jsonl —
//             instrument state, machine-local, gitignored. A crashed, cleared or interrupted run
//             resumes from its journal (`resume <run-id>`): done steps are skipped, an act that
//             was cut mid-flight is re-gated if it could have reached outside. A step the plan
//             marks `remember` lands a shared note (memory.js remember) once it has passed —
//             memory flows up.
//
// The same six beats are the protocol an LLM-driven Court Body follows by hand when the work is
// not scriptable; there the gate is the host's own permission prompt (Claude Code / OpenCode),
// never worked around. Design and the plan format: .isekai/canon/agent-loop.md.
//
// Plan: JSON {as?, budget?: {steps, minutes}, steps: [...]}, a bare JSON array, or JSONL (one
// step per line). A step is
//   {id, ask, run?, verify?, class?, retries?, recall?: false | "<question>", toolbox?: false,
//    remember?: "<note>" | {text, kind, tag}, cwd?}
// Exit: 0 DONE · 2 FAIL (usage, unreadable plan) · 3 ESCALATE · 4 DENIED · 5 CHECKPOINT (budget)
//
// Usage:
//   node loop.js [root] run <plan> [--as <body>] [--approve id,…] [--strict] [--dry-run]
//                            [--max-steps N] [--max-minutes N] [--retries N] [--backoff-ms N]
//                            [--recall-k N] [--tool-budget N] [--step-timeout-s N] [--json]
//   node loop.js [root] resume <run-id> [same flags]
//   node loop.js [root] explain <plan>
//   node loop.js [root] status [run-id] [--json]
//   node loop.js [root] selftest   (Law 5: a throwaway world under .isekai/tmp/, cleaned up)
'use strict';
const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');
const readline = require('readline');
const { spawnSync } = require('child_process');

// ------------------------------------------------------------------ args
const argv = process.argv.slice(2);
const flags = {}; const pos = [];
for (let i = 0; i < argv.length; i++) {
  const a = argv[i];
  if (a === '--json' || a === '--strict' || a === '--dry-run') flags[a.slice(2)] = true;
  else if (a.startsWith('--')) flags[a.slice(2)] = argv[++i];
  else pos.push(a);
}
const fail = (...holes) => { console.error(['@S FAIL', ...holes.map(h => '@? ' + h)].join('\n')); process.exit(2); };
// The world root: an explicit positional, else the nearest ancestor of cwd that holds .isekai/.
let root = null;
if (pos[0] && fs.existsSync(path.join(pos[0], '.isekai'))) root = path.resolve(pos.shift());
else { let d = process.cwd(); for (;;) { if (fs.existsSync(path.join(d, '.isekai'))) { root = d; break; } const up = path.dirname(d); if (up === d) break; d = up; } }
if (!root) fail(`no .isekai/ at or above ${process.cwd()}`);
const cmd = pos.shift() || 'status';
const HOME = path.join(root, '.isekai');
const JOURNALS = path.join(HOME, 'instruments', 'loop');
const MEMORY = path.join(__dirname, 'memory.js');   // the sibling instrument; a throwaway world borrows it unchanged
const TOOLBOX = path.join(__dirname, 'toolbox.js'); // the other sibling: level-1 manifests for each step, never a body
let AS = String(flags.as || 'court-body').toLowerCase();   // the plan's `as` applies when no --as was given
const CLASSES = ['read', 'write', 'outward', 'destructive'];   // ordinal: a declaration may only move right
const RANK = c => CLASSES.indexOf(c);
const EXIT = { DONE: 0, FAIL: 2, ESCALATE: 3, DENIED: 4, CHECKPOINT: 5 };
const TAIL = 400;   // bytes of stdout/stderr kept per attempt — the journal points, it does not carry

const rd = p => { try { return fs.readFileSync(p, 'utf8'); } catch { return null; } };
const mkd = p => fs.mkdirSync(p, { recursive: true });
const rel = p => path.relative(root, p) || '.';
const now = () => new Date().toISOString();
const md5 = s => crypto.createHash('md5').update(s).digest('hex');
const own = (o, k) => Object.prototype.hasOwnProperty.call(o, k) ? o[k] : undefined;
const sleep = ms => new Promise(r => setTimeout(r, ms));
const num = (v, dflt, what, min) => { if (v === undefined) return dflt; const n = Number(v); if (!(n >= (min === undefined ? 0 : min))) fail(`${what} needs a number ≥ ${min === undefined ? 0 : min}, got ${v}`); return n; };
const tail = s => { s = String(s || ''); return s.length > TAIL ? '…' + s.slice(-TAIL) : s; };
function appendLine(p, obj) { mkd(path.dirname(p)); fs.appendFileSync(p, JSON.stringify(obj) + '\n'); }   // one O_APPEND write; atomic below PIPE_BUF
function readJsonl(p) { return (rd(p) || '').split('\n').filter(Boolean).map(l => { try { return JSON.parse(l); } catch { return null; } }).filter(x => x && typeof x === 'object'); }

// ------------------------------------------------------------------ plan: small, explicit, validated before anything runs
function loadPlan(src) {
  const p = path.resolve(process.cwd(), src);
  const text = rd(p); if (text === null) fail(`plan not found: ${src}`);
  let obj = null;
  try { obj = JSON.parse(text); } catch {
    const lines = text.split('\n').filter(l => l.trim());
    const steps = []; for (const [i, l] of lines.entries()) { try { steps.push(JSON.parse(l)); } catch { fail(`plan line ${i + 1} is neither JSON nor a JSONL step`); } }
    obj = { steps };
  }
  if (Array.isArray(obj)) obj = { steps: obj };
  if (!obj || !Array.isArray(obj.steps)) fail('plan needs a `steps` array (or a JSONL file of steps)');
  const plan = { src: rel(p), hash: md5(text), as: obj.as || null, budget: Object.assign({}, obj.budget || {}), steps: obj.steps.map(s => validateStep(s)) };
  const ids = new Set();
  for (const s of plan.steps) { if (ids.has(s.id)) fail(`duplicate step id ${s.id}`); ids.add(s.id); }
  if (!plan.steps.length) fail('plan has no steps');
  return plan;
}
function validateStep(s) {
  if (!s || typeof s !== 'object') fail('a step must be an object');
  if (typeof s.id !== 'string' || !/^[A-Za-z0-9_.-]{1,64}$/.test(s.id)) fail(`step id must match [A-Za-z0-9_.-]{1,64}, got ${JSON.stringify(s.id)}`);
  if (typeof s.ask !== 'string' || !s.ask.trim()) fail(`step ${s.id}: ask must be a non-empty string — it is the step's question, what recall searches for and what the journal answers`);
  if (s.run !== undefined && (typeof s.run !== 'string' || !s.run.trim())) fail(`step ${s.id}: run must be a shell command string`);
  if (s.verify !== undefined && (typeof s.verify !== 'string' || !s.verify.trim())) fail(`step ${s.id}: verify must be a shell command string`);
  if (s.verify && !s.run) fail(`step ${s.id}: verify without run — nothing to verify`);
  if (s.class !== undefined && !CLASSES.includes(s.class)) fail(`step ${s.id}: class must be ${CLASSES.join('|')}, got ${s.class}`);
  if (s.retries !== undefined && !(Number.isInteger(s.retries) && s.retries >= 0)) fail(`step ${s.id}: retries must be an integer ≥ 0`);
  if (s.recall !== undefined && s.recall !== false && (typeof s.recall !== 'string' || !s.recall.trim())) fail(`step ${s.id}: recall must be false or a question string`);
  if (s.toolbox !== undefined && s.toolbox !== false) fail(`step ${s.id}: toolbox must be false (the manifest is on by default and follows the ask)`);
  if (s.remember !== undefined) {
    const r = typeof s.remember === 'string' ? { text: s.remember } : s.remember;
    if (!r || typeof r.text !== 'string' || !r.text.trim()) fail(`step ${s.id}: remember must be a note string or {text, kind?, tag?}`);
    if (r.kind !== undefined && !['law', 'colony', 'territory'].includes(r.kind)) fail(`step ${s.id}: remember.kind must be law|colony|territory`);
    s.remember = r;
  }
  if (s.cwd !== undefined && typeof s.cwd !== 'string') fail(`step ${s.id}: cwd must be a path string`);
  if (!s.run && !s.remember && s.recall === false) fail(`step ${s.id}: nothing to do — no run, no remember, recall off`);
  return { id: s.id, ask: s.ask.trim(), run: s.run, verify: s.verify, class: s.class, retries: s.retries, recall: s.recall === undefined ? true : s.recall, toolbox: s.toolbox === undefined ? true : s.toolbox, remember: s.remember, cwd: s.cwd };
}

// ------------------------------------------------------------------ classifier: a heuristic, not a proof — it errs toward asking
// A command is split at pipes and separators; the strongest class of any segment wins. A pattern
// matches at a command position (line start, after ; & | ( ` $ or a quote) so `bash -c "git push"`
// is still outward. Unknown commands (scripts, interpreters) default to write — inside the world,
// no approval unless --strict; declare `class` to tighten when the tool knows better.
const SYSTEM = ['/dev/', '/usr/', '/bin/', '/sbin/', '/lib', '/etc/', '/proc/', '/sys/', '/opt/', '/snap/'];
const at = s => new RegExp('(?:^|[\\s;&|(`$\'"])' + s);
const RECORD = '(isekai\\.md|log\\.md|canon/[^\\s]*\\.md|notes\\.jsonl)';
const DESTRUCTIVE = [
  [at('rm(\\s|$)'), 'rm'], [at('(shred|wipe|srm)(\\s|$)'), 'shred'], [at('(mv|rename)\\s'), 'mv overwrites'], [at('(truncate|dd|mkfs)(\\s|$)'), 'truncate/dd/mkfs'],
  [at('git\\s+(reset|clean|filter-branch|filter-repo|rebase|gc|prune|rm|mv)\\b'), 'git history/tree rewrite'], [at('git\\s+branch\\b[^|;&]*\\s-[dDM]\\b'), 'git branch delete/rename'],
  [at('git\\s+push\\b[^|;&]*(\\s--force\\b|\\s-f\\b|\\s\\+)'), 'git push --force'], [at('git\\s+(checkout|restore)\\s+(--\\s|\\.(\\s|$))'), 'git discard of working changes'],
  [at('git\\s+stash\\s+(drop|clear|pop)'), 'git stash drop'], [at('git\\s+tag\\s+-d\\b'), 'git tag delete'], [/\s-delete(\s|$)/, 'find -delete'],
  [new RegExp('(^|[^>])>\\s*\\S*' + RECORD), 'overwrite of a record (> path)'], [new RegExp('sed\\s+(-\\S*i|--in-place)[^|;&]*' + RECORD), 'sed -i on a record'],
  [new RegExp('tee\\s+(?!-a)[^|;&]*' + RECORD), 'tee over a record'], [new RegExp('(cp|install)\\s+[^|;&]*' + RECORD + '\\s*($|[;&|])'), 'cp over a record'],
];
const OUTWARD = [
  [at('git\\s+(push|fetch|pull|clone|ls-remote|submodule\\s+(update|add))\\b'), 'git ↔ remote'], [at('git\\s+remote\\s+(add|set-url|remove|rm|prune|update)'), 'git remote change'],
  [at('(curl|wget|ssh|scp|sftp|rsync|nc|ncat|netcat|telnet|ping|dig|nslookup|ftp|socat)\\s'), 'network'], [at('(npm|pnpm|yarn)\\s+(publish|install|i|add|ci|update|upgrade|exec|x|link)\\b'), 'package network'],
  [at('npx\\s'), 'npx fetches'], [at('pip3?\\s+(install|download)'), 'pip network'], [at('(docker|podman)\\s+(push|pull|login|build|run)\\b'), 'container registry'],
  [at('(gh|glab|hub|aws|gcloud|az|kubectl|helm|terraform|heroku|flyctl|vercel|netlify|firebase)\\s'), 'external service CLI'], [at('(mail|sendmail|mutt|msmtp)\\s'), 'mail'],
  [/https?:\/\//, 'URL'], [at('(xdg-open|open)\\s'), 'opens outside'],
];
const WRITE = [
  [/(^|[^>])>>?/, 'redirect'], [at('(tee|cp|mkdir|touch|ln|chmod|chown|chgrp|install|patch|unzip|tar)\\s'), 'writes files'], [at('sed\\s+(-\\S*i|--in-place)'), 'sed -i'],
  [at('git\\s+(add|commit|checkout|switch|merge|stash|tag|init|apply|cherry-pick|notes|worktree)\\b'), 'git local write'], [at('memory\\.js\\s+[^|;&]*\\b(remember|index|forget)\\b'), 'memory write'],
];
const READ_ONLY = /^(cat|ls|head|tail|wc|grep|egrep|fgrep|rg|ag|find|stat|file|which|type|echo|printf|test|\[|\[\[|true|false|pwd|date|env|printenv|sleep|sort|uniq|cut|tr|diff|cmp|md5sum|sha\d*sum|jq|yq|awk|sed|less|more|basename|dirname|realpath|readlink|du|df|tree|column|nl|tac|rev|od|xxd|hexdump|strings|seq|expr|bc|comm|paste|join|fold|fmt|xargs|git\s+(status|log|diff|show|rev-parse|ls-files|blame|describe|cat-file|branch(\s+(-a|-r|-v|-vv|--list|-l))*\s*$|remote(\s+-v)?\s*$|shortlog|grep|count-objects|rev-list)|node\s+\S*memory\.js\s+[^|;&]*\b(recall|status)\b)(\s|$)/;
function segments(cmd) { return String(cmd).split(/\|\||&&|;|\|(?!\|)|\n/).map(s => s.replace(/^\s*(\w+=\S*\s+)*(sudo\s+|env\s+|time\s+|nice\s+)*/, '').trim()).filter(Boolean); }
function outsideWorld(cmd, cwd) {
  const inward = p => p === root || p.startsWith(root + path.sep) || SYSTEM.some(s => p.startsWith(s)) || p.startsWith(path.join(os.homedir(), '.isekai'));
  for (const tok of String(cmd).split(/[\s"'`=:,]+/)) {
    let p = null;
    if (/^~(\/|$)/.test(tok)) p = path.join(os.homedir(), tok.slice(1));
    else if (/^\/[^/]/.test(tok) || tok === '/') p = path.normalize(tok);
    else if (/(^|\/)\.\.(\/|$)/.test(tok)) p = path.resolve(cwd, tok);
    if (p && !inward(p.replace(/[)\]}>;,.]+$/, ''))) return tok;
  }
  return null;
}
function classify(cmd, cwd) {   // {cls, why} — the strongest reason found
  let best = { cls: 'read', why: 'read-only' };
  const bump = (cls, why) => { if (RANK(cls) > RANK(best.cls)) best = { cls, why }; };
  const escaped = outsideWorld(cmd, cwd || root); if (escaped) bump('outward', `path outside the world: ${escaped}`);
  for (const seg of segments(cmd)) {
    for (const [re, why] of DESTRUCTIVE) if (re.test(seg)) bump('destructive', why);
    for (const [re, why] of OUTWARD) if (re.test(seg)) bump('outward', why);
    for (const [re, why] of WRITE) if (re.test(seg)) bump('write', why);
    if (!READ_ONLY.test(seg) && !/^cd\s/.test(seg)) bump('write', `unknown command: ${seg.split(/\s/)[0]}`);
    if (/^cd\s/.test(seg) && outsideWorld(seg.slice(3), cwd || root)) bump('outward', 'cd outside the world');
  }
  return best;
}
function settleClass(step) {   // heuristic over run + verify; the plan's declaration may only tighten
  const cwd = step.cwd ? path.resolve(root, step.cwd) : root;
  const h = step.run ? classify(step.run, cwd) : { cls: 'read', why: 'no command' };
  if (step.verify) { const v = classify(step.verify, cwd); if (RANK(v.cls) > RANK(h.cls)) { h.cls = v.cls; h.why = `verify: ${v.why}`; } }
  if (step.cwd && outsideWorld(cwd, root) && RANK(h.cls) < RANK('outward')) { h.cls = 'outward'; h.why = `cwd outside the world: ${step.cwd}`; }
  const declared = step.class || null;
  const effective = declared && RANK(declared) > RANK(h.cls) ? declared : h.cls;
  const holes = declared && RANK(declared) < RANK(h.cls) ? [`declared ${declared} but the command reads as ${h.cls} (${h.why}) — a declaration only tightens; ${h.cls} kept`] : [];
  return { heuristic: h.cls, why: h.why, declared, effective, holes };
}
const needsGate = (cls, strict) => RANK(cls) >= RANK('outward') || (strict && cls === 'write');

// ------------------------------------------------------------------ memory: recall before, remember after, status for the budget
function mem(...args) {
  if (!fs.existsSync(MEMORY)) return { ok: false, j: null, why: `memory.js missing beside loop.js (${MEMORY})` };
  const r = spawnSync(process.execPath, [MEMORY, root, ...args], { encoding: 'utf8', env: process.env });
  let j = null; try { j = JSON.parse((r.stdout || '').trim().split('\n').pop()); } catch { /* not json */ }
  return { ok: r.status === 0 && !!j, j, why: r.status === 0 ? null : `memory.js ${args[0]} exit ${r.status}: ${tail(r.stderr || r.stdout).slice(-160)}` };
}
function recallFor(step, K) {
  const q = step.recall === true ? step.ask : step.recall;
  if (q === false) return { q: null, hits: [], holes: [], skipped: true };
  const ask = () => mem('recall', q, '--as', AS, '-k', String(K), '--json');
  let r = ask(), rebuilt = false;
  const stale = x => x.ok && x.j['@?'].some(h => /no long-term index|older than|older memory\.js|unreadable/.test(h));
  if (stale(r)) { const ix = mem('index', '--json'); rebuilt = ix.ok; if (ix.ok) r = ask(); }
  if (!r.ok) return { q, hits: [], holes: [r.why || 'recall answered nothing'], rebuilt, cache: null };
  return { q, cache: r.j['@S'], rebuilt, hits: r.j.results.map(x => ({ src: x.src, sec: x.sec || null, score: x.score, kind: x.kind, title: x.title })), holes: r.j['@?'].filter(h => !/no shared notes yet/.test(h)) };
}
// ------------------------------------------------------------------ toolbox: level 1 only — names, paths, costs; the body loads on its own decision
function tbx(...args) {
  if (!fs.existsSync(TOOLBOX)) return { ok: false, j: null, why: `toolbox.js missing beside loop.js (${TOOLBOX})` };
  const r = spawnSync(process.execPath, [TOOLBOX, root, ...args], { encoding: 'utf8', env: process.env });
  let j = null; try { j = JSON.parse((r.stdout || '').trim().split('\n').pop()); } catch { /* not json */ }
  return { ok: r.status === 0 && !!j, j, why: r.status === 0 ? null : `toolbox.js ${args[0]} exit ${r.status}: ${tail(r.stderr || r.stdout).slice(-160)}` };
}
function toolsFor(step, budget) {
  if (step.toolbox === false) return { q: null, picks: [], lines: [], holes: [], skipped: true };
  const q = step.ask;
  const ask = () => tbx('brief', q, '--as', AS, '--budget', String(budget), '--json');
  let r = ask(), rebuilt = false;
  const stale = x => x.ok && x.j['@?'].some(h => /no registry on disk|older than its sources|older toolbox\.js|unreadable/.test(h));
  if (stale(r)) { const ix = tbx('index', '--json'); rebuilt = ix.ok; if (ix.ok) r = ask(); }
  if (!r.ok) return { q, picks: [], lines: [], holes: [r.why || 'toolbox answered nothing'], rebuilt };
  return { q, rebuilt, cost: r.j.cost, picks: r.j.picks.map(p => ({ kind: p.kind, name: p.name, path: p.path, resident: p.resident, full: p.full, why: p.why })), lines: r.j.lines, holes: r.j['@?'] };
}
function rememberFor(step) {
  const r = step.remember, a = ['remember', r.text, '--as', AS, '--json'];
  if (r.kind) a.push('--kind', r.kind); if (r.tag) a.push('--tag', r.tag);
  const x = mem(...a);
  return x.ok ? { src: x.j.src, bytes: x.j.bytes, kind: r.kind || null, holes: x.j['@?'] } : { src: null, holes: [x.why] };
}
function contextReading() {
  const x = mem('status', '--as', AS, '--json');
  if (!x.ok) return { available: false, why: x.why };
  const c = x.j.short && x.j.short.contextWindow;
  return c && c.available ? { available: true, tokens: c.tokens, limit: c.limit, stress: c.stress, zone: c.zone } : { available: false, why: (c && c.why) || 'no reading' };
}

// ------------------------------------------------------------------ the gate: a real question, never a default yes
function askTTY(prompt) {
  return new Promise(res => { const rl = readline.createInterface({ input: process.stdin, output: process.stderr }); rl.question(prompt, a => { rl.close(); res(/^y(es)?$/i.test(a.trim())); }); });
}
async function gate(step, cls, opts, interrupted) {
  if (!needsGate(cls.effective, opts.strict)) return { needed: false, decision: 'not-needed' };
  const why = cls.declared === cls.effective && cls.declared !== cls.heuristic ? `declared ${cls.declared}` : cls.why;
  if (opts.approve.has(step.id) || (!interrupted && opts.approveJournaled.has(step.id))) return { needed: true, decision: 'approved', by: '--approve', why };
  if (opts.dryRun) return { needed: true, decision: 'would-ask', by: 'dry-run', why };
  if (process.stdin.isTTY && process.stderr.isTTY) {
    const yes = await askTTY(`@? gate ${step.id} — ${cls.effective} (${why})\n   run: ${step.run}${step.verify ? `\n   verify: ${step.verify}` : ''}\n   approve? [y/N] `);
    return { needed: true, decision: yes ? 'approved' : 'denied', by: 'tty', why };
  }
  return { needed: true, decision: 'denied', by: 'no-tty', why: `${why}; no TTY to ask — pass --approve ${step.id} to pre-approve` };
}

// ------------------------------------------------------------------ journal: the run's own instrument; the truth a resume reads
function journalPath(id) { return path.join(JOURNALS, `${id}.jsonl`); }
function journalState(events) {   // per step: what already happened, read back honestly
  const st = {}; let run = null, end = null, cls = {};
  const S = id => st[id] || (st[id] = { done: false, failed: false, denied: false, approved: false, actStarted: false, acts: 0, attempts: 0 });
  for (const e of events) {
    if (e.t === 'run') run = e;
    else if (e.t === 'end') end = e;
    else if (e.t === 'gate') { S(e.id).approved = e.decision === 'approved'; S(e.id).denied = e.decision === 'denied'; cls[e.id] = e.effective; }
    else if (e.t === 'act' && e.start) { S(e.id).actStarted = true; S(e.id).acts++; S(e.id).attempts = e.attempt; }
    else if (e.t === 'act' && !e.start) S(e.id).actStarted = false;
    else if (e.t === 'record') { const s = S(e.id); s.done = e.status === 'done'; s.failed = e.status === 'failed'; s.denied = e.status === 'denied'; s.actStarted = false; s.attempts = e.attempts || s.attempts; }
  }
  return { run, end, steps: st, cls };
}

// ------------------------------------------------------------------ the loop
async function drive(plan, runId, opts, prior) {
  const J = journalPath(runId);
  const log = e => appendLine(J, Object.assign({ t: e.t, at: now() }, e));
  const started = Date.now();
  const maxSteps = opts.maxSteps, maxMs = opts.maxMinutes * 60000;
  const report = { runId, as: AS, journal: opts.dryRun ? null : rel(J), status: null, steps: [], holes: [] };
  const hole = h => { if (!report.holes.includes(h)) report.holes.push(h); };
  let executed = 0, contextSeen = false;
  const prev = prior ? prior.steps : {};
  const bail = (status, why) => { report.status = status; if (why) hole(why); if (!opts.dryRun) log({ t: 'end', status, executed, ms: Date.now() - started }); return report; };
  const resumeHint = `resume: node ${path.relative(process.cwd(), __filename)} resume ${runId}`;
  for (const step of plan.steps) report.steps.push({ id: step.id, status: 'pending', class: null, attempts: 0, ms: 0, recall: null, tools: null, remembered: false });
  for (const [i, step] of plan.steps.entries()) {
    const row = report.steps[i];
    const was = own(prev, step.id);
    if (was && was.done) { row.status = 'done (before)'; row.attempts = was.attempts; continue; }
    // PERCEIVE — budgets are readings; past any of them the run checkpoints, it does not press on
    if (executed >= maxSteps) return bail('CHECKPOINT', `step budget reached (${maxSteps}) before ${step.id} — ${resumeHint}`);
    if (Date.now() - started >= maxMs) return bail('CHECKPOINT', `wall-clock budget reached (${opts.maxMinutes} min) before ${step.id} — ${resumeHint}`);
    if (!opts.dryRun) {
      const c = contextReading();
      if (!c.available && !contextSeen) hole(`context window unmeasured — ${c.why}`);
      contextSeen = true;
      if (c.available && c.tokens >= c.stress) { log({ t: 'budget', id: step.id, context: c }); return bail('CHECKPOINT', `context in the stress zone (${c.tokens} ≥ ${c.stress} tok) before ${step.id} — write what is not yet durable, then resume in a fresh session: node ${rel(__filename)} resume ${runId}`); }
    }
    // PLAN — the class, then the gate
    const cls = settleClass(step); row.class = cls.effective; cls.holes.forEach(hole);
    const interrupted = !!(was && was.actStarted && RANK(cls.effective) >= RANK('outward'));
    if (interrupted) hole(`${step.id}: act was interrupted mid-flight on the previous run — its side effect may have reached outside; it is gated again and needs a fresh approval`);
    const g = await gate(step, cls, opts, interrupted);
    if (!opts.dryRun) log({ t: 'gate', id: step.id, heuristic: cls.heuristic, declared: cls.declared, effective: cls.effective, why: cls.why, needed: g.needed, decision: g.decision, by: g.by || null });
    if (g.decision === 'denied') { row.status = 'denied'; if (!opts.dryRun) log({ t: 'record', id: step.id, status: 'denied', attempts: 0 }); return bail('DENIED', `${step.id} denied at the gate (${cls.effective}: ${g.why}) — the human decides; ${resumeHint} [--approve ${step.id}]`); }
    if (opts.dryRun) { row.status = g.decision === 'would-ask' ? 'would-ask' : g.decision === 'approved' ? 'would-run (pre-approved)' : 'would-run'; row.gate = g.why; executed++; continue; }
    // RECALL — point, don't carry
    const rc = recallFor(step, opts.recallK);
    row.recall = rc.skipped ? null : rc.hits.length;
    log({ t: 'recall', id: step.id, q: rc.q, cache: rc.cache || null, rebuilt: !!rc.rebuilt, hits: rc.hits, holes: rc.holes });
    for (const h of rc.holes) hole(`${step.id} recall: ${h}`);
    // TOOLBOX (still the recall beat) — the manifest that fits the ask and the tool budget: pointers with costs, never a body
    const tb = toolsFor(step, opts.toolBudget);
    row.tools = tb.skipped ? null : tb.picks.length;
    log({ t: 'toolbox', id: step.id, q: tb.q, skipped: !!tb.skipped, rebuilt: !!tb.rebuilt, budget: tb.skipped ? null : opts.toolBudget, cost: tb.cost === undefined ? null : tb.cost, picks: tb.picks, holes: tb.holes });
    for (const h of tb.holes) hole(`${step.id} toolbox: ${h}`);
    // ACT → VERIFY, retried with bounded backoff; a failed verify is the act's failure
    const retries = step.retries === undefined ? opts.retries : step.retries;
    const env = Object.assign({}, process.env, { LOOP_ROOT: root, LOOP_RUN_ID: runId, LOOP_STEP: step.id, LOOP_AS: AS, LOOP_PID: String(process.pid), LOOP_JOURNAL: J,
      LOOP_RECALL: JSON.stringify(rc.hits.map(h => h.src + (h.sec ? '#' + h.sec : ''))), LOOP_TOOLS: tb.lines.join('\n') });
    const cwd = step.cwd ? path.resolve(root, step.cwd) : root;
    let passed = !step.run, last = null;
    const t0 = Date.now();
    for (let attempt = 1; step.run && attempt <= retries + 1; attempt++) {
      row.attempts = attempt; env.LOOP_ATTEMPT = String(attempt);
      log({ t: 'act', id: step.id, attempt, start: true, run: step.run });
      const a = sh(step.run, cwd, env, opts.stepTimeoutS);
      log({ t: 'act', id: step.id, attempt, code: a.code, signal: a.signal, ms: a.ms, out: tail(a.out), err: tail(a.err) });
      last = { beat: 'act', code: a.code, signal: a.signal, err: a.err || a.out };
      if (a.code === 0 && step.verify) {
        const v = sh(step.verify, cwd, env, opts.stepTimeoutS);
        log({ t: 'verify', id: step.id, attempt, code: v.code, signal: v.signal, ms: v.ms, out: tail(v.out), err: tail(v.err) });
        if (v.code !== 0) last = { beat: 'verify', code: v.code, signal: v.signal, err: v.err || v.out };
        else { passed = true; break; }
      } else if (a.code === 0) { passed = true; break; }
      if (attempt <= retries) { const wait = Math.min(opts.backoffMs * Math.pow(2, attempt - 1), 10000); log({ t: 'retry', id: step.id, attempt, waitMs: wait, because: last.beat }); await sleep(wait); }
    }
    row.ms = Date.now() - t0;
    if (!passed) {
      row.status = 'failed'; log({ t: 'record', id: step.id, status: 'failed', attempts: row.attempts, last });
      const because = `${last.beat} exit ${last.signal || last.code}${last.err ? ' — ' + String(last.err).trim().split('\n').pop().slice(0, 120) : ''}`;
      return bail('ESCALATE', `${step.id} failed after ${row.attempts} attempt${row.attempts === 1 ? '' : 's'} (${because}) — one hop up: the dispatcher decides; journal ${rel(J)}; ${resumeHint}`);
    }
    // RECORD — and memory flows up
    if (step.remember) { const m = rememberFor(step); row.remembered = !!m.src; log({ t: 'remember', id: step.id, src: m.src, bytes: m.bytes || null, kind: m.kind || null, holes: m.holes || [] }); (m.holes || []).forEach(h => hole(`${step.id} remember: ${h}`)); }
    row.status = 'done'; log({ t: 'record', id: step.id, status: 'done', attempts: row.attempts, ms: row.ms });
    executed++;
  }
  return bail(opts.dryRun ? 'DRY' : 'DONE');
}
function sh(command, cwd, env, timeoutS) {
  const t0 = Date.now();
  const r = spawnSync(command, { shell: true, cwd, env, encoding: 'utf8', timeout: timeoutS * 1000, killSignal: 'SIGKILL' });
  return { code: r.status === null ? (r.signal ? 128 : 1) : r.status, signal: r.signal || (r.error ? r.error.code : null), out: r.stdout || '', err: r.stderr || '', ms: Date.now() - t0 };
}
function options(plan, journaled) {
  const j = journaled || {};
  const approve = new Set(String(flags.approve || '').split(',').map(s => s.trim()).filter(Boolean));   // this invocation's own word
  const approveJournaled = new Set(j.approve || []);   // the first run's pre-approvals: honoured on resume, except for an act cut mid-flight
  return {
    approve, approveJournaled, strict: !!flags.strict || !!j.strict, dryRun: !!flags['dry-run'],
    maxSteps: num(flags['max-steps'], plan.budget.steps || 50, '--max-steps', 1), maxMinutes: num(flags['max-minutes'], plan.budget.minutes || 30, '--max-minutes', 0.0001),
    retries: Math.floor(num(flags.retries, j.retries === undefined ? 2 : j.retries, '--retries', 0)), backoffMs: num(flags['backoff-ms'], j.backoffMs === undefined ? 500 : j.backoffMs, '--backoff-ms', 0),
    recallK: Math.floor(num(flags['recall-k'], j.recallK === undefined ? 3 : j.recallK, '--recall-k', 1)), stepTimeoutS: num(flags['step-timeout-s'], j.stepTimeoutS === undefined ? 600 : j.stepTimeoutS, '--step-timeout-s', 1),
    toolBudget: Math.floor(num(flags['tool-budget'], j.toolBudget === undefined ? 1500 : j.toolBudget, '--tool-budget', 1)),
  };
}
function print(report, plan) {   // the wire: @S · @F per step · @? per hole · @E
  if (flags.json) return console.log(JSON.stringify(Object.assign({ '@S': report.status }, report, { '@?': report.holes })));
  const done = report.steps.filter(s => /^done/.test(s.status)).length;
  console.log(`@S ${report.status} run=${report.runId} as=${AS} steps=${done}/${plan.steps.length} ${report.journal ? 'journal=' + report.journal : 'dry-run, nothing ran, no journal'}`);
  for (const s of report.steps) console.log(`@F ${s.id} — ${s.status}${s.class ? ' — ' + s.class : ''}${s.gate ? ' (' + s.gate + ')' : ''}${s.attempts ? ` — ${s.attempts} attempt${s.attempts === 1 ? '' : 's'}` : ''}${s.ms ? ` — ${s.ms}ms` : ''}${s.recall !== null && s.recall !== undefined ? ` — recall ${s.recall} hit${s.recall === 1 ? '' : 's'}` : ''}${s.tools !== null && s.tools !== undefined ? ` — tools ${s.tools}` : ''}${s.remembered ? ' — remembered' : ''}`);
  for (const h of report.holes) console.log(`@? ${h}`);
  console.log(`@E ${Buffer.byteLength(JSON.stringify(report))}`);
}

// ------------------------------------------------------------------ commands
async function run() {
  const src = pos.shift(); if (!src) fail('run needs a plan path');
  const plan = loadPlan(src);
  if (plan.as && !flags.as) AS = String(plan.as).toLowerCase();
  const opts = options(plan);
  const runId = `${path.basename(plan.src).replace(/\.[^.]+$/, '').replace(/[^A-Za-z0-9_-]/g, '-')}-${now().replace(/[-:T]/g, '').slice(0, 14)}-${crypto.randomBytes(2).toString('hex')}`;
  if (!opts.dryRun) appendLine(journalPath(runId), { t: 'run', at: now(), id: runId, as: AS, node: process.version, plan, approve: [...opts.approve], strict: opts.strict, retries: opts.retries, backoffMs: opts.backoffMs, recallK: opts.recallK, toolBudget: opts.toolBudget, stepTimeoutS: opts.stepTimeoutS });
  const report = await drive(plan, runId, opts, null);
  print(report, plan);
  process.exit(EXIT[report.status] === undefined ? 0 : EXIT[report.status]);
}
async function resume() {
  const runId = pos.shift(); if (!runId) fail('resume needs a run-id (see `loop.js status`)');
  const J = journalPath(runId); if (!fs.existsSync(J)) fail(`no journal ${rel(J)}`);
  const st = journalState(readJsonl(J));
  if (!st.run || !st.run.plan) fail(`journal ${rel(J)} has no run line — nothing to resume from`);
  const plan = st.run.plan;
  if (!flags.as && st.run.as) AS = String(st.run.as).toLowerCase();
  const opts = options(plan, st.run);
  if (opts.dryRun) fail('resume does not take --dry-run — explain the plan instead');
  const holes = [];
  const cur = rd(path.join(root, plan.src)); if (cur !== null && md5(cur) !== plan.hash) holes.push(`plan file ${plan.src} changed since the run began — the journaled plan is what resumes`);
  if (st.end && st.end.status === 'DONE') holes.push('run had already ended DONE — nothing left to resume');
  appendLine(J, { t: 'resume', at: now(), node: process.version, approve: [...opts.approve], strict: opts.strict, prior: st.end ? st.end.status : 'INTERRUPTED' });
  const report = await drive(plan, runId, opts, st);
  report.holes.unshift(...holes);
  print(report, plan);
  process.exit(EXIT[report.status] === undefined ? 0 : EXIT[report.status]);
}
function explain() {
  const src = pos.shift(); if (!src) fail('explain needs a plan path');
  const plan = loadPlan(src);
  if (plan.as && !flags.as) AS = String(plan.as).toLowerCase();
  const as = AS;
  const opts = options(plan);
  const gated = plan.steps.filter(s => needsGate(settleClass(s).effective, opts.strict)).length;
  console.log(`plan ${plan.src} · as ${as} · ${plan.steps.length} step${plan.steps.length === 1 ? '' : 's'} · budget ${opts.maxSteps} steps / ${opts.maxMinutes} min / context ${'200k'} (stress 180k) · tools ${opts.toolBudget}tok · retries ${opts.retries} · backoff ${opts.backoffMs}ms · ${gated} gated${opts.strict ? ' (--strict: writes too)' : ''}`);
  plan.steps.forEach((s, i) => {
    const c = settleClass(s);
    const gateWord = !needsGate(c.effective, opts.strict) ? 'no' : opts.approve.has(s.id) ? 'pre-approved (--approve)' : `ASK — ${c.effective}: ${c.declared === c.effective && c.declared !== c.heuristic ? 'declared' : c.why}`;
    console.log(`${String(i + 1).padStart(3)}. ${s.id}  [${c.effective}${c.declared ? `, declared ${c.declared}` : ''}${c.declared && c.declared !== c.heuristic ? `, reads as ${c.heuristic}` : ''}]  gate: ${gateWord}`);
    console.log(`       ask:      ${s.ask}`);
    if (s.run) console.log(`       run:      ${s.run}${s.cwd ? `   (cwd ${s.cwd})` : ''}`);
    if (s.verify) console.log(`       verify:   ${s.verify}`);
    console.log(`       recall:   ${s.recall === false ? 'off' : s.recall === true ? `the ask, k=${opts.recallK}` : `"${s.recall}", k=${opts.recallK}`}`);
    console.log(`       toolbox:  ${s.toolbox === false ? 'off' : `the ask, budget=${opts.toolBudget}tok`}`);
    if (s.remember) console.log(`       remember: ${s.remember.text}${s.remember.kind ? ` [${s.remember.kind}]` : ''}${s.remember.tag ? ` #${s.remember.tag}` : ''}`);
    console.log(`       retries:  ${s.retries === undefined ? opts.retries : s.retries}`);
    for (const h of c.holes) console.log(`       @? ${h}`);
  });
  console.log(`\nbeats per step: perceive (budgets) → recall (memory, toolbox) → plan (class, gate) → act → verify → record (journal .isekai/instruments/loop/<run-id>.jsonl, remember)`);
}
function status() {
  const id = pos.shift();
  const files = fs.existsSync(JOURNALS) ? fs.readdirSync(JOURNALS).filter(f => f.endsWith('.jsonl')).map(f => ({ f, t: fs.statSync(path.join(JOURNALS, f)).mtimeMs })).sort((a, b) => b.t - a.t) : [];
  if (!id) {
    const rows = files.map(({ f }) => { const st = journalState(readJsonl(path.join(JOURNALS, f))); const n = st.run && st.run.plan ? st.run.plan.steps.length : 0;
      return { id: f.slice(0, -6), status: st.end ? st.end.status : 'INTERRUPTED', as: st.run ? st.run.as : null, plan: st.run && st.run.plan ? st.run.plan.src : null, done: Object.values(st.steps).filter(s => s.done).length, steps: n, at: st.run ? st.run.at : null }; });
    if (flags.json) return console.log(JSON.stringify({ '@S': rows.length ? 'RUNS' : 'EMPTY', dir: rel(JOURNALS), runs: rows }));
    console.log(`@S ${rows.length ? 'RUNS' : 'EMPTY'} ${rows.length} in ${rel(JOURNALS)}`);
    for (const r of rows) console.log(`@F ${r.id} — ${r.status} — ${r.done}/${r.steps} done — as ${r.as} — ${r.plan} — ${r.at}`);
    return;
  }
  const J = journalPath(id); if (!fs.existsSync(J)) fail(`no journal ${rel(J)}`);
  const ev = readJsonl(J), st = journalState(ev);
  const rows = (st.run && st.run.plan ? st.run.plan.steps : []).map(s => { const x = st.steps[s.id] || {}; const rc = ev.filter(e => e.t === 'recall' && e.id === s.id).pop(), tb = ev.filter(e => e.t === 'toolbox' && e.id === s.id).pop();
    return { id: s.id, status: x.done ? 'done' : x.failed ? 'failed' : x.denied ? 'denied' : x.actStarted ? 'interrupted mid-act' : 'pending', class: st.cls[s.id] || null, attempts: x.attempts || 0, recall: rc ? rc.hits.length : null, tools: tb && !tb.skipped ? tb.picks.length : null }; });
  const out = { '@S': st.end ? st.end.status : 'INTERRUPTED', id, as: st.run ? st.run.as : null, plan: st.run && st.run.plan ? st.run.plan.src : null, events: ev.length, steps: rows, journal: rel(J) };
  if (flags.json) return console.log(JSON.stringify(out));
  console.log(`@S ${out['@S']} run=${id} as=${out.as} plan=${out.plan} events=${ev.length} journal=${out.journal}`);
  for (const r of rows) console.log(`@F ${r.id} — ${r.status}${r.class ? ' — ' + r.class : ''}${r.attempts ? ` — ${r.attempts} attempt${r.attempts === 1 ? '' : 's'}` : ''}${r.recall !== null ? ` — recall ${r.recall} hits` : ''}${r.tools !== null ? ` — tools ${r.tools}` : ''}`);
}

// ------------------------------------------------------------------ selftest: a throwaway world under .isekai/tmp/ (Law 5)
// Drives the real CLI as child processes (never a TTY, so every gate answer is --approve or a
// denial), with HOME pointed inside the throwaway so memory.js's machine tier and transcript
// lookup never touch the real ~/.isekai or ~/.claude. Cleans up in finally.
async function selftest() {
  const T = path.join(HOME, 'tmp', 'loop-selftest'), W = path.join(T, 'world'), H = path.join(T, 'home');
  const realLoopDir = () => fs.existsSync(JOURNALS) ? fs.readdirSync(JOURNALS).filter(f => f !== '.gitkeep').length : 0;
  const before = realLoopDir();
  const fails = []; let checks = 0;
  const ok = (cond, what) => { checks++; if (!cond) fails.push(what); };
  const w = (p, s) => { mkd(path.dirname(p)); fs.writeFileSync(p, s); };
  const env = Object.assign({}, process.env, { HOME: H, USERPROFILE: H });
  const parse = s => { try { return JSON.parse(s.trim().split('\n').pop()); } catch { return null; } };
  const run = (...a) => { const r = spawnSync(process.execPath, [__filename, W, ...a], { env, encoding: 'utf8' }); return { code: r.status, signal: r.signal, out: (r.stdout || '') + (r.stderr || ''), j: parse(r.stdout || '') }; };
  const journals = () => fs.existsSync(path.join(W, '.isekai', 'instruments', 'loop')) ? fs.readdirSync(path.join(W, '.isekai', 'instruments', 'loop')).filter(f => f.endsWith('.jsonl')) : [];
  const journal = id => readJsonl(path.join(W, '.isekai', 'instruments', 'loop', `${id}.jsonl`));
  const plan = (name, obj) => { const p = path.join(T, `${name}.json`); w(p, JSON.stringify(obj)); return p; };
  try {
    fs.rmSync(T, { recursive: true, force: true });
    w(path.join(W, '.isekai', 'name'), 'loop-selftest\n');
    w(path.join(W, '.isekai', 'isekai.md'), '# Law\n\n## The gate\nNo change lands without its Orc\'s pass.\n\n## Containment\nNothing leaves outward without the human agreeing; git push is outward.\n');
    w(path.join(W, '.isekai', 'log.md'), '# Chronicle\n\n### [2026-09-20 17:54] rimuru — World reincarnated\n- **Task:** /isekai\n\n### [2026-09-22 01:00] rimuru — Notes file written by a court body\n- **Learned:** the notes file lives under notes/ and the loop journals every beat\n');
    w(path.join(W, 'README.md'), '# selftest world\n\n## About\nA throwaway world for the loop selftest; the notes file is notes/out.txt.\n');
    w(path.join(W, '.claude', 'skills', 'notes-mind', 'SKILL.md'), '---\nname: notes-mind\ndescription: Where the notes file lives and how the loop writes it.\n---\n# Notes mind\n\nBODYSENTINEL-NOTES\n');
    // --- classifier: the heuristic errs toward asking; a declaration only tightens
    const C = (c, cls) => ok(classify(c, W).cls === cls, `classify ${JSON.stringify(c)} → ${cls}, got ${classify(c, W).cls} (${classify(c, W).why})`);
    C('cat README.md', 'read'); C('git status && git log -1', 'read'); C('echo hi > out.txt', 'write'); C('node build.js', 'write'); C('mkdir -p a/b', 'write');
    C('git push origin main', 'outward'); C('curl -s https://x.y', 'outward'); C('bash -c "wget x"', 'outward'); C('ssh host ls', 'outward'); C('cat /etc/hosts', 'read');
    C('cat /home/nobody/secret', 'outward'); C('cp a ../../../../../../../../outside', 'outward'); C('rm -rf build', 'destructive'); C('git reset --hard', 'destructive'); C('git branch -D x', 'destructive');
    C('git push --force', 'destructive'); C('find . -name x -delete', 'destructive'); C('echo x > .isekai/log.md', 'destructive'); C('echo x >> .isekai/log.md', 'write'); C('sed -i s/a/b/ .isekai/isekai.md', 'destructive');
    C('ls | xargs rm', 'destructive'); C(`node ${rel(MEMORY)} recall "q" --json`, 'read'); C(`node ${rel(MEMORY)} remember "n"`, 'write');
    ok(settleClass({ run: 'git push', class: 'read' }).effective === 'outward', 'declared read cannot lower outward');
    ok(settleClass({ run: 'echo x > f', class: 'outward' }).effective === 'outward', 'declared outward raises a write');
    ok(settleClass({ run: 'cat f', class: 'destructive' }).effective === 'destructive', 'declared destructive raises a read');
    // --- run A: read, write+remember, verify-fails-then-passes; recall hits populated after an automatic index rebuild
    const A = plan('plan-a', { as: 'slime-notes', steps: [
      { id: 'read1', ask: 'where does the notes file live', run: 'cat README.md' },
      { id: 'write1', ask: 'write the notes file', run: 'mkdir -p notes && echo hello > notes/out.txt', verify: 'test -f notes/out.txt', remember: { text: 'notes/out.txt is written by the loop', kind: 'colony', tag: 'loop' } },
      { id: 'flaky', ask: 'append twice before the verify holds', run: 'echo x >> flaky.txt', verify: 'test $(wc -l < flaky.txt) -ge 2', retries: 2 },
      { id: 'tools', ask: 'where does the notes file live', run: 'printf \'%s\' "$LOOP_TOOLS" > tools.txt', verify: 'grep -q "^@T mind notes-mind" tools.txt' },
      { id: 'quiet', ask: 'a step with recall off', run: 'test -n "$LOOP_RUN_ID" && test "$LOOP_STEP" = quiet && test -z "$LOOP_TOOLS"', recall: false, toolbox: false },
    ] });
    let r = run('run', A, '--backoff-ms', '5', '--json');
    ok(r.code === 0 && r.j && r.j['@S'] === 'DONE' && r.j.as === 'slime-notes', `run A: ${r.out.slice(0, 300)}`);
    ok(r.j && r.j.steps.map(s => s.status).join(',') === 'done,done,done,done,done', `run A statuses: ${r.j && r.j.steps.map(s => s.status).join(',')}`);
    ok(r.j && r.j.steps[0].class === 'read' && r.j.steps[1].class === 'write' && r.j.steps[1].remembered === true, 'run A: read/write classes, note remembered');
    ok(r.j && r.j.steps[2].attempts === 2 && fs.readFileSync(path.join(W, 'flaky.txt'), 'utf8') === 'x\nx\n', `run A: flaky verify failed then passed on retry (attempts ${r.j && r.j.steps[2].attempts})`);
    ok(r.j && r.j.steps[4].recall === null && r.j.steps[0].recall > 0, 'run A: recall off for `quiet`, hits for `read1`');
    ok(r.j && r.j.steps[0].tools > 0 && r.j.steps[3].tools > 0 && r.j.steps[4].tools === null, `run A: tools offered for read1/tools, none for quiet (toolbox: false) — ${r.j && JSON.stringify(r.j.steps.map(s => s.tools))}`);
    const evA = journal(r.j.runId);
    const rc1 = evA.find(e => e.t === 'recall' && e.id === 'read1');
    ok(rc1 && rc1.rebuilt === true && rc1.hits.length > 0 && rc1.hits.every(h => typeof h.src === 'string' && !('snippet' in h)) && rc1.hits.some(h => /README\.md|log\.md/.test(h.src)), `run A: index rebuilt on first recall, hits are paths+anchors — ${JSON.stringify(rc1 && rc1.hits.slice(0, 2))}`);
    ok(evA.filter(e => e.t === 'verify' && e.id === 'flaky').map(e => e.code).join(',') === '1,0', 'run A journal: verify 1 then 0 for flaky');
    const tb1 = evA.find(e => e.t === 'toolbox' && e.id === 'read1'), tbQ = evA.find(e => e.t === 'toolbox' && e.id === 'quiet');
    ok(tb1 && tb1.rebuilt === true && tb1.budget === 1500 && tb1.picks.some(p => p.kind === 'mind' && p.name === 'notes-mind' && typeof p.resident === 'number' && typeof p.full === 'number') && tb1.picks.every(p => !('text' in p) && !('line' in p)), `run A journal: registry rebuilt on first brief, picks are names + costs — ${JSON.stringify(tb1 && tb1.picks)}`);
    ok(tbQ && tbQ.skipped === true && tbQ.picks.length === 0 && tbQ.budget === null, 'run A journal: toolbox: false journals a skipped beat with no picks');
    ok(!/BODYSENTINEL/.test(rd(path.join(W, '.isekai', 'instruments', 'loop', `${r.j.runId}.jsonl`))), 'run A journal: no tool body crossed into the journal');
    const toolsTxt = rd(path.join(W, 'tools.txt')) || '';
    ok(/^@T mind notes-mind — \.claude\/skills\/notes-mind\/SKILL\.md — load≈\d+tok — Where the notes file lives/.test(toolsTxt) && !/BODYSENTINEL/.test(toolsTxt) && !/^@TOOLS/.test(toolsTxt), `$LOOP_TOOLS carries @T lines only — ${JSON.stringify(toolsTxt.slice(0, 160))}`);
    ok(fs.existsSync(path.join(W, '.isekai', 'toolbox', 'registry.json')) && readJsonl(path.join(W, '.isekai', 'instruments', 'toolbox', 'loads.jsonl')).some(x => x.ev === 'offer' && x.by === 'slime-notes' && x.names.includes('notes-mind')), 'run A: the toolbox journaled the offer as the body');
    r = run('run', A, '--backoff-ms', '5', '--tool-budget', '1', '--json');
    ok(r.code === 3 && r.j.steps[0].tools === 0 && r.j['@?'].some(h => /read1 toolbox: .*fit the turn but not the budget.*notes-mind/.test(h)), `--tool-budget 1: nothing fits, the cut is a @?, and the tools step's verify fails honestly — ${r.out.slice(0, 200)}`);
    ok(run('run', A, '--tool-budget', '0').code === 2 && run('run', plan('bad-tb', { steps: [{ id: 'x', ask: 'q', run: 'true', toolbox: 'yes' }] })).code === 2, '--tool-budget 0 and toolbox: "yes" are FAILs');
    ok(evA.some(e => e.t === 'retry' && e.id === 'flaky' && e.because === 'verify'), 'run A journal: the retry names the failing verify as the act\'s failure');
    ok(evA.some(e => e.t === 'remember' && e.id === 'write1' && /notes\.jsonl/.test(e.src)) && readJsonl(path.join(W, '.isekai', 'memory', 'shared', 'notes.jsonl')).some(n => n.kind === 'colony' && n.by === 'slime-notes'), 'run A: shared note landed with kind colony as the body');
    ok(evA[0].t === 'run' && evA[0].plan.steps.length === 5 && evA[0].toolBudget === 1500 && evA[evA.length - 1].t === 'end' && evA[evA.length - 1].status === 'DONE', 'run A journal: run line first (tool budget journaled), end DONE last');
    ok(r.j['@?'].some(h => /context window unmeasured/.test(h)), 'run A: no transcript is a @? finding, not a stop');
    ok(/^@S DONE run=plan-a-/.test(run('run', A, '--backoff-ms', '5').out) , 'run A text: wire report opens with @S DONE');
    // --- run B: retries exhausted → ESCALATE (exit 3), one hop up, never a loop forever
    const B = plan('plan-b', { steps: [{ id: 'doom', ask: 'a command that always fails', run: 'echo boom >&2; false', retries: 1 }, { id: 'after', ask: 'never reached', run: 'touch never.txt' }] });
    r = run('run', B, '--backoff-ms', '5', '--json');
    ok(r.code === 3 && r.j && r.j['@S'] === 'ESCALATE' && r.j.steps[0].status === 'failed' && r.j.steps[0].attempts === 2 && r.j.steps[1].status === 'pending', `run B: ${r.out.slice(0, 300)}`);
    ok(r.j && r.j['@?'].some(h => /doom failed after 2 attempts.*boom.*resume/.test(h)) && !fs.existsSync(path.join(W, 'never.txt')), 'run B: @? names the failure and the resume; the next step never ran');
    ok(journal(r.j.runId).filter(e => e.t === 'act' && !e.start).length === 2, 'run B journal: exactly two acts');
    // --- run C: the human gate — dry-run says what it would ask; no TTY denies; --approve pre-approves; declared can only tighten
    const C1 = plan('plan-c', { steps: [{ id: 'push', ask: 'publish the branch', run: 'git push origin main' }] });
    const nJ = journals().length;
    r = run('run', C1, '--dry-run', '--json');
    ok(r.code === 0 && r.j && r.j['@S'] === 'DRY' && r.j.steps[0].status === 'would-ask' && r.j.steps[0].class === 'outward' && journals().length === nJ, `run C dry-run: would ask, nothing journaled — ${r.out.slice(0, 200)}`);
    ok(/would-ask — outward \(git ↔ remote\)/.test(run('run', C1, '--dry-run').out), 'run C dry-run text names the class and the reason');
    r = run('run', C1, '--json');
    ok(r.code === 4 && r.j && r.j['@S'] === 'DENIED' && r.j.steps[0].status === 'denied' && journal(r.j.runId).some(e => e.t === 'gate' && e.decision === 'denied' && e.by === 'no-tty'), `run C no-tty: denied — ${r.out.slice(0, 200)}`);
    ok(r.j && r.j['@?'].some(h => /--approve push/.test(h)), 'run C denied: the hole says how to pre-approve');
    const C2 = plan('plan-c2', { steps: [{ id: 'push2', ask: 'a harmless command declared outward', run: 'echo pushed > pushed.txt', class: 'outward', verify: 'test -f pushed.txt' }] });
    r = run('run', C2, '--json');
    ok(r.code === 4 && !fs.existsSync(path.join(W, 'pushed.txt')), 'run C2: declared outward is gated even though the command only writes');
    r = run('run', C2, '--approve', 'push2', '--json');
    ok(r.code === 0 && r.j['@S'] === 'DONE' && fs.existsSync(path.join(W, 'pushed.txt')) && journal(r.j.runId).some(e => e.t === 'gate' && e.decision === 'approved' && e.by === '--approve'), `run C2 --approve: ran — ${r.out.slice(0, 200)}`);
    const C3 = plan('plan-c3', { steps: [{ id: 'lower', ask: 'declare read on a push', run: 'git push origin main', class: 'read' }] });
    r = run('run', C3, '--json');
    ok(r.code === 4 && r.j.steps[0].class === 'outward' && r.j['@?'].some(h => /declared read but the command reads as outward/.test(h)), 'run C3: a declaration cannot lower the class');
    r = run('run', plan('plan-c4', { steps: [{ id: 'w', ask: 'a plain write under strict', run: 'touch strict.txt' }] }), '--strict', '--json');
    ok(r.code === 4 && !fs.existsSync(path.join(W, 'strict.txt')), 'run C4: --strict gates a write too');
    // --- run D: budgets are readings — step budget checkpoints (exit 5) and resume finishes; wall-clock too
    const D = plan('plan-d', { steps: [1, 2, 3].map(i => ({ id: `s${i}`, ask: `step ${i}`, run: `echo ${i} >> d.txt` })) });
    r = run('run', D, '--max-steps', '2', '--json');
    ok(r.code === 5 && r.j['@S'] === 'CHECKPOINT' && r.j.steps.map(s => s.status).join(',') === 'done,done,pending' && r.j['@?'].some(h => /step budget reached \(2\) before s3/.test(h)), `run D: ${r.out.slice(0, 300)}`);
    const dId = r.j.runId;
    r = run('resume', dId, '--json');
    ok(r.code === 0 && r.j['@S'] === 'DONE' && r.j.steps.map(s => s.status).join(',') === 'done (before),done (before),done' && fs.readFileSync(path.join(W, 'd.txt'), 'utf8') === '1\n2\n3\n', `resume D: ${r.out.slice(0, 300)}`);
    ok(journal(dId).filter(e => e.t === 'act' && e.start && e.id === 's1').length === 1 && journal(dId).some(e => e.t === 'resume'), 'resume D journal: s1 ran once; a resume line was written');
    r = run('resume', dId, '--json');
    ok(r.code === 0 && r.j['@?'].some(h => /already ended DONE/.test(h)), 'resume D twice: nothing to do is a @?, not a crash');
    r = run('run', plan('plan-w', { steps: [{ id: 'slow', ask: 'sleep', run: 'sleep 0.3' }, { id: 'next', ask: 'after', run: 'true' }] }), '--max-minutes', '0.002', '--json');
    ok(r.code === 5 && /wall-clock budget/.test(r.j['@?'].join()) && r.j.steps[1].status === 'pending', `run W wall-clock: ${r.out.slice(0, 200)}`);
    // --- run E: an act cut mid-flight (the step kills the loop) resumes from the journal
    const E = plan('plan-e', { steps: [{ id: 'first', ask: 'first', run: 'echo 1 >> e.txt' }, { id: 'cut', ask: 'kill the loop the first time', run: 'test -f cut.txt || { touch cut.txt; kill -9 $LOOP_PID; }' }, { id: 'last', ask: 'last', run: 'echo 3 >> e.txt' }] });
    r = run('run', E, '--json');
    const eId = journals().map(f => f.slice(0, -6)).find(id => /^plan-e-/.test(id));
    ok(r.signal === 'SIGKILL' && eId && !journal(eId).some(e => e.t === 'end'), `run E: loop was killed mid-act, journal open — signal ${r.signal}`);
    let s = run('status', '--json');
    ok(s.j && s.j.runs.some(x => x.id === eId && x.status === 'INTERRUPTED' && x.done === 1), 'status: the cut run shows INTERRUPTED with 1 done');
    s = run('status', eId, '--json');
    ok(s.j && s.j.steps[1].status === 'interrupted mid-act' && s.j['@S'] === 'INTERRUPTED', `status <id>: names the interrupted step — ${JSON.stringify(s.j && s.j.steps)}`);
    r = run('resume', eId, '--json');
    ok(r.code === 0 && r.j['@S'] === 'DONE' && r.j.steps.map(x => x.status).join(',') === 'done (before),done,done' && fs.readFileSync(path.join(W, 'e.txt'), 'utf8') === '1\n3\n', `resume E: ${r.out.slice(0, 300)}`);
    // an interrupted OUTWARD act is re-gated on resume, never re-run on the old approval
    const F = plan('plan-f', { steps: [{ id: 'out', ask: 'an outward act that cuts the loop', run: 'test -f f.txt || { touch f.txt; kill -9 $LOOP_PID; }', class: 'outward' }] });
    run('run', F, '--approve', 'out', '--json');
    const fId = journals().map(f => f.slice(0, -6)).find(id => /^plan-f-/.test(id));
    r = run('resume', fId, '--json');
    ok(r.code === 4 && r.j['@S'] === 'DENIED' && r.j['@?'].some(h => /interrupted mid-flight.*fresh approval/.test(h)), `resume F without a fresh approval: denied, the old --approve does not carry — ${r.out.slice(0, 200)}`);
    r = run('resume', fId, '--approve', 'out', '--json');
    ok(r.code === 0 && r.j['@S'] === 'DONE' && journal(fId).filter(e => e.t === 'gate').length === 3, `resume F with a fresh --approve: runs — ${r.out.slice(0, 200)}`);
    r = run('resume', dId, '--json');   // a journaled approval still carries for a step that was never started
    ok(r.code === 0, 'resume: journaled approvals carry for steps that were not cut mid-flight');
    // --- context budget: a transcript in the stress zone checkpoints before the first step
    const slug = W.replace(/[^a-zA-Z0-9]/g, '-');
    w(path.join(H, '.claude', 'projects', slug, 's.jsonl'), JSON.stringify({ type: 'assistant', message: { usage: { input_tokens: 1000, cache_read_input_tokens: 185000 } } }) + '\n');
    r = run('run', D, '--json');
    ok(r.code === 5 && r.j['@S'] === 'CHECKPOINT' && r.j.steps[0].status === 'pending' && r.j['@?'].some(h => /stress zone \(186000 ≥ 180000/.test(h)) && journal(r.j.runId).some(e => e.t === 'budget' && e.context.zone === 'STRESS'), `context budget: ${r.out.slice(0, 300)}`);
    fs.rmSync(path.join(H, '.claude'), { recursive: true, force: true });
    // --- explain, plan validation, status text
    r = run('explain', A);
    ok(r.code === 0 && /plan .*plan-a\.json · as slime-notes · 5 steps/.test(r.out) && /1\. read1  \[read/.test(r.out) && /remember: notes\/out\.txt.*\[colony\] #loop/.test(r.out) && /recall:   off/.test(r.out), `explain: ${r.out.slice(0, 300)}`);
    ok(/· tools 1500tok ·/.test(r.out) && /toolbox:  the ask, budget=1500tok/.test(r.out) && /toolbox:  off/.test(r.out), 'explain: the tool budget and each step\'s toolbox setting');
    r = run('explain', C3);
    ok(/gate: ASK — outward/.test(r.out) && /@\? declared read/.test(r.out), 'explain: shows the gate and the tightened class');
    ok(run('run', plan('bad-1', { steps: [{ id: 'x', run: 'true' }] })).code === 2, 'plan without ask is a FAIL');
    ok(run('run', plan('bad-2', { steps: [{ id: 'x', ask: 'q', run: 'true' }, { id: 'x', ask: 'q', run: 'true' }] })).code === 2, 'duplicate ids are a FAIL');
    ok(run('run', plan('bad-3', { steps: [{ id: 'x', ask: 'q', run: 'true', class: 'network' }] })).code === 2, 'unknown class is a FAIL');
    ok(run('run', plan('bad-4', { steps: [{ id: 'x', ask: 'q', verify: 'true' }] })).code === 2, 'verify without run is a FAIL');
    const jl = path.join(T, 'plan.jsonl'); w(jl, '{"id":"a","ask":"one","run":"true"}\n{"id":"b","ask":"two","run":"true","recall":false}\n');
    ok(run('run', jl, '--json').j['@S'] === 'DONE', 'JSONL plan runs');
    ok(run('resume', 'no-such-run').code === 2 && run('run').code === 2 && run('nonsense').code === 2, 'missing run-id / plan / command are FAILs');
    ok(/^@S RUNS \d+ in/.test(run('status').out), 'status text lists runs');
  } finally {
    fs.rmSync(T, { recursive: true, force: true });
  }
  ok(realLoopDir() === before, 'the real .isekai/instruments/loop/ was not touched');
  ok(!fs.existsSync(T), 'throwaway world removed');
  console.log(fails.length ? '@S FAIL' : `@S PASS ${checks} checks · node ${process.version}`);
  for (const f of fails) console.log(`@F selftest — ${f}`);
  if (fails.length) process.exit(1);
}

const go = { run, resume, explain, status, selftest }[cmd];
if (!go) fail(`unknown command ${cmd} — run <plan> | resume <run-id> | explain <plan> | status [run-id] | selftest`);
Promise.resolve(go()).catch(e => fail(`${cmd} crashed: ${e && e.stack || e}`));
