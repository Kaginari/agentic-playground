const pptxgen = require("pptxgenjs");
const path = require("path");

const ICON = (name) => path.join(__dirname, "icons", `${name}.png`);
const PORTRAIT = (race) => path.join(__dirname, "icons", "portraits", `${race}.png`);

// ---- palette (from tempest.js's own validated race colors — brand tie-in) ----
const BG_DARK = "070B14";
const BG_LIGHT = "F6F8FB";
const CARD_LIGHT = "FFFFFF";
const INK_DARK = "12182A";
const INK_LIGHT = "ECF1FA";
const DIM = "6B7A90";
const DIM_LIGHT = "9FB3CC";
const CYAN = "2695BD";
const VIOLET = "7C5CD6";
const GOLD = "BD8C24";
const APPENDIX_BG = "171F30";

const F_HEAD = "Cambria";
const F_BODY = "Calibri";

const pres = new pptxgen();
pres.layout = "LAYOUT_WIDE"; // 13.3" x 7.5"
const PW = 13.33, PH = 7.5;

pres.defineSlideMaster({
  title: "BLANK",
  background: { color: BG_LIGHT },
});

// ---------------------------------------------------------------- helpers
function footer(slide, dark) {
  slide.addText("ISEKAI ⋄ A CONVENTION FOR LIVING WORLDS", {
    x: 0.5, y: PH - 0.5, w: 8, h: 0.3, fontFace: F_BODY, fontSize: 9,
    color: dark ? DIM_LIGHT : DIM, charSpacing: 1, isTextBox: true, margin: 0,
  });
}
let PAGE = 1; // slide 1 carries no number; every numbered slide bumps the counter
function pageNum(slide, _n, dark) {
  PAGE += 1;
  slide.addText(String(PAGE), {
    x: PW - 1, y: PH - 0.5, w: 0.5, h: 0.3, fontFace: F_BODY, fontSize: 9,
    color: dark ? DIM_LIGHT : DIM, align: "right", isTextBox: true, margin: 0,
  });
}
function slideTitle(slide, text, dark, color) {
  slide.addText(text, {
    x: 0.7, y: 0.55, w: PW - 1.4, h: 0.9, fontFace: F_HEAD, bold: true,
    fontSize: 30, color: color || (dark ? INK_LIGHT : INK_DARK), isTextBox: true, margin: 0,
  });
}
function iconCircleCard(slide, { x, y, w, iconName, title, body, accent, dark }) {
  const iconSize = 0.85;
  slide.addImage({ path: ICON(iconName), x: x, y: y, w: iconSize, h: iconSize });
  slide.addText(title, {
    x: x + iconSize + 0.22, y: y - 0.05, w: w - iconSize - 0.22, h: 0.4,
    fontFace: F_HEAD, bold: true, fontSize: 15, color: dark ? INK_LIGHT : INK_DARK,
    isTextBox: true, margin: 0, valign: "top",
  });
  slide.addText(body, {
    x: x + iconSize + 0.22, y: y + 0.35, w: w - iconSize - 0.22, h: iconSize - 0.3,
    fontFace: F_BODY, fontSize: 11, color: dark ? DIM_LIGHT : DIM, isTextBox: true,
    margin: 0, valign: "top", lineSpacingMultiple: 1.12,
  });
}
function numberBadge(slide, { x, y, n, color, size = 0.42 }) {
  slide.addShape("ellipse", { x, y, w: size, h: size, fill: { color }, line: { type: "none" } });
  slide.addText(String(n), {
    x, y, w: size, h: size, align: "center", valign: "middle", fontFace: F_HEAD,
    bold: true, fontSize: 15, color: "FFFFFF", isTextBox: true, margin: 0,
  });
}

// ================================================================== Slide 1 — Title
{
  const s = pres.addSlide();
  s.background = { color: BG_DARK };
  s.addText("⋄", { x: 0, y: 1.15, w: PW, h: 1.1, align: "center", fontFace: F_HEAD,
    fontSize: 46, color: CYAN, isTextBox: true, margin: 0 });
  s.addText("ISEKAI", { x: 0, y: 2.35, w: PW, h: 1.3, align: "center", fontFace: F_HEAD,
    bold: true, fontSize: 60, color: INK_LIGHT, charSpacing: 6, isTextBox: true, margin: 0 });
  s.addText("Reincarnating a Directory as a Living, Self-Documenting Multi-Agent World",
    { x: 1.6, y: 3.75, w: PW - 3.2, h: 0.7, align: "center", fontFace: F_BODY, fontSize: 17,
      color: DIM_LIGHT, isTextBox: true, margin: 0 });
  s.addShape("line", { x: PW/2 - 0.55, y: 4.55, w: 1.1, h: 0, line: { color: GOLD, width: 1.5 } });
  s.addText("A CROSS-HARNESS CONVENTION", { x: 0, y: 4.75, w: PW, h: 0.4, align: "center",
    fontFace: F_BODY, fontSize: 12, color: GOLD, charSpacing: 3, bold: true, isTextBox: true, margin: 0 });
  s.addText("convention-jura  ·  2026", { x: 0, y: PH - 0.75, w: PW, h: 0.35, align: "center",
    fontFace: F_BODY, fontSize: 10, color: DIM, isTextBox: true, margin: 0 });
  s.addNotes("Title slide. This deck explains the Isekai convention from OpenCode's perspective, "
    + "since OpenCode is where Minds and Bodies are natively authored. A Claude Code compatibility "
    + "appendix follows the main content.");
}

// ================================================================== Slide 2 — The problem
{
  const s = pres.addSlide();
  slideTitle(s, "AI sessions forget everything. Isekai doesn't.");
  s.addShape("rect", { x: 0.7, y: 1.85, w: 4.6, h: 3.9, fill: { color: INK_DARK }, line: { type: "none" }, rectRadius: 0.12, shape: "roundRect" });
  s.addText("0", { x: 0.7, y: 2.15, w: 4.6, h: 1.7, align: "center", fontFace: F_HEAD, bold: true,
    fontSize: 92, color: GOLD, isTextBox: true, margin: 0 });
  s.addText("context carried into the\nnext session, by default", { x: 0.9, y: 3.9, w: 4.2, h: 1.2,
    align: "center", fontFace: F_BODY, fontSize: 14, color: INK_LIGHT, isTextBox: true, margin: 0 });
  s.addText([
    { text: "Every AI coding session starts cold.", options: { bold: true, breakLine: true, color: INK_DARK } },
    { text: "It re-derives the codebase's shape, re-learns who owns what, and re-discovers the same gotchas the last session already found — because nothing survived the session boundary.", options: { breakLine: true, paraSpaceAfter: 14 } },
    { text: "Isekai answers this with disk, not memory: a directory that adopts the convention gets a ", options: { breakLine: false } },
    { text: ".isekai/", options: { fontFace: "Courier New", bold: true, color: CYAN, breakLine: false } },
    { text: " folder holding the world's law, its change log, and the creatures that work in it — so the next session picks up exactly where the last one left off, by reading what's on disk instead of guessing.", options: { breakLine: true } },
  ], { x: 5.7, y: 2.0, w: PW - 6.4, h: 3.7, fontFace: F_BODY, fontSize: 14, color: INK_DARK, isTextBox: true, margin: 0, valign: "top" });
  footer(s, false); pageNum(s, 2, false);
  s.addNotes("Framing slide. The core value proposition: persistence across session boundaries, via disk, not model memory.");
}

