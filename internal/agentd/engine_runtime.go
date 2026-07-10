package agentd

import (
	"context"
	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/agent/memory/artifact"
	artifactconnectors "manifold/internal/agent/memory/artifact/connectors"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/agent/memory/magma"
	"manifold/internal/config"
	"manifold/internal/embedding"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/persistence"
	"manifold/internal/policy"
	"manifold/internal/rag/retrieve"
	"manifold/internal/specialists"
	agenttools "manifold/internal/tools/agents"
	"strings"
	"time"

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
func (a *app) cloneEngineForUser(ctx context.Context, userID int64, sessionID, projectID, objectiveID string, settingsOpt ...chatMemoryRunSettings) *agent.Engine {
	eng := a.cloneEngine()
	if eng == nil {
		return nil
	}
	settings := withChatMemorySettings(settingsOpt)
	a.configureBeliefRunState(eng, userID, sessionID, projectID, objectiveID, "orchestrator")

	if a.cfg.Auth.Enabled && userID != systemUserID {
		eng.System = a.composeSystemPromptForUser(ctx, userID)
		eng.UserPromptContext = a.composeUserPromptContextForUser(ctx, userID)
	}

	em := a.attachSessionEvolvingMemory(eng, userID, sessionID, settings.EvolvingMemoryEnabled)
	if a.cfg.Auth.Enabled && userID != systemUserID {
		a.applyUserOrchestratorOverlay(ctx, eng, userID)
	}

	delegator := a.newRunDelegator(ctx, eng, userID, em, settings)
	eng.Delegator = delegator
	eng.TeamDelegator = a

	return eng
}

func (a *app) applyUserOrchestratorOverlay(ctx context.Context, eng *agent.Engine, userID int64) {
	sp, ok, err := a.specStore.GetByName(ctx, userID, specialists.OrchestratorName)
	if err != nil || !ok {
		return
	}
	a.applyUserOrchestratorLLM(eng, sp)
	eng.Tools = a.chatToolRegistry(sp.EnableTools, sp.AllowTools, sp.AutoDiscover, sp.RequestInfoEnabled)
	if sp.Harness != nil {
		harnessCfg := harnessOverrideConfig(a.cfg.Harness, harnessConfigFromPersist(sp.Harness))
		eng.HarnessEnabled = harnessCfg.Enabled
		eng.HarnessConfig = harnessRunConfig(harnessCfg)
	}
	if sp.System != "" {
		eng.System = a.composeSystemPromptForUserWithOverride(ctx, userID, sp.System)
		eng.UserPromptContext = a.composeUserPromptContextForUser(ctx, userID)
	}
}

func (a *app) applyUserOrchestratorLLM(eng *agent.Engine, sp persistence.Specialist) {
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
	if currentModel := currentSpecialistModel(sp, llmCfg, provider); currentModel != "" {
		eng.Model = currentModel
	}
}

func currentSpecialistModel(sp persistence.Specialist, llmCfg config.LLMClientConfig, provider string) string {
	currentModel := strings.TrimSpace(sp.Model)
	if currentModel != "" {
		return currentModel
	}
	switch config.ProviderBackend(provider) {
	case "anthropic":
		return strings.TrimSpace(llmCfg.Anthropic.Model)
	case "google":
		return strings.TrimSpace(llmCfg.Google.Model)
	default:
		return strings.TrimSpace(llmCfg.OpenAI.Model)
	}
}

func (a *app) newRunDelegator(ctx context.Context, eng *agent.Engine, userID int64, em *memory.EvolvingMemory, settings chatMemoryRunSettings) *agenttools.Delegator {
	reg := a.specRegistry
	if a.cfg.Auth.Enabled && userID != systemUserID {
		if userReg, err := a.specialistsRegistryForUser(ctx, userID); err == nil && userReg != nil {
			reg = userReg
		}
	}
	delegator := agenttools.NewDelegator(eng.Tools, reg, a.workspaceManager, a.cfg.MaxSteps)
	delegator.SetDefaultTimeout(a.cfg.AgentRunTimeoutSeconds)
	applyChatMemorySettingsToEngine(eng, settings)
	if eng.DisableEvolvingMemory {
		em = nil
	}
	a.configureUnifiedMemoryRuntime(eng, em, settings)
	delegator.SetMemoryRuntime(eng.Memory)
	delegator.SetEvolvingMemory(em)
	delegator.SetBeliefMemory(eng.BeliefStore)
	delegator.SetBeliefDistiller(eng.BeliefDistiller)
	delegator.SetBeliefRetriever(eng.BeliefRetriever, eng.BeliefMaxBeliefsPerPrompt, eng.BeliefPromptTokenBudget)
	delegator.SetBeliefLifecycle(eng.BeliefGraph, eng.BeliefPromotionThreshold)
	delegator.SetPolicyEnforcer(eng.PolicyEnforcer)
	delegator.SetTeamDelegator(a)
	if eng.ReMemEnabled {
		delegator.ConfigureReMem(a.evolvingCfg.LLM, a.evolvingCfg.Model, a.rememMaxInnerSteps)
	}
	return delegator
}

func (a *app) configureUnifiedMemoryRuntime(eng *agent.Engine, em *memory.EvolvingMemory, settings chatMemoryRunSettings) {
	if eng == nil {
		return
	}
	settings = normalizeChatMemoryRunSettings(settings)
	if a.cfg == nil || !settings.MemoryEnabled {
		eng.Memory = nil
		eng.DisableMemory = true
		return
	}
	eng.DisableMemory = false
	var policyProvider policy.ContextProvider
	if provider, ok := eng.PolicyEnforcer.(policy.ContextProvider); ok {
		policyProvider = provider
	}
	var magmaRetriever memory.MagmaRetriever
	if magmaService := a.ragServiceMagma(); magmaService != nil {
		magmaRetriever = runtimeMagmaRetriever{service: magmaService}
	}
	eng.Memory = &memory.Runtime{
		Config: memory.RuntimeConfig{
			Enabled:            true,
			MaxTokensPerPrompt: a.cfg.Memory.Retrieval.MaxTokensPerPrompt,
			Timeout:            time.Duration(a.cfg.Memory.Retrieval.TimeoutMs) * time.Millisecond,
			IncludeRecent:      a.cfg.Memory.Retrieval.IncludeRecent,
		},
		Evolving:                  em,
		Belief:                    eng.BeliefRetriever,
		PolicyProvider:            policyProvider,
		Magma:                     magmaRetriever,
		BeliefMaxBeliefs:          eng.BeliefMaxBeliefsPerPrompt,
		BeliefPromptTokenBudget:   eng.BeliefPromptTokenBudget,
		BeliefMinConfidence:       eng.BeliefRetrievalMinConfidence,
		BeliefContradictions:      eng.BeliefIncludeContradictions,
		MagmaIntentClassification: a.cfg.Magma.Retrieval.IntentClassification,
		MagmaContextFormat:        a.cfg.Magma.Retrieval.ContextFormat,
		MagmaMaxHops:              a.cfg.Magma.Retrieval.DefaultHops,
		MagmaMaxNodes:             a.cfg.Magma.Retrieval.DefaultMaxNodes,
	}
	// Decision lane: deterministic store reads only, enabled when archaeology
	// is on for the server and memory is on for this session.
	if a.cfg.Archaeology.Enabled && a.mgr != nil && a.mgr.Decision != nil {
		eng.Memory.Decision = decision.NewStoreRetriever(a.mgr.Decision, a.mgr.Belief)
		eng.Memory.DecisionMaxPerPrompt = a.cfg.Archaeology.Retrieval.MaxDecisionsPerPrompt
		eng.Memory.DecisionPromptTokenBudget = a.cfg.Archaeology.Retrieval.MaxTokensPerPrompt
		eng.Memory.DecisionTimeout = time.Duration(a.cfg.Archaeology.Retrieval.TimeoutMs) * time.Millisecond
	}
}

func (a *app) ragServiceMagma() *magma.Service {
	if a == nil || a.ragService == nil {
		return nil
	}
	return a.ragService.MagmaService()
}

type runtimeMagmaRetriever struct {
	service *magma.Service
}

func (r runtimeMagmaRetriever) RetrieveMagmaContext(ctx context.Context, req memory.MagmaRequest) (memory.MagmaContext, error) {
	magmaCtx, err := (magma.QueryEngine{Service: r.service}).Query(ctx, req.Query, magma.QueryOptions{
		Tenant:               req.Tenant,
		MaxHops:              req.MaxHops,
		MaxNodes:             req.MaxNodes,
		ContextFormat:        req.ContextFormat,
		IntentClassification: req.IntentClassification,
	})
	if err != nil {
		return memory.MagmaContext{}, err
	}
	return memory.MagmaContext{Text: magmaCtx.Text, Items: len(magmaCtx.RawEvents)}, nil
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
		eng.BeliefLifecyclePolicy = belief.PromotionPolicy{
			MinEvidenceFor:       a.cfg.BeliefMemory.Lifecycle.MinEvidenceForPromotion,
			MaxEvidenceAgainst:   a.cfg.BeliefMemory.Lifecycle.MaxEvidenceAgainstPromotion,
			ConfidenceThreshold:  a.cfg.BeliefMemory.PromotionThreshold,
			StaleAfter:           durationDays(a.cfg.BeliefMemory.Lifecycle.StaleAfterDays),
			StaleConfidenceDecay: a.cfg.BeliefMemory.Lifecycle.StaleConfidenceDecay,
		}
		eng.BeliefEnforcementPolicy = belief.EnforcementPolicy{
			AutoEnable:                   a.cfg.BeliefMemory.Enforcement.AutoEnable,
			SoftPolicyThreshold:          a.cfg.BeliefMemory.Enforcement.SoftPolicyThreshold,
			HardConstraintThreshold:      a.cfg.BeliefMemory.Enforcement.HardConstraintThreshold,
			HardConstraintMinEvidenceFor: a.cfg.BeliefMemory.Enforcement.HardConstraintMinEvidenceFor,
		}
		if a.cfg.BeliefMemory.EnableDistillation {
			eng.BeliefDistiller = a.newBeliefDistiller()
		} else {
			eng.BeliefDistiller = nil
		}
		if a.cfg.BeliefMemory.EnableRetrieval {
			base := belief.NewGraphEnrichedRetriever(a.mgr.Belief, a.mgr.Graph, 2)
			eng.BeliefRetriever = a.applyRAGEvidenceBlend(base)
			eng.BeliefMaxBeliefsPerPrompt = a.cfg.BeliefMemory.MaxBeliefsPerPrompt
			eng.BeliefPromptTokenBudget = a.cfg.BeliefMemory.Retrieval.MaxTokensPerPrompt
			eng.BeliefRetrievalMinConfidence = a.cfg.BeliefMemory.Retrieval.MinConfidence
			eng.BeliefIncludeContradictions = a.cfg.BeliefMemory.Retrieval.IncludeContradictions
		} else {
			eng.BeliefRetriever = nil
			eng.BeliefMaxBeliefsPerPrompt = 0
			eng.BeliefPromptTokenBudget = 0
			eng.BeliefRetrievalMinConfidence = 0
			eng.BeliefIncludeContradictions = false
		}
		if a.cfg.BeliefMemory.EnableConstraintEnforcement && a.transitService != nil {
			eng.PolicyEnforcer = policy.NewTransitEnforcer(a.transitService)
			if a.cfg.BeliefMemory.Enforcement.AutoEnable {
				eng.BeliefPolicySink = beliefPolicySink{service: a.transitService}
			} else {
				eng.BeliefPolicySink = nil
			}
		} else {
			eng.PolicyEnforcer = nil
			eng.BeliefPolicySink = nil
		}
		if a.cfg.Magma.Enabled && a.ragService != nil && a.ragService.MagmaService() != nil {
			eng.BeliefMagmaSink = beliefMagmaSink{service: a.ragService.MagmaService(), workerCount: a.cfg.Magma.Consolidation.WorkerCount}
		} else {
			eng.BeliefMagmaSink = nil
		}
		a.configureArchaeologyRunState(eng)
	} else {
		eng.BeliefStore = nil
		eng.BeliefDistiller = nil
		eng.BeliefRetriever = nil
		eng.BeliefGraph = nil
		eng.BeliefMaxBeliefsPerPrompt = 0
		eng.BeliefPromptTokenBudget = 0
		eng.BeliefRetrievalMinConfidence = 0
		eng.BeliefIncludeContradictions = false
		eng.BeliefPromotionThreshold = 0
		eng.BeliefLifecyclePolicy = belief.PromotionPolicy{}
		eng.BeliefEnforcementPolicy = belief.EnforcementPolicy{}
		eng.BeliefPolicySink = nil
		eng.BeliefMagmaSink = nil
		eng.DecisionStore = nil
		eng.DecisionDistiller = nil
		eng.DecisionService = nil
		eng.ArtifactCapture = nil
		eng.PolicyEnforcer = nil
	}
}

