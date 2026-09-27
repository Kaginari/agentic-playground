package config

// defaultsJSON is layer 1, one table for both distributions; [dist] and [dist-dir] are
// substituted by the lexicon at decode. Every feature object carries `enabled`.
const defaultsJSON = `{
  "models": {
    "default": "anthropic/claude-opus-5",
    "roles": {},
    "ranks": {},
    "members": {},
    "tasks": {}
  },
  "smallModel": "",
  "mode": "build",
  "providers": {
    "anthropic":  { "enabled": true,  "type": "anthropic", "apiKeyEnv": "ANTHROPIC_API_KEY", "baseURL": "https://api.anthropic.com", "maxOutputTokens": 8192, "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "thinking": "adaptive", "fallbacks": "default", "models": {} },
    "openai":     { "enabled": true,  "type": "openai", "apiKeyEnv": "OPENAI_API_KEY", "baseURL": "https://api.openai.com/v1", "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "openrouter": { "enabled": false, "type": "openai", "apiKeyEnv": "OPENROUTER_API_KEY", "baseURL": "https://openrouter.ai/api/v1", "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "ollama":     { "enabled": false, "type": "openai", "apiKeyEnv": "", "baseURL": "http://127.0.0.1:11434/v1", "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "mock":       { "enabled": false, "type": "mock", "script": "" }
  },
  "guard": { "enabled": true, "files": [], "patterns": [] },
  "tools": {
    "profile":   "max",
    "read":      { "enabled": true, "maxLines": 2000, "maxBytes": 51200 },
    "ls":        { "enabled": true, "limit": 500 },
    "glob":      { "enabled": true, "limit": 100 },
    "grep":      { "enabled": true, "limit": 100 },
    "write":     { "enabled": true },
    "edit":      { "enabled": true },
    "multiedit": { "enabled": true },
    "patch":     { "enabled": true },
    "bash":      { "enabled": true, "shell": "", "timeout": "2m", "maxTimeout": "10m", "background": true, "jobsDir": "[dist-dir]/tmp/jobs", "sandbox": "bwrap", "envAllow": [] },
    "git":       { "enabled": true, "timeout": "2m" },
    "webfetch":  { "enabled": true, "maxBytes": 5242880, "timeout": "1m" },
    "websearch": { "enabled": true, "timeout": "1m", "backend": "" },
    "ask":       { "enabled": true },
    "dispatch":  { "enabled": true, "maxDepth": 1, "background": false },
    "recall":    { "enabled": true },
    "remember":  { "enabled": true },
    "toolbox":   { "enabled": true },
    "onto":      { "enabled": true },
    "skill":     { "enabled": true },
    "workingNotes":      { "enabled": true, "limit": 5 },
    "custom":    {},
    "output":    { "maxLines": 2000, "maxBytes": 51200, "dumpDir": "[dist-dir]/tmp/tool-out", "keepDays": 7 },
    "missing":   { "doomLoopRepeats": 2, "proposeOnSecondNaming": true, "liveReload": true }
  },
  "policy": {
    "principles":      { "enabled": true },
    "humanGate":  { "enabled": true, "strict": false, "approve": [], "dryRun": false },
    "gate":       { "enabled": true, "rightAuthor": true, "invariantsHold": true, "dutiesDone": true, "docTruthful": true, "retries": 1, "testsIntact": true },
    "docsAsCode":   { "enabled": true },
    "ownership":  { "enabled": true },
    "wire":       { "enabled": true, "cap": 2048, "requireUnsaid": true, "raw": false },
    "log":        { "enabled": true, "path": "[dist-dir]/log.md" },
    "escalation": { "enabled": true, "maxRetries": 2 },
    "budget":     { "contextTokens": 200000, "stressTokens": 180000, "steps": 40, "minutes": 60 },
    "doomLoop":   { "enabled": false, "threshold": 3 }
  },
  "memory": {
    "short":  { "enabled": true, "cacheDir": "[dist-dir]/memory/short" },
    "long":   { "enabled": true, "index": "[dist-dir]/memory/long/index.json", "rebuildOnStale": true },
    "shared": { "workspace":   { "enabled": true, "path": "[dist-dir]/memory/shared/notes.jsonl" },
                "machine": { "enabled": true, "path": "~/.[dist]/shared/notes.jsonl" } },
    "recall": { "k": 5, "relationBoost": true }
  },
  "toolbox":  { "enabled": true, "budgetTokens": 1500, "registry": "[dist-dir]/toolbox/registry.json", "extra": "[dist-dir]/toolbox/extra.jsonl" },
  "ontology": { "enabled": true, "schema": "[dist-dir]/ontology/schema.ttl", "graphDir": "[dist-dir]/ontology/graph", "projectBudgetTokens": 800, "validate": true },
  "instruments": {
    "context": { "enabled": true },
    "loop":    { "enabled": true, "journalDir": "[dist-dir]/instruments/loop" },
    "toolbox": { "enabled": true, "journalDir": "[dist-dir]/instruments/toolbox" },
    "status":  { "showOff": true }
  },
  "permissions": {
    "enabled": true,
    "rules": [],
    "import": { "claudeCode": { "enabled": true }, "opencode": { "enabled": true } }
  },
  "rules": [],
  "hooks": { "enabled": true, "timeout": "10s", "preTool": [], "postTool": [], "sessionStart": [], "preCompact": [], "stop": [], "userPrompt": [] },
  "mcp": {
    "enabled": true,
    "timeout": "5s",
    "import": { "claudeCode": { "enabled": true, "path": ".mcp.json" }, "opencode": { "enabled": true } },
    "servers": {}
  },
  "discovery": {
    "instructions": { "enabled": true, "files": ["AGENTS.md", "CLAUDE.md"],
      "global": ["~/.config/[dist]/AGENTS.md", "~/.config/opencode/AGENTS.md", "~/.claude/CLAUDE.md"], "walkUp": true },
    "skills":   { "enabled": true, "paths": ["[dist-dir]/skills", ".claude/skills", ".opencode/skills", ".opencode/skill", ".agents/skills",
                  "~/.config/[dist]/skills", "~/.claude/skills", "~/.config/opencode/skills", "~/.config/opencode/skill"] },
    "commands": { "enabled": true, "paths": ["[dist-dir]/commands", ".claude/commands", ".opencode/commands", ".opencode/command",
                  "~/.config/[dist]/commands", "~/.claude/commands", "~/.config/opencode/commands", "~/.config/opencode/command"] },
    "agents":   { "enabled": true, "paths": ["[dist-dir]/{[rank-dirs]}", ".claude/agents", ".opencode/agents", ".opencode/agent",
                  "~/.config/[dist]/agents", "~/.claude/agents", "~/.config/opencode/agents", "~/.config/opencode/agent"] }
  },
  "sessions": { "enabled": true, "dir": "~/.local/share/[dist]/sessions", "keepDays": 30, "title": true },
  "compaction": {
    "enabled": true,
    "strategy": "drain",
    "trigger": { "tokens": 180000, "fraction": 0.85 },
    "keepRecentTurns": 4,
    "passes": { "pointerize": { "enabled": true }, "trimSpent": { "enabled": true }, "unsaid": { "enabled": true },
                "workingNotes": { "enabled": true }, "episode": { "enabled": true }, "verify": { "enabled": true } },
    "journal": true
  },
  "plan":   { "enabled": true, "dir": "[dist-dir]/tmp/plans" },
  "undo":   { "enabled": true, "dir": "~/.local/share/[dist]/undo", "keepDays": 7 },
  "output": { "format": "text", "stream": true, "thinking": false, "color": "auto" },
  "budgets": { "session": { "tokens": 0, "usd": 0 }, "subagent": { "tokens": 0, "usd": 0 } },
  "ui": { "statusLine": true, "announceSubagents": true, "board": { "autostart": true, "port": 7411 } },
  "logLevel": "WARN"
}`
