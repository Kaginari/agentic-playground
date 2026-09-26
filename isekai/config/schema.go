package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Kaginari/agentic-playground/isekai/config/yaml"
)

// Config is the effective configuration: the typed schema, plus what the loader learned.
type Config struct {
	Schema     string `json:"$schema"`
	SmallModel string `json:"smallModel"`
	Mode       string `json:"mode"` // build | plan

	Models    Models               `json:"models"`
	Providers map[string]*Provider `json:"providers"`
	Tools     Tools                `json:"tools"`
	Law       Law                  `json:"law"`
	Memory    Memory               `json:"memory"`
	Toolbox   Toolbox              `json:"toolbox"`
	Ontology  Ontology             `json:"ontology"`

	Instruments Instruments         `json:"instruments"`
	Permissions Permissions         `json:"permissions"`
	Rules       []*Rule             `json:"rules"`
	Hooks       Hooks               `json:"hooks"`
	MCP         MCP                 `json:"mcp"`
	Discovery   Discovery           `json:"discovery"`
	Sessions    Sessions            `json:"sessions"`
	Compaction  Compaction          `json:"compaction"`
	Plan        Plan                `json:"plan"`
	Undo        Undo                `json:"undo"`
	Output      Output              `json:"output"`
	LogLevel    string              `json:"logLevel"`
	Budgets     Budgets             `json:"budgets"`
	UI          UI                  `json:"ui"`
	RankSet     string              `json:"rankSet"` // extend | replace
	RankDefs    map[string]*RankDef `json:"ranks"`

	// Learned by the loader.
	tree    *yaml.Node
	ranks   []Rank
	Dist    Dist              `json:"-"`
	Root    string            `json:"-"` // the world root (dir holding <dist-dir>)
	Layers  []Layer           `json:"-"`
	Origins map[string]Origin `json:"-"` // dotted key → where its live value came from
	Holes   []string          `json:"-"` // unknown keys and other @? findings
	opts    Options
}

// Origin is where a live value came from.
type Origin struct {
	Layer string // default | global | project | local | env-file | env | flag
	File  string // path, "env:VAR", "flag:--x" or "default"
	Line  int
}

func (o Origin) String() string {
	if o.Line > 0 {
		return fmt.Sprintf("%s:%d", o.File, o.Line)
	}
	return o.File
}

// Provider is one named model endpoint. Keys come from env only.
type Provider struct {
	Enabled         bool                   `json:"enabled"`
	Type            string                 `json:"type"` // anthropic | openai | mock
	BaseURL         string                 `json:"baseURL"`
	APIKeyEnv       string                 `json:"apiKeyEnv"`
	Headers         map[string]string      `json:"headers"`
	Models          map[string]*ModelEntry `json:"models"`
	Timeout         Duration               `json:"timeout"`
	TLS             TLS                    `json:"tls"`
	MaxOutputTokens int                    `json:"maxOutputTokens"`
	ToolCalls       string                 `json:"toolCalls"` // native | text
	GuidedDecoding  bool                   `json:"guidedDecoding"`
	ContextWindow   AutoInt                `json:"contextWindow"` // auto | <n>
	Thinking        string                 `json:"thinking"`      // anthropic: adaptive | off
	Fallbacks       string                 `json:"fallbacks"`     // anthropic: default | off (refusal fallbacks)
	Script          string                 `json:"script"`        // mock only
}

// Models routes calls by creature, office, rank and task; the session runs on Default.
// Office keys are canonical (great-sage, raphael, ciel); agent-one's analyst/judge/drafter
// and its rank words are accepted on input and folded to the canonical keys.
type Models struct {
	Default   ModelRef            `json:"default"`
	Offices   map[string]ModelRef `json:"offices"`
	Ranks     map[string]ModelRef `json:"ranks"`
	Creatures map[string]ModelRef `json:"creatures"`
	Tasks     map[string]ModelRef `json:"tasks"`
}

// ModelRef is "provider/model" or {model, effort, fallback}.
type ModelRef struct {
	Model    string `json:"model"`
	Effort   string `json:"effort"`
	Fallback string `json:"fallback"`
}