// ================================================================== Slide 3 — Four ranks
{
  const s = pres.addSlide();
  slideTitle(s, "Intelligence, distributed across four ranks");
  // The convention already has official race portraits (.isekai/portraits/) — use
  // those, not a generic icon, for the four ranks they actually depict.
  const cards = [
    { race: "rimuru", title: "Rimuru — the session agent", body: "Thinks, decides, orders. A machine-global throne body: one agent, installed once per machine, ruling whichever world it stands in. Works through the Elf by default." },
    { race: "elf", title: "Elf — the shared mind & voice", body: "Thinks across domains, routes work to Orcs, and is the one who speaks back up to the human. Every world gets at least one." },
    { race: "orc", title: "Orc — the domain ruler", body: "Rules one domain and holds the gate: verifies the right Slime authored, traits hold, duties done, doc truthful. Nothing lands without its pass." },
    { race: "slime", title: "Slime — the ground truth", body: "One narrow zone, held deeply. Authors changes there directly — its doc is the one fact anyone trusts about that zone." },
  ];
  const colW = (PW - 1.4 - 0.5) / 2;
  const portraitSize = 1.05;
  cards.forEach((c, i) => {
    const col = i % 2, row = Math.floor(i / 2);
    const x = 0.7 + col * (colW + 0.5);
    const y = 1.95 + row * 2.15;
    s.addShape("roundRect", { x, y, w: colW, h: 1.9, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.1,
      shadow: { type: "outer", color: "1B2333", opacity: 0.12, blur: 8, offset: 3, angle: 90 } });
    const px = x + 0.28, py = y + 0.42;
    s.addImage({ path: PORTRAIT(c.race), x: px, y: py, w: portraitSize, h: portraitSize });
    s.addText(c.title, {
      x: px + portraitSize + 0.22, y: y + 0.3, w: colW - portraitSize - 0.7, h: 0.45,
      fontFace: F_HEAD, bold: true, fontSize: 14.5, color: INK_DARK, isTextBox: true, margin: 0, valign: "top",
    });
    s.addText(c.body, {
      x: px + portraitSize + 0.22, y: y + 0.72, w: colW - portraitSize - 0.7, h: 1.1,
      fontFace: F_BODY, fontSize: 10.5, color: DIM, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.12,
    });
  });
  footer(s, false); pageNum(s, 3, false);
  s.addNotes("The four ranks, shown with their official portraits from .isekai/portraits/. Rimuru is the only rank excluded from model branding — it's model-agnostic by rule ('the throne does not choose its horse').");
}

// ================================================================== Slide 4 — Nine Natures
{
  const s = pres.addSlide();
  slideTitle(s, "Nine Natures — the crest, read first, always");
  const natures = [
    ["Vitality", "Code and doc change together, or the doc dies."],
    ["Symbiosis", "Each race does exactly its role — no duplication."],
    ["Evolution", "Every mistake mutates the genome, at once."],
    ["Genesis", "Nothing is born silently — a need is named twice."],
    ["Memory", "Thoughts are kept to ~5, distilled, let go."],
    ["Swarm", "Colonies emerge from observed convergence."],
    ["Containment", "Territory flows inward; nothing leaves without consent."],
    ["The Wire", "One terse channel between machines; humans get courtesy."],
    ["Perception", "The world watches itself — instruments stay lit."],
  ];
  const cols = 3, gw = (PW - 1.4 - 0.6) / cols, gh = 1.5;
  const cyc = [CYAN, VIOLET, GOLD];
  natures.forEach(([name, desc], i) => {
    const col = i % cols, row = Math.floor(i / cols);
    const x = 0.7 + col * (gw + 0.3), y = 1.85 + row * (gh + 0.22);
    s.addShape("roundRect", { x, y, w: gw, h: gh, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.08 });
    numberBadge(s, { x: x + 0.2, y: y + 0.2, n: i + 1, color: cyc[i % 3] });
    s.addText(name, { x: x + 0.75, y: y + 0.15, w: gw - 0.95, h: 0.4, fontFace: F_HEAD, bold: true,
      fontSize: 14, color: INK_DARK, isTextBox: true, margin: 0 });
    s.addText(desc, { x: x + 0.22, y: y + 0.68, w: gw - 0.44, h: gh - 0.78, fontFace: F_BODY,
      fontSize: 10.5, color: DIM, isTextBox: true, margin: 0, lineSpacingMultiple: 1.1 });
  });
  footer(s, false); pageNum(s, 4, false);
  s.addNotes("Condensed from the Crest. Full text lives in .isekai/isekai.md — this is the memorable nine-breath version.");
}

// ================================================================== Slide 5 — Nine Natures, as nature already does them
{
  const s = pres.addSlide();
  slideTitle(s, "Nine Natures — as nature already does them");
  s.addText("Each law is borrowed from a real phenomenon. The convention didn't invent these rules; biology did.",
    { x: 0.7, y: 1.32, w: PW - 1.4, h: 0.35, fontFace: F_BODY, fontSize: 12, color: DIM, isTextBox: true, margin: 0 });
  // [nature, phenomenon, one-breath description, icon file under icons/natures/]
  const phenomena = [
    ["Vitality", "DNA checkpoints & apoptosis",
     "A cell never divides before its genome is fully copied and repaired; a copy that can't be repaired triggers apoptosis — the cell removes itself rather than pass on a stale record.",
     "01-vitality-dna-checkpoint"],
    ["Symbiosis", "Lichen",
     "A fungus builds the body, holds water and mines minerals; an alga or cyanobacterium photosynthesizes and feeds both. Neither does the other's job, and together they live on bare rock.",
     "02-symbiosis-lichen"],
    ["Evolution", "CRISPR–Cas immunity in bacteria",
     "A bacterium that survives a phage stores a snippet of the invader's DNA in its own genome — at once. The next time that phage comes, Cas proteins cut it. The same mistake never repeats.",
     "03-evolution-crispr"],
    ["Genesis", "Quorum sensing (Vibrio fischeri)",
     "Every cell secretes a signal molecule; one cell's is noise. Only when the signal crosses a threshold — many cells naming the same need — does the colony switch on bioluminescence.",
     "04-genesis-quorum-sensing"],
    ["Memory", "Sleep-dependent consolidation",
     "The hippocampus holds the day in a small buffer, replays it to the neocortex during slow-wave sleep, which distils it into durable schemas, then clears the buffer. Analysis up, wisdom down.",
     "05-memory-sleep-consolidation"],
    ["Swarm", "Dictyostelium slime mold",
     "Solitary amoebae live alone while food lasts. When it runs out they pulse cAMP, converge into one migrating slug, fruit, and disperse. Nobody declares the colony; it emerges and dissolves.",
     "06-swarm-slime-mold"],
    ["Containment", "The blood–brain barrier",
     "Tight junctions seal the brain's capillaries: nothing crosses by default. What the brain needs passes only through specific gated transporters. Consent is the tissue's shape, not a checklist.",
     "07-containment-blood-brain-barrier"],
    ["The Wire", "The honeybee waggle dance",
     "A forager doesn't carry the field home — she points to it in a fixed schema: run angle = direction from the sun, run duration = distance. Known field order, so the field names disappear.",
     "08-the-wire-waggle-dance"],
    ["Perception", "Proprioception",
     "Muscle spindles and tendon organs measure the body's own stretch and tension continuously; position is read, never guessed. When that instrument goes silent, every move must be checked by eye.",
     "09-perception-proprioception"],
  ];
  const NATURE_ICON = (file) => path.join(__dirname, "icons", "natures", `${file}.png`);
  const cols = 3, gw = (PW - 1.4 - 0.6) / cols, gh = 1.58, gap = 0.14;
  const iconSize = 0.5;
  phenomena.forEach(([nature, name, desc, file], i) => {
    const col = i % cols, row = Math.floor(i / cols);
    const x = 0.7 + col * (gw + 0.3), y = 1.78 + row * (gh + gap);
    s.addShape("roundRect", { x, y, w: gw, h: gh, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.08 });
    s.addImage({ path: NATURE_ICON(file), x: x + 0.18, y: y + 0.16, w: iconSize, h: iconSize });
    s.addText(`${i + 1} · ${nature}`, { x: x + 0.18 + iconSize + 0.15, y: y + 0.12, w: gw - iconSize - 0.5, h: 0.24,
      fontFace: F_BODY, fontSize: 9, color: DIM, charSpacing: 1, isTextBox: true, margin: 0 });
    s.addText(name, { x: x + 0.18 + iconSize + 0.15, y: y + 0.34, w: gw - iconSize - 0.5, h: 0.34,
      fontFace: F_HEAD, bold: true, fontSize: 12.5, color: INK_DARK, isTextBox: true, margin: 0, valign: "top" });
    s.addText(desc, { x: x + 0.18, y: y + 0.76, w: gw - 0.36, h: gh - 0.84, fontFace: F_BODY,
      fontSize: 8.5, color: DIM, isTextBox: true, margin: 0, lineSpacingMultiple: 1.08, valign: "top" });
  });
  footer(s, false); pageNum(s, 5, false);
  s.addNotes("Companion to the previous slide: one real biological phenomenon per Nature, chosen for the closest structural match, not just the name. "
    + "Vitality = the G2/M checkpoint + p53-triggered apoptosis (change and record land together, or the cell is sacrificed — the convention's 'stale slime' verdict). "
    + "Symbiosis = lichen (and, for the 'merge' verdict, endosymbiosis: mitochondria were once free-living bacteria). "
    + "Evolution = CRISPR-Cas: the mistake literally mutates the genome, and the spacer is inherited by every daughter cell. "
    + "Genesis = quorum sensing, e.g. Vibrio fischeri lighting up only at high density in the bobtail squid's light organ — 'one naming is an observation, two is a pattern'. "
    + "Memory = hippocampal-neocortical consolidation; working memory itself holds only ~4 chunks (Cowan), the convention's '~5 thoughts'. "
    + "Swarm = Dictyostelium discoideum — a slime that forms a colony only on observed convergence, apt for a convention whose ground-truth creatures are Slimes. "
    + "Containment = the blood-brain barrier, a barrier that is structure rather than policy. "
    + "The Wire = von Frisch's waggle dance: 'point, don't carry' and 'fix the schema' in one behaviour. "
    + "Perception = proprioception; Ian Waterman's deafferentation case is the 'silent instrument is itself a finding' example. "
    + "Icons are Font Awesome glyphs built locally (make_icons.js), not photos — Nature 7 forbids fetching from the network without the human's agreement.");
}

