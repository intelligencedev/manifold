package agentd

import "context"

func (a *app) buildOrchestratorChatEngine(ctx context.Context, req chatEngineBuildRequest) chatEngineBuildResult {
	return a.chatService().BuildOrchestrator(ctx, req)
}

func (a *app) buildSpecialistChatEngine(ctx context.Context, req chatEngineBuildRequest) chatEngineBuildResult {
	return a.chatService().BuildSpecialist(ctx, req)
}

func (a *app) buildTeamChatEngine(ctx context.Context, req chatEngineBuildRequest) chatEngineBuildResult {
	return a.chatService().BuildTeam(ctx, req)
}
