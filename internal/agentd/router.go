package agentd

import (
	"fmt"
	"net/http"
)

func newRouter(a *app) *http.ServeMux {
	mux := http.NewServeMux()

	if a.playgroundHandler != nil {
		mux.Handle("/api/v1/playground", a.playgroundHandler)
		mux.Handle("/api/v1/playground/", a.playgroundHandler)
	}

	if a.cfg.Auth.Enabled && a.authProvider != nil {
		mux.HandleFunc("/auth/login", a.authLoginHandler())
		mux.HandleFunc("/auth/callback", a.authCallbackHandler())
		mux.HandleFunc("/auth/logout", a.authLogoutHandler())
		mux.HandleFunc("/api/me", a.meHandler())
	}

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ready")
	})

	mux.HandleFunc("/api/projects", a.projectsHandler())
	mux.HandleFunc("/api/projects/", a.projectDetailHandler())

	mux.HandleFunc("/api/runs", a.runsHandler())
	mux.HandleFunc("/api/chat/sessions", a.chatSessionsHandler())
	mux.HandleFunc("/api/chat/sessions/", a.chatSessionDetailHandler())

	if a.cfg.Auth.Enabled && a.authStore != nil {
		mux.HandleFunc("/api/users", a.usersHandler())
		mux.HandleFunc("/api/users/", a.userDetailHandler())
	}

	mux.HandleFunc("/api/status", a.statusHandler())
	mux.HandleFunc("/api/specialists", a.specialistsHandler())
	mux.HandleFunc("/api/specialists/", a.specialistDetailHandler())

	mux.HandleFunc("/api/metrics/tokens", a.metricsTokensHandler())
	// Agentd configuration (GET + POST/PUT/PATCH)
	mux.HandleFunc("/api/config/agentd", a.agentdConfigHandler())
	mux.HandleFunc("/api/warpp/tools", a.warppToolsHandler())
	mux.HandleFunc("/api/warpp/workflows", a.warppWorkflowsHandler())
	mux.HandleFunc("/api/warpp/workflows/", a.warppWorkflowDetailHandler())
	mux.HandleFunc("/api/warpp/run", a.warppRunHandler())

	mux.HandleFunc("/agent/run", a.agentRunHandler())
	mux.HandleFunc("/agent/vision", a.agentVisionHandler())
	mux.HandleFunc("/api/prompt", a.promptHandler())

	mux.HandleFunc("/audio/", a.audioServeHandler())
	mux.HandleFunc("/stt", a.sttHandler())

	mux.HandleFunc("/api/mcp/servers", a.mcpServersHandler())
	mux.HandleFunc("/api/mcp/servers/", a.mcpServerDetailHandler())
	mux.HandleFunc("/api/mcp/oauth/start", a.mcpOAuthStartHandler())
	mux.HandleFunc("/api/mcp/oauth/callback", a.mcpOAuthCallbackHandler())

	return mux
}