// ================================================================== Slide 5 — World structure
{
  const s = pres.addSlide();
  slideTitle(s, "Every reincarnated world has the same shape");
  s.addShape("roundRect", { x: 0.7, y: 1.9, w: 5.7, h: 4.4, fill: { color: INK_DARK }, line: { type: "none" }, rectRadius: 0.1 });
  const tree = [
    ".isekai/",
    "├── isekai.md      the law every creature reads",
    "├── log.md         append-only change chronicle",
    "├── portraits/     one image per race",
    "├── instruments/   the world watching itself",
    "├── elf/orc/slime/ creatures live here",
    "└── tmp/           the proving grounds",
  ];
  s.addText(tree.map((l, i) => ({ text: l, options: { breakLine: i < tree.length - 1,
    color: i === 0 ? GOLD : INK_LIGHT, bold: i === 0 } })), {
    x: 1.0, y: 2.2, w: 5.1, h: 3.8, fontFace: "Courier New", fontSize: 13.5, isTextBox: true,
    margin: 0, valign: "top", lineSpacingMultiple: 1.35,
  });
  s.addText([
    { text: "isekai.md", options: { bold: true, color: CYAN, breakLine: false } },
    { text: " — the Nine Natures, Principles, Laws, and the gate. Every creature reads it before working; it changes only on the human's order.", options: { breakLine: true, paraSpaceAfter: 12 } },
    { text: "log.md", options: { bold: true, color: VIOLET, breakLine: false } },
    { text: " — one entry per change: task, files, gate verdict, result, what should be remembered. Never edited, only appended to.", options: { breakLine: true, paraSpaceAfter: 12 } },
    { text: "One command founds it: ", options: { breakLine: false } },
    { text: "/isekai", options: { fontFace: "Courier New", bold: true, color: GOLD, breakLine: false } },
    { text: " — and on a fresh reincarnation, it survey-populates the world in the same pass.", options: { breakLine: true } },
  ], { x: 6.8, y: 2.0, w: PW - 7.5, h: 4.1, fontFace: F_BODY, fontSize: 13, color: INK_DARK,
    isTextBox: true, margin: 0, valign: "top" });
  footer(s, false); pageNum(s, 6, false);
  s.addNotes("The .isekai/ skeleton. Same shape whether founded from OpenCode or Claude Code.");
}

// ================================================================== Slide 6 — Minds & Bodies
{
  const s = pres.addSlide();
  slideTitle(s, "Minds & Bodies — how creatures exist");
  const half = (PW - 1.4 - 0.5) / 2;
  // Minds
  s.addShape("roundRect", { x: 0.7, y: 1.95, w: half, h: 4.3, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.1 });
  s.addImage({ path: ICON("lightbulb"), x: 1.0, y: 2.25, w: 1.0, h: 1.0 });
  s.addText("Minds", { x: 2.15, y: 2.4, w: half - 1.5, h: 0.6, fontFace: F_HEAD, bold: true, fontSize: 22, color: INK_DARK, isTextBox: true, margin: 0 });
  s.addText([
    { text: "Reusable, stateless know-how. No memory of its own, does no work alone — a creature dons it for one task, then moves on.", options: { breakLine: true, paraSpaceAfter: 10 } },
    { text: "Native home: ", options: { breakLine: false } },
    { text: ".opencode/skills/<name>/SKILL.md", options: { fontFace: "Courier New", fontSize: 11.5, color: CYAN, breakLine: true } },
  ], { x: 1.0, y: 3.3, w: half - 0.6, h: 2.8, fontFace: F_BODY, fontSize: 13, color: DIM, isTextBox: true, margin: 0, valign: "top" });
  // Bodies
  const x2 = 0.7 + half + 0.5;
  s.addShape("roundRect", { x: x2, y: 1.95, w: half, h: 4.3, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.1 });
  s.addImage({ path: ICON("robot"), x: x2 + 0.3, y: 2.25, w: 1.0, h: 1.0 });
  s.addText("Bodies", { x: x2 + 1.45, y: 2.4, w: half - 1.5, h: 0.6, fontFace: F_HEAD, bold: true, fontSize: 22, color: INK_DARK, isTextBox: true, margin: 0 });
  s.addText([
    { text: "The vessel that actually runs and holds context. Never minted generic — born already named for the rank it serves: orc-security, slime-auth-zone.", options: { breakLine: true, paraSpaceAfter: 10 } },
    { text: "Native home: ", options: { breakLine: false } },
    { text: ".opencode/agents/*.md", options: { fontFace: "Courier New", fontSize: 11.5, color: VIOLET, breakLine: true } },
  ], { x: x2 + 0.3, y: 3.3, w: half - 0.6, h: 2.8, fontFace: F_BODY, fontSize: 13, color: DIM, isTextBox: true, margin: 0, valign: "top" });
  footer(s, false); pageNum(s, 7, false);
  s.addNotes("OpenCode is where both are natively authored — .opencode/ is the source-of-truth format the canon assumes.");
}

