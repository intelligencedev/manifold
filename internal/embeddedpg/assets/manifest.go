package assets

// RuntimeAsset is the PostgreSQL runtime payload embedded into a release
// binary for exactly one target platform.
type RuntimeAsset struct {
	RuntimeID   string
	OS          string
	Arch        string
	PGMajor     int
	ArchiveName string
	Archive     []byte
	Manifest    Manifest
}

// Manifest describes the exact runtime bundle contents and versions.
type Manifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	RuntimeID     string               `json:"runtimeID"`
	OS            string               `json:"os"`
	Arch          string               `json:"arch"`
	Postgres      PostgresManifest     `json:"postgres"`
	Extensions    map[string]string    `json:"extensions"`
	Dependencies  []DependencyManifest `json:"dependencies"`
	Files         []FileManifest       `json:"files"`
}

// PostgresManifest pins the bundled PostgreSQL version.
type PostgresManifest struct {
	Major   int    `json:"major"`
	Version string `json:"version"`
}

// DependencyManifest records native runtime dependencies bundled alongside
// PostgreSQL. The shape is intentionally small until per-platform builders need
// richer metadata.
type DependencyManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license,omitempty"`
}

// FileManifest records one checksummed file in the extracted runtime.
type FileManifest struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}
