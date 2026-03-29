package agentd

import (
	"context"
	"testing"

	"manifold/internal/config"
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

func TestCreateTeamForUserNormalizesOrchestrator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	app := newSpecialistTeamTestApp()
	app.cfg.SystemPrompt = "system-base"

	team, err := app.createTeamForUser(ctx, 31, persistence.SpecialistTeam{Name: "ops"})
	if err != nil {
		t.Fatalf("createTeamForUser: %v", err)
	}
	if team.Orchestrator.Name != "ops-orchestrator" {
		t.Fatalf("expected generated orchestrator name, got %q", team.Orchestrator.Name)
	}
	if team.Orchestrator.Provider != "openai" {
		t.Fatalf("expected default provider, got %q", team.Orchestrator.Provider)
	}
	if team.Orchestrator.Model != "gpt-4.1" {
		t.Fatalf("expected provider default model, got %q", team.Orchestrator.Model)
	}
	if team.Orchestrator.System != "system-base" {
		t.Fatalf("expected system prompt fallback, got %q", team.Orchestrator.System)
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
