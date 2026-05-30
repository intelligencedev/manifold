package specialists

import (
	"context"
	"errors"
	"testing"

	"manifold/internal/config"
	"manifold/internal/persistence"

	"github.com/stretchr/testify/require"
)

type stubSpecialistsStore struct {
	list      []persistence.Specialist
	listErr   error
	upserts   []persistence.Specialist
	upsertErr error
}

func (s *stubSpecialistsStore) Init(context.Context) error { return nil }

func (s *stubSpecialistsStore) List(context.Context, int64) ([]persistence.Specialist, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]persistence.Specialist, len(s.list))
	copy(out, s.list)
	return out, nil
}

func (s *stubSpecialistsStore) GetByName(context.Context, int64, string) (persistence.Specialist, bool, error) {
	return persistence.Specialist{}, false, nil
}

func (s *stubSpecialistsStore) Upsert(_ context.Context, _ int64, sp persistence.Specialist) (persistence.Specialist, error) {
	if s.upsertErr != nil {
		return persistence.Specialist{}, s.upsertErr
	}
	s.upserts = append(s.upserts, sp)
	s.list = append(s.list, sp)
	return sp, nil
}

func (s *stubSpecialistsStore) Delete(context.Context, int64, string) error { return nil }

func TestConfigsFromStore(t *testing.T) {
	t.Parallel()

	list := []persistence.Specialist{
		{Name: OrchestratorName},
		{
			Name:         "alpha",
			Provider:     "google",
			Description:  "desc",
			Model:        "model",
			AutoDiscover: boolPtrStore(true),
			Harness: &persistence.SpecialistHarness{
				Enabled:           true,
				Mode:              "workflow",
				MaxRetriesPerStep: 6,
				TerminalTools:     []string{"agent_response"},
				RequiredSteps:     []string{"search"},
				ToolPrerequisites: map[string][]persistence.SpecialistHarnessPrerequisite{
					"fetch": {{Tool: "search", MatchArg: "query"}},
				},
				Compact: persistence.SpecialistHarnessCompact{Enabled: true, KeepRecentSteps: 5, PhaseThresholds: []float64{0.5, 0.8}},
			},
		},
	}

	out := ConfigsFromStore(list)

	require.Len(t, out, 1)
	require.Equal(t, "alpha", out[0].Name)
	require.Equal(t, "google", out[0].Provider)
	require.Equal(t, "desc", out[0].Description)
	require.Equal(t, "model", out[0].Model)
	require.NotNil(t, out[0].AutoDiscover)
	require.True(t, *out[0].AutoDiscover)
	require.NotNil(t, out[0].Harness)
	require.True(t, out[0].Harness.Enabled)
	require.Equal(t, "workflow", out[0].Harness.Mode)
	require.Equal(t, 6, out[0].Harness.MaxRetriesPerStep)
	require.Equal(t, []string{"search"}, out[0].Harness.RequiredSteps)
	require.Equal(t, "query", out[0].Harness.ToolPrerequisites["fetch"][0].MatchArg)
	require.Equal(t, []float64{0.5, 0.8}, out[0].Harness.Compact.PhaseThresholds)
}

func TestSeedStore(t *testing.T) {
	t.Parallel()

	store := &stubSpecialistsStore{
		list: []persistence.Specialist{{Name: "alpha"}},
	}
	defaults := []config.SpecialistConfig{
		{Name: "alpha", Provider: "openai"},
		{
			Name:        "beta",
			Provider:    "anthropic",
			Description: "seeded",
			Harness: &config.HarnessConfig{
				Enabled:       true,
				Mode:          "guarded_chat",
				TerminalTools: []string{"agent_response"},
			},
		},
		{Name: "  "},
	}

	err := SeedStore(context.Background(), store, 0, defaults)

	require.NoError(t, err)
	require.Len(t, store.upserts, 1)
	require.Equal(t, "beta", store.upserts[0].Name)
	require.Equal(t, "anthropic", store.upserts[0].Provider)
	require.Equal(t, "seeded", store.upserts[0].Description)
	require.NotNil(t, store.upserts[0].Harness)
	require.Equal(t, "guarded_chat", store.upserts[0].Harness.Mode)
}

func TestSeedStore_ListError(t *testing.T) {
	t.Parallel()

	store := &stubSpecialistsStore{listErr: errors.New("boom")}
	defaults := []config.SpecialistConfig{{Name: "alpha"}}

	err := SeedStore(context.Background(), store, 0, defaults)

	require.Error(t, err)
}

func boolPtrStore(value bool) *bool {
	v := value
	return &v
}
