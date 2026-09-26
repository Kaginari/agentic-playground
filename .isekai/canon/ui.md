# UI — how the world draws its pages

Every page the world serves or ships — the board, a report, a creature's dashboard — follows one
design system, so a reader who learned one page can read them all.

## Layout: the Bootstrap grid

- **One layout system: Bootstrap 5.** Pages use its container → row → column grid, 12 columns,
  and its breakpoints (`sm` 576 · `md` 768 · `lg` 992 · `xl` 1200 · `xxl` 1400). A panel says how
  wide it is at each breakpoint (`col-12 col-md-6 col-xl-4`); nothing is sized in fixed pixels
  wider than a phone.
- **Mobile first.** The base class is the phone layout; wider breakpoints only add columns. A
  page never scrolls sideways at 360px.
- **Components before custom markup.** Navbar, nav tabs, cards, tables (`table-responsive`),
  badges, progress bars, list groups, modals, offcanvas — Bootstrap's, restyled by the theme,
  never rebuilt.
- **Pages, not one endless scroll.** A page shows one question well (what is running, what did it
  cost, who owns this path); navigation moves between questions.

## Theme: isekai over Bootstrap

- The theme is CSS custom properties layered over Bootstrap's (`--bs-*`), one token per rank and
  mind lane (`--r-slime`, `--r-orc`, `--r-elf`, …, `--l-zone`, `--l-verdict`, `--l-global`,
  `--l-shared`), in a dark and a light variant, both checked with the `palette-audit` Mind
  (contrast, CVD separation) — never the same hex reused for both.
- `data-bs-theme="dark|light"` switches the whole page; the choice follows the system unless the
  reader picks one.
- Motion respects `prefers-reduced-motion`.

## Data on the page

- Charts and graphs are inline SVG sized by their grid column (`viewBox` + `width:100%`), redrawn
  on resize — never a fixed-width canvas. The `dataviz` Mind governs chart form and colour.
- Numbers are instruments: every figure on a page says where it came from (file, time), and a
  silent or stale instrument is shown as such, never as zero.
- Assets are embedded or served locally; a page works offline.
