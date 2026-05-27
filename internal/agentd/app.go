package agentd

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/auth"
	codeqaservice "manifold/internal/codeqa/service"
	"manifold/internal/config"
	"manifold/internal/constitution"
	"manifold/internal/durable"
	"manifold/internal/embeddedpg"
	"manifold/internal/fleet"
	llmpkg "manifold/internal/llm"
	"manifold/internal/matrixgw"
	"manifold/internal/mcpclient"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/projects"
	ragservice "manifold/internal/rag/service"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	tooldiscovery "manifold/internal/tools/discovery"
	terminaltool "manifold/internal/tools/terminal"
	transitdomain "manifold/internal/transit"
	"manifold/internal/trust"
	"manifold/internal/workspaces"
)

const systemUserID int64 = 0

const (
	defaultEvolvingPersistDebounce = 250 * time.Millisecond
	defaultEvolvingSessionTTL      = time.Hour
	defaultEvolvingJanitorInterval = 15 * time.Minute
)

type app struct {
	cfg                *config.Config
	httpClient         *http.Client
	mgr                *databases.Manager
	llm                llmpkg.Provider
	baseToolRegistry   tools.Registry
	toolRegistry       tools.Registry
	toolIndex          *tooldiscovery.ToolIndex
	specRegistry       *specialists.Registry
	specRegMu          sync.RWMutex
	userSpecRegs       map[int64]*specialists.Registry
	summaryLLM         llmpkg.Provider
	durableStore       durable.Store
	durableClient      *durable.Client
	durableRegistry    *durable.Registry
	durableWorker      *durable.Worker
	flowV2             *flowV2Runtime
	codeQARuntime      *codeQARuntime
	codeQAService      *codeqaservice.Service
	terminalManager    *terminaltool.Manager
	evolvingMu         sync.RWMutex
	userEvolving       map[int64]map[string]*memory.EvolvingMemory
	evolvingLastUsed   map[int64]map[string]time.Time
	evolvingCfg        memory.EvolvingMemoryConfig
	evolvingSessionTTL time.Duration
	rememMaxInnerSteps int
	beliefLLM          llmpkg.Provider
	beliefModel        string
	engine             *agent.Engine
	chatStore          persist.ChatStore
	matrixMessageStore persist.MatrixMessageStore
	activityStore      persist.SpecialistActivityStore
	chatMemory         *memory.Manager
	runs               *runStore
	inputRequests      *inputRequestBroker
	playgroundHandler  http.Handler
	projectsService    projects.ProjectService
	workspaceManager   workspaces.WorkspaceManager
	warppToolMu        sync.Mutex
	warppToolNames     []string
	authStore          *auth.Store
	authProvider       auth.Provider
	specStore          persist.SpecialistsStore
	teamStore          persist.SpecialistTeamsStore
	mcpStore           persist.MCPStore
	userPrefsStore     persist.UserPreferencesStore
	mcpManager         *mcpclient.Manager
	mcpPool            *mcpclient.MCPServerPool
	startupMCPOAuthIDs []int64
	tokenMetrics       []tokenMetricsProvider
	memoryMetrics      []memoryMetricsProvider
	traceMetrics       []traceMetricsProvider
	runMetrics         *clickhouseRunMetrics
	logMetrics         []logMetricsProvider
	transitService     *transitdomain.Service
	embeddedRuntime    *embeddedpg.Runtime
	ragService         *ragservice.Service
	matrixGateway      *matrixgw.Service
	pulseRuntime       *pulseRuntime
	fleetBus           *fleet.Bus
	trustService       *trust.Service
	constitutionSvc    *constitution.Service
	extraPools         []*pgxpool.Pool
}

type tokenMetricsProvider interface {
	TokenTotals(ctx context.Context, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error)
	TokenTotalsForUser(ctx context.Context, userID int64, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error)
	Source() string
}

type searchIndexerAdapter struct{ search databases.FullTextSearch }

func (a searchIndexerAdapter) Index(ctx context.Context, id string, text string, metadata map[string]string) error {
	return a.search.Index(ctx, id, text, metadata)
}

func (a searchIndexerAdapter) Remove(ctx context.Context, id string) error {
	return a.search.Remove(ctx, id)
}

func (a searchIndexerAdapter) Search(ctx context.Context, query string, limit int) ([]transitdomain.SearchIndexResult, error) {
	results, err := a.search.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]transitdomain.SearchIndexResult, 0, len(results))
	for _, result := range results {
		out = append(out, transitdomain.SearchIndexResult{
			ID:       result.ID,
			Score:    result.Score,
			Snippet:  result.Snippet,
			Text:     result.Text,
			Metadata: result.Metadata,
		})
	}
	return out, nil
}

type vectorIndexerAdapter struct{ vector databases.VectorStore }

func (a vectorIndexerAdapter) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	return a.vector.Upsert(ctx, id, vector, metadata)
}

func (a vectorIndexerAdapter) Delete(ctx context.Context, id string) error {
	return a.vector.Delete(ctx, id)
}

func (a vectorIndexerAdapter) SimilaritySearch(ctx context.Context, vector []float32, k int, filter map[string]string) ([]transitdomain.VectorIndexResult, error) {
	results, err := a.vector.SimilaritySearch(ctx, vector, k, filter)
	if err != nil {
		return nil, err
	}
	out := make([]transitdomain.VectorIndexResult, 0, len(results))
	for _, result := range results {
		out = append(out, transitdomain.VectorIndexResult{
			ID:       result.ID,
			Score:    result.Score,
			Metadata: result.Metadata,
		})
	}
	return out, nil
}

// cloneEngine returns a shallow copy of the base orchestrator engine so that
// per-request callbacks (OnDelta/OnTool/etc) don't race across concurrent
// requests. Callers can safely mutate the returned engine without affecting
