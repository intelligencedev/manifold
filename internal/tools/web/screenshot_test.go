package web

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScreenshotScrollOffsets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pageHeight     int
		viewportHeight int
		want           []int
	}{
		{name: "invalid dimensions", pageHeight: 0, viewportHeight: 100, want: []int{0}},
		{name: "page fits viewport", pageHeight: 100, viewportHeight: 200, want: []int{0}},
		{name: "exact slices", pageHeight: 300, viewportHeight: 100, want: []int{0, 100, 200}},
		{name: "overlapping final slice", pageHeight: 250, viewportHeight: 100, want: []int{0, 100, 150}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, screenshotScrollOffsets(test.pageHeight, test.viewportHeight))
		})
	}
}

func TestStitchViewportScreenshots(t *testing.T) {
	t.Parallel()

	metrics := pageScreenshotMetrics{Height: 250, ViewportHeight: 100}
	captures := []capturedViewport{
		{Image: solidImage(10, 100, color.RGBA{R: 0xff, A: 0xff}), ScrollY: 0},
		{Image: solidImage(10, 100, color.RGBA{G: 0xff, A: 0xff}), ScrollY: 100},
		{Image: solidImage(10, 100, color.RGBA{B: 0xff, A: 0xff}), ScrollY: 150},
	}

	stitched, err := stitchViewportScreenshots(metrics, captures)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 10, 250), stitched.Bounds())
	require.Equal(t, color.RGBA{R: 0xff, A: 0xff}, stitched.RGBAAt(5, 50))
	require.Equal(t, color.RGBA{G: 0xff, A: 0xff}, stitched.RGBAAt(5, 125))
	require.Equal(t, color.RGBA{B: 0xff, A: 0xff}, stitched.RGBAAt(5, 175))
	require.Equal(t, color.RGBA{B: 0xff, A: 0xff}, stitched.RGBAAt(5, 249))
}

func TestScreenshotTempDirIsProjectLocal(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	require.Equal(t, filepath.Join(base, ".tmp", "web-screenshot"), screenshotTempDir(base))
}

func solidImage(width, height int, fill color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, fill)
		}
	}
	return img
}
