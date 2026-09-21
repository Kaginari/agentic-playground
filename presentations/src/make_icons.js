const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");

const FA = path.join(__dirname, "node_modules/@fortawesome/fontawesome-free/svgs/solid");
const OUT = path.join(__dirname, "icons");

const icons = [
  { name: "lightbulb", fa: "lightbulb", color: "bd8c24" },
  { name: "robot", fa: "robot", color: "2695bd" },
  { name: "eye", fa: "eye", color: "2695bd" },
  { name: "link", fa: "link", color: "7c5cd6" },
  { name: "rocket", fa: "rocket", color: "bd8c24" },
];

// One glyph per Nature law's real-world phenomenon (slide "as nature already does
// them" + README). Colors cycle cyan/violet/gold to match the numbered badges on
// the Nine Natures slide. Also copied to <world>/.isekai/natures/ for the README.
const NATURE_CYCLE = ["2695bd", "7c5cd6", "bd8c24"];
const natures = [
  { name: "01-vitality-dna-checkpoint",   fa: "dna" },
  { name: "02-symbiosis-lichen",          fa: "leaf" },
  { name: "03-evolution-crispr",          fa: "virus" },
  { name: "04-genesis-quorum-sensing",    fa: "bacteria" },
  { name: "05-memory-sleep-consolidation", fa: "brain" },
  { name: "06-swarm-slime-mold",          fa: "disease" },
  { name: "07-containment-blood-brain-barrier", fa: "shield-halved" },
  { name: "08-the-wire-waggle-dance",     fa: "bugs" },
  { name: "09-perception-proprioception", fa: "person-walking" },
].map((n, i) => ({ ...n, color: NATURE_CYCLE[i % 3], dir: "natures" }));

const NATURES_REPO = path.join(__dirname, "..", "..", ".isekai", "natures");
fs.mkdirSync(path.join(OUT, "natures"), { recursive: true });
fs.mkdirSync(NATURES_REPO, { recursive: true });

for (const { name, fa, color, dir } of [...icons, ...natures]) {
  const src = fs.readFileSync(path.join(FA, `${fa}.svg`), "utf8");
  const viewBox = (src.match(/viewBox="([^"]+)"/) || [])[1];
  const pathD = (src.match(/<path[^>]*\sd="([^"]+)"/) || [])[1];
  if (!viewBox || !pathD) throw new Error(`could not parse ${fa}.svg`);
  const [, , vw, vh] = viewBox.split(/\s+/).map(Number);
  // Fit the icon into a 128x128 box centered in the 240x240 canvas, preserving
  // its native aspect ratio (Font Awesome glyphs are not all square).
  const box = 128, cx = 120, cy = 120;
  const scale = Math.min(box / vw, box / vh);
  const iw = vw * scale, ih = vh * scale;
  const ix = cx - iw / 2, iy = cy - ih / 2;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="240" height="240" viewBox="0 0 240 240">
<circle cx="120" cy="120" r="112" fill="#${color}"/>
<g transform="translate(${ix.toFixed(2)},${iy.toFixed(2)}) scale(${scale.toFixed(4)})">
<path d="${pathD}" fill="#ffffff"/>
</g>
</svg>`;
  const outDir = dir ? path.join(OUT, dir) : OUT;
  const svgPath = path.join(outDir, `${name}.svg`);
  fs.writeFileSync(svgPath, svg);
  // -type TrueColorAlpha + PNG color-type 6: forces full RGBA output instead of a
  // quantized palette, which was dithering a thin dark ring at the circle's edge.
  const pngPath = path.join(outDir, `${name}.png`);
  execSync(`convert -background none -density 300 "${svgPath}" -type TrueColorAlpha -define png:color-type=6 "${pngPath}"`);
  if (dir === "natures") fs.copyFileSync(pngPath, path.join(NATURES_REPO, `${name}.png`));
  console.log(`built ${name}.png from FA "${fa}" (viewBox ${viewBox})`);
}