func (a *app) configureArchaeologyRunState(eng *agent.Engine) {
	if a == nil || a.cfg == nil || a.mgr.Decision == nil || !a.cfg.Archaeology.Enabled {
		eng.DecisionStore = nil
		eng.DecisionDistiller = nil
		eng.DecisionService = nil
		eng.ArtifactCapture = nil
		return
	}
	eng.DecisionStore = a.mgr.Decision
	if a.cfg.Archaeology.DecisionDistiller {
		eng.DecisionDistiller = a.newDecisionDistiller()
	}
	if a.mgr.Artifact != nil {
		connectors := []artifact.Connector{artifactconnectors.GitConnector{}}
		if a.mgr.Chat != nil {
			connectors = append(connectors, artifactconnectors.ChatConnector{Store: a.mgr.Chat})
		}
		if a.mgr.Transit != nil {
			connectors = append(connectors, artifactconnectors.TransitConnector{Store: a.mgr.Transit})
		}
		eng.ArtifactCapture = &artifact.CaptureManager{
			Store:      a.mgr.Artifact,
			Connectors: connectors,
			Timeout:    10 * time.Second,
			OnError: func(kind artifact.ArtifactKind, err error) {
				log.Debug().Err(err).Str("kind", string(kind)).Msg("artifact_capture_skipped")
			},
		}
	}
	service := &decision.Service{Store: a.mgr.Decision, Belief: eng.BeliefStore, Config: decision.ServiceConfig{
		AutoActivateCandidates:    a.cfg.Archaeology.AutoActivate.Enabled,
		AutoActivateMinConfidence: a.cfg.Archaeology.AutoActivate.MinConfidence,
		ConflictSimilarityFloor:   a.cfg.Archaeology.AutoActivate.ConflictSimilarityFloor,
	}}
	eng.DecisionService = service
	reactor := &decision.Reactor{
		Decisions: service,
		Beliefs:   eng.BeliefStore,
		Config: decision.ReactorConfig{
			ConfidenceFloor:     a.cfg.Archaeology.Reactor.ConfidenceFloor,
			ConfidenceDropDelta: a.cfg.Archaeology.Reactor.ConfidenceDropDelta,
		},
	}
	if notifying, ok := eng.BeliefStore.(*belief.NotifyingStore); ok {
		notifying.RegisterListener(reactor)
		return
	}
	if eng.BeliefStore != nil {
		notifying := belief.NewNotifyingStore(eng.BeliefStore, 256)
		notifying.RegisterListener(reactor)
		eng.BeliefStore = notifying
	}
}

