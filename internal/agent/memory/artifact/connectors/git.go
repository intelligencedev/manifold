package connectors

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"manifold/internal/agent/memory/artifact"
)

const maxArtifactExcerptBytes = 8 << 10

// GitConnector captures git commits from a local repository path.
type GitConnector struct{}

// Kind returns the artifact kind captured by this connector.
func (GitConnector) Kind() artifact.ArtifactKind { return artifact.ArtifactGitCommit }

// Capture captures commits since the requested time. Hints: repoPath, since, branch.
func (GitConnector) Capture(ctx context.Context, req artifact.CaptureRequest) ([]artifact.Artifact, error) {
	repoPath := strings.TrimSpace(req.Hints["repoPath"])
	if repoPath == "" {
		return nil, artifact.ErrConnectorUnavailable
	}
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	args := []string{"-C", repoPath, "log", "--format=%H%x1f%an%x1f%aI%x1f%s%x1f%b%x1e"}
	if since := strings.TrimSpace(req.Hints["since"]); since != "" {
		args = append(args, "--since="+since)
	}
	if branch := strings.TrimSpace(req.Hints["branch"]); branch != "" {
		args = append(args, branch)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseGitLog(req.TenantID, repoPath, output), nil
}

func parseGitLog(tenantID int64, repoPath string, output []byte) []artifact.Artifact {
	out := []artifact.Artifact{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Split(splitRecords)
	now := time.Now().UTC()
	for scanner.Scan() {
		record := strings.Trim(scanner.Text(), "\x1e\n\r\t ")
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\x1f", 5)
		if len(parts) < 5 {
			continue
		}
		sha := strings.TrimSpace(parts[0])
		author := strings.TrimSpace(parts[1])
		authoredAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(parts[2]))
		subject := strings.TrimSpace(parts[3])
		body := strings.TrimSpace(parts[4])
		content := strings.TrimSpace(subject + "\n\n" + body)
		hash := sha256.Sum256([]byte(content))
		out = append(out, artifact.Artifact{
			TenantID:    tenantID,
			Kind:        artifact.ArtifactGitCommit,
			ExternalID:  sha,
			URI:         gitFileURI(repoPath, sha),
			Title:       subject,
			Excerpt:     truncateBytes(content, maxArtifactExcerptBytes),
			ContentHash: fmt.Sprintf("%x", hash),
			AuthoredBy:  author,
			AuthoredAt:  authoredAt,
			CapturedAt:  now,
			Metadata:    map[string]any{"repoPath": repoPath},
		})
	}
	return out
}

func splitRecords(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, 0x1e); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func gitFileURI(repoPath, sha string) string {
	u := url.URL{Scheme: "file", Path: repoPath}
	return u.String() + "#" + sha
}

func truncateBytes(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit]
}
