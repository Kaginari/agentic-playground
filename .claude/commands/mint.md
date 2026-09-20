---
description: Mint a Body — convert an OpenCode agent (.opencode/agents/) into a Claude Code sub-agent
argument-hint: "[agent-name-or-path] [--as <creature-id>]  (no args: list candidates)"
allowed-tools: Bash(test:*), Bash(ls:*), Bash(mkdir:*), Bash(find:*), Read, Write, Edit
---

# /mint — bring a Body into Claude Code

A Body is an agent: the vessel that actually runs and holds context. Bodies are never minted
generic — a Body is born already named for the rank it will serve (`orc-security`,
`slime-auth-zone`). This command brings a Body defined for OpenCode across into this world,
naming it on the way in.

**Argument:** `$ARGUMENTS`
- No arguments → scan `.opencode/agent(s)/*.md` (project) and `~/.config/opencode/agent(s)/*.md`
  (global) and list every agent found (filename + `mode` + first line of description). Stop
  and ask which to mint.
- A name or path → convert that one agent.
- `--as <creature-id>` → name the resulting Body explicitly (e.g. `orc-security`). If omitted,
  ask what rank and territory this Body serves before minting — never mint unnamed.

## Steps

1. **Find the source** agent file (`.md` with YAML frontmatter; body is the system prompt).
2. **Read the frontmatter.** Typical fields: `description`, `mode` (`primary` | `subagent` |
   `all`), and possibly `model`, `temperature`, `tools`.
3. **Map `mode` to a Claude Code Body:**
   - `mode: subagent` → a **Court** Body. This is the direct fit: write it straight to
     `.claude/agents/<creature-id>.md` as an ordinary Claude Code sub-agent, invoked through
     the Agent tool, context discarded when the task ends.
   - `mode: all` → a **Keeper** Body. Claude Code has no built-in "switch to this as my main
     agent" concept the way OpenCode's Tab-switch does, so a Keeper is approximated as a
     sub-agent that's expected to be invoked repeatedly across a session rather than once.
     Say this limitation out loud to the user rather than silently downgrading it.
   - `mode: primary` → **no clean Claude Code equivalent.** Tell the user plainly: this was a
     user-facing persona OpenCode lets you switch the whole session into, and Claude Code has
     no matching mechanism. Offer to mint it as a Court sub-agent anyway (usable via the Agent
     tool, just not session-switchable), or to skip it. Do not guess — ask.
4. **Translate `tools`** (if the frontmatter restricts tools) using the same table `/don` uses
   for `allowed-tools`. Carry over `model` and `temperature` as comments if Claude Code's
   sub-agent frontmatter in this project doesn't have a matching field — never drop information
   silently, note what didn't survive the crossing.
5. **Write** `.claude/agents/<creature-id>.md` with:
   - frontmatter: the creature id as `name`, the (possibly trimmed) `description`, translated
     `tools`, and `model` if one carried over cleanly.
   - body: the original system prompt, unchanged, unless it references OpenCode-specific
     tools or paths that no longer resolve — flag those inline as `<!-- TODO(mint): ... -->`
     rather than silently rewriting the creature's voice.
6. **Report**: source, destination, the mode mapping used, and anything flagged for a human
   look.

## Notes

- Never mint into `.claude/agents/` without a name. If `--as` wasn't given and the user has no
  answer yet, stop and ask rather than picking a name for them.
- This command never edits the OpenCode source. It only reads from it.
- If no `.opencode/agent(s)/` exists anywhere searched, say so plainly and stop.
