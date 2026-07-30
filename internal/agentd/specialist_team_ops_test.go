package agentd

import (
	"context"
	"testing"

	"manifold/internal/config"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"
)

func TestListSpecialistsForUserIncludesOrchestratorAndTeams(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()

	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{Name: "alpha", Provider: "openai", Model: "gpt-4.1"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 7, persistence.SpecialistTeam{Name: "ops"})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}
	if err := app.teamStore.AddMember(ctx, 7, "ops", "alpha"); err != nil {
		t.Fatalf("add member: %v", err)
	}

	list, err := app.listSpecialistsForUser(ctx, 7)
	if err != nil {
		t.Fatalf("listSpecialistsForUser: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected orchestrator plus one specialist, got %d", len(list))
	}
	if list[0].Name != specialists.OrchestratorName {
		t.Fatalf("expected orchestrator first, got %q", list[0].Name)
	}
	if list[1].Name != "alpha" {
		t.Fatalf("expected alpha second, got %q", list[1].Name)
	}
	if len(list[1].Teams) != 1 || list[1].Teams[0] != "ops" {
		t.Fatalf("expected alpha team membership, got %#v", list[1].Teams)
	}
}

func TestCreateSpecialistForUserAppliesTeamMemberships(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()

	_, err := app.teamStore.Upsert(ctx, 11, persistence.SpecialistTeam{Name: "research"})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	saved, status, err := app.createSpecialistForUser(ctx, 11, persistence.Specialist{
		Name:         "analyst",
		Model:        "gpt-4.1-mini",
		AutoDiscover: boolPtrTeam(true),
		Teams:        []string{"research"},
	})
	if err != nil {
		t.Fatalf("createSpecialistForUser: %v", err)
	}
	if status != httpStatusCreated {
		t.Fatalf("expected created status, got %d", status)
	}
	if len(saved.Teams) != 1 || saved.Teams[0] != "research" {
		t.Fatalf("expected saved team membership, got %#v", saved.Teams)
	}
	if saved.AutoDiscover == nil || !*saved.AutoDiscover {
		t.Fatalf("expected autoDiscover persisted, got %#v", saved.AutoDiscover)
	}
	promptID, promptVersionID := defaultPromptIDs(11)
	if saved.PromptID != promptID || saved.PromptVersionID != promptVersionID {
		t.Fatalf("expected default manifold prompt reference, got %q / %q", saved.PromptID, saved.PromptVersionID)
	}
	if saved.System == "" {
		t.Fatal("expected default manifold system prompt")
	}

	memberships := app.teamMembershipsForUser(ctx, 11)
	if len(memberships["analyst"]) != 1 || memberships["analyst"][0] != "research" {
		t.Fatalf("expected team membership to be persisted, got %#v", memberships)
	}
}

func TestDeleteSpecialistForUserRemovesMemberships(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()

	_, err := app.teamStore.Upsert(ctx, 21, persistence.SpecialistTeam{Name: "ops"})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 21, persistence.Specialist{Name: "runner", Provider: "openai"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	if err := app.teamStore.AddMember(ctx, 21, "ops", "runner"); err != nil {
		t.Fatalf("add member: %v", err)
	}

	if err := app.deleteSpecialistForUser(ctx, 21, "runner"); err != nil {
		t.Fatalf("deleteSpecialistForUser: %v", err)
	}

	if memberships := app.teamMembershipsForUser(ctx, 21); len(memberships["runner"]) != 0 {
		t.Fatalf("expected memberships removed, got %#v", memberships)
	}
	if _, ok, err := app.specStore.GetByName(ctx, 21, "runner"); err != nil || ok {
		t.Fatalf("expected specialist removed, ok=%v err=%v", ok, err)
	}
}

func TestSpecialistsResponsesRedactSecrets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()

	_, err := app.specStore.Upsert(ctx, 41, persistence.Specialist{
		Name:   "alpha",
		APIKey: "alpha-secret",
		ExtraHeaders: map[string]string{
			"Authorization": "Bearer header-secret",
			"X-Trace":       "trace-id",
		},
		ExtraParams: map[string]any{
			"password": "param-secret",
			"visible":  "shown",
		},
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}

	list, err := app.listSpecialistsForUser(ctx, 41)
	if err != nil {
		t.Fatalf("listSpecialistsForUser: %v", err)
	}
	redactedList := observability.RedactValue(list).([]any)
	for _, item := range redactedList {
		sp := item.(map[string]any)
		if sp["apiKey"] != observability.RedactedValue {
			t.Fatalf("expected apiKey redacted in list item %#v", sp)
		}
	}
	alpha := redactedList[1].(map[string]any)
	headers := alpha["extraHeaders"].(map[string]any)
	if headers["Authorization"] != observability.RedactedValue || headers["X-Trace"] != "trace-id" {
		t.Fatalf("unexpected header redaction: %#v", headers)
	}
	params := alpha["extraParams"].(map[string]any)
	if params["password"] != observability.RedactedValue || params["visible"] != "shown" {
		t.Fatalf("unexpected params redaction: %#v", params)
	}
}
func TestUpdateSpecialistForUserPreservesAPIKeyOnPauseWithRedactedPayload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()

	_, err := app.specStore.Upsert(ctx, 51, persistence.Specialist{
		Name:     "alpha",
		Provider: "openai",
		Model:    "gpt-4.1",
		APIKey:   "alpha-secret",
		ExtraHeaders: map[string]string{
			"Authorization": "Bearer header-secret",
			"X-Trace":       "trace-id",
		},
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}

	saved, err := app.updateSpecialistForUser(ctx, 51, "alpha", persistence.Specialist{
		Name:     "alpha",
		Provider: "openai",
		Model:    "gpt-4.1",
		APIKey:   observability.RedactedValue,
		Paused:   true,
		ExtraHeaders: map[string]string{
			"Authorization": observability.RedactedValue,
			"X-Trace":       "trace-id",
		},
	})
	if err != nil {
		t.Fatalf("updateSpecialistForUser: %v", err)
	}
	if !saved.Paused {
		t.Fatalf("expected specialist to be paused")
	}

	got, ok, err := app.specStore.GetByName(ctx, 51, "alpha")
	if err != nil || !ok {
		t.Fatalf("get specialist: ok=%v err=%v", ok, err)
	}
	if got.APIKey != "alpha-secret" {
		t.Fatalf("expected stored api key preserved, got %q", got.APIKey)
	}
	if got.ExtraHeaders["Authorization"] != "Bearer header-secret" {
		t.Fatalf("expected authorization header preserved, got %#v", got.ExtraHeaders)
	}
	if !got.Paused {
		t.Fatalf("expected paused state persisted")
	}
}

func TestCreateTeamForUserRequiresSelectedMemberOrchestrator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()

	_, err := app.specStore.Upsert(ctx, 31, persistence.Specialist{Name: "lead", Provider: "openai", Model: "gpt-4.1"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}

	team, err := app.createTeamForUser(ctx, 31, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead"},
	})
	if err != nil {
		t.Fatalf("createTeamForUser: %v", err)
	}
	if team.OrchestratorName != "lead" {
		t.Fatalf("expected selected orchestrator, got %q", team.OrchestratorName)
	}
	if len(team.Members) != 1 || team.Members[0] != "lead" {
		t.Fatalf("expected lead membership, got %#v", team.Members)
	}
}

