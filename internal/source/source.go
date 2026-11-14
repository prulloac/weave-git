// Package source defines the weave.source/v1 wire types.
package source

const ProtocolVersion = "weave.source/v1"

// Options controls one source materialization.
type Options struct {
	Worktree        bool           `json:"worktree,omitempty"`
	Submodules      bool           `json:"submodules,omitempty"`
	LFS             bool           `json:"lfs,omitempty"`
	Depth           int            `json:"depth,omitempty"`
	Permissive      bool           `json:"permissive,omitempty"`
	History         bool           `json:"history,omitempty"`
	HistoryLimit    int            `json:"history_limit,omitempty"`
	ProviderOptions map[string]any `json:"provider_options,omitempty"`
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
	Revision string `json:"revision,omitempty"`
	Commit   string `json:"commit,omitempty"`
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
	HostKind    string `json:"host_kind,omitempty"`
	WebURL      string `json:"web_url,omitempty"`
	CommitURL   string `json:"commit_url,omitempty"`
}

// Project describes one manifest-backed project in the snapshot.
type Project struct {
	Path         string       `json:"path"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Manifest     string       `json:"manifest"`
	Ecosystem    string       `json:"ecosystem"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
}

// Dependency describes one declared project dependency.
type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

// Commit is one commit in the opt-in history window.
type Commit struct {
	SHA     string   `json:"sha"`
	Parents []string `json:"parents,omitempty"`
	Author  string   `json:"author,omitempty"`
	Date    string   `json:"date,omitempty"`
	Subject string   `json:"subject,omitempty"`
}

// Document is one generic synthetic graph document.
type Document struct {
	ID          string         `json:"id"`
	Path        string         `json:"path,omitempty"`
	Type        string         `json:"type"`
	ContentKind string         `json:"content_kind,omitempty"`
	MediaType   string         `json:"media_type,omitempty"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	Body        string         `json:"body,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Scope       *string        `json:"scope,omitempty"`
	Consumes    []string       `json:"consumes,omitempty"`
}

// Relation is one generic synthetic graph relation.
type Relation struct {
	From   string  `json:"from"`
	To     string  `json:"to,omitempty"`
	ToPath string  `json:"to_path,omitempty"`
	Kind   string  `json:"kind"`
	Raw    *string `json:"raw,omitempty"`
}

// Artifact is one extension-owned export artifact.
type Artifact struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type,omitempty"`
	Content   any    `json:"content"`
}

// Response is one source-provider reply.
type Response struct {
	V              string         `json:"v"`
	Type           string         `json:"type,omitempty"`
	Snapshot       Snapshot       `json:"snapshot"`
	Repository     Repository     `json:"repository"`
	Projects       []Project      `json:"projects,omitempty"`
	Commits        []Commit       `json:"commits,omitempty"`
	Documents      []Document     `json:"documents,omitempty"`
	Relations      []Relation     `json:"relations,omitempty"`
	Artifacts      []Artifact     `json:"artifacts,omitempty"`
	SourceMetadata map[string]any `json:"source_metadata,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}
