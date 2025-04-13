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

// DatadogNodeRequest represents the structure of the incoming request from the frontend.
type DatadogNodeRequest struct {
	APIKey    string `json:"apiKey"`
	AppKey    string `json:"appKey"`
	Site      string `json:"site"`
	Operation string `json:"operation"`
	Query     string `json:"query"`
	FromTime  string `json:"fromTime"`
	ToTime    string `json:"toTime"`
}

// DatadogNodeResponse represents the structure of the response sent back to the frontend.
type DatadogNodeResponse struct {
	Result struct {
		Output interface{} `json:"output"`
	} `json:"result"`
}

// SDRequest defines the JSON structure for the stable diffusion request.
type SDRequest struct {
	DiffusionModel  string  `json:"diffusion_model"`
	Type            string  `json:"type"`
	ClipL           string  `json:"clip_l"`
	T5xxl           string  `json:"t5xxl"`
	VAE             string  `json:"vae"`
	CfgScale        float64 `json:"cfg_scale"`
	Steps           int     `json:"steps"`
	SamplingMethod  string  `json:"sampling_method"`
	Height          int     `json:"height"`
	Width           int     `json:"width"`
	Seed            int     `json:"seed"`
	Prompt          string  `json:"prompt"`
	Output          string  `json:"output"`
	Threads         int     `json:"threads,omitempty"`
	NegativePrompt  string  `json:"negative_prompt,omitempty"`
	StyleRatio      float64 `json:"style_ratio,omitempty"`
	ControlStrength float64 `json:"control_strength,omitempty"`
	ClipSkip        int     `json:"clip_skip,omitempty"`
	SLGScale        float64 `json:"slg_scale,omitempty"`
	SkipLayers      []int   `json:"skip_layers,omitempty"`
	SkipLayerStart  float64 `json:"skip_layer_start,omitempty"`
	SkipLayerEnd    float64 `json:"skip_layer_end,omitempty"`
	// Add other parameters as needed, matching the `sd` command's flags.
}

// FMLXRequest defines the JSON structure for the FMLX request.
type FMLXRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Steps   int    `json:"steps"`
	Seed    int    `json:"seed"`
	Quality int    `json:"quality"`
	Output  string `json:"output"`
}

// ProcessTextRequest defines the JSON structure for the text processing request in the rad pipeline
type ProcessTextRequest struct {
	Text         string `json:"text"`
	Language     string `json:"language"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
	FilePath     string `json:"file_path"`
}
