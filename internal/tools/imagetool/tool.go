package imagetool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/llm"
	"manifold/internal/llm/openai"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
)

// ProviderFactory allows building a provider with an alternate base URL.
type ProviderFactory func(baseURL string) llm.Provider

type DescribeTool struct {
	Provider       llm.Provider
	Workdir        string
	DefaultModel   string
	NewWithBaseURL ProviderFactory
}

func NewDescribeTool(p llm.Provider, workdir, defaultModel string, f ProviderFactory) *DescribeTool {
	return &DescribeTool{Provider: p, Workdir: workdir, DefaultModel: defaultModel, NewWithBaseURL: f}
}

func (t *DescribeTool) Name() string { return "describe_image" }

func (t *DescribeTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Describe an image file located under the locked working directory. The image will be sent to the LLM as an inline data URL.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":     map[string]any{"type": "string", "description": "Relative path to an image file under WORKDIR (e.g., images/photo.jpg)"},
				"prompt":   map[string]any{"type": "string", "description": "Optional additional instruction or question about the image"},
				"model":    map[string]any{"type": "string", "description": "Optional model override"},
				"base_url": map[string]any{"type": "string", "description": "Optional API base URL override"},
			},
			"required": []string{"path"},
		},
	}
}

func (t *DescribeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Path    string `json:"path"`
		Prompt  string `json:"prompt"`
		Model   string `json:"model"`
		BaseURL string `json:"base_url"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	rel, err := sandbox.SanitizeArg(t.Workdir, args.Path)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	full := filepath.Join(t.Workdir, rel)
	f, err := os.Open(full)
	if err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("open: %v", err)}, nil
	}
	defer f.Close()

	// Read a bounded prefix to sniff content type, then the rest
	hdr := make([]byte, 512)
	n, _ := io.ReadFull(f, hdr)
	// reset to beginning
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	content, err := io.ReadAll(f)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	mime := http.DetectContentType(hdr[:n])

	// Try to decode and resize the image in memory so the larger dimension is 512px.
	// If decoding or encoding fails, fall back to the original bytes.
	resizedBytes := content
	if strings.HasPrefix(mime, "image/") {
		if img, format, err := image.Decode(bytes.NewReader(content)); err == nil {
			w := img.Bounds().Dx()
			h := img.Bounds().Dy()
			// compute target size so the smaller dimension becomes 512
			var tw, th int
			if w <= h {
				// width is the smaller dimension -> set width to 512
				tw = 512
				th = int(float64(h) * (512.0 / float64(w)))
				if th < 1 {
					th = 1
				}
			} else {
				// height is the smaller dimension -> set height to 512
				th = 512
				tw = int(float64(w) * (512.0 / float64(h)))
				if tw < 1 {
					tw = 1
				}
			}

			// If already the required size, skip resizing
			if !(w == tw && h == th) {
				dst := image.NewRGBA(image.Rect(0, 0, tw, th))
				// Use a simple nearest-neighbor scale to avoid an external dependency.
				nearestNeighborScale(dst, img)

				var buf bytes.Buffer
				// Try to preserve original format when encoding; fallback to PNG for GIF/unknown
				switch strings.ToLower(format) {
				case "jpeg", "jpg":
					_ = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85})
					mime = "image/jpeg"
				case "png":
					_ = png.Encode(&buf, dst)
					mime = "image/png"
				case "gif":
					// gif.Encode will attempt to quantize; encode as GIF but if it fails, fall back to PNG
					if err := gif.Encode(&buf, dst, nil); err != nil {
						_ = png.Encode(&buf, dst)
						mime = "image/png"
					} else {
						mime = "image/gif"
					}
				default:
					// unknown: encode as PNG
					_ = png.Encode(&buf, dst)
					mime = "image/png"
				}
				// only replace resizedBytes if encoding succeeded
				if buf.Len() > 0 {
					resizedBytes = buf.Bytes()
				}
			}
		}
	}

	b64 := base64.StdEncoding.EncodeToString(resizedBytes)

	// Build messages
	sys := "You are a helpful image understanding assistant. Answer concisely and describe visual details, objects, colors, text, and any notable attributes."
	userContent := ""
	if args.Prompt != "" {
		userContent = args.Prompt + "\n\n"
	} else {
		userContent = "Describe the image below in plain text. Include objects, colors, scene, and any readable text."
	}
	// Use markdown image data URL so the assistant can detect an image in the text
	userContent = userContent + "\n\n![image](data:" + mime + ";base64," + b64 + ")\n"

	msgs := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: userContent}}

	// Prefer provider from context if the caller (agent/specialist) propagated one.
	p := t.Provider
	if ctxProvider := tools.ProviderFromContext(ctx); ctxProvider != nil {
		p = ctxProvider
	}
	// If caller requested a baseURL override, build a new provider using the
	// supplied factory so the tool uses the correct base URL and headers.
	if args.BaseURL != "" && t.NewWithBaseURL != nil {
		if np := t.NewWithBaseURL(args.BaseURL); np != nil {
			p = np
		}
	}
	model := args.Model
	// Don't fallback to t.DefaultModel - let the provider use its own default model
	// This ensures that when providers are propagated from context (specialists/agents),
	// tools use the same model as the invoking agent/specialist.

	// Try to use OpenAI-specific image attachment method if available
	if openaiClient, ok := p.(*openai.Client); ok {
		out, err := openaiClient.ChatWithImageAttachment(ctx, msgs, mime, b64, nil, model)
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}, nil
		}
		return map[string]any{"ok": true, "output": out.Content}, nil
	}

	// Fallback to original data URL method for other providers
	out, err := p.Chat(ctx, msgs, nil, model)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "output": out.Content}, nil
}

// nearestNeighborScale scales src into dst using nearest-neighbor sampling.
// dst must already be allocated with the target bounds.
func nearestNeighborScale(dst *image.RGBA, src image.Image) {
	sw := src.Bounds().Dx()
	sh := src.Bounds().Dy()
	dw := dst.Bounds().Dx()
	dh := dst.Bounds().Dy()

	for y := 0; y < dh; y++ {
		// compute source y
		sy := int(float64(y) * float64(sh) / float64(dh))
		if sy >= sh {
			sy = sh - 1
		}
		for x := 0; x < dw; x++ {
			sx := int(float64(x) * float64(sw) / float64(dw))
			if sx >= sw {
				sx = sw - 1
			}
			c := src.At(src.Bounds().Min.X+sx, src.Bounds().Min.Y+sy)
			dst.Set(x+dst.Bounds().Min.X, y+dst.Bounds().Min.Y, c)
		}
	}
}
