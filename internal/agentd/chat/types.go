package chat

import (
	"context"
	"net/http"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/harness"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/durable"
	"manifold/internal/fleet"
	"manifold/internal/mcpclient"
	persist "manifold/internal/persistence"
	"manifold/internal/projects"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	tooldiscovery "manifold/internal/tools/discovery"
	"manifold/internal/workspaces"
)

// MemoryRunSettings contains the per-run memory switches used by chat
// builders. The settings intentionally collapse to one enabled state for the
// three coordinated memory lanes.
type MemoryRunSettings struct {
	MemoryEnabled         bool
	EvolvingMemoryEnabled bool
	BeliefMemoryEnabled   bool
}

// BuildRequest describes the target and request-specific options needed to
// construct a chat engine.
type BuildRequest struct {
	Name                 string
	SystemPromptOverride string
	SessionID            string
	ProjectID            string
	ObjectiveID          string
	Owner                int64
	CheckedOutWorkspace  *workspaces.Workspace
	MemorySettings       MemoryRunSettings
}

// BuildResult is the builder output consumed by both HTTP and durable chat
// dispatch.
type BuildResult struct {
	Engine          *agent.Engine
	ModelLabel      string
	ImageGeneration bool
	VideoGeneration bool
	StatusCode      int
	Err             error
}

// Deps is the chat dependency slice supplied by the composition root. It is
// deliberately free of agentd types so the chat package can be moved again
// without importing the HTTP server package.
type Deps struct {
	Cfg              *config.Config
	HTTPClient       *http.Client
	BaseToolRegistry tools.Registry
	ToolIndex        *tooldiscovery.ToolIndex
	SpecStore        persist.SpecialistsStore
	TeamStore        persist.SpecialistTeamsStore
	MCPPool          *mcpclient.MCPServerPool
	ChatStore        persist.ChatStore
	ActivityStore    persist.SpecialistActivityStore
	LLMRequestStore  persist.LLMRequestStore
	DurableClient    *durable.Client
	DurableStore     durable.Store
	DurableRegistry  *durable.Registry
	FleetBus         *fleet.Bus
	WorkspaceManager workspaces.WorkspaceManager
	Projects         projects.ProjectService
	ToolRegistry     tools.Registry
	SpecRegistry     func(owner int64) *specialists.Registry

	CloneEngineForUser                     func(context.Context, int64, string, string, string, ...MemoryRunSettings) *agent.Engine
	SpecialistsRegistryForUser             func(context.Context, int64) (*specialists.Registry, error)
	ComposeSystemPromptForUserWithOverride func(context.Context, int64, string) string
	ConfigureBeliefRunState                func(*agent.Engine, int64, string, string, string, string)
	AttachSessionEvolvingMemory            func(*agent.Engine, int64, string, ...bool) *memory.EvolvingMemory
	ConfigureUnifiedMemoryRuntime          func(*agent.Engine, *memory.EvolvingMemory, MemoryRunSettings)
	ResolveTeamOrchestratorSpecialist      func(context.Context, int64, persist.SpecialistTeam) (persist.Specialist, int, error)
	TeamDelegator                          agent.TeamDelegator
	EvolvingConfig                         memory.EvolvingMemoryConfig
	ReMemMaxInnerSteps                     int

	NewSkillReadTool      func(string) tools.Tool
	NewSkillSearchTool    func(string) tools.Tool
	HarnessRunConfig      func(config.HarnessConfig) harness.RunConfig
	HarnessOverrideConfig func(config.HarnessConfig, *config.HarnessConfig) config.HarnessConfig
	SummaryCallTimeout    func(*config.Config) time.Duration
}