// ================================================================== Three memories, every creature
{
  const s = pres.addSlide();
  slideTitle(s, "Three memories — one shape for every creature");
  const tiers = [
    { name: "Short", color: CYAN, sub: "per body, per task — dies with the session",
      rows: [["context window", "the live context — measured, never felt"],
             ["working memory", "the ~5-thought desk on the worn Mind"],
             ["semantic cache", "recent recalls keyed by meaning; local, disposable"]] },
    { name: "Long", color: VIOLET, sub: "the world's — files are the truth, git is the record",
      rows: [["episodic", "what happened — log.md, one memory per entry"],
             ["procedural", "how to do — Minds, commands, tools"],
             ["semantic", "what is true — the law, creature docs, canon"]] },
    { name: "Shared", color: GOLD, sub: "across bodies, across worlds — append-only",
      rows: [["world", ".isekai/memory/shared — travels by git"],
             ["machine", "~/.isekai/shared — one Rimuru, every world"],
             ["the bus", "what must outlive a Court Body goes here first"]] },
  ];
  const colW = (PW - 1.4 - 0.8) / 3;
  tiers.forEach((t, i) => {
    const x = 0.7 + i * (colW + 0.4), y = 1.75;
    s.addShape("roundRect", { x, y, w: colW, h: 4.05, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.1 });
    s.addShape("rect", { x, y, w: colW, h: 0.09, fill: { color: t.color }, line: { type: "none" } });
    s.addText(t.name, { x: x + 0.3, y: y + 0.3, w: colW - 0.6, h: 0.5, fontFace: F_HEAD, bold: true, fontSize: 22, color: t.color, isTextBox: true, margin: 0 });
    s.addText(t.sub, { x: x + 0.3, y: y + 0.82, w: colW - 0.6, h: 0.5, fontFace: F_BODY, fontSize: 11, color: DIM, italic: true, isTextBox: true, margin: 0 });
    const runs = [];
    t.rows.forEach((r, j) => {
      runs.push({ text: r[0], options: { bold: true, color: INK_DARK, breakLine: true } });
      runs.push({ text: r[1], options: { color: DIM, breakLine: true, paraSpaceAfter: j < 2 ? 9 : 0 } });
    });
    s.addText(runs, { x: x + 0.3, y: y + 1.4, w: colW - 0.6, h: 2.5, fontFace: F_BODY, fontSize: 12, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.1 });
  });
  s.addText([
    { text: "Files are the truth; every index is derived and rebuildable. ", options: { bold: true, color: INK_DARK, breakLine: false } },
    { text: "memory.js", options: { fontFace: "Courier New", color: CYAN, breakLine: false } },
    { text: " indexes them, recalls by meaning then by relation (the same typed bonds the colony draws), and reports each tier as an instrument reading — a silent tier is a finding, not a gap to route around.", options: { color: DIM } },
  ], { x: 0.7, y: 5.95, w: PW - 1.4, h: 0.8, fontFace: F_BODY, fontSize: 12, isTextBox: true, margin: 0, valign: "top" });
  footer(s, false); pageNum(s, 0, false);
  s.addNotes("Nature 5 is the biology (hippocampal buffer, sleep consolidation, cleared traces); this is where each memory lives. Short → distilled → long; shared is the bus between bodies. The DB tier, when a world needs it, is the same boundary with a bigger engine — see canon/memory-tiers.md.");
}

// ================================================================== The unsaid (dark, pop)
{
  const s = pres.addSlide();
  s.background = { color: BG_DARK };
  s.addText("“The unsaid is your real knowledge.”", { x: 0.7, y: 0.75, w: PW - 1.4, h: 0.95, fontFace: F_HEAD, bold: true, fontSize: 32, color: INK_LIGHT, isTextBox: true, margin: 0 });
  s.addText("What a creature wrote down is the smaller part of what it knows. The larger part sits in the head that did the work — and a head in this world is a context that dies.",
    { x: 0.7, y: 1.7, w: PW - 1.4, h: 0.75, fontFace: F_BODY, fontSize: 14, color: DIM_LIGHT, isTextBox: true, margin: 0 });
  const kinds = [
    ["Law", "institutional knowledge", "The rules, definitions and decisions the isekai runs on.", "how data is modelled", "semantic long memory — the law, canon, creature docs", CYAN],
    ["Colony", "tribal knowledge", "What the colony knows but rarely writes down anywhere.", "how queries are executed", "the unwritten — desks, shared notes, what Court Bodies carry and lose", GOLD],
    ["Territory", "domain context", "What the numbers and entities actually mean in your territory.", "metadata", "the Slime's own doc — the zone's ground truth", VIOLET],
  ];
  const colW = (PW - 1.4 - 0.8) / 3;
  kinds.forEach((k, i) => {
    const x = 0.7 + i * (colW + 0.4), y = 2.7;
    s.addShape("roundRect", { x, y, w: colW, h: 3.0, fill: { color: APPENDIX_BG }, line: { color: k[5], width: 1 }, rectRadius: 0.1 });
    s.addText(k[0], { x: x + 0.3, y: y + 0.22, w: colW - 0.6, h: 0.45, fontFace: F_HEAD, bold: true, fontSize: 20, color: k[5], isTextBox: true, margin: 0 });
    s.addText(k[1], { x: x + 0.3, y: y + 0.66, w: colW - 0.6, h: 0.3, fontFace: F_BODY, fontSize: 10.5, color: DIM_LIGHT, italic: true, isTextBox: true, margin: 0 });
    s.addText([
      { text: k[2], options: { color: INK_LIGHT, breakLine: true, paraSpaceAfter: 6 } },
      { text: "Analogy: " + k[3], options: { color: DIM_LIGHT, italic: true, breakLine: true, paraSpaceAfter: 6 } },
      { text: "Home: " + k[4], options: { color: DIM_LIGHT, breakLine: true } },
    ], { x: x + 0.3, y: y + 1.05, w: colW - 0.6, h: 1.9, fontFace: F_BODY, fontSize: 11.5, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.1 });
  });
  s.addText([
    { text: "On the wire: ", options: { bold: true, color: INK_LIGHT, breakLine: false } },
    { text: "@U <kind> …", options: { fontFace: "Courier New", color: GOLD, breakLine: false } },
    { text: " — one piece of knowledge that was in the worker's head and nowhere on disk. A Court Body's report with no @U line either had nothing unsaid or failed its duty; the dispatcher may ask with ", options: { color: DIM_LIGHT, breakLine: false } },
    { text: "@ASK … +unsaid", options: { fontFace: "Courier New", color: GOLD, breakLine: false } },
    { text: ". Surface it before the context dies, before the gate verdict, before the distill wave.", options: { color: DIM_LIGHT } },
  ], { x: 0.7, y: 5.9, w: PW - 1.4, h: 0.9, fontFace: F_BODY, fontSize: 12, isTextBox: true, margin: 0, valign: "top" });
  footer(s, true); pageNum(s, 0, true);
  s.addNotes("The three kinds are the human's own (institutional / tribal / domain context), renamed into the world's vocabulary — Law, Colony, Territory — with the definitions kept exact. Colony is the kind the principle is really about: it is where the world's real knowledge leaks.");
}