// ModelEntry is what a provider does not report about one model; Tier orders the triad,
// Price prices its usage (absent = unpriced: tokens are shown, a cost is never guessed).
type ModelEntry struct {
	ID            string `json:"id"`
	Tier          int    `json:"tier"`
	ContextWindow int    `json:"contextWindow"`
	MaxOutput     int    `json:"maxOutput"`
	Reasoning     bool   `json:"reasoning"`
	Price         *Price `json:"price"`
}

// Price is USD per million tokens.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// Cost prices a usage; the four counts are tokens.
func (p *Price) Cost(input, output, cacheRead, cacheWrite int) float64 {
	if p == nil {
		return 0
	}
	return (float64(input)*p.Input + float64(output)*p.Output + float64(cacheRead)*p.CacheRead + float64(cacheWrite)*p.CacheWrite) / 1e6
}

// Budgets are the honest stop lines per session and per Court; 0 is off.
type Budgets struct {
	Session Cap `json:"session"`
	Court   Cap `json:"court"`
}

type Cap struct {
	Tokens int     `json:"tokens"`
	USD    float64 `json:"usd"`
}

func (c Cap) Off() bool { return c.Tokens == 0 && c.USD == 0 }

// UI is the live session's chrome.
type UI struct {
	StatusLine     bool    `json:"statusLine"`
	AnnounceCourts bool    `json:"announceCourts"`
	Board          BoardUI `json:"board"`
}

// BoardUI is ui.board: the board the binary serves on 127.0.0.1.
type BoardUI struct {
	Autostart bool `json:"autostart"`
	Port      int  `json:"port"`
}

type TLS struct {
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	CAFile             string `json:"caFile"`
}

// Tools is the toolset: the builtins, custom tools, the profile and the missing-tool policy.
type Tools struct {
	Profile   string                 `json:"profile"` // max | anthropic | openai | minimal
	Read      BuiltinTool            `json:"read"`
	Ls        BuiltinTool            `json:"ls"`
	Glob      BuiltinTool            `json:"glob"`
	Grep      BuiltinTool            `json:"grep"`
	Write     BuiltinTool            `json:"write"`
	Edit      BuiltinTool            `json:"edit"`
	Multiedit BuiltinTool            `json:"multiedit"`
	Patch     BuiltinTool            `json:"patch"`
	Bash      BuiltinTool            `json:"bash"`
	Git       BuiltinTool            `json:"git"`
	Webfetch  BuiltinTool            `json:"webfetch"`
	Websearch BuiltinTool            `json:"websearch"`
	Ask       BuiltinTool            `json:"ask"`
	Dispatch  BuiltinTool            `json:"dispatch"`
	Recall    BuiltinTool            `json:"recall"`
	Remember  BuiltinTool            `json:"remember"`
	Toolbox   BuiltinTool            `json:"toolbox"`
	Onto      BuiltinTool            `json:"onto"`
	Skill     BuiltinTool            `json:"skill"`
	Desk      BuiltinTool            `json:"desk"`
	Custom    map[string]*CustomTool `json:"custom"`
	Output    ToolOutput             `json:"output"`
	Missing   Missing                `json:"missing"`
}

// BuiltinTool is the union of every builtin's knobs; a knob a tool does not use is ignored.
type BuiltinTool struct {
	Enabled     bool     `json:"enabled"`
	Description string   `json:"description"` // override; "" keeps the builtin text
	Class       string   `json:"class"`       // tighten-only against the builtin floor
	Timeout     Duration `json:"timeout"`
	MaxTimeout  Duration `json:"maxTimeout"`
	MaxLines    int      `json:"maxLines"`
	MaxBytes    int      `json:"maxBytes"`
	Limit       int      `json:"limit"`
	Shell       string   `json:"shell"`
	Background  bool     `json:"background"`
	JobsDir     string   `json:"jobsDir"`
	Sandbox     string   `json:"sandbox"`  // bash: bwrap | none
	EnvAllow    []string `json:"envAllow"` // bash: env vars that survive scrubbing
	MaxDepth    int      `json:"maxDepth"` // dispatch
	Backend     string   `json:"backend"`  // websearch: a URL with {query}, answering SearXNG-style JSON
}