func (a *app) newDecisionDistiller() decision.Distiller {
	var embed decision.EmbedFunc
	cfg := a.cfg.Embedding
	if strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "" {
		embed = func(ctx context.Context, texts []string) ([][]float32, error) {
			return embedding.EmbedText(ctx, cfg, texts)
		}
	}
	if a.beliefLLM != nil {
		return decision.LLMDistiller{Config: decision.LLMDistillerConfig{
			LLM:                    a.beliefLLM,
			Model:                  a.beliefModel,
			MaxCandidates:          a.cfg.BeliefMemory.Distillation.MaxCandidatesPerEpisode,
			MinCandidateConfidence: a.cfg.BeliefMemory.Distillation.MinCandidateConfidence,
			Embed:                  embed,
		}}
	}
	return decision.SimpleDistiller{Embed: embed}
}

func (a *app) newBeliefDistiller() belief.Distiller {
	if a == nil || a.cfg == nil {
		return belief.SimpleDistiller{}
	}
	cfg := a.cfg.Embedding
	var embed belief.EmbedFunc
	if strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "" {
		embed = func(ctx context.Context, texts []string) ([][]float32, error) {
			return embedding.EmbedText(ctx, cfg, texts)
		}
	}
	if a.cfg.BeliefMemory.Distillation.Mode == "llm" && a.beliefLLM != nil {
		return belief.LLMDistiller{Config: belief.LLMDistillerConfig{
			LLM:                    a.beliefLLM,
			Model:                  a.beliefModel,
			MaxCandidates:          a.cfg.BeliefMemory.Distillation.MaxCandidatesPerEpisode,
			MinCandidateConfidence: a.cfg.BeliefMemory.Distillation.MinCandidateConfidence,
			AutoApplyMinConfidence: a.cfg.BeliefMemory.Distillation.AutoApplyMinConfidence,
			DefaultConfidence:      a.cfg.BeliefMemory.DefaultConfidence,
			Embed:                  embed,
		}}
	}
	return belief.SimpleDistiller{Embed: embed}
}

func durationDays(days int) time.Duration {
	if days <= 0 {
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
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
				Rerank:         a.cfg.Reranking.Enabled,
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
