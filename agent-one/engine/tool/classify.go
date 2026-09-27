package tool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A port of loop.js's classifier: a heuristic, not a proof — it errs toward asking. A command is
// split at pipes and separators; the strongest class of any segment wins. Patterns match at a
// command position (line start, after ; & | ( ` $ or a quote) so `bash -c "git push"` is still
// outward. Unknown commands default to write; declare a class to tighten.

var systemPrefixes = []string{"/dev/", "/usr/", "/bin/", "/sbin/", "/lib", "/etc/", "/proc/", "/sys/", "/opt/", "/snap/"}

func at(s string) *regexp.Regexp { return regexp.MustCompile("(?:^|[\\s;&|(`$'\"])" + s) }

const record = `(AGENT-ONE\.md|log\.md|design/[^\s]*\.md|notes\.jsonl)`

// git's global options may sit between `git` and the verb (`git -C . push`, `git -c k=v push`,
// `git --no-pager push`); the verb still decides the class.
const gitOpt = `(?:-[cC]\s+\S+|-[cC]\S+|--(?:git-dir|work-tree|namespace|exec-path|super-prefix|config-env)\s+\S+|--?[A-Za-z][\w-]*(?:=\S*)?)`
const git = `git(?:\s+` + gitOpt + `)*\s+`

type rule struct {
	re  *regexp.Regexp
	why string
}

var destructiveRules = []rule{
	{at(`rm(\s|$)`), "rm"}, {at(`(shred|wipe|srm)(\s|$)`), "shred"}, {at(`(mv|rename)\s`), "mv overwrites"}, {at(`(truncate|dd|mkfs(\.\w+)?)(\s|$)`), "truncate/dd/mkfs"}, {at(`diskutil\s+(erase\w*|partitionDisk|zeroDisk|randomDisk|secureErase|reformat)`), "disk erase"},
	{at(git + `(reset|clean|filter-branch|filter-repo|rebase|gc|prune|rm|mv)\b`), "git history/tree rewrite"}, {at(git + `branch\b[^|;&]*\s-[dDM]\b`), "git branch delete/rename"},
	{at(git + `push\b[^|;&]*(\s--force\b|\s-f\b|\s\+)`), "git push --force"}, {at(git + `(checkout|restore)\s+(--\s|\.(\s|$))`), "git discard of working changes"},
	{at(git + `stash\s+(drop|clear|pop)`), "git stash drop"}, {at(git + `reflog\s+(expire|delete)`), "git reflog purge"},
	{regexp.MustCompile(`:\(\)\s*\{`), "fork bomb"}, {regexp.MustCompile(`>\s*/dev/(r?disk|sd|nvme|hd|mmcblk|xvd|vd)`), "write to a raw disk"}, {at(git + `tag\s+-d\b`), "git tag delete"}, {regexp.MustCompile(`\s-delete(\s|$)`), "find -delete"},
	{regexp.MustCompile(`(^|[^>])>\s*\S*` + record), "overwrite of a record (> path)"}, {regexp.MustCompile(`sed\s+(-\S*i|--in-place)[^|;&]*` + record), "sed -i on a record"},
	{regexp.MustCompile(`tee\s+(-[^a\s]\S*\s+)*[^-|;&][^|;&]*` + record), "tee over a record"}, {regexp.MustCompile(`(cp|install)\s+[^|;&]*` + record + `\s*($|[;&|])`), "cp over a record"},
}

var outwardRules = []rule{
	{at(git + `(push|fetch|pull|clone|ls-remote|submodule\s+(update|add))\b`), "git ↔ remote"}, {at(git + `remote\s+(add|set-url|remove|rm|prune|update)`), "git remote change"},
	{at(`(curl|wget|ssh|scp|sftp|rsync|nc|ncat|netcat|telnet|ping|dig|nslookup|ftp|socat)\s`), "network"}, {at(`(npm|pnpm|yarn)\s+(publish|install|i|add|ci|update|upgrade|exec|x|link)\b`), "package network"},
	{at(`npx\s`), "npx fetches"}, {at(`pip3?\s+(install|download)`), "pip network"}, {at(`(docker|podman)\s+(push|pull|login|build|run)\b`), "container registry"},
	{at(`(gh|glab|hub|aws|gcloud|az|kubectl|helm|terraform|heroku|flyctl|vercel|netlify|firebase)\s`), "external service CLI"}, {at(`(mail|sendmail|mutt|msmtp)\s`), "mail"},
	{regexp.MustCompile(`https?://`), "URL"}, {at(`(xdg-open|open)\s`), "opens outside"},
	{at(`go\s+(get|install|mod\s+(download|tidy))\b`), "go module network"},
	// a secret store's secrets would cross the workspace's border: anchored at the segment's start, so
	// the word in prose (`echo "pass the token"`) is not a secret store
	{regexp.MustCompile(`^(pass|gopass|op|bw|bws|lpass|keepassxc-cli|rbw|nordpass)(\s|$)`), "secret store"},
	{regexp.MustCompile(`^security\s+(-\S+\s+)*(dump-keychain|find-generic-password|find-internet-password|export)`), "keychain"},
	{regexp.MustCompile(`^gpg\s[^|;&]*--export-secret`), "secret key export"}, {at(`brew\s+(install|uninstall|reinstall|upgrade|tap)\b`), "system packages"},
}

