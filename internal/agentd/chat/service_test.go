package chat

import (
	"context"
	"testing"

	"manifold/internal/agent"
)

func TestServiceNilReceiverReturnsUnavailable(t *testing.T) {
	var service *Service

	for _, build := range []func() BuildResult{
		func() BuildResult { return service.BuildOrchestrator(context.Background(), BuildRequest{}) },
		func() BuildResult { return service.BuildSpecialist(context.Background(), BuildRequest{}) },
		func() BuildResult { return service.BuildTeam(context.Background(), BuildRequest{}) },
	} {
		result := build()
		if result.Err == nil {
			t.Fatal("expected nil service to return an error")
		}
	}
}

func TestSanitizeImageGenerationBuild(t *testing.T) {
	engine := &agent.Engine{
		System:            "system",
		UserPromptContext: "context",
		MaxSteps:          10,
		SummaryEnabled:    true,
		HarnessEnabled:    true,
	}

	result := SanitizeImageGenerationBuild(BuildResult{Engine: engine, ImageGeneration: true})
	if result.Engine.MaxSteps != 1 || result.Engine.System != "" || !result.Engine.DisableMemory {
		t.Fatalf("image build was not sanitized: %+v", result.Engine)
	}
}
