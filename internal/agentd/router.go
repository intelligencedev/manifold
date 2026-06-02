package agentd

import (
	"fmt"
	"net/http"
)

func newRouter(a *app) *http.ServeMux {
	mux := http.NewServeMux()

	registerDocsRoutes(mux, a)
	registerAuthRoutes(mux, a)
	registerCoreAPIRoutes(mux, a)
	registerAgentRoutes(mux, a)
	registerMCPRoutes(mux, a)
	registerDebugRoutes(mux, a)
	return mux
}

func registerDocsRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("/openapi.json", a.openapiSpecHandler())
	mux.HandleFunc("/api/openapi.json", a.openapiSpecHandler())
	mux.HandleFunc("/api-docs", a.openapiDocsHandler())
	mux.HandleFunc("/api-docs/", a.openapiDocsHandler())
	mux.HandleFunc("/api/docs", a.openapiDocsHandler())
	mux.HandleFunc("/api/docs/", a.openapiDocsHandler())

	if a.playgroundHandler != nil {
		mux.Handle("/api/v1/playground", a.playgroundHandler)
		mux.Handle("/api/v1/playground/", a.playgroundHandler)
	}
}

func registerAuthRoutes(mux *http.ServeMux, a *app) {
	if a.cfg.Auth.Enabled && a.authProvider != nil {
		mux.HandleFunc("/auth/login", a.authLoginHandler())
		mux.HandleFunc("/auth/callback", a.authCallbackHandler())
		mux.HandleFunc("/auth/logout", a.authLogoutHandler())
		mux.HandleFunc("/api/me", a.meHandler())
	}

	// User preferences endpoints (available with or without auth)
	mux.HandleFunc("/api/me/preferences", a.userPreferencesHandler())
	mux.HandleFunc("/api/me/preferences/project", a.setActiveProjectHandler())
	if a.cfg.Auth.Enabled && a.authStore != nil {
		mux.HandleFunc("/api/users", a.usersHandler())
		mux.HandleFunc("/api/users/", a.userDetailHandler())
	}
}

func registerCoreAPIRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ready")
	})

	mux.HandleFunc("/api/projects", a.projectsHandler())
	mux.HandleFunc("/api/projects/", a.projectDetailHandler())
	mux.HandleFunc("/api/matrix/rooms", a.matrixRoomsHandler())
	mux.HandleFunc("/api/matrix/rooms/", a.matrixRoomDetailHandler())
	mux.HandleFunc("/api/codeqa/runs", a.codeQARunsHandler())
	mux.HandleFunc("/api/codeqa/runs/", a.codeQARunDetailHandler())

	mux.HandleFunc("/api/durable/tasks", a.durableTasksHandler())
	mux.HandleFunc("/api/durable/tasks/", a.durableTaskDetailHandler())
	mux.HandleFunc("/api/durable/events", a.durableEventsHandler())
	mux.HandleFunc("/api/durable/queues", a.durableQueuesHandler())

	mux.HandleFunc("/api/runs", a.runsHandler())
	mux.HandleFunc("/api/runs/", a.runTimelineHandler())
	mux.HandleFunc("/api/chat/sessions", a.chatSessionsHandler())
	mux.HandleFunc("/api/chat/sessions/", a.chatSessionDetailHandler())
	mux.HandleFunc("/api/chat/runs", a.chatRunsHandler())
	mux.HandleFunc("/api/chat/runs/", a.chatRunDetailHandler())
	mux.HandleFunc("/api/chat/input-requests/", a.chatInputRequestHandler())
	mux.HandleFunc("/api/fleet/state", a.fleetStateHandler())
	mux.HandleFunc("/api/fleet/events", a.fleetEventsHandler())
	mux.HandleFunc("/api/trust/budgets", a.trustBudgetsHandler())
	mux.HandleFunc("/api/trust/budgets/", a.trustBudgetActionHandler())
	mux.HandleFunc("/api/constitution/versions", a.constitutionVersionsHandler())
	mux.HandleFunc("/api/constitution/versions/", a.constitutionVersionActionHandler())
	if a.cfg.Transit.Enabled {
		mux.HandleFunc("/api/transit/memories", a.transitMemoriesHandler())
		mux.HandleFunc("/api/transit/memories/", a.transitMemoryDetailHandler())
		mux.HandleFunc("/api/transit/keys", a.transitKeysHandler())
		mux.HandleFunc("/api/transit/recent", a.transitRecentHandler())
		mux.HandleFunc("/api/transit/search", a.transitSearchHandler())
		mux.HandleFunc("/api/transit/discover", a.transitDiscoverHandler())
	}

	mux.HandleFunc("/api/status", a.statusHandler())
	mux.HandleFunc("/api/specialists/defaults", a.specialistDefaultsHandler())
	mux.HandleFunc("/api/specialists", a.specialistsHandler())
	mux.HandleFunc("/api/specialists/", a.specialistDetailHandler())
	mux.HandleFunc("/api/teams", a.teamsHandler())
	mux.HandleFunc("/api/teams/", a.teamDetailHandler())

	mux.HandleFunc("/api/metrics/tokens", a.metricsTokensHandler())
	mux.HandleFunc("/api/metrics/memory", a.metricsMemoryHandler())
	mux.HandleFunc("/api/metrics/traces", a.metricsTracesHandler())
	mux.HandleFunc("/api/metrics/logs", a.metricsLogsHandler())
	mux.HandleFunc("/api/metrics/logs/detail", a.metricsLogDetailHandler())
	mux.HandleFunc("/api/observability/memory", a.memoryObservabilityHandler())
	mux.HandleFunc("/api/observability/memory/", a.memoryObservabilityHandler())
	// Agentd configuration (GET + POST/PUT/PATCH)
	mux.HandleFunc("/api/config/agentd", a.agentdConfigHandler())
	mux.HandleFunc("/api/flows/v2/tools", a.flowV2ToolsHandler())
	mux.HandleFunc("/api/flows/v2/workflows", a.flowV2WorkflowsHandler())
	mux.HandleFunc("/api/flows/v2/workflows/", a.flowV2WorkflowDetailHandler())
	mux.HandleFunc("/api/flows/v2/validate", a.flowV2ValidateHandler())
	mux.HandleFunc("/api/flows/v2/run", a.flowV2RunHandler())
	mux.HandleFunc("/api/flows/v2/runs/", a.flowV2RunEventsHandler())
}

func registerAgentRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("/agent/run", a.agentRunHandler())
	mux.HandleFunc("/agent/vision", a.agentVisionHandler())
	mux.HandleFunc("/api/prompt", a.promptHandler())
	mux.HandleFunc("/v1/chat/completions", a.openAIChatCompletionsProxyHandler())

	mux.HandleFunc("/audio/", a.audioServeHandler())
	mux.HandleFunc("/stt", a.sttHandler())
}

func registerMCPRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("/api/mcp/servers", a.mcpServersHandler())
	mux.HandleFunc("/api/mcp/servers/", a.mcpServerDetailHandler())
	mux.HandleFunc("/api/mcp/oauth/start", a.mcpOAuthStartHandler())
	mux.HandleFunc("/api/mcp/oauth/bootstrap", a.mcpOAuthBootstrapHandler())
	mux.HandleFunc("/api/mcp/oauth/callback", a.mcpOAuthCallbackHandler())
}

func registerDebugRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("/debug/memory", a.debugMemoryHandler())
	mux.HandleFunc("/debug/memory/", a.debugMemoryHandler())
	mux.HandleFunc("/api/debug/memory", a.debugMemoryHandler())
	mux.HandleFunc("/api/debug/memory/", a.debugMemoryHandler())
	mux.HandleFunc("/debug/beliefs", a.debugBeliefsHandler())
	mux.HandleFunc("/debug/beliefs/", a.debugBeliefsHandler())
	mux.HandleFunc("/api/debug/beliefs", a.debugBeliefsHandler())
	mux.HandleFunc("/api/debug/beliefs/", a.debugBeliefsHandler())
	mux.HandleFunc("/api/beliefs", a.debugBeliefsHandler())
	mux.HandleFunc("/api/beliefs/", a.debugBeliefsHandler())
}
