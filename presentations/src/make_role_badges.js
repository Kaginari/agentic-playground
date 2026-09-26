// Role badges for agent-one: the same white rounded tile as the isekai portraits, with a Tabler
// icon (MIT) per role, tinted with the role's colour from the board palette. Principal roles
// carry a crown badge. Output: 512×512 PNGs, one per agent-one rank name.
//   node make_role_badges.js icons/tabler ../../agent-one/portraits   (renders with headless Firefox)
const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const [iconDir, outDir] = process.argv.slice(2);
if (!iconDir || !outDir) {
  console.error("usage: node make_role_badges.js <tabler-icons-dir> <out-dir>");
  process.exit(2);
}

const ROLES = [
  // file name (agent-one rank)  icon              colour     principal
  ["operator",               "user-shield",     "#3a4a63", false],
  ["orchestrator",           "topology-star-3", "#1f6f8b", false],
  ["coordinator",            "route",           "#bd8c24", false],
  ["principal-coordinator",  "route",           "#9a6f12", true],
  ["domain-owner",           "shield-check",    "#7c5cd6", false],
  ["principal-domain-owner", "shield-check",    "#8a7300", true],
  ["zone-worker",            "code",            "#2695bd", false],
  ["service-owner",          "server-cog",      "#c53d34", false],
  ["auditor",                "file-search",     "#d6402a", false],
];

// The inner paths of a Tabler outline icon (24×24 viewBox), minus its invisible bounding path.
function iconPaths(name) {
  const svg = fs.readFileSync(path.join(iconDir, `${name}.svg`), "utf8");
  return (svg.match(/<path[^>]*\/>/g) || []).filter((p) => !p.includes('stroke="none"')).join("");
}

function badge(icon, colour, principal) {
  const crown = principal
    ? `<g transform="translate(352 72)">
         <circle cx="44" cy="44" r="46" fill="${colour}" stroke="#ffffff" stroke-width="8"/>
         <g transform="translate(18 18) scale(2.2)" fill="none" stroke="#ffffff" stroke-width="2"
            stroke-linecap="round" stroke-linejoin="round">${iconPaths("crown")}</g>
       </g>`
    : "";
  return `<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">
  <defs>
    <filter id="sh" x="-10%" y="-10%" width="120%" height="125%">
      <feDropShadow dx="0" dy="6" stdDeviation="8" flood-color="#0b1220" flood-opacity="0.22"/>
    </filter>
    <linearGradient id="tile" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#ffffff"/><stop offset="1" stop-color="#eef2f7"/>
    </linearGradient>
  </defs>
  <rect width="512" height="512" fill="#ffffff"/>
  <rect x="24" y="20" width="464" height="464" rx="64" fill="url(#tile)" filter="url(#sh)"/>
  <circle cx="256" cy="252" r="172" fill="${colour}" fill-opacity="0.12"/>
  <circle cx="256" cy="252" r="172" fill="none" stroke="${colour}" stroke-opacity="0.35" stroke-width="4"/>
  <g transform="translate(112 108) scale(12)" fill="none" stroke="${colour}" stroke-width="1.5"
     stroke-linecap="round" stroke-linejoin="round">${iconPaths(icon)}</g>
  ${crown}
</svg>`;
}

(() => {
  fs.mkdirSync(outDir, { recursive: true });
  for (const [name, icon, colour, principal] of ROLES) {
    const out = path.join(outDir, `${name}.png`);
    const svgPath = out.replace(/\.png$/, ".svg");
    fs.writeFileSync(svgPath, badge(icon, colour, principal));
    // Firefox renders the drop-shadow filter; ImageMagick's internal SVG renderer does not.
    execFileSync("firefox", ["--headless", "--window-size=512,512", "--screenshot", out, "file://" + path.resolve(svgPath)],
      { stdio: "ignore", env: { ...process.env, MOZ_HEADLESS: "1" } });
    fs.unlinkSync(svgPath);
    console.log(out);
  }
})();
