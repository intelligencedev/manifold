package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"manifold/internal/sandbox"
)

type screenshotTool struct{}

const (
	defaultScreenshotWidth  = 2560
	defaultScreenshotHeight = 1440
)

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
				"width":           map[string]any{"type": "integer", "minimum": 1920, "maximum": 8192, "default": defaultScreenshotWidth},
				"height":          map[string]any{"type": "integer", "minimum": 1080, "maximum": 8192, "default": defaultScreenshotHeight},
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
		args.Width = defaultScreenshotWidth
	}
	if args.Height <= 0 {
		args.Height = defaultScreenshotHeight
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
		chromedp.ActionFunc(func(ctx context.Context) error {
			windowID, _, err := browser.GetWindowForTarget().Do(ctx)
			if err != nil {
				return err
			}
			return browser.SetWindowBounds(windowID, &browser.Bounds{WindowState: browser.WindowStateFullscreen}).Do(ctx)
		}),
		chromedp.EmulateViewport(int64(args.Width), int64(args.Height)),
		chromedp.Navigate(args.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	}
	if full {
		tasks = append(tasks, captureFullPageScreenshot(&png, args.Height))
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

type pageScreenshotMetrics struct {
	Width          int `json:"width"`
	Height         int `json:"height"`
	ViewportWidth  int `json:"viewportWidth"`
	ViewportHeight int `json:"viewportHeight"`
}

type capturedViewport struct {
	Image   image.Image
	ScrollY int
}

func captureFullPageScreenshot(res *[]byte, fallbackViewportHeight int) chromedp.Action {
	if res == nil {
		panic("res cannot be nil")
	}

	return chromedp.ActionFunc(func(ctx context.Context) error {
		metrics, err := measurePageScreenshotMetrics(ctx)
		if err != nil {
			return err
		}
		if metrics.ViewportHeight <= 0 {
			metrics.ViewportHeight = fallbackViewportHeight
		}
		if metrics.Width <= 0 || metrics.Height <= 0 || metrics.ViewportHeight <= 0 {
			buf, err := page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatPng).
				WithFromSurface(true).
				Do(ctx)
			if err != nil {
				return err
			}
			*res = buf
			return nil
		}

		captures := make([]capturedViewport, 0, int(math.Ceil(float64(metrics.Height)/float64(metrics.ViewportHeight))))
		for _, offset := range screenshotScrollOffsets(metrics.Height, metrics.ViewportHeight) {
			scrollY, err := scrollPageTo(ctx, offset)
			if err != nil {
				return err
			}
			buf, err := page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatPng).
				WithFromSurface(true).
				Do(ctx)
			if err != nil {
				return err
			}

			viewportImage, err := png.Decode(bytes.NewReader(buf))
			if err != nil {
				return fmt.Errorf("decode viewport screenshot: %w", err)
			}
			captures = append(captures, capturedViewport{Image: viewportImage, ScrollY: scrollY})
		}

		stitched, err := stitchViewportScreenshots(metrics, captures)
		if err != nil {
			return err
		}
		var output bytes.Buffer
		if err := png.Encode(&output, stitched); err != nil {
			return fmt.Errorf("encode stitched screenshot: %w", err)
		}
		*res = output.Bytes()
		_, _ = scrollPageTo(ctx, 0)
		return nil
	})
}

func measurePageScreenshotMetrics(ctx context.Context) (pageScreenshotMetrics, error) {
	var metrics pageScreenshotMetrics
	err := chromedp.Evaluate(`(() => {
		const doc = document.documentElement;
		const body = document.body || {};
		return {
			width: Math.ceil(Math.max(doc.scrollWidth, doc.offsetWidth, doc.clientWidth, body.scrollWidth || 0, body.offsetWidth || 0, body.clientWidth || 0, window.innerWidth || 0)),
			height: Math.ceil(Math.max(doc.scrollHeight, doc.offsetHeight, doc.clientHeight, body.scrollHeight || 0, body.offsetHeight || 0, body.clientHeight || 0, window.innerHeight || 0)),
			viewportWidth: Math.ceil(window.innerWidth || doc.clientWidth || 0),
			viewportHeight: Math.ceil(window.innerHeight || doc.clientHeight || 0),
		};
	})()`, &metrics).Do(ctx)
	return metrics, err
}

func scrollPageTo(ctx context.Context, offset int) (int, error) {
	var scrollY int
	err := chromedp.Evaluate(fmt.Sprintf(`new Promise(resolve => {
		window.scrollTo(0, %d);
		requestAnimationFrame(() => requestAnimationFrame(() => {
			resolve(Math.round(window.scrollY || document.documentElement.scrollTop || document.body.scrollTop || 0));
		}));
	})`, offset), &scrollY, func(params *runtime.EvaluateParams) *runtime.EvaluateParams {
		return params.WithAwaitPromise(true)
	}).Do(ctx)
	return scrollY, err
}

func screenshotScrollOffsets(pageHeight, viewportHeight int) []int {
	if pageHeight <= 0 || viewportHeight <= 0 {
		return []int{0}
	}
	if pageHeight <= viewportHeight {
		return []int{0}
	}

	lastOffset := pageHeight - viewportHeight
	offsets := make([]int, 0, int(math.Ceil(float64(pageHeight)/float64(viewportHeight))))
	for offset := 0; offset < pageHeight; offset += viewportHeight {
		if offset > lastOffset {
			offset = lastOffset
		}
		if len(offsets) == 0 || offsets[len(offsets)-1] != offset {
			offsets = append(offsets, offset)
		}
		if offset == lastOffset {
			break
		}
	}
	return offsets
}

func stitchViewportScreenshots(metrics pageScreenshotMetrics, captures []capturedViewport) (*image.RGBA, error) {
	if metrics.Height <= 0 {
		return nil, fmt.Errorf("invalid page height %d", metrics.Height)
	}
	if metrics.ViewportHeight <= 0 {
		return nil, fmt.Errorf("invalid viewport height %d", metrics.ViewportHeight)
	}
	if len(captures) == 0 {
		return nil, fmt.Errorf("no viewport screenshots captured")
	}

	firstBounds := captures[0].Image.Bounds()
	if firstBounds.Dx() <= 0 || firstBounds.Dy() <= 0 {
		return nil, fmt.Errorf("invalid viewport screenshot dimensions %dx%d", firstBounds.Dx(), firstBounds.Dy())
	}
	scale := float64(firstBounds.Dy()) / float64(metrics.ViewportHeight)
	targetWidth := firstBounds.Dx()
	targetHeight := int(math.Ceil(float64(metrics.Height) * scale))
	stitched := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	for _, capture := range captures {
		bounds := capture.Image.Bounds()
		if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
			return nil, fmt.Errorf("invalid viewport screenshot dimensions %dx%d", bounds.Dx(), bounds.Dy())
		}
		destinationY := int(math.Round(float64(capture.ScrollY) * scale))
		if destinationY >= targetHeight {
			continue
		}
		copyHeight := min(bounds.Dy(), targetHeight-destinationY)
		destination := image.Rect(0, destinationY, min(targetWidth, bounds.Dx()), destinationY+copyHeight)
		source := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+destination.Dx(), bounds.Min.Y+copyHeight)
		draw.Draw(stitched, destination, capture.Image, source.Min, draw.Src)
	}

	return stitched, nil
}
