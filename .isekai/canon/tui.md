# The terminal UI — Claude Code style

The live session is an inline terminal UI at the level of Claude Code: the conversation flows into
the terminal's own scrollback, a bordered input sits at the bottom, and every event of the loop is
drawn as a readable block, not a raw line. Built on the Charm libraries, v2 (`charm.land/…`: Bubble Tea, Bubbles, Lip Gloss, Glamour) — the
one place the binary takes third-party code; versions pinned in go.sum.

## Layout

- **Inline, not full-screen.** Finished blocks are printed above the live area and stay in the
  normal scrollback (copyable, searchable, survive exit); only the bottom region is live: the
  spinner line, the input box, the footer.
- **A width change reprints.** A terminal rewraps the live area's full-width lines when it
  narrows, and the renderer, counting the rows it drew, then leaves copies behind. So every
  finished block is kept as a render at a width; once a drag settles (~120 ms) the screen and the
  scrollback are cleared and the transcript is printed again at the new width — Claude Code's
  answer too. A height-only change reprints nothing; `ctrl+l` reprints on demand; `/clear` empties
  the transcript. The welcome waits for the first size, so it is never drawn at a guessed width.
- **The intro** — on a real terminal the mascot (isekai's slime, agent-one's agent: half-block
  sprites, `tui/mascot.go`) drops in on a Harmonica spring, squashes as it lands, blinks, and the
  welcome box then prints around the very place it landed. About a second; `<PREFIX>NO_INTRO=1`
  skips it; tests never run it.
- **Colour in motion** — the title on the mascot's gradient; the spinner's verb shimmers (a soft
  highlight sweeps across it with each spinner frame); the footer's context is a meter, each cell
  coloured along green → amber → red.
