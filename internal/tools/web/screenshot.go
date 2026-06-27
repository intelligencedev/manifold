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
	var args screenshotArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if args.URL == "" {
		return map[string]any{"ok": false, "error": "missing url"}, nil
	}
	args = args.withDefaults()

	base, ok := sandbox.BaseDirFromContext(ctx)
	if !ok || base == "" {
		return map[string]any{"ok": false, "error": "no project base directory in context; screenshot tool must run inside a project"}, nil
	}

	png, err := capturePageScreenshot(ctx, base, args)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	path, err := saveScreenshot(base, args.OutputPath, png)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}

	b64 := base64.StdEncoding.EncodeToString(png)
	return map[string]any{"ok": true, "content_type": "image/png", "png_base64": b64, "size": len(png), "path": path}, nil
}

type screenshotArgs struct {
	URL           string `json:"url"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	TimeoutSecond int    `json:"timeout_seconds"`
	FullPage      *bool  `json:"full_page"`
	OutputPath    string `json:"output_path"`
}

func (a screenshotArgs) withDefaults() screenshotArgs {
	if a.Width <= 0 {
		a.Width = defaultScreenshotWidth
	}
	if a.Height <= 0 {
		a.Height = defaultScreenshotHeight
	}
	return a
}

func (a screenshotArgs) timeout() time.Duration {
	if a.TimeoutSecond > 0 {
		return time.Duration(a.TimeoutSecond) * time.Second
	}
	return 15 * time.Second
}

func (a screenshotArgs) fullPage() bool {
	return a.FullPage == nil || *a.FullPage
}

func capturePageScreenshot(ctx context.Context, base string, args screenshotArgs) ([]byte, error) {
	if err := os.MkdirAll(screenshotTempDir(base), 0o755); err != nil {
		return nil, fmt.Errorf("create screenshot temp dir: %w", err)
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, screenshotAllocatorOptions(base, args)...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	runCtx, cancelRun := context.WithTimeout(browserCtx, args.timeout())
	defer cancelRun()

	var png []byte
	tasks := screenshotTasks(args, &png)
	if err := chromedp.Run(runCtx, tasks); err != nil {
		return nil, err
	}
	return png, nil
}

func screenshotAllocatorOptions(base string, args screenshotArgs) []chromedp.ExecAllocatorOption {
	tmpDir := screenshotTempDir(base)
	profileDir := filepath.Join(tmpDir, "chrome-profile")
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Env("TMPDIR="+tmpDir, "TEMP="+tmpDir, "TMP="+tmpDir),
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("start-maximized", true),
		chromedp.Flag("start-fullscreen", true),
		chromedp.Flag("kiosk", true),
		chromedp.Flag("window-size", fmt.Sprintf("%d,%d", args.Width, args.Height)),
	)
	if p := os.Getenv("CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	return opts
}

func screenshotTempDir(base string) string {
	return filepath.Join(base, ".tmp", "web-screenshot")
}

func screenshotTasks(args screenshotArgs, png *[]byte) chromedp.Tasks {
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
	if args.fullPage() {
		return append(tasks, captureFullPageScreenshot(png, args.Height))
	}
	return append(tasks, chromedp.CaptureScreenshot(png))
}

func saveScreenshot(base, outputPath string, png []byte) (string, error) {
	out := outputPath
	if out == "" {
		out = "web_screenshot.png"
	}
	rel, err := sandbox.SanitizeArg(base, out)
	if err != nil {
		return "", fmt.Errorf("invalid output_path: %v", err)
	}
	fullPath := filepath.Clean(filepath.Join(base, rel))
	if dir := filepath.Dir(fullPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create dirs: %v", err)
		}
	}
	if err := os.WriteFile(fullPath, png, 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}
	return fullPath, nil
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
		if _, err := scrollPageTo(ctx, 0); err != nil {
			return fmt.Errorf("reset scroll after stitched screenshot: %w", err)
		}
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
