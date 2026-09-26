// Two decks from one outline: the isekai binary (isekai vocabulary, portraits) and agent-one
// (IT vocabulary, role badges). Every terminal panel is a real capture from the released binary
// (captures/<dist>-*.txt) and every screenshot is the real board (captures/<dist>-*.png).
//   node build_binary.js            → ../isekai-binary.pptx and ../agent-one-binary.pptx
const fs = require("fs");
const path = require("path");
const pptxgen = require("pptxgenjs");

const CAP = (f) => path.join(__dirname, "captures", f);
const ROOT = path.join(__dirname, "..", "..");
const F_HEAD = "Cambria", F_BODY = "Calibri", F_MONO = "Courier New";

// ---- the two distributions: every word and colour that differs lives here -------------------
const DIST = {
  isekai: {
    out: "isekai-binary.pptx", bin: "isekai", title: "ISEKAI", version: "v0.1.3",
    tagline: "The law as a harness", repo: "github.com/Kaginari/isekai (private)",
    sub: "One Go binary where the convention is enforced in code — not a prompt the model may skip",
    footer: "ISEKAI ⋄ THE LAW AS A HARNESS",
    bg: "070B14", ink: "ECF1FA", inkDark: "12182A", dim: "6B7A90", dimLight: "9FB3CC", light: "F6F8FB",
    a1: "2695BD", a2: "7C5CD6", a3: "BD8C24", warn: "D6402A",
    world: "world", World: "World", worlds: "worlds", law: "law", Law: "Law", lawFile: "isekai.md",
    session: "Rimuru", sessionRole: "the session — thinks, decides, orders", human: "Veldora (you)",
    court: "Court Body", courts: "Court Bodies", gateHolder: "Orc", author: "Slime", router: "Elf",
    offices: ["Great Sage", "Raphael", "Ciel"], officeJobs: ["reads — facts, file:line", "judges — verdicts with evidence", "writes — drafts, the log, the desk"],
    unsaid: "the unsaid (law · colony · territory)", colonyPage: "Colony", courtPage: "Court", dir: ".isekai/",
    ranks: [
      { img: "rimuru", name: "Rimuru", job: "the session: thinks, decides, orders" },
      { img: "elf", name: "Elf", job: "the shared mind and voice; routes work" },
      { img: "orc", name: "Orc", job: "rules a domain; holds the gate" },
      { img: "slime", name: "Slime", job: "ground truth of one zone; authors changes" },
      { img: "kijin", name: "Kijin", job: "standing lead of one subsystem (Keeper)" },
      { img: "dark_elf", name: "Dark Elf", job: "auditor: reviews verdicts and the log" },
    ],
    portrait: (n) => path.join(ROOT, ".isekai", "portraits", `${n}.png`),
    cover: "rimuru",
  },
  "agent-one": {
    out: "agent-one-binary.pptx", bin: "agent-one", title: "AGENT-ONE", version: "v0.1.2",
    tagline: "Policies enforced in code", repo: "github.com/Kaginari/agent-one",
    sub: "One Go binary where review gates, approvals, the sandbox and memory are part of the runtime",
    footer: "AGENT-ONE ⋄ POLICIES ENFORCED IN CODE",
    bg: "0B1A2B", ink: "EEF3F9", inkDark: "10202F", dim: "5F6F82", dimLight: "A9B8C9", light: "F4F7FA",
    a1: "1F6F8B", a2: "7C5CD6", a3: "BD8C24", warn: "C53D34",
    world: "workspace", World: "Workspace", worlds: "workspaces", law: "policy", Law: "Policy", lawFile: "AGENT-ONE.md",
    session: "Orchestrator", sessionRole: "the session — plans, decides, dispatches", human: "the operator (you)",
    court: "ephemeral subagent", courts: "Ephemeral subagents", gateHolder: "Domain owner", author: "Zone worker", router: "Coordinator",
    offices: ["Analyst", "Judge", "Drafter"], officeJobs: ["reads — facts, file:line", "judges — verdicts with evidence", "writes — drafts, the log, notes"],
    unsaid: "undocumented knowledge (policy · team · domain)", colonyPage: "Team", courtPage: "Subagents", dir: ".agent-one/",
    ranks: [
      { img: "orchestrator", name: "Orchestrator", job: "the session: plans, decides, dispatches" },
      { img: "coordinator", name: "Coordinator", job: "shared context; routes work across domains" },
      { img: "domain-owner", name: "Domain owner", job: "owns a domain; holds the review gate" },
      { img: "zone-worker", name: "Zone worker", job: "owns one zone; authors changes" },
      { img: "service-owner", name: "Service owner", job: "long-running owner of one subsystem" },
      { img: "auditor", name: "Auditor", job: "reviews verdicts and the change log" },
    ],
    portrait: (n) => path.join(ROOT, "agent-one", "portraits", `${n}.png`),
    cover: "orchestrator",
  },
};

// ---- captures ---------------------------------------------------------------------------------
function cap(dist, name, { maxLines = 18, width = 92, from = 0 } = {}) {
  const t = fs.readFileSync(CAP(`${dist}-${name}.txt`), "utf8").replace(/\s+$/, "");
  return t.split("\n").slice(from, from + maxLines).map((l) => (l.length > width ? l.slice(0, width - 1) + "…" : l));
}