// ================================================================== The loop
{
  const s = pres.addSlide();
  slideTitle(s, "The loop — six beats a Court Body runs on its own");
  const beats = [
    ["perceive", "read the instruments: steps left, wall clock, context window", CYAN],
    ["recall", "ask memory by the step's question; carry anchors, not payloads", CYAN],
    ["plan", "one step, one class: read · write · outward · destructive", VIOLET],
    ["act", "outward and destructive pass the human gate first — never auto-approved", GOLD],
    ["verify", "an instrument reading; a failed verify is a failed act", VIOLET],
    ["record", "journal the beat; land what was learned as a shared note", CYAN],
  ];
  const bw = (PW - 1.4 - 5 * 0.18) / 6;
  beats.forEach((b, i) => {
    const x = 0.7 + i * (bw + 0.18), y = 1.8;
    s.addShape("roundRect", { x, y, w: bw, h: 2.15, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.1 });
    numberBadge(s, { x: x + 0.2, y: y + 0.2, n: i + 1, color: b[2], size: 0.38 });
    s.addText(b[0], { x: x + 0.68, y: y + 0.18, w: bw - 0.8, h: 0.42, fontFace: F_HEAD, bold: true, fontSize: 15, color: INK_DARK, isTextBox: true, margin: 0, valign: "middle" });
    s.addText(b[1], { x: x + 0.2, y: y + 0.72, w: bw - 0.4, h: 1.35, fontFace: F_BODY, fontSize: 10.5, color: DIM, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.12 });
    if (i < 5) s.addText("›", { x: x + bw - 0.02, y: y + 0.85, w: 0.22, h: 0.4, fontFace: F_HEAD, fontSize: 18, color: DIM, align: "center", isTextBox: true, margin: 0 });
  });
  const cards = [
    ["Autonomy, bounded by readings", "Budgets — steps, minutes, context tokens — are read from instruments before every step, never felt. Past any of them the run checkpoints and stops honestly.", CYAN],
    ["Failure recovers, then escalates", "Every beat is journaled; a crashed or cleared run resumes from its journal. Retries are bounded; past them the run ends with @? to its dispatcher — never a guess, never forever.", VIOLET],
    ["One gate for side effects", "Outward (network, other repos, git push) and destructive (rm, reset) steps need a real answer: a TTY y/N, an explicit pre-approval, or a dry run that only says what it would ask. An interrupted outward act is gated again, not replayed.", GOLD],
  ];
  const cw = (PW - 1.4 - 0.8) / 3;
  cards.forEach((c, i) => {
    const x = 0.7 + i * (cw + 0.4), y = 4.25;
    s.addShape("rect", { x, y, w: 0.06, h: 2.0, fill: { color: c[2] }, line: { type: "none" } });
    s.addText(c[0], { x: x + 0.25, y, w: cw - 0.3, h: 0.4, fontFace: F_HEAD, bold: true, fontSize: 13.5, color: INK_DARK, isTextBox: true, margin: 0 });
    s.addText(c[1], { x: x + 0.25, y: y + 0.45, w: cw - 0.3, h: 1.55, fontFace: F_BODY, fontSize: 11, color: DIM, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.12 });
  });
  s.addText([
    { text: "Tool and protocol, one shape. ", options: { bold: true, color: INK_DARK, breakLine: false } },
    { text: "loop.js run <plan> --as <body>", options: { fontFace: "Courier New", color: CYAN, breakLine: false } },
    { text: " drives a scripted plan; an LLM-driven body follows the same six beats by hand, and there the gate is the host's own permission prompt — never worked around.", options: { color: DIM } },
  ], { x: 0.7, y: 6.35, w: PW - 1.4, h: 0.5, fontFace: F_BODY, fontSize: 11.5, isTextBox: true, margin: 0, valign: "top" });
  footer(s, false); pageNum(s, 0, false);
  s.addNotes("Where it fits: the dispatch seam. Rimuru dispatching a Court Body is a loop run; the Elf's routing is the plan file; the Orc's gate is the verify beat plus the human gate above it for anything outward or destructive.");
}

// ================================================================== The toolbox (dark, pop)
{
  const s = pres.addSlide();
  s.background = { color: BG_DARK };
  slideTitle(s, "The toolbox — a big registry, a small prompt", true, INK_LIGHT);
  s.addText("Only the tools that fit the turn are injected — as pointers with known costs, never as bodies. The registry may grow without bound; the prompt does not.",
    { x: 0.7, y: 1.45, w: PW - 1.4, h: 0.6, fontFace: F_BODY, fontSize: 13.5, color: DIM_LIGHT, isTextBox: true, margin: 0 });
  // three stages
  const stages = [
    ["Registry", "derived, never hand-kept", "Every Mind, command, world tool, Body and named external — harvested from the world, each entry priced before injection: resident cost (its one-line description) and full cost (its body).", CYAN],
    ["Level 1 — manifest", "resident, per turn, budgeted", "The turn's ask is ranked against the registry by trigger, meaning and relation; the picks fill a token budget (1,500 by default). What the body receives: names, one-liners, paths, and the cost of level 2 — the way a skill's frontmatter already sits in context.", GOLD],
    ["Level 2 — load", "on the body's own decision", "Only when the body decides to use a tool does it pull the full SKILL.md — whole, or one section by anchor. Every load is journaled, so status reports what was loaded against what was merely offered.", VIOLET],
  ];
  const cw = (PW - 1.4 - 0.8) / 3;
  stages.forEach((t, i) => {
    const x = 0.7 + i * (cw + 0.4), y = 2.25;
    s.addShape("roundRect", { x, y, w: cw, h: 3.05, fill: { color: APPENDIX_BG }, line: { color: t[3], width: 1 }, rectRadius: 0.1 });
    s.addText(t[0], { x: x + 0.3, y: y + 0.22, w: cw - 0.6, h: 0.45, fontFace: F_HEAD, bold: true, fontSize: 18, color: t[3], isTextBox: true, margin: 0 });
    s.addText(t[1], { x: x + 0.3, y: y + 0.66, w: cw - 0.6, h: 0.3, fontFace: F_BODY, fontSize: 10.5, color: DIM_LIGHT, italic: true, isTextBox: true, margin: 0 });
    s.addText(t[2], { x: x + 0.3, y: y + 1.05, w: cw - 0.6, h: 1.95, fontFace: F_BODY, fontSize: 11.5, color: INK_LIGHT, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.12 });
    if (i < 2) s.addText("›", { x: x + cw + 0.06, y: y + 1.2, w: 0.3, h: 0.5, fontFace: F_HEAD, fontSize: 26, color: DIM_LIGHT, align: "center", isTextBox: true, margin: 0 });
  });
  // wire sample
  s.addShape("roundRect", { x: 0.7, y: 5.55, w: PW - 1.4, h: 1.3, fill: { color: "0D1424" }, line: { color: "22304A", width: 1 }, rectRadius: 0.08 });
  s.addText([
    { text: "@TOOLS as=slime-auth k=2 cost=203/1500 — level 2 on decision only: toolbox.js load <name> [--sec N]", options: { color: GOLD, breakLine: true } },
    { text: "@T mind tdd — .opencode/skills/tdd/SKILL.md — 95tok (load≈842) — trigger “test”", options: { color: INK_LIGHT, breakLine: true } },
    { text: "@T mind code-review — .opencode/skills/code-review/SKILL.md — 108tok (load≈1648) — meaning 0.42", options: { color: INK_LIGHT } },
  ], { x: 0.95, y: 5.7, w: PW - 1.9, h: 1.05, fontFace: "Courier New", fontSize: 10.5, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.15 });
  footer(s, true); pageNum(s, 0, true);
  s.addNotes("This is the law's own sentence — 'loading is already two-tiered' — made an instrument and extended past Minds to commands, tools, Bodies and externals. In Claude Code level 2 is the Skill / ToolSearch by name; in OpenCode it is loading the skill; for a scripted Court Body it is toolbox.js load. The loop runs a pick before every step.");
}

// ================================================================== Slide 7 — Commands
{
  const s = pres.addSlide();
  slideTitle(s, "Four commands, native to OpenCode");
  const cmds = [
    ["/isekai", "Reincarnates a directory: writes the law, the chronicle, the portraits — then, on a fresh founding, survey-populates it in the same pass.", CYAN],
    ["/genesis", "Scans the code and decides, itself, which Elves, Orcs and Slimes it needs, then births them. Offer-first, never silent.", VIOLET],
    ["/don", "Brings a Mind across from another ecosystem's skill format into this one — a bridge, not a native OpenCode need.", GOLD],
    ["/mint", "Brings a Body across the same way — names it on the way in, never mints one unnamed.", "8A6BD6"],
  ];
  const colW = (PW - 1.4 - 0.5) / 2;
  cmds.forEach((c, i) => {
    const col = i % 2, row = Math.floor(i / 2);
    const x = 0.7 + col * (colW + 0.5), y = 1.95 + row * 2.15;
    s.addShape("roundRect", { x, y, w: colW, h: 1.9, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.1 });
    s.addText(c[0], { x: x + 0.3, y: y + 0.22, w: colW - 0.6, h: 0.5, fontFace: "Courier New", bold: true,
      fontSize: 19, color: c[2], isTextBox: true, margin: 0 });
    s.addText(c[1], { x: x + 0.3, y: y + 0.78, w: colW - 0.6, h: 1.0, fontFace: F_BODY, fontSize: 11.5,
      color: DIM, isTextBox: true, margin: 0, lineSpacingMultiple: 1.15 });
  });
  footer(s, false); pageNum(s, 8, false);
  s.addNotes("/isekai and /genesis run identically in OpenCode and Claude Code. /don and /mint only make sense from the Claude Code side, since .opencode/ is already the native format.");
}

