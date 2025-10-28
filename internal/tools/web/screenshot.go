package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"

	"manifold/internal/sandbox"
)

type screenshotTool struct{}

// NewScreenshotTool constructs the web_screenshot tool.
func NewScreenshotTool() *screenshotTool { return &screenshotTool{} }

func (t *screenshotTool) Name() string { return "web_screenshot" }

func (t *screenshotTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Take a PNG screenshot of a web page using a real Chrome/Chromium browser controlled by chromedp. Returns base64-encoded PNG.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":             map[string]any{"type": "string", "description": "Absolute URL to capture (http or https)."},
				"width":           map[string]any{"type": "integer", "minimum": 1920, "maximum": 8192, "default": 2560},
				"height":          map[string]any{"type": "integer", "minimum": 1080, "maximum": 8192, "default": 1440},
				"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 120, "default": 15},
				"full_page":       map[string]any{"type": "boolean", "description": "If true (default), capture full page; otherwise capture viewport area."},
				"output_path":     map[string]any{"type": "string", "description": "Filesystem path (relative to current project) to save PNG. Defaults to web_screenshot.png.", "default": "web_screenshot.png"},
			},
			"required": []string{"url"},
		},
	}
}

func (t *screenshotTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		URL           string `json:"url"`
		Width         int    `json:"width"`
		Height        int    `json:"height"`
		TimeoutSecond int    `json:"timeout_seconds"`
		FullPage      *bool  `json:"full_page"`
		OutputPath    string `json:"output_path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if args.URL == "" {
		return map[string]any{"ok": false, "error": "missing url"}, nil
	}
	if args.Width <= 0 {
		args.Width = 1280
	}
	if args.Height <= 0 {
		args.Height = 800
	}
	timeout := 15 * time.Second
	if args.TimeoutSecond > 0 {
		timeout = time.Duration(args.TimeoutSecond) * time.Second
	}
	full := true
	if args.FullPage != nil {
		full = *args.FullPage
	}

	// Enforce that tools run inside a selected project by requiring a base dir
	// set on the context (see sandbox.WithBaseDir). This prevents writing
	// outside the current project's filesystem area.
	base, ok := sandbox.BaseDirFromContext(ctx)
	if !ok || base == "" {
		return map[string]any{"ok": false, "error": "no project base directory in context; screenshot tool must run inside a project"}, nil
	}

	// Create an allocator and browser context with timeout. Launch a real (non-headless)
	// browser by overriding the headless flag. Optionally set the browser executable
	// from the CHROME_PATH environment variable.
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("start-maximized", true),
		// Request fullscreen/kiosk where supported so the browser opens maximized/fullscreen
		chromedp.Flag("start-fullscreen", true),
		chromedp.Flag("kiosk", true),
		chromedp.Flag("window-size", fmt.Sprintf("%d,%d", args.Width, args.Height)),
	)
	if p := os.Getenv("CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	runCtx, cancelRun := context.WithTimeout(browserCtx, timeout)
	defer cancelRun()

	var png []byte
	var err error

	tasks := chromedp.Tasks{
		chromedp.EmulateViewport(int64(args.Width), int64(args.Height)),
		chromedp.Navigate(args.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	}
	if full {
		// Full page screenshot
		tasks = append(tasks, chromedp.FullScreenshot(&png, 100))
	} else {
		tasks = append(tasks, chromedp.CaptureScreenshot(&png))
	}

	if err = chromedp.Run(runCtx, tasks); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}

	// Save to disk. Default to web_screenshot.png if not provided. Only allow
	// paths that remain under the project's base directory.
	out := args.OutputPath
	if out == "" {
		out = "web_screenshot.png"
	}
	// Sanitize and ensure the output path remains under base
	rel, err := sandbox.SanitizeArg(base, out)
	if err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid output_path: %v", err)}, nil
	}
	fullPath := filepath.Clean(filepath.Join(base, rel))
	// Ensure destination directory exists
	if dir := filepath.Dir(fullPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return map[string]any{"ok": false, "error": fmt.Sprintf("failed to create dirs: %v", err)}, nil
		}
	}
	if err := os.WriteFile(fullPath, png, 0o644); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("failed to write file: %v", err)}, nil
	}

	b64 := base64.StdEncoding.EncodeToString(png)
	return map[string]any{"ok": true, "content_type": "image/png", "png_base64": b64, "size": len(png), "path": fullPath}, nil
}
