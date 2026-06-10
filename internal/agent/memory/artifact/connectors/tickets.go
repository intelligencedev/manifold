package connectors

import (
	"context"

	"manifold/internal/agent/memory/artifact"
)

// TicketConnector reserves the ticket capture interface for future integrations.
type TicketConnector struct{}

// Kind returns the artifact kind captured by this connector.
func (TicketConnector) Kind() artifact.ArtifactKind { return artifact.ArtifactTicket }

// Capture reports tickets as unavailable until a concrete ticket backend is configured.
func (TicketConnector) Capture(context.Context, artifact.CaptureRequest) ([]artifact.Artifact, error) {
	return nil, artifact.ErrConnectorUnavailable
}
