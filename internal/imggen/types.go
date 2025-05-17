package imggen

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
