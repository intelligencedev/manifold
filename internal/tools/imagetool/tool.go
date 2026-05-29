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
	DefaultBaseURL string
	NewWithBaseURL ProviderFactory
}

func NewDescribeTool(p llm.Provider, workdir, defaultModel, defaultBaseURL string, f ProviderFactory) *DescribeTool {
	return &DescribeTool{Provider: p, Workdir: workdir, DefaultModel: defaultModel, DefaultBaseURL: defaultBaseURL, NewWithBaseURL: f}
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
	var args describeImageArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	imageData, err := readDescribeImage(ctx, t.Workdir, args.Path)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}

	sys := "You are a helpful image understanding assistant. Answer concisely and describe visual details, objects, colors, text, and any notable attributes."
	userContent := describeImagePrompt(args.Prompt)
	msgs := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: userContent}}
	p := t.provider(ctx, args.BaseURL)
	model := strings.TrimSpace(args.Model)
	if model == "" {
		model = strings.TrimSpace(t.DefaultModel)
	}

	if openaiClient, ok := p.(*openai.Client); ok {
		out, err := openaiClient.ChatWithImageAttachment(ctx, msgs, imageData.mime, imageData.base64, nil, model)
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}, nil
		}
		return map[string]any{"ok": true, "output": out.Content}, nil
	}

	msgs[1].Content = userContent + "\n\n![image](data:" + imageData.mime + ";base64," + imageData.base64 + ")\n"
	out, err := p.Chat(ctx, msgs, nil, model)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "output": out.Content}, nil
}

type describeImageArgs struct {
	Path    string `json:"path"`
	Prompt  string `json:"prompt"`
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
}

type describeImageData struct {
	mime   string
	base64 string
}

func readDescribeImage(ctx context.Context, workdir, path string) (describeImageData, error) {
	base := sandbox.ResolveBaseDir(ctx, workdir)
	rel, err := sandbox.SanitizeArg(base, path)
	if err != nil {
		return describeImageData{}, err
	}
	content, mime, err := readImageFile(filepath.Join(base, rel))
	if err != nil {
		return describeImageData{}, err
	}
	resized, mime := resizeDescribeImage(content, mime)
	return describeImageData{mime: mime, base64: base64.StdEncoding.EncodeToString(resized)}, nil
}

func readImageFile(full string) ([]byte, string, error) {
	f, err := os.Open(full)
	if err != nil {
		return nil, "", fmt.Errorf("open: %v", err)
	}
	defer f.Close()
	hdr := make([]byte, 512)
	n, _ := io.ReadFull(f, hdr)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	content, err := io.ReadAll(f)
	if err != nil {
		return nil, "", err
	}
	return content, http.DetectContentType(hdr[:n]), nil
}

func resizeDescribeImage(content []byte, mime string) ([]byte, string) {
	if !strings.HasPrefix(mime, "image/") {
		return content, mime
	}
	img, format, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return content, mime
	}
	width, height := describeImageTargetSize(img.Bounds().Dx(), img.Bounds().Dy())
	if img.Bounds().Dx() == width && img.Bounds().Dy() == height {
		return content, mime
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	nearestNeighborScale(dst, img)
	if encoded, encodedMime := encodeDescribeImage(dst, format); len(encoded) > 0 {
		return encoded, encodedMime
	}
	return content, mime
}

func describeImageTargetSize(width, height int) (int, int) {
	if width <= height {
		return 512, max(int(float64(height)*(512.0/float64(width))), 1)
	}
	return max(int(float64(width)*(512.0/float64(height))), 1), 512
}

func encodeDescribeImage(img image.Image, format string) ([]byte, string) {
	var buf bytes.Buffer
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
		return buf.Bytes(), "image/jpeg"
	case "png":
		_ = png.Encode(&buf, img)
		return buf.Bytes(), "image/png"
	case "gif":
		if err := gif.Encode(&buf, img, nil); err == nil {
			return buf.Bytes(), "image/gif"
		}
	}
	buf.Reset()
	_ = png.Encode(&buf, img)
	return buf.Bytes(), "image/png"
}

func describeImagePrompt(prompt string) string {
	if prompt != "" {
		return prompt + "\n\n"
	}
	return "Describe the image below in plain text. Include objects, colors, scene, and any readable text."
}

func (t *DescribeTool) provider(ctx context.Context, baseURL string) llm.Provider {
	p := t.Provider
	if ctxProvider := tools.ProviderFromContext(ctx); ctxProvider != nil {
		p = ctxProvider
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(t.DefaultBaseURL)
	}
	if baseURL != "" && t.NewWithBaseURL != nil {
		if np := t.NewWithBaseURL(baseURL); np != nil {
			p = np
		}
	}
	return p
}

// nearestNeighborScale scales src into dst using nearest-neighbor sampling.
// dst must already be allocated with the target bounds.
func nearestNeighborScale(dst *image.RGBA, src image.Image) {
	sw := src.Bounds().Dx()
	sh := src.Bounds().Dy()
	dw := dst.Bounds().Dx()
	dh := dst.Bounds().Dy()

	for y := range dh {
		// compute source y
		sy := int(float64(y) * float64(sh) / float64(dh))
		if sy >= sh {
			sy = sh - 1
		}
		for x := range dw {
			sx := int(float64(x) * float64(sw) / float64(dw))
			if sx >= sw {
				sx = sw - 1
			}
			c := src.At(src.Bounds().Min.X+sx, src.Bounds().Min.Y+sy)
			dst.Set(x+dst.Bounds().Min.X, y+dst.Bounds().Min.Y, c)
		}
	}
}