func TestCreateTeamForUserRejectsInvalidOrchestrator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()
	_, err := app.specStore.Upsert(ctx, 32, persistence.Specialist{Name: "lead", Provider: "openai", Model: "gpt-4.1"})
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 32, persistence.Specialist{Name: "paused", Provider: "openai", Model: "gpt-4.1", Paused: true})
	if err != nil {
		t.Fatalf("upsert paused: %v", err)
	}

	cases := []persistence.SpecialistTeam{
		{Name: "missing-name", Members: []string{"lead"}},
		{Name: "reserved", OrchestratorName: specialists.OrchestratorName, Members: []string{specialists.OrchestratorName}},
		{Name: "not-member", OrchestratorName: "lead", Members: []string{}},
		{Name: "unknown", OrchestratorName: "missing", Members: []string{"missing"}},
		{Name: "paused", OrchestratorName: "paused", Members: []string{"paused"}},
	}
	for _, tc := range cases {
		if _, err := app.createTeamForUser(ctx, 32, tc); err == nil {
			t.Fatalf("expected create team %q to fail", tc.Name)
		}
	}
}

func TestCannotRemoveOrDeleteTeamOrchestrator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()
	_, err := app.specStore.Upsert(ctx, 33, persistence.Specialist{Name: "lead", Provider: "openai", Model: "gpt-4.1"})
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	_, err = app.createTeamForUser(ctx, 33, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead"},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := app.removeSpecialistFromTeamForUser(ctx, 33, "ops", "lead"); err == nil {
		t.Fatal("expected removing team orchestrator to fail")
	}
	if err := app.deleteSpecialistForUser(ctx, 33, "lead"); err == nil {
		t.Fatal("expected deleting team orchestrator to fail")
	}
}

func TestParseTeamMemberPath(t *testing.T) {
	t.Parallel()

	team, specialist, ok := parseTeamMemberPath("ops/members/alpha")
	if !ok || team != "ops" || specialist != "alpha" {
		t.Fatalf("unexpected parse result: ok=%v team=%q specialist=%q", ok, team, specialist)
	}

	_, _, ok = parseTeamMemberPath("ops/members/")
	if ok {
		t.Fatal("expected invalid member path to fail")
	}
}

func newSpecialistTeamTestApp() *app {
	return &app{
		cfg: &config.Config{
			EnableTools: true,
			LLMClient: config.LLMClientConfig{
				Provider: "openai",
				OpenAI: config.OpenAIConfig{
					Model:   "gpt-4.1",
					BaseURL: "https://api.example.com",
					APIKey:  "secret",
				},
			},
		},
		specStore:    databases.NewSpecialistsStore(nil),
		teamStore:    databases.NewSpecialistTeamsStore(nil),
		userSpecRegs: map[int64]*specialists.Registry{},
	}
}

func boolPtrTeam(value bool) *bool {
	v := value
	return &v
}