// ================================================================== Slide 8 — Perception (dark, pop)
{
  const s = pres.addSlide();
  s.background = { color: BG_DARK };
  slideTitle(s, "Nature 9 — the world watches itself", true, INK_LIGHT);
  s.addImage({ path: ICON("eye"), x: PW/2 - 0.55, y: 1.75, w: 1.1, h: 1.1 });
  const stats = [
    ["1", "app for the whole machine"],
    ["∞", "worlds, each at its own /name/ sub-path"],
    ["1", "global dashboard, live"],
  ];
  const cw = (PW - 1.4) / 3;
  stats.forEach((st, i) => {
    const x = 0.7 + i * cw;
    s.addText(st[0], { x, y: 3.1, w: cw, h: 1.1, align: "center", fontFace: F_HEAD, bold: true,
      fontSize: 60, color: [CYAN, GOLD, VIOLET][i], isTextBox: true, margin: 0 });
    s.addText(st[1], { x: x + 0.3, y: 4.15, w: cw - 0.6, h: 0.7, align: "center", fontFace: F_BODY,
      fontSize: 12, color: DIM_LIGHT, isTextBox: true, margin: 0 });
  });
  s.addText("tempest.js — a zero-dependency instrument board. Reads every registered world fresh, per request, and reads real session usage from both OpenCode's own store and Claude Code's transcripts — nothing cached, nothing guessed.",
    { x: 1.6, y: 5.15, w: PW - 3.2, h: 0.85, align: "center", fontFace: F_BODY, fontSize: 12.5,
      color: INK_LIGHT, isTextBox: true, margin: 0 });

  // Field-tested, not just described: real values captured on this machine while building
  // this deck, 2026-09-20 (OpenCode actually installed and run; a real usage bug found and
  // fixed by testing, not assumed away).
  s.addShape("line", { x: 2.4, y: 6.05, w: PW - 4.8, h: 0, line: { color: "1a2436", width: 1 } });
  const field = [
    ["1.18.31", "OpenCode installed + run live here"],
    ["842M", "real input tokens correctly counted (was 0.0M — undercount found by testing)"],
    ["3", "real bugs found and fixed by an actual test script, not assumed away"],
  ];
  const fcw = (PW - 1.4) / 3;
  field.forEach((f, i) => {
    const x = 0.7 + i * fcw;
    s.addText(f[0], { x, y: 6.2, w: fcw, h: 0.45, align: "center", fontFace: F_HEAD, bold: true,
      fontSize: 20, color: [CYAN, GOLD, VIOLET][i], isTextBox: true, margin: 0 });
    s.addText(f[1], { x: x + 0.25, y: 6.62, w: fcw - 0.5, h: 0.55, align: "center", fontFace: F_BODY,
      fontSize: 9.5, color: DIM_LIGHT, isTextBox: true, margin: 0, lineSpacingMultiple: 1.05 });
  });
  footer(s, true); pageNum(s, 9, true);
  s.addNotes("Instruments are perception, not memory — what the world can check about itself right now, versus what a document claims. The bottom row is field-tested, not illustrative: OpenCode was actually installed and run on the machine this deck was built on, the 842M figure is the real, corrected total after fixing a Claude Code input-token undercount, and the 3 bugs (Claude cache-token undercount, a silent node:sqlite failure, and an OpenCode cache-token undercount) were each caught by an actual deterministic test script (.isekai/tools/test-opencode-integration.js), not by inspection.");
}

// ================================================================== Slide 9 — Models
{
  const s = pres.addSlide();
  slideTitle(s, "One throne, several mounts — model per rank");
  const rows = [
    ["Rank / office", "Suggested model", "Why"],
    ["Great Sage", "Haiku 4.5", "Bulk retrieval, terse facts — no judgment, no writes."],
    ["Raphael", "Sonnet 5", "Real reasoning for a verdict — Opus for high-stakes domains."],
    ["Ciel", "Sonnet 5 · Opus 5 · Fable 5.1", "Default prose; Opus for heavy synthesis; Fable when voice is the point."],
    ["Elf", "Sonnet 5 · Opus 5 · Fable 5.1", "Coordination default; Opus for conflict; Fable for the outward voice itself."],
    ["Orc", "Sonnet 5", "Reasoning-critical gate work — Opus for high-stakes domains."],
    ["Slime", "Sonnet 5", "Authors real changes — narrow scope, not low judgment. Haiku only for mechanical zones."],
    ["Ascended ranks", "Opus 5", "Cross-session judgment over an archive, or the last systemic check — task-driven, not just rarity."],
  ];
  const colW = [3.1, 2.6, PW - 1.4 - 3.1 - 2.6];
  s.addTable(rows.map((r, ri) => r.map((c, ci) => ({
    text: c,
    options: {
      bold: ri === 0 || ci === 0, fontFace: F_BODY, fontSize: ri === 0 ? 12 : 11.5,
      color: ri === 0 ? "FFFFFF" : (ci === 1 ? CYAN : INK_DARK),
      fill: { color: ri === 0 ? INK_DARK : (ri % 2 === 0 ? "FFFFFF" : "F0F3F9") },
      align: ci === 0 ? "left" : "left", valign: "middle",
    },
  }))), { x: 0.7, y: 1.9, w: PW - 1.4, colW, rowH: 0.56, border: { type: "solid", color: "E3E8F0", pt: 0.5 } });
  footer(s, false); pageNum(s, 10, false);
  s.addNotes("Rimuru is deliberately excluded — the throne's mount is the human's choice, never hardcoded. Tuned by what each task actually demands, not by rank prestige: narrow scope (Slime) is not the same as low judgment, since a Slime authors real changes — retuned from Haiku to Sonnet 5 default accordingly. Ciel was the only office with no Opus escalation path at all; added one for heavy-synthesis pieces (a session report weaving many threads), kept separate from the Fable axis (voice-consistency specifically). This table is reference guidance today, not yet wired into /mint's actual minting flow.");
}

// ================================================================== Slide 10 — Advantages
{
  const s = pres.addSlide();
  slideTitle(s, "Why run a codebase this way");
  const advs = [
    ["Memory survives the session", "The next session reads what the last one wrote — no re-deriving the same facts twice."],
    ["Nothing lands without a gate", "An Orc verifies the right Slime authored it, traits hold, the doc is truthful — before it counts as done."],
    ["Observability is built in, not bolted on", "One instrument board, live, across every world on the machine — not a dashboard you have to remember to check."],
    ["Harness-agnostic by design", "Minds and Bodies cross from OpenCode into Claude Code and back — one convention, any coding harness, not a plugin bound to one vendor."],
  ];
  advs.forEach((a, i) => {
    const y = 1.95 + i * 1.15;
    numberBadge(s, { x: 0.7, y: y + 0.05, n: i + 1, color: [CYAN, VIOLET, GOLD, "8A6BD6"][i], size: 0.5 });
    s.addText(a[0], { x: 1.45, y: y - 0.05, w: PW - 2.2, h: 0.4, fontFace: F_HEAD, bold: true,
      fontSize: 15, color: INK_DARK, isTextBox: true, margin: 0 });
    s.addText(a[1], { x: 1.45, y: y + 0.38, w: PW - 2.2, h: 0.6, fontFace: F_BODY, fontSize: 12,
      color: DIM, isTextBox: true, margin: 0 });
  });
  footer(s, false); pageNum(s, 11, false);
  s.addNotes("The pitch slide. Four advantages, each tied to a Nature: Memory, The gate, Perception, and cross-ecosystem Minds & Bodies.");
}