// CustomTool is a tool declared in config: argv with {{param}} placeholders, or a shell
// template with params as $P_<name> env vars — never shell-interpolated params.
type CustomTool struct {
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description"`
	Params      map[string]*Param `json:"params"`
	Run         []string          `json:"run"`
	Shell       string            `json:"shell"`
	Class       string            `json:"class"`
	Timeout     Duration          `json:"timeout"`
	Cwd         string            `json:"cwd"`
	Sandbox     string            `json:"sandbox"` // inherit | bwrap | none
	Override    bool              `json:"override"`
	Profiles    []string          `json:"profiles"`
}

// Param is one flat JSON-schema property of a custom tool.
type Param struct {
	Type        string   `json:"type"` // string | integer | number | boolean
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     string   `json:"default"`
	Enum        []string `json:"enum"`
}

type ToolOutput struct {
	MaxLines int    `json:"maxLines"`
	MaxBytes int    `json:"maxBytes"`
	DumpDir  string `json:"dumpDir"`
	KeepDays int    `json:"keepDays"`
}

// Missing is the policy when the model calls a tool that is off or absent.
type Missing struct {
	DoomLoopRepeats       int  `json:"doomLoopRepeats"`
	ProposeOnSecondNaming bool `json:"proposeOnSecondNaming"`
	LiveReload            bool `json:"liveReload"`
}

// Law is the convention as code paths.
type Law struct {
	Crest      Feature    `json:"crest"`
	HumanGate  HumanGate  `json:"humanGate"`
	Gate       Gate       `json:"gate"`
	Vitality   Feature    `json:"vitality"`
	Territory  Feature    `json:"territory"`
	Wire       Wire       `json:"wire"`
	Log        LogFile    `json:"log"`
	Escalation Escalation `json:"escalation"`
	Budget     Budget     `json:"budget"`
	DoomLoop   DoomLoop   `json:"doomLoop"`
}

type Feature struct {
	Enabled bool `json:"enabled"`
}

type HumanGate struct {
	Enabled bool     `json:"enabled"`
	Strict  bool     `json:"strict"`
	Approve []string `json:"approve"`
	DryRun  bool     `json:"dryRun"`
}

type Gate struct {
	Enabled     bool `json:"enabled"`
	RightAuthor bool `json:"rightAuthor"`
	TraitsHold  bool `json:"traitsHold"`
	DutiesDone  bool `json:"dutiesDone"`
	DocTruthful bool `json:"docTruthful"`
}

type Wire struct {
	Enabled       bool `json:"enabled"`
	Cap           int  `json:"cap"`
	RequireUnsaid bool `json:"requireUnsaid"`
	Raw           bool `json:"raw"`
}

type LogFile struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type Escalation struct {
	Enabled    bool `json:"enabled"`
	MaxRetries int  `json:"maxRetries"`
}

type Budget struct {
	ContextTokens int     `json:"contextTokens"`
	StressTokens  int     `json:"stressTokens"`
	Steps         int     `json:"steps"`
	Minutes       float64 `json:"minutes"`
}

type DoomLoop struct {
	Enabled   bool `json:"enabled"`
	Threshold int  `json:"threshold"`
}

type Memory struct {
	Short  MemoryShort  `json:"short"`
	Long   MemoryLong   `json:"long"`
	Shared MemoryShared `json:"shared"`
	Recall MemoryRecall `json:"recall"`
}

type MemoryShort struct {
	Enabled  bool   `json:"enabled"`
	CacheDir string `json:"cacheDir"`
}

type MemoryLong struct {
	Enabled        bool   `json:"enabled"`
	Index          string `json:"index"`
	RebuildOnStale bool   `json:"rebuildOnStale"`
}

type MemoryShared struct {
	World   PathFeature `json:"world"`
	Machine PathFeature `json:"machine"`
}

type PathFeature struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type MemoryRecall struct {
	K             int  `json:"k"`
	RelationBoost bool `json:"relationBoost"`
}

