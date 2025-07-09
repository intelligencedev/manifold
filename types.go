package main

// Model represents a language model, either gguf or mlx.
type LanguageModel struct {
	ID                int64   `gorm:"primaryKey" json:"id"`
	Name              string  `gorm:"uniqueIndex:idx_name_type" json:"name"`       // Model name
	Path              string  `gorm:"uniqueIndex:idx_name_type" json:"path"`       // Full path to the model file
	ModelType         string  `gorm:"uniqueIndex:idx_name_type" json:"model_type"` // "gguf" or "mlx"
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int     `json:"top_k"`
	RepetitionPenalty float64 `json:"repetition_penalty"`
	Ctx               int     `json:"ctx"`
}

// FileData represents a single file's metadata and content.
type FileData struct {
	Path    string `json:"path"`    // File path relative to the repository root.
	Content string `json:"content"` // Full file content.
}

// ProcessTextRequest defines the JSON structure for the text processing request in the rad pipeline
type ProcessTextRequest struct {
	Text         string `json:"text"`
	Language     string `json:"language"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
	FilePath     string `json:"file_path"`
}
