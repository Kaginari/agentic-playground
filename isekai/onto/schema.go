package onto

import _ "embed"

// DefaultSchema is the binary's built-in copy of the world schema. The world's
// own copy at .isekai/ontology/schema.ttl is the truth when present.
//
//go:embed schema.ttl
var DefaultSchema string

// DefaultPrefixes is the one prefix block the world writes with.
func DefaultPrefixes() map[string]string { return map[string]string{"is": NS} }