type Toolbox struct {
	Enabled      bool   `json:"enabled"`
	BudgetTokens int    `json:"budgetTokens"`
	Registry     string `json:"registry"`
	Extra        string `json:"extra"`
}

type Ontology struct {
	Enabled             bool   `json:"enabled"`
	Schema              string `json:"schema"`
	GraphDir            string `json:"graphDir"`
	ProjectBudgetTokens int    `json:"projectBudgetTokens"`
	Validate            bool   `json:"validate"`
}

type Instruments struct {
	Context Feature     `json:"context"`
	Loop    DirFeature  `json:"loop"`
	Toolbox DirFeature  `json:"toolbox"`
	Status  StatusKnobs `json:"status"`
}

type DirFeature struct {
	Enabled    bool   `json:"enabled"`
	JournalDir string `json:"journalDir"`
}

type StatusKnobs struct {
	ShowOff bool `json:"showOff"`
}

// Permissions overlays the class gate with ordered, most-specific-wins rules.
type Permissions struct {
	Enabled bool        `json:"enabled"`
	Rules   []*PermRule `json:"rules"`
	Import  Imports     `json:"import"`
}

// PermRule is `{match: "<tool>:<glob>", action: allow|ask|deny}`.
type PermRule struct {
	Match  string `json:"match"`
	Action string `json:"action"`

	Tool    string `json:"-"` // split from Match
	Pattern string `json:"-"`
	Origin  Origin `json:"-"`
}

type Imports struct {
	ClaudeCode Feature `json:"claudeCode"`
	Opencode   Feature `json:"opencode"`
}

// Rule is an injected law: prose for the prompt and the ontology, or a check the gate runs.
type Rule struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	File    string   `json:"file"`
	Scope   string   `json:"scope"` // all | rank:<race> | creature:<name>
	Check   string   `json:"check"`
	Timeout Duration `json:"timeout"`

	Origin   Origin `json:"-"`
	Resolved string `json:"-"` // absolute path of File, when set
}

type Hooks struct {
	Enabled      bool     `json:"enabled"`
	Timeout      Duration `json:"timeout"`
	PreTool      []*Hook  `json:"preTool"`
	PostTool     []*Hook  `json:"postTool"`
	SessionStart []*Hook  `json:"sessionStart"`
	PreCompact   []*Hook  `json:"preCompact"`
	Stop         []*Hook  `json:"stop"`
	UserPrompt   []*Hook  `json:"userPrompt"`
}

type Hook struct {
	Match   string `json:"match"`
	Command string `json:"command"`
}

// MCP is the client: servers by name, imports from the other harnesses.
type MCP struct {
	Enabled bool                  `json:"enabled"`
	Timeout Duration              `json:"timeout"`
	Import  MCPImports            `json:"import"`
	Servers map[string]*MCPServer `json:"servers"`
}

type MCPImports struct {
	ClaudeCode PathFeature `json:"claudeCode"`
	Opencode   Feature     `json:"opencode"`
}

// MCPServer is one server: stdio (a command under the sandbox) or streamable http.
type MCPServer struct {
	Enabled    bool                `json:"enabled"`
	Type       string              `json:"type"` // stdio | http
	Command    []string            `json:"command"`
	Args       []string            `json:"args"`
	Env        map[string]string   `json:"env"`
	EnvAllow   []string            `json:"envAllow"`
	Cwd        string              `json:"cwd"`
	Network    bool                `json:"network"`
	Sandbox    string              `json:"sandbox"` // inherit | bwrap | none
	URL        string              `json:"url"`
	Headers    map[string]string   `json:"headers"`
	HeadersEnv map[string]string   `json:"headersEnv"` // header → env var holding its value
	Inward     bool                `json:"inward"`
	Tools      map[string]*MCPTool `json:"tools"`
	Timeout    Duration            `json:"timeout"`
}

type MCPTool struct {
	Enabled     bool   `json:"enabled"`
	Class       string `json:"class"`
	Description string `json:"description"`
}

type Discovery struct {
	Instructions Instructions `json:"instructions"`
	Skills       PathsFeature `json:"skills"`
	Commands     PathsFeature `json:"commands"`
	Agents       PathsFeature `json:"agents"`
}

