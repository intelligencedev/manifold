// Package supertonic runs the Supertonic TTS pipeline in pure Go on top of the
// Born ONNX runtime fork. It ports the browser reference
// (web/agentd-ui/src/lib/tts/supertonic/vendor/helper.mjs) — tokenization,
// noise sampling, the flow-matching loop, and WAV assembly.
package supertonic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the fields the pipeline needs from tts.json.
type Config struct {
	SampleRate          int
	BaseChunkSize       int
	ChunkCompressFactor int
	LatentDim           int
}

func loadConfig(modelDir string) (Config, error) {
	raw, err := os.ReadFile(filepath.Join(modelDir, "onnx", "tts.json"))
	if err != nil {
		return Config{}, fmt.Errorf("read tts.json: %w", err)
	}
	var doc struct {
		AE struct {
			SampleRate    int `json:"sample_rate"`
			BaseChunkSize int `json:"base_chunk_size"`
		} `json:"ae"`
		TTL struct {
			ChunkCompressFactor int `json:"chunk_compress_factor"`
			LatentDim           int `json:"latent_dim"`
		} `json:"ttl"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Config{}, fmt.Errorf("parse tts.json: %w", err)
	}
	return Config{
		SampleRate:          doc.AE.SampleRate,
		BaseChunkSize:       doc.AE.BaseChunkSize,
		ChunkCompressFactor: doc.TTL.ChunkCompressFactor,
		LatentDim:           doc.TTL.LatentDim,
	}, nil
}

func loadIndexer(modelDir string) ([]int64, error) {
	raw, err := os.ReadFile(filepath.Join(modelDir, "onnx", "unicode_indexer.json"))
	if err != nil {
		return nil, fmt.Errorf("read unicode_indexer.json: %w", err)
	}
	var idx []int64
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse unicode_indexer.json: %w", err)
	}
	return idx, nil
}

// VoiceStyle holds the flattened TTL and DP style tensors and their shapes.
type VoiceStyle struct {
	TTLDims []int
	TTLData []float32
	DPDims  []int
	DPData  []float32
}

type styleTensor struct {
	Dims []int           `json:"dims"`
	Data json.RawMessage `json:"data"`
}

func flattenNumeric(raw json.RawMessage) ([]float32, error) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	var out []float32
	var walk func(node interface{})
	walk = func(node interface{}) {
		switch n := node.(type) {
		case float64:
			out = append(out, float32(n))
		case []interface{}:
			for _, c := range n {
				walk(c)
			}
		}
	}
	walk(v)
	return out, nil
}

func loadVoiceStyle(modelDir, voiceID string) (VoiceStyle, error) {
	raw, err := os.ReadFile(filepath.Join(modelDir, "voice_styles", voiceID+".json"))
	if err != nil {
		return VoiceStyle{}, fmt.Errorf("read voice %s: %w", voiceID, err)
	}
	var doc struct {
		TTL styleTensor `json:"style_ttl"`
		DP  styleTensor `json:"style_dp"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return VoiceStyle{}, fmt.Errorf("parse voice %s: %w", voiceID, err)
	}
	ttl, err := flattenNumeric(doc.TTL.Data)
	if err != nil {
		return VoiceStyle{}, fmt.Errorf("voice %s ttl: %w", voiceID, err)
	}
	dp, err := flattenNumeric(doc.DP.Data)
	if err != nil {
		return VoiceStyle{}, fmt.Errorf("voice %s dp: %w", voiceID, err)
	}
	return VoiceStyle{TTLDims: doc.TTL.Dims, TTLData: ttl, DPDims: doc.DP.Dims, DPData: dp}, nil
}
