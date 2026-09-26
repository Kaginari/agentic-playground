package onto

import _ "embed"

// DefaultSchema is the binary's built-in copy of the workspace schema. The workspace's
// own copy at .agent-one/ontology/schema.ttl is the truth when present.
//
//go:embed schema.ttl
var DefaultSchema string

// DefaultPrefixes is the one prefix block the workspace writes with.
func DefaultPrefixes() map[string]string { return map[string]string{"ao": NS} }