var writeRules = []rule{
	{regexp.MustCompile(`(^|[^>])>>?`), "redirect"}, {at(`(tee|cp|mkdir|touch|ln|chmod|chown|chgrp|install|patch|unzip|tar)\s`), "writes files"}, {at(`sed\s+(-\S*i|--in-place)`), "sed -i"},
	{at(git + `(add|commit|checkout|switch|merge|stash|tag|init|apply|cherry-pick|notes|worktree)\b`), "git local write"}, {at(`memory\.js\s+[^|;&]*\b(remember|index|forget)\b`), "memory write"},
}

var readOnly = regexp.MustCompile(`^(cat|ls|head|tail|wc|grep|egrep|fgrep|rg|ag|find|stat|file|which|type|echo|printf|test|\[|\[\[|true|false|pwd|date|env|printenv|sleep|sort|uniq|cut|tr|diff|cmp|md5sum|sha\d*sum|jq|yq|awk|sed|less|more|basename|dirname|realpath|readlink|du|df|tree|column|nl|tac|rev|od|xxd|hexdump|strings|seq|expr|bc|comm|paste|join|fold|fmt|xargs|` + git + `(status|log|diff|show|rev-parse|ls-files|blame|describe|cat-file|branch(\s+(-a|-r|-v|-vv|--list|-l))*\s*$|remote(\s+-v)?\s*$|shortlog|grep|count-objects|rev-list)|node\s+\S*memory\.js\s+[^|;&]*\b(recall|status)\b|go\s+(version|env|list|vet|test|build|fmt|doc)\b)(\s|$)`)

var (
	segmentLead = regexp.MustCompile(`^\s*(\w+=\S*\s+)*(sudo\s+|env\s+|time\s+|nice\s+)*`)
	tokenSplit  = regexp.MustCompile("[\\s\"'`=:,]+")
	tokenTrail  = regexp.MustCompile(`[)\]}>;,.]+$`)
	tildePath   = regexp.MustCompile(`^~(/|$)`)
	homeVar     = regexp.MustCompile(`^\$(HOME|\{HOME\})(/|$)`)
	absPath     = regexp.MustCompile(`^/[^/]`)
	climbPath   = regexp.MustCompile(`(^|/)\.\.(/|$)`)
	cdCmd       = regexp.MustCompile(`^cd\s`)
)