// ================================================================== Slide 11 — Flow sequence
{
  const s = pres.addSlide();
  slideTitle(s, "One fix, start to finish");
  s.addText([
    { text: "Not hypothetical — this is how ", options: { breakLine: false } },
    { text: "slime-db-role", options: { fontFace: "Courier New", color: CYAN, breakLine: false } },
    { text: "'s ", options: { breakLine: false } },
    { text: "Update", options: { fontFace: "Courier New", color: CYAN, breakLine: false } },
    { text: " path — a raw collection delete where every sibling used the proper ", options: { breakLine: false } },
    { text: "dropRole", options: { fontFace: "Courier New", color: CYAN, breakLine: false } },
    { text: " command — actually got fixed, found by ", options: { breakLine: false } },
    { text: "/genesis --depth deep", options: { fontFace: "Courier New", color: GOLD, breakLine: false } },
    { text: "'s git archaeology.", options: { breakLine: true } },
  ], { x: 0.7, y: 1.25, w: PW - 1.4, h: 0.55, fontFace: F_BODY, fontSize: 11.5, color: DIM, isTextBox: true, margin: 0 });

  const steps = [
    ["Rimuru", "Confirms scope with the human, routes the fix down.", "3A4250"],
    ["Elf", "One domain, nothing to arbitrate — routes straight to the Orc.", GOLD],
    ["Orc", "Dispatches Great Sage for recon (read-wide, terse facts: every raw-delete / dropRole site), then the owning Slime.", VIOLET],
    ["Slime", "Reads its own territory, mirrors dropRole the way Delete and the sibling resource already do — updates its own doc's Traits in the same change.", CYAN],
    ["Orc", "Dispatches Raphael for the gate verdict: file:line evidence, right Slime authored, traits hold, doc truthful.", VIOLET],
    ["Orc", "Raphael PASSes with evidence — lands the change, records the verdict.", VIOLET],
    ["Ciel + Elf", "Ciel journals it in log.md — Sonnet 5, its own floor: turning facts into house-format prose is voice work, never below Raphael's tier. Elf reports back to the human, one line, full courtesy.", GOLD],
  ];
  const top = 2.05, rowH = 0.685, badgeSize = 0.4, lineX = 0.7 + badgeSize / 2;
  steps.forEach((st, i) => {
    const y = top + i * rowH;
    if (i < steps.length - 1) {
      s.addShape("line", { x: lineX, y: y + badgeSize, w: 0, h: rowH - badgeSize, line: { color: "D7DEE9", width: 1.5 } });
    }
    numberBadge(s, { x: 0.7, y, n: i + 1, color: st[2], size: badgeSize });
    s.addText(st[0], { x: 1.35, y: y - 0.06, w: 1.55, h: 0.5, fontFace: F_HEAD, bold: true,
      fontSize: 13, color: INK_DARK, isTextBox: true, margin: 0, valign: "top" });
    s.addText(st[1], { x: 2.85, y: y - 0.07, w: PW - 3.55, h: rowH, fontFace: F_BODY, fontSize: 10.8,
      color: DIM, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.08 });
  });
  footer(s, false); pageNum(s, 12, false);
  s.addNotes("The full sequence, grounded in a real fix from this session rather than a hypothetical. Context flows down (Rimuru -> Elf -> Orc -> Slime); the gate (Orc + Raphael) is what stands between a Slime's change and it landing. Ciel's journaling step runs on Sonnet 5, its own floor -- the deterministic-script principle still applies, but only to inputs mechanical enough to bypass the Ciel office entirely (a raw field append with no prose); the moment Ciel itself is actually dispatched, its tier never drops below Raphael's, per the canon's own Great Sage -> Raphael -> Ciel evolutionary ordering.");
}

// ================================================================== Slide 12 — Escalation, when a rank hits its ceiling
{
  const s = pres.addSlide();
  slideTitle(s, "When a rank hits its ceiling: escalate, never guess");
  s.addText([
    { text: "Absolute Rule III — ", options: { bold: true, breakLine: false, color: INK_DARK } },
    { text: "Confirmation and Escalation", options: { fontFace: "Courier New", color: GOLD, breakLine: false } },
    { text: ". Escalation moves by rank, one hop at a time — never skipping a level, never a handoff of the work itself, and it never guesses past confusion.", options: { breakLine: true } },
  ], { x: 0.7, y: 1.25, w: PW - 1.4, h: 0.55, fontFace: F_BODY, fontSize: 11.5, color: DIM, isTextBox: true, margin: 0 });

  const steps = [
    ["Slime", "Hits something it can't resolve at its own level: a request it can't parse, a doc that contradicts the code, an instrument reading it can't explain. Asks its immediate parent — never further up, never sideways.", CYAN],
    ["Orc", "If the Orc is also stuck, it escalates again in turn — the same question, one hop further, not a rewrite of the problem.", VIOLET],
    ["Elf", "Genuine cross-domain judgment lands here — per the model table, this is where Elf steps up to Opus 5 for real arbitration, not routine coordination.", GOLD],
    ["Rimuru", "If even the Elf can't resolve it, Rimuru is asked to bring it to Veldora directly — the one siphon that never narrows, in full human courtesy.", "3A4250"],
    ["Veldora (the human)", "Confirms or clarifies. Rimuru carries the resolved answer back down the same chain it came up.", GOLD],
  ];
  const top = 2.1, rowH = 0.85, badgeSize = 0.42, lineX = 0.7 + badgeSize / 2;
  steps.forEach((st, i) => {
    const y = top + i * rowH;
    if (i < steps.length - 1) {
      s.addShape("line", { x: lineX, y: y + badgeSize, w: 0, h: rowH - badgeSize, line: { color: "D7DEE9", width: 1.5 } });
    }
    numberBadge(s, { x: 0.7, y, n: i + 1, color: st[2], size: badgeSize });
    s.addText(st[0], { x: 1.4, y: y - 0.06, w: 1.9, h: 0.5, fontFace: F_HEAD, bold: true,
      fontSize: 13, color: INK_DARK, isTextBox: true, margin: 0, valign: "top" });
    s.addText(st[1], { x: 3.3, y: y - 0.08, w: PW - 4.0, h: rowH, fontFace: F_BODY, fontSize: 10.5,
      color: DIM, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.1 });
  });
  s.addText([
    { text: "Note: ", options: { bold: true, breakLine: false, color: INK_DARK } },
    { text: "escalation is about rank, not model — Fable 5.1 is only the Elf/Ciel office's voice-drafting model choice (model table), not an escalation step of its own.", options: { italic: true, breakLine: true } },
  ], { x: 0.7, y: 6.42, w: PW - 1.4, h: 0.45, fontFace: F_BODY, fontSize: 10, color: DIM,
    isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.15 });
  footer(s, false); pageNum(s, 13, false);
  s.addNotes("Direct answer to: can a Slime escalate past its own ceiling? Yes -- Absolute Rule III, named 'Confirmation and Escalation'. Verified this section is actually present and correctly readable by testing it live: asked a free, non-Claude OpenCode model (nemotron-3.5-lightning-free) to read isekai.md's Rule III cold and explain the escalation chain -- it correctly answered 'asks its immediate parent, never skipping a rank' without prompting, confirming the doc itself is the mechanism, not something Claude-specific.");
}

