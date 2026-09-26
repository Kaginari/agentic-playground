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
  (± lines, line numbers), collapsed past ~20 lines.
- **Courts** — a dispatched Court is one block with its rank, office and model, a live state, and
  on return its report rendered (status, findings, holes, the unsaid) — not the raw wire text.
- **The gate verdict** — one quiet line after a turn that wrote: `✓ gate · n/a (no orcs) · log.md`.
- **Spinner line** — while a turn runs: a spinner, a verb for the current beat (`Thinking…`,
  `Running bash…`, `Waiting for approval…`), elapsed time, tokens so far, `esc to interrupt`.
- **Footer** — under the input: the model · context % · cost (or "no calls yet") · live courts ·
  the world dir; `? for shortcuts`.

## Interaction

- **Input** — multiline (`shift+enter` or `\` + enter for a newline), history with ↑/↓, paste of
  large text collapsed to `[pasted N lines]`, `@path` completes files in the world.
- **Slash commands** — typing `/` opens an inline menu of built-in and discovered commands with
  their descriptions, filtered as you type; tab/enter selects.
- **Mid-turn input** — typing while a turn runs queues the line (shown as queued) and it reaches the
  body at its next tool step, as the loop already does.
- **Approvals** — the human gate is an inline choice block: what, its class, why it needs you
  (`outward: git push origin main`), and options `1 Yes · 2 Yes, and don't ask again for
  bash:git push* (writes a rule to .isekai/config.local.yaml) · 3 No, and tell the model why`;
  arrow keys + enter or the digit. Denial reasons go back to the model.
- **Keys** — `esc` interrupts the turn (twice: clear input), `ctrl+c` twice exits, `ctrl+o` expands
  the last collapsed block, `ctrl+l` redraws, `?` shows the shortcuts.
- **Non-interactive** — without a TTY (pipes, CI, `run`) the plain line output stays: the TUI is
  only for a terminal. `--plain` forces the line REPL anywhere.

## Voice

Both distributions share the TUI; every label comes from the lexicon (agent-one says Subagents,
Roles, Workspace). Colours: the board's rank and lane tokens, adapted to 256-colour and truecolor
terminals; no colour when `NO_COLOR` is set.

## Proving it

The ladder applies, with the render rung of `ui.md`: model/update logic under Bubble Tea's test
harness (teatest), and screens rendered through a terminal emulator at 80×24 and 120×40 — welcome,
a streamed markdown answer, a tool block with a diff, a collapsed output, an approval prompt, a
Court block, the slash menu — and looked at before it is called done.
