package main

// RepoConcatRequest represents the request body for the /api/repoconcat endpoint.
type RepoConcatRequest struct {
	Paths         []string `json:"paths"`
	Types         []string `json:"types"`
	Recursive     bool     `json:"recursive"`
	IgnorePattern string   `json:"ignorePattern"`
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
}
