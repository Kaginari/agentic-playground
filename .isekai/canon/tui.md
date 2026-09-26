# The terminal UI — Claude Code style

The live session is an inline terminal UI at the level of Claude Code: the conversation flows into
the terminal's own scrollback, a bordered input sits at the bottom, and every event of the loop is
drawn as a readable block, not a raw line. Built on the Charm libraries (Bubble Tea, Bubbles, Lip
Gloss, Glamour) — the one place the binary takes third-party code; versions pinned in go.sum.

## Layout

- **Inline, not full-screen.** Finished blocks are printed above the live area and stay in the
  normal scrollback (copyable, searchable, survive exit); only the bottom region is live: the
  spinner line, the input box, the footer.
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

- **Input** — multiline (`\` + enter, `alt+enter` or `ctrl+j` for a newline; a terminal that
  does not report `shift+enter` apart from `enter` cannot be told apart, so it sends), history
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

## Voice

Both distributions share the TUI; every label comes from the lexicon (agent-one says Subagents,
Roles, Workspace). Colours: the board's rank and lane tokens, adapted to 256-colour and truecolor
terminals; no colour when `NO_COLOR` is set (an ASCII colour profile drops every escape, so a
plain run is the same text). The theme is dark unless `<PREFIX>THEME=light` or `COLORFGBG`
names a light background — the terminal is never queried, so a start is never held by an
unanswered escape.

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
