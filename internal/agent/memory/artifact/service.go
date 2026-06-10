package artifact

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrConnectorUnavailable is returned by configured but unavailable connectors.
	ErrConnectorUnavailable = errors.New("artifact connector unavailable")
)

// Connector captures artifacts relevant to an episode.
type Connector interface {
	Kind() ArtifactKind
	Capture(ctx context.Context, req CaptureRequest) ([]Artifact, error)
}

// CaptureRequest identifies the episode/scope and connector-specific hints.
type CaptureRequest struct {
	TenantID  int64
	ScopeID   string
	EpisodeID string
	Hints     map[string]string
}

// CaptureManager fans out episode-end artifact capture without failing the run.
type CaptureManager struct {
	Store      Store
	Connectors []Connector
	Timeout    time.Duration
	OnError    func(kind ArtifactKind, err error)
}

// Capture runs configured connectors and upserts any returned artifacts.
func (m CaptureManager) Capture(ctx context.Context, req CaptureRequest) []Artifact {
	if m.Store == nil || len(m.Connectors) == 0 {
		return nil
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	var mu sync.Mutex
	out := []Artifact{}
	var wg sync.WaitGroup
	for _, connector := range m.Connectors {
		if connector == nil {
			continue
		}
		wg.Add(1)
		go func(c Connector) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			items, err := c.Capture(cctx, req)
			if err != nil {
				if m.OnError != nil {
					m.OnError(c.Kind(), err)
				}
				return
			}
			for _, item := range items {
				saved, err := m.Store.UpsertArtifact(ctx, item)
				if err != nil {
					if m.OnError != nil {
						m.OnError(c.Kind(), err)
					}
					continue
				}
				mu.Lock()
				out = append(out, saved)
				mu.Unlock()
			}
		}(connector)
	}
	wg.Wait()
	return out
}
