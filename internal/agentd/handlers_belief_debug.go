package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"manifold/internal/agent/belief"
	"manifold/internal/auth"
	"manifold/internal/policy"
	transitdomain "manifold/internal/transit"
)

func (a *app) debugBeliefsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.mgr == nil || a.mgr.Belief == nil {
			http.NotFound(w, r)
			return
		}
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		basePath := "/debug/beliefs"
		if strings.HasPrefix(r.URL.Path, "/api/debug/beliefs") {
			basePath = "/api/debug/beliefs"
		}
		path := strings.Trim(strings.TrimPrefix(r.URL.Path, basePath), "/")
		if path == "" {
			writeJSON(w, http.StatusOK, map[string]string{
				"search":     basePath + "/search",
				"belief":     basePath + "/{beliefID}",
				"evidence":   basePath + "/evidence",
				"promotions": basePath + "/promotions",
				"influence":  basePath + "/influence",
				"policies":   basePath + "/policies",
			})
			return
		}

		switch {
		case path == "search":
			a.handleDebugBeliefSearch(w, r)
		case path == "evidence":
			a.handleDebugBeliefEvidence(w, r)
		case path == "promotions":
			a.handleDebugBeliefPromotions(w, r)
		case path == "influence":
			a.handleDebugBeliefInfluence(w, r)
		case path == "policies":
			a.handleDebugBeliefPolicies(w, r)
		case strings.HasSuffix(path, "/retract"):
			a.handleDebugBeliefRetract(w, r, strings.TrimSuffix(path, "/retract"))
		case strings.HasSuffix(path, "/supersede"):
			a.handleDebugBeliefSupersede(w, r, strings.TrimSuffix(path, "/supersede"))
		default:
			a.handleDebugBeliefDetail(w, r, path)
		}
	}
}

