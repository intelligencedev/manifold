package agentd

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"manifold/internal/warpp"
)

func buildWarppAllowedTools(workflow warpp.Workflow) map[string]bool {
	allowed := map[string]bool{}
	for _, step := range workflow.Steps {
		if step.Tool != nil {
			allowed[step.Tool.Name] = true
		}
	}
	return allowed
}

func (a *app) handleWarppAgentRun(w http.ResponseWriter, r *http.Request, req chatRunRequest, owner int64) bool {
	if r.URL.Query().Get("warpp") != "true" {
		return false
	}

	seconds := a.cfg.WorkflowTimeoutSeconds
	if seconds <= 0 {
		seconds = a.cfg.AgentRunTimeoutSeconds
	}
	ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
	defer cancel()

	if dur > 0 {
		log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("mode", "warpp").Msg("using configured workflow timeout")
	} else {
		log.Debug().Str("endpoint", "/agent/run").Str("mode", "warpp").Msg("no timeout configured; running until completion")
	}

	runner, err := a.warppRunnerForUser(ctx, owner)
	if err != nil {
		log.Error().Err(err).Msg("warpp_runner_for_user")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return true
	}

	intent := runner.DetectIntent(ctx, req.Prompt)
	workflow, err := runner.Workflows.Get(intent)
	if err != nil {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return true
	}

	attrs := warpp.Attrs{"utter": req.Prompt}
	workflow, _, attrs, err = runner.Personalize(ctx, workflow, attrs)
	if err != nil {
		log.Error().Err(err).Msg("personalize")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return true
	}

	final, err := runner.Execute(ctx, workflow, buildWarppAllowedTools(workflow), attrs, nil)
	if err != nil {
		log.Error().Err(err).Msg("warpp")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"result": final})
	return true
}
