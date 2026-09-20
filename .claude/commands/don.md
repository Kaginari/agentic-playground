---
description: Don a Mind — convert an OpenCode skill (.opencode/skill(s)/) into a Claude Code skill
argument-hint: "[skill-name-or-path] [--project|--global]  (no args: list candidates)"
allowed-tools: Bash(test:*), Bash(ls:*), Bash(mkdir:*), Bash(cp:*), Bash(find:*), Read, Write, Edit
---

# /don — bring a Mind into Claude Code

A Mind is a skill: stateless know-how a creature wears as a hat for one task. This command
brings a Mind minted for OpenCode across into this world's Rimuru, so it can be worn here too.

The two formats are close cousins — both implement the same open **Agent Skills** standard
(a `SKILL.md` file with YAML frontmatter + a markdown body, plus optional `scripts/` and
`references/` beside it). Converting is mostly relocation and validation, not rewriting.

**Argument:** `$ARGUMENTS`
- No arguments → scan for candidates and list them; stop and ask which to don.
- A name or path → convert that one skill.
- `--project` → install to `.claude/skills/<name>/` (this world only). Default.
- `--global` → install to `~/.claude/skills/<name>/` (every world Rimuru rules, since
  Rimuru is machine-global).

## Steps

1. **Find the source.** Look in, in order: the given path; `./.opencode/skill/<name>/`;
   `./.opencode/skills/<name>/`; `~/.config/opencode/skill(s)/<name>/`. If nothing is given,
   list every `SKILL.md` found under those roots (name + first line of description) and stop.
2. **Read `SKILL.md`.** Parse the YAML frontmatter and the markdown body.
3. **Validate, and fix only with the user's OK:**
   - `name` must match the directory name exactly (Claude Code enforces this too). If it
     doesn't, tell the user and ask whether to rename the directory or the frontmatter field.
   - `description` should be present and specific — short of ~20 characters, flag it as thin
     rather than silently accepting it.
   - `license` and `metadata` carry over unchanged; both formats support them.
4. **Translate `allowed-tools`**, if present, using this table. Keep anything not listed as-is
   and flag it for the user to check by hand:

   | OpenCode tool | Claude Code tool |
   |---|---|
   | `read` | `Read` |
   | `write` | `Write` |
   | `edit` / `patch` | `Edit` |
   | `bash` | `Bash` |
   | `grep` | `Grep` |
   | `glob` | `Glob` |
   | `list` | `Glob` or `Bash(ls:*)` — ask which fits |
   | `webfetch` | `WebFetch` |
   | `task` | `Agent` |
   | `todowrite` / `todoread` | `TaskCreate` / `TaskList` — ask if this project has a task tool; drop otherwise |

5. **Copy the directory**, not just the file: `SKILL.md`, `scripts/`, `references/`, and any
   other sibling files, into the destination (`.claude/skills/<name>/` or
   `~/.claude/skills/<name>/`). Do not overwrite an existing skill at the destination without
   asking first.
6. **Report**: source path, destination path, what was translated, what needs a human look
   (thin description, unmapped tools, name mismatch), and remind the user this Mind is now
   worn only where it was installed — `--project` skills don't follow to other worlds.

## Notes

- This command never edits the OpenCode source. It only reads from it.
- If `.opencode/skill(s)/` doesn't exist anywhere searched, say so plainly and stop — don't
  invent a skill to convert.