func (a *app) handleDebugBeliefSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	query := belief.SearchQuery{
		TenantID: tenantID,
		ScopeIDs: splitQueryCSV(r, "scope_id"),
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Statuses: parseBeliefStatuses(r),
		Limit:    parseIntQuery(r, "limit"),
	}
	results, err := a.mgr.Belief.SearchBeliefs(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (a *app) handleDebugBeliefDetail(w http.ResponseWriter, r *http.Request, beliefID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	beliefID = strings.Trim(beliefID, "/")
	item, ok, err := a.mgr.Belief.GetBelief(r.Context(), tenantID, beliefID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	evidence, _ := a.mgr.Belief.ListEvidence(r.Context(), belief.EvidenceQuery{TenantID: tenantID, BeliefID: beliefID, Limit: 50})
	promotions, _ := a.mgr.Belief.ListPromotions(r.Context(), belief.PromotionQuery{TenantID: tenantID, BeliefID: beliefID, Limit: 50})
	writeJSON(w, http.StatusOK, map[string]any{"belief": item, "evidence": evidence, "promotions": promotions})
}

func (a *app) handleDebugBeliefEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	items, err := a.mgr.Belief.ListEvidence(r.Context(), belief.EvidenceQuery{
		TenantID:  tenantID,
		BeliefID:  strings.TrimSpace(r.URL.Query().Get("belief_id")),
		EpisodeID: strings.TrimSpace(r.URL.Query().Get("episode_id")),
		SourceID:  strings.TrimSpace(r.URL.Query().Get("source_id")),
		Source:    belief.SourceKind(strings.TrimSpace(r.URL.Query().Get("source_kind"))),
		Polarity:  belief.EvidencePolarity(strings.TrimSpace(r.URL.Query().Get("polarity"))),
		Limit:     parseIntQuery(r, "limit"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *app) handleDebugBeliefPromotions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	items, err := a.mgr.Belief.ListPromotions(r.Context(), belief.PromotionQuery{
		TenantID:  tenantID,
		BeliefID:  strings.TrimSpace(r.URL.Query().Get("belief_id")),
		FromScope: strings.TrimSpace(r.URL.Query().Get("from_scope")),
		ToScope:   strings.TrimSpace(r.URL.Query().Get("to_scope")),
		Limit:     parseIntQuery(r, "limit"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *app) handleDebugBeliefInfluence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	retriever := a.applyRAGEvidenceBlend(belief.NewGraphEnrichedRetriever(a.mgr.Belief, a.mgr.Graph, 2))
	results, err := retriever.Retrieve(r.Context(), belief.RetrievalRequest{
		TenantID:    tenantID,
		UserID:      tenantID,
		ProjectID:   strings.TrimSpace(r.URL.Query().Get("project_id")),
		ObjectiveID: strings.TrimSpace(r.URL.Query().Get("objective_id")),
		SessionID:   strings.TrimSpace(r.URL.Query().Get("session_id")),
		Role:        strings.TrimSpace(r.URL.Query().Get("role")),
		Query:       strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:       parseIntQuery(r, "limit"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	prompt := belief.BuildPromptSection(results, belief.PromptOptions{MaxBeliefs: parseIntQuery(r, "max_beliefs"), MaxTokens: parseIntQuery(r, "max_tokens")})
	labelled := make([]map[string]any, 0, len(results))
	for _, item := range results {
		source := "belief"
		if item.Belief.Metadata != nil {
			if v, ok := item.Belief.Metadata["source"].(string); ok && v != "" {
				source = v
			}
		}
		labelled = append(labelled, map[string]any{
			"source": source,
			"result": item,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": labelled, "prompt": prompt})
}

func (a *app) handleDebugBeliefPolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.transitService == nil {
		http.NotFound(w, r)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	prefixes := []string{fmt.Sprintf("constraint/org/%d/", tenantID), fmt.Sprintf("policy/org/%d/", tenantID)}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if projectID != "" {
		prefixes = append(prefixes, "constraint/project/"+projectID+"/")
		if role != "" {
			prefixes = append(prefixes, "policy/project/"+projectID+"/role/"+role+"/")
		}
	}
	keys := make([]string, 0)
	for _, prefix := range prefixes {
		items, err := a.transitService.ListKeys(r.Context(), tenantID, transitdomain.ListRequest{Prefix: prefix, Limit: 100})
		if err != nil {
			writeTransitError(w, err)
			return
		}
		for _, item := range items {
			keys = append(keys, item.KeyName)
		}
	}
	if len(keys) == 0 {
		writeJSON(w, http.StatusOK, []policy.Record{})
		return
	}
	records, err := a.transitService.GetMemory(r.Context(), tenantID, keys)
	if err != nil {
		writeTransitError(w, err)
		return
	}
	out := make([]policy.Record, 0, len(records))
	for _, record := range records {
		if decoded, ok := policy.DecodeTransitRecord(record); ok {
			out = append(out, decoded)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *app) handleDebugBeliefRetract(w http.ResponseWriter, r *http.Request, beliefID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	actorID := tenantID
	item, err := (belief.LifecycleService{Store: a.mgr.Belief, Graph: a.mgr.Graph}).Retract(r.Context(), tenantID, strings.Trim(beliefID, "/"), req.Reason, &actorID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *app) handleDebugBeliefSupersede(w http.ResponseWriter, r *http.Request, beliefID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, err := a.debugBeliefTenantID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		Reason     string  `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_, replacement, err := (belief.LifecycleService{Store: a.mgr.Belief, Graph: a.mgr.Graph}).Supersede(r.Context(), tenantID, strings.Trim(beliefID, "/"), belief.Belief{Statement: req.Statement, Confidence: req.Confidence}, req.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, replacement)
}

func (a *app) debugBeliefTenantID(r *http.Request) (int64, error) {
	if a.cfg.Auth.Enabled {
		return a.requireUserID(r)
	}
	if value := parseIntQuery(r, "user_id"); value > 0 {
		return int64(value), nil
	}
	return systemUserID, nil
}

func parseBeliefStatuses(r *http.Request) []belief.BeliefStatus {
	values := splitQueryCSV(r, "status")
	out := make([]belief.BeliefStatus, 0, len(values))
	for _, value := range values {
		out = append(out, belief.BeliefStatus(value))
	}
	return out
}

func splitQueryCSV(r *http.Request, key string) []string {
	values := r.URL.Query()[key]
	if len(values) == 0 {
		values = []string{r.URL.Query().Get(key)}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}
