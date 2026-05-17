package agentd

import (
	"context"
	"manifold/internal/agent"
	"manifold/internal/agent/belief"
	"manifold/internal/embedding"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/policy"
	"manifold/internal/rag/retrieve"
	"manifold/internal/specialists"
	agenttools "manifold/internal/tools/agents"
	"strings"

	"github.com/rs/zerolog/log"
)

func (a *app) cloneEngine() *agent.Engine {
	if a.engine == nil {
		return nil
	}
	clone := *a.engine
	clone.OnAssistant = nil
	clone.OnDelta = nil
	clone.OnTool = nil
	clone.OnToolStart = nil
	return &clone
}

// cloneEngineForUser returns a shallow copy of the base engine with user-specific
// orchestrator settings applied (particularly the tool allowlist). This enables
// per-user orchestrator configurations.
func (a *app) cloneEngineForUser(ctx context.Context, userID int64, sessionID, projectID, objectiveID string) *agent.Engine {
	eng := a.cloneEngine()
	if eng == nil {
		return nil
	}
	a.configureBeliefRunState(eng, userID, sessionID, projectID, objectiveID, "orchestrator")

	// Ensure the specialists catalog in the system prompt is user-scoped.
	// The base engine prompt is composed with the system (user=0) specialists
	// registry; without this override, non-system users can see system specialists.
	//
	// Do this before applying any per-user orchestrator overlay so we can build a
	// base prompt with the correct catalog.
	if a.cfg.Auth.Enabled && userID != systemUserID {
		eng.System = a.composeSystemPromptForUser(ctx, userID)
		eng.UserPromptContext = a.composeUserPromptContextForUser(ctx, userID)
	}

	em := a.attachSessionEvolvingMemory(eng, userID, sessionID)

	// Look up user's orchestrator overlay
	if a.cfg.Auth.Enabled && userID != systemUserID {
		sp, ok, err := a.specStore.GetByName(ctx, userID, specialists.OrchestratorName)
		if err == nil && ok {
			// Apply user's LLM overrides (provider/model/extra params).
			llmCfg, provider := specialists.ApplyLLMClientOverride(a.cfg.LLMClient, sp)
			userCfg := *a.cfg
			userCfg.LLMClient = llmCfg
			if provider == "" || provider == "openai" || provider == "local" {
				userCfg.OpenAI = llmCfg.OpenAI
			}
			if userLLM, err := llmproviders.Build(userCfg, a.httpClient); err != nil {
				log.Warn().Err(err).Msg("failed to build per-user llm provider")
			} else {
				eng.LLM = userLLM
			}
			currentModel := strings.TrimSpace(sp.Model)
			if currentModel == "" {
				switch provider {
				case "anthropic":
					currentModel = strings.TrimSpace(llmCfg.Anthropic.Model)
				case "google":
					currentModel = strings.TrimSpace(llmCfg.Google.Model)
				default:
					currentModel = strings.TrimSpace(llmCfg.OpenAI.Model)
				}
			}
			if currentModel != "" {
				eng.Model = currentModel
			}

			// Apply user's tool configuration
			eng.Tools = a.chatToolRegistry(sp.EnableTools, sp.AllowTools, sp.AutoDiscover)

			// Apply user's system prompt if set.
			// This should preserve the user-scoped specialists catalog.
			if sp.System != "" {
				eng.System = a.composeSystemPromptForUserWithOverride(ctx, userID, sp.System)
				eng.UserPromptContext = a.composeUserPromptContextForUser(ctx, userID)
			}
		}
	}

	// Create a per-request delegator so ask_agent/agent_call uses the
	// user-specific specialists registry (including tool allowlists).
	reg := a.specRegistry
	if a.cfg.Auth.Enabled && userID != systemUserID {
		if userReg, err := a.specialistsRegistryForUser(ctx, userID); err == nil && userReg != nil {
			reg = userReg
		}
	}
	delegator := agenttools.NewDelegator(eng.Tools, reg, a.workspaceManager, a.cfg.MaxSteps)
	delegator.SetDefaultTimeout(a.cfg.AgentRunTimeoutSeconds)
	delegator.SetEvolvingMemory(em)
	delegator.SetBeliefMemory(eng.BeliefStore)
	delegator.SetBeliefDistiller(eng.BeliefDistiller)
	delegator.SetBeliefRetriever(eng.BeliefRetriever, eng.BeliefMaxBeliefsPerPrompt, eng.BeliefPromptTokenBudget)
	delegator.SetBeliefLifecycle(eng.BeliefGraph, eng.BeliefPromotionThreshold)
	delegator.SetPolicyEnforcer(eng.PolicyEnforcer)
	delegator.SetTeamDelegator(a)
	if a.engine != nil && a.engine.ReMemEnabled {
		delegator.ConfigureReMem(a.evolvingCfg.LLM, a.evolvingCfg.Model, a.rememMaxInnerSteps)
	}
	eng.Delegator = delegator
	eng.TeamDelegator = a

	return eng
}