func segments(cmd string) []string {
	// `|` not followed by `|`: the regexp consumes the next char, so re-split by hand.
	var parts []string
	cur := strings.Builder{}
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '|' && i+1 < len(rs) && rs[i+1] == '|', c == '&' && i+1 < len(rs) && rs[i+1] == '&':
			parts = append(parts, cur.String())
			cur.Reset()
			i++
		case c == '|' || c == ';' || c == '\n':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(c)
		}
	}
	parts = append(parts, cur.String())
	var out []string
	for _, p := range parts {
		s := strings.TrimSpace(segmentLead.ReplaceAllString(p, ""))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// OutsideWorkspace returns the first token of cmd that names a path outside the workspace, or "".
func (e Env) OutsideWorkspace(cmd string) string {
	home, _ := os.UserHomeDir()
	for _, tok := range tokenSplit.Split(cmd, -1) {
		if tok == "" {
			continue
		}
		p := ""
		switch {
		case tildePath.MatchString(tok):
			p = filepath.Join(home, tok[1:])
		case homeVar.MatchString(tok):
			p = filepath.Join(home, homeVar.ReplaceAllString(tok, "$2"))
		case absPath.MatchString(tok) || tok == "/":
			p = filepath.Clean(tok)
		case climbPath.MatchString(tok):
			p = filepath.Clean(filepath.Join(e.Dir(), tok))
		}
		if p != "" && !e.Inside(tokenTrail.ReplaceAllString(p, "")) {
			return tok
		}
	}
	return ""
}

// ClassifyCommand reads a shell command and returns the strongest class any segment reaches,
// with the reason.
func (e Env) ClassifyCommand(cmd string) Classification {
	best := Classification{Class: Read, Why: "read-only"}
	bump := func(c Class, why string) {
		if c > best.Class {
			best = Classification{Class: c, Why: why}
		}
	}
	if esc := e.OutsideWorkspace(cmd); esc != "" {
		bump(Outward, "path outside the workspace: "+esc)
	}
	for _, seg := range segments(cmd) {
		for _, r := range destructiveRules {
			if r.re.MatchString(seg) {
				bump(Destructive, r.why)
			}
		}
		for _, r := range outwardRules {
			if r.re.MatchString(seg) {
				bump(Outward, r.why)
			}
		}
		for _, r := range writeRules {
			if r.re.MatchString(seg) {
				bump(Write, r.why)
			}
		}
		if !readOnly.MatchString(seg) && !cdCmd.MatchString(seg) {
			bump(Write, "unknown command: "+strings.Fields(seg)[0])
		}
		if cdCmd.MatchString(seg) && e.OutsideWorkspace(seg[3:]) != "" {
			bump(Outward, "cd outside the workspace")
		}
	}
	return best
}

var recordPath = regexp.MustCompile(`(^|/)(AGENT-ONE\.md|log\.md|design/[^/]*\.md|notes\.jsonl)$`)

// IsRecord reports whether a path is one of the workspace's records (never overwritten: Policy 4).
func IsRecord(p string) bool { return recordPath.MatchString(filepath.ToSlash(p)) }

// ClassifyPath settles the class of a file write: outside the workspace is outward, onto a record
// is destructive, else write.
func (e Env) ClassifyPath(abs string) Classification {
	if !e.Inside(abs) {
		return Classification{Class: Outward, Why: "path outside the workspace: " + abs, Paths: []string{abs}}
	}
	if e.isRecord(abs) {
		return Classification{Class: Destructive, Why: "overwrite of a record: " + e.Rel(abs), Paths: []string{abs}}
	}
	return Classification{Class: Write, Why: "writes " + e.Rel(abs), Paths: []string{abs}}
}

// wrappers run the command that follows them; a permission rule is matched after each.
var wrappers = regexp.MustCompile(`^(?:(?:sudo|env|nice|time|nohup|command|exec|eval|xargs|timeout)\b\s*|(?:sh|bash|zsh|dash)\s+-c\s+)`)

// gitLead is `git` with at least one global option before the verb.
var gitLead = regexp.MustCompile(`^git(?:\s+` + gitOpt + `)+\s+`)

// CommandForms lists the shapes a permission rule is also matched against, so a rule on
// `git push*` holds when the same act hides behind env, sudo, xargs, a quoted `sh -c`
// argument, a `cd … &&` prefix, a subshell, or git's global options. Arguments are not
// commands: `echo git push` yields no `git push`. A rule matched on a form only tightens
// (deny or ask) — an allow still needs the whole command (app.decideHook).
func CommandForms(cmd string) []string {
	whole := strings.TrimSpace(cmd)
	seen := map[string]bool{}
	var out []string
	var explore func(s string)
	explore = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		if s != whole {
			out = append(out, s)
		}
		if m := gitLead.FindString(s); m != "" {
			explore("git " + s[len(m):])
		}
		if m := wrappers.FindString(s); m != "" {
			explore(s[len(m):])
		}
		// a wrapper's own options (`sudo -u x`, `timeout 5`): one token stripped, then two
		if f := strings.Fields(s); len(f) > 1 && (strings.HasPrefix(f[0], "-") || f[0][0] >= '0' && f[0][0] <= '9') {
			explore(strings.Join(f[1:], " "))
			if len(f) > 2 && strings.HasPrefix(f[0], "-") {
				explore(strings.Join(f[2:], " "))
			}
		}
		// a quoted or subshell command: what follows the opener, and the same closed again
		if i := strings.IndexAny(s, "'\"`("); i >= 0 {
			rest := s[i+1:]
			explore(rest)
			explore(strings.TrimRight(rest, "'\"`) "))
		}
	}
	for _, seg := range segments(cmd) {
		explore(seg)
	}
	return out
}
