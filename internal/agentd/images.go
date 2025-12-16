package agentd

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/llm"
	"manifold/internal/sandbox"
)

type savedImage struct {
	Name     string
	MIME     string
	DataURL  string
	RelPath  string
	FullPath string
	URL      string
}

func saveGeneratedImages(baseDir string, imgs []llm.GeneratedImage, projectID string) []savedImage {
	out := make([]savedImage, 0, len(imgs))
	if len(imgs) == 0 {
		return out
	}
	baseDir = strings.TrimSpace(baseDir)
	for idx, img := range imgs {
		mimeType := strings.TrimSpace(img.MIMEType)
		if mimeType == "" {
			mimeType = "image/png"
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(img.Data))
		entry := savedImage{
			MIME:    mimeType,
			DataURL: dataURL,
		}

		if baseDir != "" {
			ext := ".png"
			if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
				ext = exts[0]
			}
			filename := fmt.Sprintf("generated_image_%d%s", time.Now().UnixNano()+int64(idx), ext)
			relCandidate := filepath.Join("images", filename)
			rel, err := sandbox.SanitizeArg(baseDir, relCandidate)
			if err != nil {
				log.Error().Err(err).Str("candidate", relCandidate).Msg("save_generated_image_sanitize")
			} else {
				full := filepath.Join(baseDir, rel)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					log.Error().Err(err).Str("path", full).Msg("save_generated_image_mkdir")
				} else if err := os.WriteFile(full, img.Data, 0o644); err != nil {
					log.Error().Err(err).Str("path", full).Msg("save_generated_image_write")
				} else {
					entry.RelPath = rel
					entry.FullPath = full
					entry.Name = filepath.Base(rel)
					entry.URL = projectFileURL(projectID, rel)
				}
			}
		} else {
			log.Warn().Msg("no base directory provided; skipping image file save")
		}

		if entry.Name == "" {
			entry.Name = fmt.Sprintf("image-%d", idx+1)
		}
		out = append(out, entry)
	}
	return out
}

func projectFileURL(projectID, relPath string) string {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(relPath) == "" {
		return ""
	}
	qp := url.Values{}
	qp.Set("path", relPath)
	return "/api/projects/" + url.PathEscape(projectID) + "/files?" + qp.Encode()
}

func appendImageSummary(content string, imgs []savedImage) string {
	if len(imgs) == 0 {
		return content
	}
	var sb strings.Builder
	sb.WriteString(content)
	if content != "" && !strings.HasSuffix(content, "\n") {
		sb.WriteString("\n")
	}
	if content != "" {
		sb.WriteString("\n")
	}
	sb.WriteString("Generated images:\n")
	for i, img := range imgs {
		name := img.RelPath
		if name == "" {
			name = img.Name
		}
		if name == "" {
			name = fmt.Sprintf("image-%d", i+1)
		}
		if img.URL != "" {
			sb.WriteString("- ")
			sb.WriteString(img.URL)
			sb.WriteString("\n")
		} else {
			sb.WriteString("- ")
			sb.WriteString(name)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