type Instructions struct {
	Enabled bool     `json:"enabled"`
	Files   []string `json:"files"`
	Global  []string `json:"global"`
	WalkUp  bool     `json:"walkUp"`
}

type PathsFeature struct {
	Enabled bool     `json:"enabled"`
	Paths   []string `json:"paths"`
}

type Sessions struct {
	Enabled  bool   `json:"enabled"`
	Dir      string `json:"dir"`
	KeepDays int    `json:"keepDays"`
	Title    bool   `json:"title"`
}

type Compaction struct {
	Enabled         bool             `json:"enabled"`
	Strategy        string           `json:"strategy"` // drain | summary
	Trigger         Trigger          `json:"trigger"`
	KeepRecentTurns int              `json:"keepRecentTurns"`
	Passes          CompactionPasses `json:"passes"`
	Journal         bool             `json:"journal"`
}

type Trigger struct {
	Tokens   int     `json:"tokens"`
	Fraction float64 `json:"fraction"`
}

type CompactionPasses struct {
	Pointerize Feature `json:"pointerize"`
	TrimSpent  Feature `json:"trimSpent"`
	Unsaid     Feature `json:"unsaid"`
	Desk       Feature `json:"desk"`
	Episode    Feature `json:"episode"`
	Verify     Feature `json:"verify"`
}

type Plan struct {
	Enabled bool   `json:"enabled"`
	Dir     string `json:"dir"`
}

type Undo struct {
	Enabled  bool   `json:"enabled"`
	Dir      string `json:"dir"`
	KeepDays int    `json:"keepDays"`
}

type Output struct {
	Format   string `json:"format"` // text | json | wire
	Stream   bool   `json:"stream"`
	Thinking bool   `json:"thinking"`
	Color    string `json:"color"`
}

// Duration reads "10m" / "30s" or a bare number of seconds.
type Duration time.Duration

func (d *Duration) decode(text string, isNumber bool) error {
	if isNumber {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return err
		}
		*d = Duration(time.Duration(f * float64(time.Second)))
		return nil
	}
	v, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("%q is not a duration (10m, 30s) or a number of seconds", text)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) String() string { return time.Duration(d).String() }

// D is the plain time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// AutoInt is "auto" or a number.
type AutoInt struct {
	Auto bool
	N    int
}

func (a *AutoInt) decode(text string, isNumber bool) error {
	if !isNumber {
		if strings.EqualFold(text, "auto") {
			*a = AutoInt{Auto: true}
			return nil
		}
		return fmt.Errorf("%q is not \"auto\" or a number", text)
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return err
	}
	*a = AutoInt{N: n}
	return nil
}

func (a AutoInt) String() string {
	if a.Auto {
		return "auto"
	}
	return strconv.Itoa(a.N)
}

// Class order: a declared class may only tighten (move right).
var classOrder = map[string]int{"read": 0, "write": 1, "outward": 2, "destructive": 3}

// Builtins maps each builtin tool to its class floor; "" means classified per call.
var Builtins = map[string]string{
	"bash": "", "git": "",
	"read": "read", "ls": "read", "glob": "read", "grep": "read",
	"write": "write", "edit": "write", "multiedit": "write", "patch": "write",
	"webfetch": "outward", "websearch": "outward",
	"ask": "read", "dispatch": "write", "recall": "read", "remember": "write",
	"toolbox": "read", "onto": "read", "skill": "read", "desk": "write",
}

// Profiles lists the tools each profile offers; "max" is every builtin.
var Profiles = map[string][]string{
	"max":       nil,
	"anthropic": nil,
	"openai":    nil,
	"minimal":   {"bash", "read", "write", "edit", "ask", "dispatch", "recall", "remember", "skill", "desk"},
}

// OutwardCapable names the tools whose acts may leave the world; an `allow` on them is a
// loosening, and a bare wildcard `allow` on them is refused.
var OutwardCapable = map[string]bool{"bash": true, "git": true, "webfetch": true, "websearch": true, "*": true}
