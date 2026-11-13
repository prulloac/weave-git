// Package source defines the weave.source/v1 wire types.
package source

const ProtocolVersion = "weave.source/v1"

// Options controls one source materialization.
type Options struct {
	Worktree   bool `json:"worktree,omitempty"`
	Submodules bool `json:"submodules,omitempty"`
	LFS        bool `json:"lfs,omitempty"`
	Depth      int  `json:"depth,omitempty"`
	Permissive bool `json:"permissive,omitempty"`
}

// Request is one source-provider request payload.
type Request struct {
	V        string  `json:"v"`
	Type     string  `json:"type"`
	SourceID string  `json:"source_id"`
	URI      string  `json:"uri"`
	Ref      string  `json:"ref,omitempty"`
	CacheDir string  `json:"cache_dir,omitempty"`
	Options  Options `json:"options,omitempty"`
}

// Snapshot describes the materialized source tree.
type Snapshot struct {
	Path     string `json:"path"`
	SourceID string `json:"source_id"`
	Commit   string `json:"commit"`
	Ref      string `json:"ref,omitempty"`
	URI      string `json:"uri,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// Repository describes the repository itself.
type Repository struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Readme      string `json:"readme,omitempty"`
	Host        string `json:"host,omitempty"`
	Owner       string `json:"owner,omitempty"`
	License     string `json:"license,omitempty"`
}

// Response is one source-provider reply.
type Response struct {
	V          string     `json:"v"`
	Type       string     `json:"type,omitempty"`
	Snapshot   Snapshot   `json:"snapshot"`
	Repository Repository `json:"repository"`
	Warnings   []string   `json:"warnings,omitempty"`
}
