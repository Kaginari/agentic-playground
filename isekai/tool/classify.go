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

const record = `(isekai\.md|log\.md|canon/[^\s]*\.md|notes\.jsonl)`

type rule struct {
	re  *regexp.Regexp
	why string
}

var destructiveRules = []rule{
	{at(`rm(\s|$)`), "rm"}, {at(`(shred|wipe|srm)(\s|$)`), "shred"}, {at(`(mv|rename)\s`), "mv overwrites"}, {at(`(truncate|dd|mkfs)(\s|$)`), "truncate/dd/mkfs"},
	{at(`git\s+(reset|clean|filter-branch|filter-repo|rebase|gc|prune|rm|mv)\b`), "git history/tree rewrite"}, {at(`git\s+branch\b[^|;&]*\s-[dDM]\b`), "git branch delete/rename"},
	{at(`git\s+push\b[^|;&]*(\s--force\b|\s-f\b|\s\+)`), "git push --force"}, {at(`git\s+(checkout|restore)\s+(--\s|\.(\s|$))`), "git discard of working changes"},
	{at(`git\s+stash\s+(drop|clear|pop)`), "git stash drop"}, {at(`git\s+tag\s+-d\b`), "git tag delete"}, {regexp.MustCompile(`\s-delete(\s|$)`), "find -delete"},
	{regexp.MustCompile(`(^|[^>])>\s*\S*` + record), "overwrite of a record (> path)"}, {regexp.MustCompile(`sed\s+(-\S*i|--in-place)[^|;&]*` + record), "sed -i on a record"},
	{regexp.MustCompile(`tee\s+(-[^a\s]\S*\s+)*[^-|;&][^|;&]*` + record), "tee over a record"}, {regexp.MustCompile(`(cp|install)\s+[^|;&]*` + record + `\s*($|[;&|])`), "cp over a record"},
}

var outwardRules = []rule{
	{at(`git\s+(push|fetch|pull|clone|ls-remote|submodule\s+(update|add))\b`), "git ↔ remote"}, {at(`git\s+remote\s+(add|set-url|remove|rm|prune|update)`), "git remote change"},
	{at(`(curl|wget|ssh|scp|sftp|rsync|nc|ncat|netcat|telnet|ping|dig|nslookup|ftp|socat)\s`), "network"}, {at(`(npm|pnpm|yarn)\s+(publish|install|i|add|ci|update|upgrade|exec|x|link)\b`), "package network"},
	{at(`npx\s`), "npx fetches"}, {at(`pip3?\s+(install|download)`), "pip network"}, {at(`(docker|podman)\s+(push|pull|login|build|run)\b`), "container registry"},
	{at(`(gh|glab|hub|aws|gcloud|az|kubectl|helm|terraform|heroku|flyctl|vercel|netlify|firebase)\s`), "external service CLI"}, {at(`(mail|sendmail|mutt|msmtp)\s`), "mail"},
	{regexp.MustCompile(`https?://`), "URL"}, {at(`(xdg-open|open)\s`), "opens outside"},
	{at(`go\s+(get|install|mod\s+(download|tidy))\b`), "go module network"},
}

var writeRules = []rule{
	{regexp.MustCompile(`(^|[^>])>>?`), "redirect"}, {at(`(tee|cp|mkdir|touch|ln|chmod|chown|chgrp|install|patch|unzip|tar)\s`), "writes files"}, {at(`sed\s+(-\S*i|--in-place)`), "sed -i"},
	{at(`git\s+(add|commit|checkout|switch|merge|stash|tag|init|apply|cherry-pick|notes|worktree)\b`), "git local write"}, {at(`memory\.js\s+[^|;&]*\b(remember|index|forget)\b`), "memory write"},
}

var readOnly = regexp.MustCompile(`^(cat|ls|head|tail|wc|grep|egrep|fgrep|rg|ag|find|stat|file|which|type|echo|printf|test|\[|\[\[|true|false|pwd|date|env|printenv|sleep|sort|uniq|cut|tr|diff|cmp|md5sum|sha\d*sum|jq|yq|awk|sed|less|more|basename|dirname|realpath|readlink|du|df|tree|column|nl|tac|rev|od|xxd|hexdump|strings|seq|expr|bc|comm|paste|join|fold|fmt|xargs|git\s+(status|log|diff|show|rev-parse|ls-files|blame|describe|cat-file|branch(\s+(-a|-r|-v|-vv|--list|-l))*\s*$|remote(\s+-v)?\s*$|shortlog|grep|count-objects|rev-list)|node\s+\S*memory\.js\s+[^|;&]*\b(recall|status)\b|go\s+(version|env|list|vet|test|build|fmt|doc)\b)(\s|$)`)

var (
	segmentLead = regexp.MustCompile(`^\s*(\w+=\S*\s+)*(sudo\s+|env\s+|time\s+|nice\s+)*`)
	tokenSplit  = regexp.MustCompile("[\\s\"'`=:,]+")
	tokenTrail  = regexp.MustCompile(`[)\]}>;,.]+$`)
	tildePath   = regexp.MustCompile(`^~(/|$)`)
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

// OutsideWorld returns the first token of cmd that names a path outside the world, or "".
func (e Env) OutsideWorld(cmd string) string {
	home, _ := os.UserHomeDir()
	for _, tok := range tokenSplit.Split(cmd, -1) {
		if tok == "" {
			continue
		}
		p := ""
		switch {
		case tildePath.MatchString(tok):
			p = filepath.Join(home, tok[1:])
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
	if esc := e.OutsideWorld(cmd); esc != "" {
		bump(Outward, "path outside the world: "+esc)
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
		if cdCmd.MatchString(seg) && e.OutsideWorld(seg[3:]) != "" {
			bump(Outward, "cd outside the world")
		}
	}
	return best
}

var recordPath = regexp.MustCompile(`(^|/)(isekai\.md|log\.md|canon/[^/]*\.md|notes\.jsonl)$`)

// IsRecord reports whether a path is one of the world's records (never overwritten: Law 4).
func IsRecord(p string) bool { return recordPath.MatchString(filepath.ToSlash(p)) }

// ClassifyPath settles the class of a file write: outside the world is outward, onto a record
// is destructive, else write.
func (e Env) ClassifyPath(abs string) Classification {
	if !e.Inside(abs) {
		return Classification{Class: Outward, Why: "path outside the world: " + abs, Paths: []string{abs}}
	}
	if e.isRecord(abs) {
		return Classification{Class: Destructive, Why: "overwrite of a record: " + e.Rel(abs), Paths: []string{abs}}
	}
	return Classification{Class: Write, Why: "writes " + e.Rel(abs), Paths: []string{abs}}
}