// ================================================================== Slide 13 — Does it actually help? We measured it.
{
  const s = pres.addSlide();
  slideTitle(s, "Does it actually help? We measured it.");
  s.addText("6 isolated Claude Code sessions (Sonnet 5) fixed the same real, documented bug in terraform-provider-mongodb -- 3 with .isekai/ present, 3 with it stripped entirely. Not a pitch: a controlled comparison, graded against objective pass/fail criteria.",
    { x: 0.7, y: 1.35, w: PW - 1.4, h: 0.7, fontFace: F_BODY, fontSize: 12.5, color: DIM,
      isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.15 });

  const stats = [
    { n: "3 / 3", label: "adopted the codebase's own canonical fix", color: CYAN,
      detail: "vs. 0/3 without -- which all settled for a narrower, incomplete patch despite the correct pattern sitting 20 lines away in the same file." },
    { n: "~3x", label: "higher cost, not lower", color: GOLD,
      detail: "$0.49 avg vs $0.17 avg per trial. The overhead is real: turns spent reading world docs, some irrelevant to the specific task." },
    { n: "3.35x", label: "gap held across a 2nd chained task", color: VIOLET,
      detail: "Ran a second real task in the same sessions (claude -c). Caching cut everyone's marginal cost vs. a cold start -- but the with/without ratio didn't shrink." },
  ];
  const cw = (PW - 1.4 - 1.0) / 3;
  stats.forEach((st, i) => {
    const x = 0.7 + i * (cw + 0.5), y = 2.35;
    s.addShape("roundRect", { x, y, w: cw, h: 3.15, fill: { color: CARD_LIGHT }, line: { color: "E3E8F0", width: 1 }, rectRadius: 0.1,
      shadow: { type: "outer", color: "1B2333", opacity: 0.1, blur: 8, offset: 3, angle: 90 } });
    s.addText(st.n, { x: x + 0.25, y: y + 0.25, w: cw - 0.5, h: 0.7, fontFace: F_HEAD, bold: true,
      fontSize: 30, color: st.color, isTextBox: true, margin: 0 });
    s.addText(st.label, { x: x + 0.25, y: y + 0.95, w: cw - 0.5, h: 0.6, fontFace: F_HEAD, bold: true,
      fontSize: 12.5, color: INK_DARK, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.1 });
    s.addText(st.detail, { x: x + 0.25, y: y + 1.6, w: cw - 0.5, h: 1.45, fontFace: F_BODY,
      fontSize: 10, color: DIM, isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.15 });
  });

  s.addText([
    { text: "Verdict: ", options: { bold: true, breakLine: false, color: INK_DARK } },
    { text: "better decisions, not cheaper ones -- at least not within one small session on an 862-line codebase. The savings case this convention actually makes (skipping expensive cold exploration) needs a larger codebase or separate sessions over time to show up; this test doesn't have either, and says so.", options: { breakLine: false, italic: true } },
  ], { x: 0.7, y: 5.75, w: PW - 1.4, h: 0.9, fontFace: F_BODY, fontSize: 11.5, color: DIM,
    isTextBox: true, margin: 0, valign: "top", lineSpacingMultiple: 1.2 });

  footer(s, false); pageNum(s, 14, false);
  s.addNotes("Real, honest empirical test -- logged in .isekai/log.md 2026-09-20. Deliberately keeps the negative/mixed finding (cost went up, not down) rather than only reporting the correctness win, per this convention's own Nature 9 (Perception: compute it, don't eyeball it) and Nature 3 (Evolution: a finding that complicates the pitch still gets recorded, not filtered out).");
}

// ================================================================== Slide 13 — Appendix: Claude Code
{
  const s = pres.addSlide();
  s.background = { color: APPENDIX_BG };
  s.addText("APPENDIX", { x: 0.7, y: 0.5, w: 6, h: 0.35, fontFace: F_BODY, fontSize: 11,
    color: GOLD, charSpacing: 3, bold: true, isTextBox: true, margin: 0 });
  slideTitle(s, "A note on Claude Code compatibility", true, INK_LIGHT);
  s.addImage({ path: ICON("link"), x: 0.7, y: 1.95, w: 1.3, h: 1.3 });
  s.addText([
    { text: "Isekai was not designed OpenCode-only — ", options: { breakLine: false } },
    { text: "isekai.md", options: { fontFace: "Courier New", color: GOLD, breakLine: false } },
    { text: " itself already treats ", options: { breakLine: false } },
    { text: ".opencode/", options: { fontFace: "Courier New", color: CYAN, breakLine: false } },
    { text: " and ", options: { breakLine: false } },
    { text: ".claude/", options: { fontFace: "Courier New", color: CYAN, breakLine: false } },
    { text: " as two coexisting homes for the same Minds and Bodies.", options: { breakLine: true } },
  ], { x: 2.35, y: 2.0, w: PW - 3.1, h: 1.2, fontFace: F_BODY, fontSize: 14, color: INK_LIGHT,
    isTextBox: true, margin: 0, valign: "top" });
  const points = [
    ["/don and /mint bridge the gap", "Claude-Code-only commands that port a Mind or Body from .opencode/ into .claude/skills/ or .claude/agents/ — a straight copy, since both ecosystems speak the same open Agent Skills format."],
    ["/isekai and /genesis run identically in both", "Same world document, same population logic, either side — a world founded from OpenCode looks no different from one founded in Claude Code."],
    ["A working proof, not just a claim", "palette-audit, a Mind built during this convention's own development, ships byte-identical in both .opencode/skills/ and .claude/skills/ right now."],
    ["tempest.js reads both ecosystems' usage", "The instrument board reports real session activity from OpenCode's own store and Claude Code's own transcripts, side by side, on the same dashboard."],
  ];
  points.forEach((p, i) => {
    const col = i % 2, row = Math.floor(i / 2);
    const cw = (PW - 1.4 - 0.5) / 2;
    const x = 0.7 + col * (cw + 0.5), y = 3.55 + row * 1.75;
    s.addText(p[0], { x, y, w: cw, h: 0.45, fontFace: F_HEAD, bold: true, fontSize: 13.5,
      color: CYAN, isTextBox: true, margin: 0 });
    s.addText(p[1], { x, y: y + 0.42, w: cw, h: 1.2, fontFace: F_BODY, fontSize: 11,
      color: DIM_LIGHT, isTextBox: true, margin: 0, lineSpacingMultiple: 1.15 });
  });
  footer(s, true); pageNum(s, 15, true);
  s.addNotes("Appendix, distinctly styled (slate background) to mark it as supplementary to the OpenCode-perspective main deck. The advantage: adopting Isekai isn't a bet on one ecosystem.");
}

// ================================================================== Slide 13 — Closing
{
  const s = pres.addSlide();
  s.background = { color: BG_DARK };
  s.addImage({ path: ICON("rocket"), x: PW/2 - 0.6, y: 1.15, w: 1.2, h: 1.2 });
  s.addText("Reincarnate any directory. One command.", { x: 0, y: 2.75, w: PW, h: 0.6,
    align: "center", fontFace: F_BODY, fontSize: 16, color: DIM_LIGHT, isTextBox: true, margin: 0 });
  s.addText("/isekai", { x: 0, y: 3.35, w: PW, h: 1.2, align: "center", fontFace: "Courier New",
    bold: true, fontSize: 56, color: GOLD, isTextBox: true, margin: 0 });
  s.addText("The world remembers in documents, because sessions forget.\nIt watches itself through instruments, because memory alone cannot see stress.",
    { x: 1.8, y: 4.7, w: PW - 3.6, h: 0.9, align: "center", fontFace: F_BODY, italic: true,
      fontSize: 12.5, color: INK_LIGHT, isTextBox: true, margin: 0, lineSpacingMultiple: 1.2 });
  footer(s, true); pageNum(s, 16, true);
  s.addNotes("Closing slide. Quote lifted directly from isekai.md's own opening.");
}

pres.writeFile({ fileName: path.join(__dirname, "..", "isekai-opencode-overview.pptx") })
  .then(() => console.log("written"))
  .catch(e => { console.error(e); process.exit(1); });
