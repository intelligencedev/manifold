package codeqa

import "time"

type RunMode string

const (
	ModeJudge    RunMode = "judge"
	ModeGate     RunMode = "gate"
	ModeOptimize RunMode = "optimize"
)

type RecommendedAction string

type RunStatus string

type RunEventType string

const (
	ActionAccept          RecommendedAction = "accept"
	ActionReject          RecommendedAction = "reject"
	ActionRevertCandidate RecommendedAction = "revert_candidate"
	ActionHumanReview     RecommendedAction = "human_review"

	StatusQueued    RunStatus = "queued"
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"

	RunEventQueued             RunEventType = "queued"
	RunEventStarted            RunEventType = "run_started"
	RunEventIterationStarted   RunEventType = "iteration_started"
	RunEventDiffPackaged       RunEventType = "diff_packaged"
	RunEventGatesCompleted     RunEventType = "gates_completed"
	RunEventJudgesCompleted    RunEventType = "judges_completed"
	RunEventIterationCompleted RunEventType = "iteration_completed"
	RunEventCompleted          RunEventType = "run_completed"
	RunEventFailed             RunEventType = "run_failed"
)

type ChangedFile struct {
	Path         string   `json:"path"`
	Status       string   `json:"status"`
	RelatedTests []string `json:"related_tests,omitempty"`
	Binary       bool     `json:"binary,omitempty"`
}

type DiffBundle struct {
	BaseRef     string            `json:"base_ref"`
	HeadRef     string            `json:"head_ref"`
	Files       []ChangedFile     `json:"files"`
	UnifiedDiff string            `json:"unified_diff"`
	SourceTrees map[string]string `json:"source_trees,omitempty"`
	RepoContext string            `json:"repo_context,omitempty"`
	Truncated   bool              `json:"truncated"`
}

type GateResult struct {
	Name       string             `json:"name"`
	Ref        string             `json:"ref,omitempty"`
	OK         bool               `json:"ok"`
	HardFail   bool               `json:"hard_fail"`
	Skipped    bool               `json:"skipped,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Stdout     string             `json:"stdout,omitempty"`
	Stderr     string             `json:"stderr,omitempty"`
	DurationMs int64              `json:"duration_ms"`
}

type JudgeVerdict struct {
	JudgeID          string             `json:"judge_id"`
	Verdict          string             `json:"verdict"`
	Confidence       float64            `json:"confidence"`
	Scores           map[string]float64 `json:"scores"`
	BlockingConcerns []string           `json:"blocking_concerns,omitempty"`
	SwapApplied      bool               `json:"swap_applied"`
	Evidence         []string           `json:"evidence,omitempty"`
}

type Aggregate struct {
	QualityDelta float64           `json:"quality_delta"`
	Confidence   float64           `json:"confidence"`
	HardFailures []string          `json:"hard_failures,omitempty"`
	Action       RecommendedAction `json:"action"`
	Rationale    string            `json:"rationale"`
}

type RunEvent struct {
	RunID      string         `json:"run_id"`
	Sequence   int64          `json:"sequence"`
	Type       RunEventType   `json:"type"`
	Payload    map[string]any `json:"payload,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type RunRequest struct {
	RunID               string   `json:"-"`
	Mode                RunMode  `json:"mode,omitempty"`
	ProjectID           string   `json:"project_id,omitempty"`
	RepositoryPath      string   `json:"repository_path"`
	BaseRef             string   `json:"base_ref,omitempty"`
	HeadRef             string   `json:"head_ref,omitempty"`
	MaxDiffBytes        int      `json:"max_diff_bytes,omitempty"`
	MaxChangedFiles     int      `json:"max_changed_files,omitempty"`
	IncludeRepoContext  bool     `json:"include_repo_context,omitempty"`
	AcceptThreshold     float64  `json:"accept_threshold,omitempty"`
	MinConfidence       float64  `json:"min_confidence,omitempty"`
	Objective           string   `json:"objective,omitempty"`
	TargetPaths         []string `json:"target_paths,omitempty"`
	MaxIterations       int      `json:"max_iterations,omitempty"`
	AutoApply           bool     `json:"auto_apply,omitempty"`
	CommitAccepted      bool     `json:"commit_accepted,omitempty"`
	IncludeCommandLogs  bool     `json:"include_command_logs,omitempty"`
	IncludeBundleOutput bool     `json:"include_bundle_output,omitempty"`
}

type RunResult struct {
	RunID       string            `json:"run_id"`
	Mode        RunMode           `json:"mode"`
	Status      RunStatus         `json:"status"`
	ProjectID   string            `json:"project_id,omitempty"`
	Repository  string            `json:"repository"`
	Error       string            `json:"error,omitempty"`
	Diff        DiffBundle        `json:"diff"`
	Gates       []GateResult      `json:"gates"`
	Judges      []JudgeVerdict    `json:"judges"`
	Aggregate   Aggregate         `json:"aggregate"`
	Artifacts   map[string]string `json:"artifacts,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at"`
}
