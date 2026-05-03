package agentd

import (
	"fmt"
	"manifold/internal/auth"
	"manifold/internal/webui"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func (a *app) close() {
	if a == nil {
		return
	}
	if a.mcpManager != nil {
		a.mcpManager.Close()
	}
	if a.mgr != nil {
		a.mgr.Close()
	}
	if a.embeddedRuntime != nil {
		if err := a.embeddedRuntime.Stop(); err != nil {
			log.Warn().Err(err).Msg("stop embedded postgres")
		}
	}
}

func (a *app) launchStartupMCPOAuthPrompts(baseURL string) {
	if len(a.startupMCPOAuthIDs) == 0 {
		return
	}

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	for _, serverID := range a.startupMCPOAuthIDs {
		bootstrapURL := fmt.Sprintf("%s/api/mcp/oauth/bootstrap?serverId=%d", baseURL, serverID)
		if err := openBrowser(bootstrapURL); err != nil {
			log.Warn().Err(err).Int64("server_id", serverID).Msg("mcp_startup_oauth_browser_open_failed")
			continue
		}
		log.Info().Int64("server_id", serverID).Msg("mcp_startup_oauth_browser_opened")
		time.Sleep(250 * time.Millisecond)
	}
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
func (a *app) wrapWithMiddleware(handler http.Handler) http.Handler {
	next := handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if requested := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers")); requested != "" {
			w.Header().Set("Access-Control-Allow-Headers", requested)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
	if a.cfg.Auth.Enabled && a.authStore != nil {
		return auth.Middleware(a.authStore, a.cfg.Auth.CookieName, false)(handler)
	}
	return handler
}

func (a *app) registerFrontend(mux *http.ServeMux) error {
	frontendProxy := os.Getenv("FRONTEND_DEV_PROXY")
	opts := webui.Options{DevProxy: frontendProxy}
	if a.cfg.Auth.Enabled {
		opts.AuthGate = func(r *http.Request) bool {
			_, ok := auth.CurrentUser(r.Context())
			return ok
		}
		opts.UnauthedRedirect = "/auth/login"
	}
	if err := webui.RegisterFrontend(mux, opts); err != nil {
		return err
	}
	if frontendProxy != "" {
		log.Info().Str("url", frontendProxy).Msg("frontend dev proxy enabled")
	}
	return nil
}