function build(key) {
  const T = DIST[key];
  const pres = new pptxgen();
  pres.layout = "LAYOUT_WIDE";
  const PW = 13.33, PH = 7.5;
  let page = 1;

  // ---- helpers -----------------------------------------------------------------------------
  const chrome = (s, dark) => {
    s.addText(T.footer, { x: 0.5, y: PH - 0.45, w: 8, h: 0.3, fontFace: F_BODY, fontSize: 9, color: dark ? T.dimLight : T.dim, charSpacing: 1, isTextBox: true, margin: 0 });
    page += 1;
    s.addText(String(page), { x: PW - 1, y: PH - 0.45, w: 0.5, h: 0.3, fontFace: F_BODY, fontSize: 9, color: dark ? T.dimLight : T.dim, align: "right", isTextBox: true, margin: 0 });
  };
  const title = (s, text, dark, sub) => {
    s.addText(text, { x: 0.6, y: 0.45, w: PW - 1.2, h: 0.8, fontFace: F_HEAD, bold: true, fontSize: 30, color: dark ? T.ink : T.inkDark, isTextBox: true, margin: 0 });
    if (sub) s.addText(sub, { x: 0.6, y: 1.2, w: PW - 1.2, h: 0.45, fontFace: F_BODY, fontSize: 14, color: dark ? T.dimLight : T.dim, isTextBox: true, margin: 0 });
  };
  const slide = (dark) => { const s = pres.addSlide(); s.background = { color: dark ? T.bg : T.light }; return s; };
  // a terminal panel: a real capture in a dark rounded frame
  const term = (s, { x, y, w, h, lines, label, size = 10 }) => {
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y, w, h, fill: { color: "0D1117" }, line: { color: "2B3240", width: 0.75 }, rectRadius: 0.1,
      shadow: { type: "outer", blur: 6, offset: 2, angle: 90, color: "000000", opacity: 0.25 } });
    s.addText(label || `$ ${T.bin}`, { x: x + 0.2, y: y + 0.1, w: w - 0.4, h: 0.3, fontFace: F_MONO, fontSize: 9, color: "7D8590", isTextBox: true, margin: 0 });
    s.addText(lines.join("\n"), { x: x + 0.2, y: y + 0.42, w: w - 0.4, h: h - 0.55, fontFace: F_MONO, fontSize: size, color: "D1D9E0", valign: "top", isTextBox: true, margin: 0, lineSpacingMultiple: 1.05 });
  };
  const card = (s, { x, y, w, h, head, body, accent, dark, size = 12 }) => {
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y, w, h, fill: { color: dark ? "111A2A" : "FFFFFF" }, line: { color: dark ? "22304A" : "DCE3EC", width: 0.75 }, rectRadius: 0.1,
      shadow: { type: "outer", blur: 4, offset: 1, angle: 90, color: "000000", opacity: dark ? 0.3 : 0.08 } });
    s.addShape(pres.shapes.OVAL, { x: x + 0.25, y: y + 0.28, w: 0.22, h: 0.22, fill: { color: accent }, line: { type: "none" } });
    s.addText(head, { x: x + 0.6, y: y + 0.18, w: w - 0.8, h: 0.42, fontFace: F_HEAD, bold: true, fontSize: 15, color: dark ? T.ink : T.inkDark, isTextBox: true, margin: 0, valign: "middle" });
    s.addText(body, { x: x + 0.25, y: y + 0.68, w: w - 0.5, h: h - 0.85, fontFace: F_BODY, fontSize: size, color: dark ? T.dimLight : "3B4656", isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.1 });
  };
  const shot = (s, file, { x, y, w, h }) => {
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: x - 0.04, y: y - 0.04, w: w + 0.08, h: h + 0.08, fill: { color: "FFFFFF" }, line: { color: "CBD5E1", width: 0.75 }, rectRadius: 0.06,
      shadow: { type: "outer", blur: 8, offset: 2, angle: 90, color: "000000", opacity: 0.18 } });
    s.addImage({ path: CAP(file), x, y, w, h, sizing: { type: "cover", w, h } });
  };
  const bullets = (items, color) => items.map((t, i) => ({ text: t, options: { bullet: true, breakLine: i < items.length - 1, paraSpaceAfter: 8, color } }));

  // ================================================================ 1 · title (dark)
  {
    const s = slide(true);
    s.addImage({ path: T.portrait(T.cover), x: PW / 2 - 0.85, y: 0.9, w: 1.7, h: 1.7, rounding: true });
    s.addText(T.title, { x: 0, y: 2.8, w: PW, h: 1.1, align: "center", fontFace: F_HEAD, bold: true, fontSize: 58, color: T.ink, charSpacing: 6, isTextBox: true, margin: 0 });
    s.addText(T.tagline, { x: 0, y: 3.85, w: PW, h: 0.5, align: "center", fontFace: F_HEAD, italic: true, fontSize: 22, color: T.a3, isTextBox: true, margin: 0 });
    s.addText(T.sub, { x: 1.8, y: 4.5, w: PW - 3.6, h: 0.8, align: "center", fontFace: F_BODY, fontSize: 16, color: T.dimLight, isTextBox: true, margin: 0 });
    s.addText(`${T.version}  ·  ${T.repo}`, { x: 0, y: PH - 0.9, w: PW, h: 0.35, align: "center", fontFace: F_MONO, fontSize: 11, color: T.dim, isTextBox: true, margin: 0 });
    s.addNotes(`Title. ${T.title} ${T.version}: a single static Go binary. Everything shown in this deck is captured from the released binary running in a demo ${T.world} against a scripted OpenAI-compatible server.`);
  }

  // ================================================================ 2 · why
  {
    const s = slide(false);
    title(s, "A model can read a rule — and still skip it", false, `Other harnesses put the ${T.law} in the prompt. Here the harness is the ${T.law}.`);
    const items = [
      { h: "The review gate", b: `Every turn that changed the ${T.world} is checked before it's called done: right author, invariants hold, work complete, doc changed with the code. The verdict is appended to log.md — even when the write came through the shell.`, c: T.a1 },
      { h: "Human approval", b: "Every action is classified before it runs — read · write · outward · destructive. Outward and destructive ask you. Per-command rules (bash:git push*) allow, ask or deny, and hold against wrapped forms like env git push or sh -c.", c: T.a2 },
      { h: "The sandbox", b: `bash is a persistent shell under bwrap: read-only filesystem except the ${T.world}, no network unless the act was approved as outward, API keys and tokens scrubbed from the environment.`, c: T.a3 },
      { h: "Native memory", b: `Recall before every step, remember after. ${cap1(T.unsaid)} is surfaced and filed. When the context window fills, it is drained to memory by pointer — not summarized away.`, c: T.warn },
    ];
    items.forEach((it, i) => card(s, { x: 0.6 + (i % 2) * 6.1, y: 1.95 + Math.floor(i / 2) * 2.45, w: 5.9, h: 2.25, head: it.h, body: it.b, accent: it.c, size: 14 }));
    chrome(s, false);
    s.addNotes("The four things the binary enforces in code. Each is proven by tests and attacked by an independent validator; the gate's shell-write blind spot and several permission bypasses were found and fixed before release.");
  }

  // ================================================================ 3 · loop vs others
  {
    const s = slide(false);
    title(s, "Same loop as Claude Code and OpenCode — plus what they lack", false, "One model call per step. The other five beats are code: no tokens, milliseconds.");
    const hdr = (t) => ({ text: t, options: { bold: true, color: "FFFFFF", fill: { color: T.inkDark }, fontFace: F_BODY, fontSize: 12 } });
    const row = (a, b, c) => [{ text: a, options: { bold: true, fontSize: 11, color: T.inkDark } }, { text: b, options: { fontSize: 11, color: "3B4656" } }, { text: c, options: { fontSize: 11, color: T.inkDark } }];
    s.addTable([
      [hdr(""), hdr("Claude Code / OpenCode"), hdr(T.title.charAt(0) + T.title.slice(1).toLowerCase())],
      row("Model calls per step", "1", "1 — the same"),
      row("What the model sees", "full history, full tool outputs, every tool schema", "pointers instead of file contents; only the tools that fit the turn"),
      row("Checking the work", "none built in — the model says it's done", "verify per step + the review gate per turn, verdict in log.md"),
      row("Context full", "one summary (lossy)", "drain: mechanical passes first, one call for the undocumented and the notes"),
      row("Subagents", "one model for all", "each role on its own model (cheap to read, strong to judge)"),
      row("Failure", "retry or stop", "bounded retries, then one hop up with a named gap — never a guess"),
    ], { x: 0.6, y: 1.9, w: PW - 1.2, colW: [2.6, 4.5, 5.03], rowH: 0.6, fontFace: F_BODY, border: { type: "solid", pt: 0.75, color: "DCE3EC" }, fill: { color: "FFFFFF" }, valign: "middle", margin: 0.08 });
    chrome(s, false);
    s.addNotes("Per step the cost is the same as other harnesses: one model call. Per task it should be lower — smaller prompts, cheaper models for reading, fewer redone steps — to be measured with Harbor.");
  }

  // ================================================================ 4 · where the model sits
  {
    const s = slide(true);
    title(s, "Where the model sits in the loop", true, "Six beats per tool step, every beat journaled.");
    const beats = [["perceive", "budgets, context %"], ["recall", "memory + tool pointers"], ["plan", "THE MODEL CALL"], ["act", "run the chosen tool"], ["verify", "exit code / check"], ["record", "journal, notes"]];
    const bw = 1.85, gap = 0.22, x0 = (PW - (6 * bw + 5 * gap)) / 2;
    beats.forEach(([n, d], i) => {
      const x = x0 + i * (bw + gap), model = n === "plan";
      s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y: 2.6, w: bw, h: 1.6, fill: { color: model ? T.a3 : "111A2A" }, line: { color: model ? T.a3 : "2A3A55", width: 1 }, rectRadius: 0.12 });
      s.addText(n, { x, y: 2.8, w: bw, h: 0.5, align: "center", fontFace: F_HEAD, bold: true, fontSize: 18, color: model ? T.bg : T.ink, isTextBox: true, margin: 0 });
      s.addText(d, { x: x + 0.1, y: 3.35, w: bw - 0.2, h: 0.7, align: "center", fontFace: F_BODY, fontSize: 11, color: model ? T.bg : T.dimLight, isTextBox: true, margin: 0 });
      if (i < 5) s.addText("›", { x: x + bw, y: 3.05, w: gap, h: 0.5, align: "center", fontFace: F_HEAD, fontSize: 20, color: T.dim, isTextBox: true, margin: 0 });
    });
    s.addText([
      { text: "Outside the step, the model is called only by the binary's own routed work: ", options: { color: T.dimLight } },
      { text: `the gate's verdict (${T.offices[1]}), the drain's notes (${T.offices[2]}), and each ${T.court}'s own loop.`, options: { color: T.ink } },
    ], { x: 1.2, y: 4.8, w: PW - 2.4, h: 0.8, fontFace: F_BODY, fontSize: 15, align: "center", isTextBox: true, margin: 0 });
    chrome(s, true);
    s.addNotes("The only token-consuming beat is plan. Budgets are readings, not feelings; past a budget the run checkpoints and prints a resume command.");
  }

  // ================================================================ 5 · a run
  {
    const s = slide(false);
    title(s, "One ask, end to end", false, `${T.bin} run — classified step, the write, the report, the gate's verdict, live cost.`);
    term(s, { x: 0.6, y: 1.9, w: 7.4, h: 2.4, lines: cap(key, "run-ok", { maxLines: 8, width: 78 }), label: `$ ${T.bin} run "Create hello.txt with the line: hello bench"`, size: 11 });
    term(s, { x: 0.6, y: 4.5, w: 7.4, h: 2.35, lines: cap(key, "log", { maxLines: 7, width: 78, from: 6 }), label: `$ tail ${T.dir}log.md`, size: 11 });
    s.addText(bullets([
      "→ s1 bash [write]: the classifier settled the class before it ran — inside the " + T.world + ", so no prompt.",
      "The review gate ran on the shell write and appended its verdict to log.md.",
      "Tokens and cost are live, priced from config — an unpriced model shows tokens, never a guess.",
      "--json gives the same run as one JSON object for automation and benchmarks.",
    ], "3B4656"), { x: 8.3, y: 1.95, w: 4.45, h: 4.9, fontFace: F_BODY, fontSize: 14, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("Real capture: the model here is a scripted OpenAI-compatible server (the benchmark's fake vLLM), so the run is reproducible and costs nothing.");
  }

  // ================================================================ 6 · human approval
  {
    const s = slide(false);
    title(s, "Human approval: the harness decides, not the model", false, "The scripted model tried to write outside the " + T.world + ". Nobody was there to approve it.");
    term(s, { x: 0.6, y: 1.9, w: 12.1, h: 1.55, lines: cap(key, "run", { maxLines: 3, width: 128 }), label: `$ ${T.bin} run "Create the file /app/hello.txt with hello bench"`, size: 10.5 });
    term(s, { x: 0.6, y: 3.7, w: 5.6, h: 3.1, label: `${T.dir}config.yaml`, size: 11, lines: [
      "permissions:", "  rules:", '    - { match: "bash:git push*",', "        action: ask }", '    - { match: "bash:git push --force*",', "        action: deny }", "tools:", "  webfetch: { enabled: false }"] });
    s.addText(bullets([
      "read · write inside the " + T.world + " — run; the gate judges the result.",
      "outward (network, outside paths, git push) · destructive — ask; with no TTY, deny and name the flag that would pre-approve.",
      "The most specific rule wins; deny beats approval. Rules match the inner forms too: env git push, sh -c '…', git -C . push, xargs, sudo.",
      "A bare \"allow everything\" on outward is refused when config loads.",
    ], "3B4656"), { x: 6.5, y: 3.75, w: 6.2, h: 3.1, fontFace: F_BODY, fontSize: 13, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("The validator attacked the deny rule with eight disguises of git push; before the fix several slipped through. All hold now, with tests.");
  }

  // ================================================================ 7 · the CLI
  {
    const s = slide(false);
    title(s, "The CLI, part by part", false, "Every command speaks the same instruments; nothing keeps a private store.");
    const rows = [
      ["repl (default)", "the live session — talk while " + T.courts.toLowerCase() + " work; /agents /send /usage /status /compact"],
      ["run \"<ask>\"", "one ask to its end, non-interactive; --json for machines"],
      ["resume · sessions", "continue or list sessions (JSONL on disk)"],
      ["status", "the instrument board and the off-list — every disabled feature, and where"],
      ["config", "show · explain · check · path · patch — every value with its origin"],
      ["memory · toolbox · onto", "the memory tiers, the two-level tool registry, the ownership graph"],
      ["usage", "tokens and cost by agent, role, rank, model and day"],
      ["bench · selftest", "a fixed task set on each model · every package's self-checks"],
      ["board · init · version", "the dashboard alone · onboard a " + T.world + " · build stamp"],
    ];
    s.addTable([[{ text: "command", options: { bold: true, color: "FFFFFF", fill: { color: T.inkDark } } }, { text: "what it does", options: { bold: true, color: "FFFFFF", fill: { color: T.inkDark } } }],
      ...rows.map(([a, b]) => [{ text: `${T.bin} ${a}`, options: { fontFace: F_MONO, fontSize: 11, color: T.a1, bold: true } }, { text: b, options: { fontSize: 12, color: "3B4656" } }])],
      { x: 0.6, y: 1.85, w: 6.9, colW: [2.75, 4.15], rowH: 0.47, fontFace: F_BODY, border: { type: "solid", pt: 0.75, color: "DCE3EC" }, fill: { color: "FFFFFF" }, valign: "middle", margin: 0.06 });
    term(s, { x: 7.75, y: 1.85, w: 5.0, h: 4.95, lines: cap(key, "help", { maxLines: 17, width: 58 }), label: `$ ${T.bin} help`, size: 8.5 });
    chrome(s, false);
    s.addNotes("One static binary, linux and macOS, amd64 and arm64. Same commands in both distributions.");
  }

  // ================================================================ 8 · status & honesty
  {
    const s = slide(false);
    title(s, "status: nothing switched off is ever silent", false, "Every feature can be disabled in config — and every disabled one is listed, with the file and line that did it.");
    term(s, { x: 0.6, y: 1.9, w: 12.1, h: 4.95, lines: cap(key, "status", { maxLines: 22, width: 130 }).map((l) => l.replace(/\s{2,}/g, "  ")), label: `$ ${T.bin} status`, size: 9.5 });
    chrome(s, false);
    s.addNotes("The honesty rule: a switched-off instrument is a finding, not silence. The ranks line, models per role and rank, the sandbox probe, the drain passes and the budgets are all readings.");
  }

  // ================================================================ 9 · config
  {
    const s = slide(true);
    title(s, "Configuration: everything on, config takes away", true, "YAML or JSON · seven layers · every value knows where it came from.");
    term(s, { x: 0.6, y: 1.9, w: 6.2, h: 4.95, label: `${T.dir}config.yaml`, size: 10.5, lines: [
      "providers:", "  vllm:", "    type: openai", "    baseURL: https://vllm.internal/v1", "    apiKeyEnv: VLLM_API_KEY   # never a key in a file",
      "models:", "  default: anthropic/claude-opus-5", "  " + (key === "isekai" ? "offices" : "roles") + ":",
      `    ${key === "isekai" ? "great-sage" : "analyst"}: anthropic/claude-haiku-4-5`, `    ${key === "isekai" ? "raphael" : "judge"}:   anthropic/claude-opus-5`, `    ${key === "isekai" ? "ciel" : "drafter"}:    anthropic/claude-fable-5-1`,
      "tools:", "  custom:", "    kube-pods:", '      run: [kubectl, get, pods, -n, "{{ns}}"]', "      class: outward", "rules:", '  - { text: "Tests pass before a change lands.",', '      check: "go test ./..." }'] });
    s.addText(bullets([
      "Layers: defaults → ~/.config → project → project-local → env file → env → flags.",
      "config explain shows every value with its origin; config patch edits keeping your comments.",
      "Strict YAML: anything it can't parse is an error with file:line — never a silent misparse.",
      "Custom tools: each {{param}} fills one argument — '; rm -rf /' stays a file name.",
      "Injected rules go to the prompt; rules with a check are enforced by the gate.",
      "Any OpenAI-compatible endpoint: vLLM, Ollama, OpenRouter — keys from env only.",
    ], T.dimLight), { x: 7.1, y: 1.95, w: 5.65, h: 4.9, fontFace: F_BODY, fontSize: 14, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, true);
    s.addNotes("While capturing this deck, the strict parser rejected two of my own config mistakes with file and line — exactly as designed.");
  }

  // ================================================================ 10 · memory
  {
    const s = slide(false);
    title(s, "Memory, native in the loop", false, "Three tiers every agent has. Files are the truth; every index is derived and rebuildable.");
    const tiers = [
      { h: "Short — per agent, per task", b: "The context window (measured, never guessed) · working notes (~5 live thoughts) · a semantic cache so the same question costs one search.", c: T.a1 },
      { h: "Long — durable, git is the record", b: "Episodic: log.md · procedural: skills, commands, tools · semantic: the " + T.law + ", docs, the ownership graph. Recall ranks by meaning, then by relation.", c: T.a2 },
      { h: "Shared — across agents", b: `The ${T.world}'s shared notes travel with git; machine-shared notes span every ${T.world} on the machine. ${cap1(T.unsaid)} lands here before a subagent's context dies.`, c: T.a3 },
    ];
    tiers.forEach((t, i) => card(s, { x: 0.6, y: 1.9 + i * 1.62, w: 6.3, h: 1.48, head: t.h, body: t.b, accent: t.c, size: 11.5 }));
    term(s, { x: 7.2, y: 1.9, w: 5.55, h: 2.3, lines: cap(key, "memory", { maxLines: 7, width: 60 }), label: `$ ${T.bin} memory status`, size: 9.5 });
    s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x: 7.2, y: 4.4, w: 5.55, h: 2.45, fill: { color: T.inkDark }, line: { type: "none" }, rectRadius: 0.1 });
    s.addText([
      { text: "Compaction = a drain, not a summary", options: { bold: true, color: "FFFFFF", fontFace: F_HEAD, fontSize: 15, breakLine: true } },
      { text: "1 pointerize: file reads become path:lines · hash\n2 trim: finished steps shrink to their journal line\n3 one call: surface the undocumented + write the notes\n4 episode: log what landed\n5 verify: ask kept, gaps kept, pointers resolve — or abort", options: { color: T.dimLight, fontFace: F_MONO, fontSize: 10.5 } },
    ], { x: 7.45, y: 4.55, w: 5.1, h: 2.2, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("Two of the five drain passes need no model at all. A verify failure keeps the old context — compaction can never lose the ask.");
  }

  // ================================================================ 11 · toolbox
  {
    const s = slide(false);
    title(s, "The toolbox: two levels, one budget", false, "The registry can grow without bound; the prompt does not.");
    card(s, { x: 0.6, y: 1.9, w: 5.9, h: 1.75, head: "Level 1 — the manifest", body: "Each turn gets only the entries that fit the ask and a token budget: name, one line, triggers, path, and the cost of loading it. Skills, commands, tools, agents, MCP servers — all priced.", accent: T.a1 });
    card(s, { x: 0.6, y: 3.85, w: 5.9, h: 1.75, head: "Level 2 — the load", body: "The agent loads a body only when it decides to use it — whole, or one section. Every load is journaled, so status reports what was loaded against what was merely offered.", accent: T.a2 });
    card(s, { x: 0.6, y: 5.8, w: 5.9, h: 1.05, head: "Missing tool? Named, never looped", body: "Error + closest alternatives; a need named twice becomes a config diff you approve.", accent: T.a3, size: 11 });
    term(s, { x: 6.8, y: 1.9, w: 5.95, h: 3.0, lines: cap(key, "toolbox", { maxLines: 6, width: 64 }), label: `$ ${T.bin} toolbox brief "edit a file and run the tests"`, size: 9.5 });
    s.addText(bullets([
      "Default toolset (max): bash, read/ls/glob/grep, write/edit/multiedit/patch, git, webfetch, websearch, ask, dispatch, recall/remember, toolbox, onto, skill, MCP.",
      "Reads .claude/ and .opencode/ skills, commands and agents as they are.",
    ], "3B4656"), { x: 6.85, y: 5.1, w: 5.9, h: 1.8, fontFace: F_BODY, fontSize: 12.5, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("An empty demo " + T.world + " has an empty registry, and the brief says so as a named gap rather than an empty answer.");
  }

  // ================================================================ 12 · ranks & roles
  {
    const s = slide(true);
    title(s, key === "isekai" ? "Ranks and offices" : "Roles and model routing", true, `Built-in ranks are the default; config can add, override, or replace them all (rankSet: replace).`);
    const pw = 1.25;
    T.ranks.forEach((r, i) => {
      const col = i % 3, rowi = Math.floor(i / 3), x = 0.6 + col * 4.1, y = 1.95 + rowi * 1.6;
      s.addImage({ path: T.portrait(r.img), x, y, w: pw, h: pw, rounding: true });
      s.addText(r.name, { x: x + pw + 0.2, y: y + 0.1, w: 2.6, h: 0.45, fontFace: F_HEAD, bold: true, fontSize: 16, color: T.ink, isTextBox: true, margin: 0 });
      s.addText(r.job, { x: x + pw + 0.2, y: y + 0.55, w: 2.6, h: 0.7, fontFace: F_BODY, fontSize: 12, color: T.dimLight, isTextBox: true, margin: 0, valign: "top" });
    });
    const ox = 0.6, oy = 5.35;
    T.offices.forEach((o, i) => {
      const x = ox + i * 4.1;
      s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y: oy, w: 3.8, h: 1.3, fill: { color: [T.a1, T.a2, T.a3][i] }, line: { type: "none" }, rectRadius: 0.1 });
      s.addText(o, { x: x + 0.25, y: oy + 0.12, w: 3.3, h: 0.45, fontFace: F_HEAD, bold: true, fontSize: 17, color: "FFFFFF", isTextBox: true, margin: 0 });
      s.addText(T.officeJobs[i] + (i < 2 ? "   →" : ""), { x: x + 0.25, y: oy + 0.6, w: 3.4, h: 0.55, fontFace: F_BODY, fontSize: 12.5, color: "FFFFFF", isTextBox: true, margin: 0 });
    });
    chrome(s, true);
    s.addNotes(`One escalating chain: ${T.offices.join(" → ")}. Each can run its own model; config refuses a chain where a later role sits on a weaker model. The session never chooses its own model — it runs on yours.`);
  }

  // ================================================================ 13 · live session
  {
    const s = slide(false);
    title(s, "A live session: keep talking while the work runs", false, `${T.courts} run in the background; tokens and cost are an instrument.`);
    term(s, { x: 0.6, y: 1.9, w: 6.4, h: 3.15, lines: cap(key, "usage", { maxLines: 12, width: 70 }), label: `$ ${T.bin} usage`, size: 10 });
    s.addText(bullets([
      "Type while a turn runs — your line reaches the agent at its next tool step; Ctrl-C interrupts.",
      `/agents and a status line: every live agent's role, model, state, context %, tokens, cost.`,
      `/send <agent> talks to a running ${T.court}; its context dies when its report is taken.`,
      "Usage journal per call, priced from config; session and subagent budgets stop honestly at the line with a resume command.",
    ], "3B4656"), { x: 7.3, y: 1.95, w: 5.45, h: 4.9, fontFace: F_BODY, fontSize: 14, valign: "top", isTextBox: true, margin: 0 });
    s.addText("The session's own calls carry no role — only dispatched agents and routed calls do.", { x: 0.6, y: 5.25, w: 6.4, h: 0.6, fontFace: F_BODY, italic: true, fontSize: 12, color: T.dim, isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("While capturing this deck, usage showed the session's calls labelled as another rank: a machine-wide agent file had been imported as the session. Found here, fixed with a regression test, released as a patch.");
  }

  // ================================================================ 14 · the board
  {
    const s = slide(false);
    title(s, "The board: the " + T.world + ", seen", false, "Served by the binary on localhost · eight pages on the Bootstrap grid · live over SSE · dark and light.");
    shot(s, `${key}-usage.png`, { x: 0.6, y: 1.85, w: 7.75, h: 4.95 });
    shot(s, `${key}-phone.png`, { x: 9.1, y: 1.85, w: 3.35, h: 4.95 });
    chrome(s, false);
    s.addNotes(`Pages: Overview · ${T.courtPage} · Usage · ${T.colonyPage} · Memory · Toolbox · Log · Config. Mobile first: the phone layout on the right. A page is proven by rendering it — that rule caught a rename that had silently broken the theme.`);
  }

  // ================================================================ 15 · board: graph + config
  {
    const s = slide(false);
    title(s, "The ownership graph, and every value with its origin", false, `The ${T.colonyPage} page draws who owns what and how knowledge flows; Config shows each value and the layer it came from.`);
    shot(s, `${key}-${key === "isekai" ? "colony" : "team"}-crop.png`, { x: 0.6, y: 1.9, w: 12.1, h: 2.59 });
    shot(s, `${key}-config-crop.png`, { x: 0.6, y: 4.75, w: 5.25, h: 2.1 });
    s.addText(bullets([
      `Lanes by rank; typed, coloured links: ground truth up to the gate holder, verdicts up to the ${T.router.toLowerCase()}.`,
      "Click a node: its neighbours light up, its doc opens beside the graph.",
      "The off-list names every disabled feature with the file and line that did it.",
    ], "3B4656"), { x: 6.15, y: 4.8, w: 6.55, h: 2.05, fontFace: F_BODY, fontSize: 13, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("The graph is a fixture world with an elf/coordinator, an orc/domain owner and two zone owners; the config page is the demo world, showing webfetch switched off.");
  }

  // ================================================================ 16 · proof
  {
    const s = slide(true);
    title(s, "Proof, not promises", true, "What was measured before each release.");
    const found = key === "isekai" ? "9 + 6" : "9 + 8";
    const stats = [["195", "selftest checks"], ["23", "test packages, race-clean"], [found, "defects fixed: by attack, then by using the releases"], ["1.0 / 0.0", "Harbor smoke: right / wrong"]];
    stats.forEach(([n, l], i) => {
      const x = 0.6 + i * 3.1;
      s.addText(n, { x, y: 2.0, w: 2.9, h: 1.1, fontFace: F_HEAD, bold: true, fontSize: 48, color: [T.a1, T.a2, T.a3, T.warn][i], isTextBox: true, margin: 0 });
      s.addText(l, { x, y: 3.1, w: 2.8, h: 0.8, fontFace: F_BODY, fontSize: 14, color: T.dimLight, isTextBox: true, margin: 0, valign: "top" });
    });
    s.addText(bullets([
      "An independent validator attacked the gate, the sandbox, the permission rules, symlinks, secrets and injection — before release. Running the released binaries for this deck found the rest; each fix ships with a regression test.",
      "Every commit: gofmt, vet, race tests, selftest, four builds, GoReleaser check and the Harbor smoke on GitHub's runners.",
      "Honest limits: providers' streaming, caching and fallbacks are tested against local fakes only — not yet live.",
    ], T.dimLight), { x: 0.6, y: 4.3, w: PW - 1.2, h: 2.4, fontFace: F_BODY, fontSize: 15, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, true);
    s.addNotes("The numbers come from the validation log and CI, not estimates.");
  }

  // ================================================================ 17 · benchmarks
  {
    const s = slide(false);
    title(s, "Benchmarks under Harbor, in Docker", false, "One container per task, the agent inside, the task's own tests deciding. Every launch recorded.");
    const steps = [["Harbor", "runs a task in its own container"], ["adapter", `installs the static ${T.bin}, seeds a ${T.world}`], [T.bin, "run --json against any OpenAI-compatible model"], ["verifier", "the task's tests: reward 1.0 or 0.0"], ["runs.jsonl", "append-only record → RESULTS.md"]];
    steps.forEach(([h, b], i) => {
      const x = 0.6 + i * 2.5;
      s.addShape(pres.shapes.ROUNDED_RECTANGLE, { x, y: 1.95, w: 2.25, h: 1.6, fill: { color: "FFFFFF" }, line: { color: "DCE3EC", width: 0.75 }, rectRadius: 0.1 });
      s.addText(h, { x: x + 0.15, y: 2.05, w: 1.95, h: 0.45, fontFace: F_MONO, bold: true, fontSize: 13, color: T.a1, isTextBox: true, margin: 0 });
      s.addText(b, { x: x + 0.15, y: 2.5, w: 1.95, h: 0.95, fontFace: F_BODY, fontSize: 12, color: "3B4656", isTextBox: true, margin: 0, valign: "top" });
    });
    s.addText(bullets([
      "€0 smoke on every commit: a scripted OpenAI-compatible server stands in for vLLM — right answer must score 1.0, wrong 0.0.",
      "Next, at no API cost: your hosted vLLM, the same tasks, " + T.bin + " against OpenCode on the same model.",
      "Claude runs only with an API key and a hard budget per task (budgets.session.usd).",
    ], "3B4656"), { x: 0.6, y: 3.95, w: PW - 1.2, h: 2.9, fontFace: F_BODY, fontSize: 15, valign: "top", isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("Harbor is the Terminal-Bench team's framework; the adapter is ~100 lines against its BaseInstalledAgent API.");
  }

  // ================================================================ 18 · next
  {
    const s = slide(false);
    title(s, "Next: v0.2", false, "Specified, not built yet.");
    const nxt = [
      { h: "Container runtime", b: "Install anything inside a disposable container; your repo, the network and pushes stay gated. Builds via rootless BuildKit — never the host Docker socket.", c: T.a1 },
      { h: "Enterprise registries & proxy", b: "Artifactory, GitLab, Nexus for docker, go, pypi, npm, cargo, maven; corporate proxy and CA everywhere; later a token-injecting egress proxy so no secret enters the container.", c: T.a2 },
      { h: key === "isekai" ? "Worlds and dimensions" : "Workspaces and systems", b: key === "isekai" ? "Worlds inside worlds are sovereign; dimensions group worlds (app · deploy · infra) with typed, learned relations; one hop per level; the board opens on the map." : "Nested workspaces are sovereign; systems group workspaces (app · deploy · infra) with typed, learned relations; one hop per level; the dashboard opens on the map.", c: T.a3 },
    ];
    nxt.forEach((n, i) => card(s, { x: 0.6 + i * 4.1, y: 1.9, w: 3.9, h: 3.3, head: n.h, body: n.b, accent: n.c, size: 13.5 }));
    s.addText("Each lands by the ladder: a layer is done only when its own tests pass against mocks of the layer below, and the next layer opens with an integration test of the two.", { x: 0.6, y: 5.55, w: PW - 1.2, h: 0.9, fontFace: F_BODY, italic: true, fontSize: 15, color: T.dim, isTextBox: true, margin: 0 });
    chrome(s, false);
    s.addNotes("v0.2 lands by the same ladder: each layer green on its own tests before the next starts.");
  }

  // ================================================================ 19 · close (dark)
  {
    const s = slide(true);
    s.addImage({ path: T.portrait(T.cover), x: PW / 2 - 0.7, y: 1.2, w: 1.4, h: 1.4, rounding: true });
    s.addText(T.title + " " + T.version, { x: 0, y: 2.85, w: PW, h: 0.9, align: "center", fontFace: F_HEAD, bold: true, fontSize: 40, color: T.ink, isTextBox: true, margin: 0 });
    s.addText(key === "isekai" ? "gh release download --repo Kaginari/isekai" : "curl -fsSL https://raw.githubusercontent.com/Kaginari/agent-one/main/install.sh | sh",
      { x: 1, y: 3.95, w: PW - 2, h: 0.5, align: "center", fontFace: F_MONO, fontSize: 15, color: T.a3, isTextBox: true, margin: 0 });
    s.addText(`${T.bin} init   ·   ${T.bin}   ·   ${T.bin} run "…"`, { x: 1, y: 4.6, w: PW - 2, h: 0.5, align: "center", fontFace: F_MONO, fontSize: 15, color: T.dimLight, isTextBox: true, margin: 0 });
    s.addText("MIT · linux/macOS · amd64/arm64 · " + T.repo, { x: 0, y: PH - 0.9, w: PW, h: 0.35, align: "center", fontFace: F_BODY, fontSize: 11, color: T.dim, isTextBox: true, margin: 0 });
    s.addNotes("Install, onboard, run.");
  }

  return pres.writeFile({ fileName: path.join(__dirname, "..", T.out) });
}

function cap1(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

(async () => {
  for (const k of Object.keys(DIST)) console.log(await build(k));
})();