- **Welcome** — one framed box at start: the world, the model (and each office's), the board URL,
  the off-list count, "/help for commands".
- **Your message** — a `>`-prefixed block, dim, as sent.
- **Assistant text** — streamed, rendered as markdown (Glamour, a theme that follows the terminal's
  dark/light background), code blocks highlighted.
- **Tool calls** — one compact line each while running and after: `● bash  go test ./...` with the
  class as a coloured tag (read · write · outward · destructive), then a status line under it
  (`⎿ ok · 1.2s · 34 lines` or the error). Output is collapsed to its first lines with
  `… +N lines (ctrl+o to expand)`; `write`/`edit`/`multiedit`/`patch` show a coloured diff
  (± lines, line numbers, two lines of context, a gap mark between hunks), collapsed past ~20
  lines. The diff is the file before and after the act, so a shell-made change to the same path
  is not shown — only the path tools diff. The blocks are drawn from the loop's `Observe` seam
  (a step at "start" once its class is settled, at "end" once its record is final); `dispatch`
  and `ask` draw no tool block — the Court block and the question block are theirs. A Court's
  own tool steps are not drawn; its block's state says `tool`.
- **Courts** — a dispatched Court is one block with its rank, office and model, a live state, and
  on return its report rendered (status, findings, holes, the unsaid) — not the raw wire text.
- **The gate verdict** — one quiet line after a turn that wrote: `✓ gate · n/a (no orcs) · log.md`.
- **Spinner line** — while a turn runs: a spinner, a verb for the current beat (`Thinking…`,
  `Running bash…`, `Waiting for approval…`), elapsed time, tokens so far, `esc to interrupt`.
- **Footer** — under the input: the model · context % · cost (or "no calls yet") · live courts ·
  the world dir; `? for shortcuts`.

## Interaction

- **Input** — multiline (`shift+enter` where the terminal reports it apart from `enter` — Bubble
  Tea v2 asks for key disambiguation — and everywhere `\` + enter, `alt+enter`, `ctrl+j`), history
  with ↑/↓ on the first/last line, a paste of three lines or more collapsed to
  `[pasted N lines]` and restored on send, `@path` completes files in the world (tab).
- **Slash commands** — typing `/` opens an inline menu of built-in and discovered commands with
  their descriptions, filtered as you type; tab completes, enter runs. `/help` prints the menu
  and the shortcuts; `/quit` ends the session.
- **Mid-turn input** — typing while a turn runs queues the line (shown as queued) and it reaches the
  body at its next tool step, as the loop already does.
- **Approvals** — the human gate is an inline choice block: what, its class, why it needs you
  (`outward: git push origin main`), and options `1 Yes · 2 Yes, and don't ask again for
  bash:git push* (writes an allow rule to .isekai/config.local.yaml and reloads the config live,
  so the next step already reads it) · 3 No, and tell the model why`; arrow keys + enter or the
  digit. A denial ends the turn (the law: a denial stops the run there); the reason reaches the
  model twice — in the denied step's result, and as your own next message, which opens the next
  turn at once. A question the model asks (`ask`) is the same block with its options and a free
  line. The proposed rule is the tool and the act's first two words (`bash:git push*`), the host
  for a URL, the subject itself otherwise; it lands in the local layer, never the project's.
- **Keys** — `esc` interrupts the turn (else closes a menu, else clears the input), `ctrl+c`
  interrupts and a second within two seconds exits, `ctrl+o` re-prints the last collapsed block
  in full, `ctrl+l` redraws, `?` on an empty input shows the shortcuts.
- **Non-interactive** — without a TTY on both stdin and stdout (pipes, CI, `run`, `TERM=dumb`) the
  plain line output stays: the TUI is only for a terminal. `--plain` (or `<PREFIX>PLAIN=1`)
  forces the line REPL anywhere.

## Blocks as cards

- **Tool cards** — every line of a tool block carries an edge (`▎`) in its class's colour: read
  cyan, write violet, outward amber, destructive red — the class is seen before it is read.
- **Diffs in colour** — added and removed lines keep their `+`/`-` and line numbers, their code in
  its language's colours (chroma, per line; catppuccin-mocha dark, github light) over a green or
  red tint to the card's edge; context lines coloured, untinted. Unknown language or no colour: the
  plain `+`/`-` lines.
- **Board tables** — Agents and Offices are Lip Gloss tables (rounded, dim bold headers, numbers
  right-aligned); the selected row carries a gold `›` and a bold name, not a background (cells keep
  their own colours).

## The terminal around it

- **Window title** — `<dist> · <world> · <state>` (idle, thinking, running bash, waiting for you).
- **Tab progress** — while a turn runs the terminal's tab shows an indeterminate progress mark
  (OSC 9;4, where the terminal draws it); an open approval shows a warning mark.
- **Links** — a tool block's path is an OSC 8 hyperlink to the file: a click opens it where the
  terminal supports links, plain text elsewhere.
- **Mouse on the board** — a click on a tab switches to it, on a row selects it (again: its
  detail), on a graph node selects it; the wheel scrolls. The session's own screen keeps the
  terminal's selection and scrollback, so it never captures the mouse.

## The board — `/board`

The web board's feeds drawn full screen in the terminal (alt screen); `esc` gives the session back
with its scrollback untouched. Blocks that finish while it is open wait and land when it closes; a
width change meanwhile reprints the transcript on the way out. Four pages, `tab` or `1`–`4`:

- **Agents** — every live body: state glyph, rank, office, state, age, a context bar, tokens,
  cost, model (columns drop by priority on a narrow screen); `enter` opens the body's detail.
- **Graph** — the reasoned ontology as a layered graph: the session on top, a hop down per row,
  each one-hop bond (truth · verdict · reports · above — the web board's own edges) drawn from the
  child up to its parent and colored by bond; `◇` marks a creature with shape findings. Arrows walk
  it. The selected creature's knowledge card sits beside it (below when narrow): its classes, the
  inferred `above` chain, what is under it, doc, territory, minds worn, facts it knows and sees by
  the flow rules, its findings — the graph answering, not a document.
- **Offices** — the triad as config resolves it: what each does, its model and fallback, where
  that was set, the ranks it serves, live bodies in it, tokens spent.
- **Usage** — the usage journal by range (`r` cycles 24h · 7d · 30d · all): totals, a by-day
  sparkline, bars by body, model and office.

## Voice

Both distributions share the TUI; every label comes from the lexicon (agent-one says Subagents,
Roles, Workspace). Colours: the board's rank and lane tokens, adapted to 256-colour and truecolor
terminals; no colour when `NO_COLOR` is set (an ASCII colour profile drops every escape, so a
plain run is the same text; Lip Gloss v2 styles in full colour and the program's writer downsamples
to the terminal's profile). `<PREFIX>THEME=light|dark` or `COLORFGBG` decide the theme when set;
otherwise the terminal is asked for its background colour and the welcome waits for the answer at
most 150 ms — a terminal that never answers keeps the dark theme, so a start is never held.

## Proving it

The ladder applies, with the render rung of `ui.md`. Rung 1 is the view model (`tui/blocks.go`:
pure functions from a view to styled text, golden tests at 80 and 120 columns with the escapes
stripped, `-goldens` rewrites them). Rung 2 is the Bubble Tea program under teatest with a fake
host. Rungs 3–4 are the junction with the real engine (`app/tui.go` under teatest: the mock
provider scripted, real tools, the real classifier behind the gate — the three approval paths,
streaming, diffs, Courts foreground and background, interrupt, queueing). Rung 5 runs the built
binary in a pseudo-terminal against a scripted world (`.isekai/tmp/tui-shots/shoot.py`), feeds
keystrokes, and renders the ANSI stream through `pyte` at 80×24 and 120×40 into plain-text
screens — welcome, a streamed markdown answer with a code block, a bash tool block, an edit with
its diff, a collapsed output and its expansion, the approval prompt and its three paths, a Court
block with its report, the slash menu, the shortcuts, `/agents`, `@path` completion — looked at
before it is called done. Finished blocks leave the program through one FIFO printer, so they
land in the scrollback in the order the loop produced them.