func (a *app) configureBeliefRunState(eng *agent.Engine, userID int64, sessionID, projectID, objectiveID, agentRole string) {
	if eng == nil {
		return
	}
	eng.UserID = userID
	eng.SessionID = strings.TrimSpace(sessionID)
	eng.ProjectID = belief.NormalizeProjectID(projectID)
	eng.ObjectiveID = strings.TrimSpace(objectiveID)
	eng.AgentRole = strings.TrimSpace(agentRole)
	if a.cfg != nil && a.cfg.BeliefMemory.Enabled && a.mgr != nil {
		eng.BeliefStore = a.mgr.Belief
		eng.BeliefGraph = a.mgr.Graph
		eng.BeliefPromotionThreshold = a.cfg.BeliefMemory.PromotionThreshold
		if a.cfg.BeliefMemory.EnableDistillation {
			eng.BeliefDistiller = a.newBeliefDistiller()
		} else {
			eng.BeliefDistiller = nil
		}
		if a.cfg.BeliefMemory.EnableRetrieval {
			base := belief.NewGraphEnrichedRetriever(a.mgr.Belief, a.mgr.Graph, 2)
			eng.BeliefRetriever = a.applyRAGEvidenceBlend(base)
			eng.BeliefMaxBeliefsPerPrompt = a.cfg.BeliefMemory.MaxBeliefsPerPrompt
			eng.BeliefPromptTokenBudget = a.cfg.BeliefMemory.MaxBeliefsPerPrompt * 140
		} else {
			eng.BeliefRetriever = nil
			eng.BeliefMaxBeliefsPerPrompt = 0
			eng.BeliefPromptTokenBudget = 0
		}
		if a.cfg.BeliefMemory.EnableConstraintEnforcement && a.transitService != nil {
			eng.PolicyEnforcer = policy.NewTransitEnforcer(a.transitService)
		} else {
			eng.PolicyEnforcer = nil
		}
	} else {
		eng.BeliefStore = nil
		eng.BeliefDistiller = nil
		eng.BeliefRetriever = nil
		eng.BeliefGraph = nil
		eng.BeliefMaxBeliefsPerPrompt = 0
		eng.BeliefPromptTokenBudget = 0
		eng.BeliefPromotionThreshold = 0
		eng.PolicyEnforcer = nil
	}
}

func (a *app) newBeliefDistiller() belief.Distiller {
	if a == nil || a.cfg == nil || strings.TrimSpace(a.cfg.Embedding.BaseURL) == "" || strings.TrimSpace(a.cfg.Embedding.Model) == "" {
		return belief.SimpleDistiller{}
	}
	cfg := a.cfg.Embedding
	return belief.SimpleDistiller{Embed: func(ctx context.Context, texts []string) ([][]float32, error) {
		return embedding.EmbedText(ctx, cfg, texts)
	}}
}

// applyRAGEvidenceBlend wraps a primary belief Retriever with a RAG-backed
// EvidenceSource when the runtime has a RAG service configured and the feature
// is enabled. Returns the inner retriever unchanged when blending is disabled.
func (a *app) applyRAGEvidenceBlend(inner belief.Retriever) belief.Retriever {
	if a == nil || a.cfg == nil || !a.cfg.BeliefMemory.EnableRAGEvidence {
		return inner
	}
	if a.ragService == nil {
		return inner
	}
	rag := a.ragService
	k := a.cfg.BeliefMemory.RAGRetrievalK
	maxItems := a.cfg.BeliefMemory.MaxRAGEvidencePerPrompt
	minScore := a.cfg.BeliefMemory.RAGMinScore
	source := belief.RAGEvidenceSource{
		K:        k,
		MinScore: minScore,
		MaxItems: maxItems,
		Retriever: func(ctx context.Context, query, tenant string, filter map[string]string, k int) ([]belief.RAGEvidenceItem, error) {
			resp, err := rag.Retrieve(ctx, query, retrieve.RetrieveOptions{
				K:              k,
				Tenant:         tenant,
				Filter:         filter,
				UseRRF:         true,
				IncludeSnippet: true,
				Diversify:      true,
			})
			if err != nil {
				return nil, err
			}
			items := make([]belief.RAGEvidenceItem, 0, len(resp.Items))
			for _, item := range resp.Items {
				var title, url string
				if item.Metadata != nil {
					title = item.Metadata["title"]
					url = item.Metadata["url"]
				}
				if title == "" {
					title = item.Doc.Title
				}
				if url == "" {
					url = item.Doc.URL
				}
				items = append(items, belief.RAGEvidenceItem{
					ID:      item.ID,
					DocID:   item.DocID,
					Score:   item.Score,
					Title:   title,
					URL:     url,
					Snippet: item.Snippet,
					Text:    item.Text,
				})
			}
			return items, nil
		},
	}
	return belief.NewBlendedRetriever(inner, maxItems, source)
}
