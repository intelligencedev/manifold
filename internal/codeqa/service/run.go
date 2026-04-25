package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/codeqa"
	"manifold/internal/codeqa/evolve"
	playartifacts "manifold/internal/playground/artifacts"
)

func (s *Service) Run(ctx context.Context, userID int64, req codeqa.RunRequest, onEvent func(codeqa.RunEvent)) (codeqa.RunResult, error) {
	mode := normalizeMode(req.Mode)
	repoPath := req.RepositoryPath
	if repoPath == "" {
		repoPath = s.opts.Workdir
	}
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		runID = uuid.NewString()
	}
	startedAt := time.Now().UTC()
	result := codeqa.RunResult{
		RunID:      runID,
		Mode:       mode,
		Status:     codeqa.StatusQueued,
		ProjectID:  strings.TrimSpace(req.ProjectID),
		Repository: filepath.Clean(repoPath),
		Artifacts:  map[string]string{},
		StartedAt:  startedAt,
	}
	if err := s.store.Save(ctx, userID, result); err != nil {
		return codeqa.RunResult{}, fmt.Errorf("save initial run: %w", err)
	}
	if _, err := s.emitEvent(ctx, userID, runID, codeqa.RunEventQueued, map[string]any{
		"repository": result.Repository,
		"project_id": result.ProjectID,
		"mode":       result.Mode,
	}, onEvent); err != nil {
		return codeqa.RunResult{}, err
	}
	if err := s.acquireRunSlot(ctx); err != nil {
		return s.failRun(ctx, userID, &result, err, onEvent)
	}
	defer s.releaseRunSlot()
	result.Status = codeqa.StatusRunning
	if err := s.store.Save(ctx, userID, result); err != nil {
		return codeqa.RunResult{}, fmt.Errorf("save running run: %w", err)
	}
	if _, err := s.emitEvent(ctx, userID, runID, codeqa.RunEventStarted, map[string]any{
		"repository": result.Repository,
		"project_id": result.ProjectID,
		"base_ref":   req.BaseRef,
		"head_ref":   req.HeadRef,
		"mode":       result.Mode,
	}, onEvent); err != nil {
		return codeqa.RunResult{}, err
	}
	if mode == codeqa.ModeOptimize {
		return s.runOptimize(ctx, userID, &result, req, repoPath, onEvent)
	}
	runPath := repoPath
	if mode != codeqa.ModeJudge {
		prepared, err := s.workspace.Prepare(ctx, repoPath, runID, mode)
		if err != nil {
			return s.failRun(ctx, userID, &result, err, onEvent)
		}
		defer prepared.Cleanup()
		runPath = prepared.Path
	}
	if err := s.evaluateRunPath(ctx, userID, &result, req, runPath, mode, onEvent, nil); err != nil {
		return s.failRun(ctx, userID, &result, err, onEvent)
	}
	result.Status = codeqa.StatusCompleted
	result.CompletedAt = time.Now().UTC()
	if err := s.persistArtifacts(ctx, req, &result); err != nil {
		return s.failRun(ctx, userID, &result, err, onEvent)
	}
	if err := s.store.Save(ctx, userID, result); err != nil {
		return codeqa.RunResult{}, fmt.Errorf("save run: %w", err)
	}
	if _, err := s.emitEvent(ctx, userID, runID, codeqa.RunEventCompleted, map[string]any{
		"action":        result.Aggregate.Action,
		"quality_delta": result.Aggregate.QualityDelta,
		"confidence":    result.Aggregate.Confidence,
	}, onEvent); err != nil {
		return codeqa.RunResult{}, err
	}
	return result, nil
}

func (s *Service) runOptimize(ctx context.Context, userID int64, result *codeqa.RunResult, req codeqa.RunRequest, repoPath string, onEvent func(codeqa.RunEvent)) (codeqa.RunResult, error) {
	iterations := evolve.NormalizeMaxIterations(req.MaxIterations)
	var (
		bestResult    *codeqa.RunResult
		bestDelta     = -math.MaxFloat64
		iterationData = map[string]string{}
	)
	for iteration := 0; iteration < iterations; iteration++ {
		iterationPath := filepath.Join(result.RunID, fmt.Sprintf("iteration-%03d", iteration))
		prepared, err := s.workspace.Prepare(ctx, repoPath, iterationPath, codeqa.ModeOptimize)
		if err != nil {
			return s.failRun(ctx, userID, result, err, onEvent)
		}
		candidateErr := func() error {
			defer prepared.Cleanup()
			if _, err := s.emitEvent(ctx, userID, result.RunID, codeqa.RunEventIterationStarted, map[string]any{
				"iteration":      iteration + 1,
				"max_iterations": iterations,
				"target_paths":   req.TargetPaths,
			}, onEvent); err != nil {
				return err
			}
			candidate, err := s.optimizer.GenerateCandidate(ctx, prepared.Path, req, iteration)
			if err != nil {
				return err
			}
			if err := s.saveOptimizeArtifacts(ctx, result.RunID, iteration, candidate, prepared.Path, iterationData); err != nil {
				return err
			}
			candidateReq := req
			candidateReq.BaseRef = "HEAD~1"
			candidateReq.HeadRef = "HEAD"
			if err := s.evaluateRunPath(ctx, userID, result, candidateReq, prepared.Path, codeqa.ModeOptimize, onEvent, map[string]any{"iteration": iteration + 1}); err != nil {
				return err
			}
			if _, err := s.emitEvent(ctx, userID, result.RunID, codeqa.RunEventIterationCompleted, map[string]any{
				"iteration":     iteration + 1,
				"summary":       candidate.Summary,
				"edited_files":  candidate.EditedFiles,
				"action":        result.Aggregate.Action,
				"quality_delta": result.Aggregate.QualityDelta,
			}, onEvent); err != nil {
				return err
			}
			if bestResult == nil || result.Aggregate.QualityDelta > bestDelta {
				bestDelta = result.Aggregate.QualityDelta
				cloned := cloneRunResult(*result)
				bestResult = &cloned
			}
			return nil
		}()
		if candidateErr != nil {
			return s.failRun(ctx, userID, result, candidateErr, onEvent)
		}
		if result.Aggregate.Action == codeqa.ActionAccept {
			break
		}
	}
	if bestResult != nil {
		*result = *bestResult
	}
	for name, path := range iterationData {
		result.Artifacts[name] = path
	}
	result.Status = codeqa.StatusCompleted
	result.CompletedAt = time.Now().UTC()
	if err := s.persistArtifacts(ctx, req, result); err != nil {
		return s.failRun(ctx, userID, result, err, onEvent)
	}
	if err := s.store.Save(ctx, userID, *result); err != nil {
		return codeqa.RunResult{}, fmt.Errorf("save optimized run: %w", err)
	}
	if _, err := s.emitEvent(ctx, userID, result.RunID, codeqa.RunEventCompleted, map[string]any{
		"action":        result.Aggregate.Action,
		"quality_delta": result.Aggregate.QualityDelta,
		"confidence":    result.Aggregate.Confidence,
	}, onEvent); err != nil {
		return codeqa.RunResult{}, err
	}
	return *result, nil
}

func (s *Service) evaluateRunPath(ctx context.Context, userID int64, result *codeqa.RunResult, req codeqa.RunRequest, runPath string, mode codeqa.RunMode, onEvent func(codeqa.RunEvent), extra map[string]any) error {
	bundle, err := s.packager.Build(ctx, runPath, req.BaseRef, req.HeadRef, req.IncludeRepoContext, req.MaxDiffBytes, req.MaxChangedFiles)
	if err != nil {
		return err
	}
	result.Diff = bundle
	if err := s.store.Save(ctx, userID, *result); err != nil {
		return fmt.Errorf("save bundled run: %w", err)
	}
	if _, err := s.emitEvent(ctx, userID, result.RunID, codeqa.RunEventDiffPackaged, mergePayload(extra, map[string]any{
		"changed_files": len(bundle.Files),
		"truncated":     bundle.Truncated,
	}), onEvent); err != nil {
		return err
	}
	gates, err := s.gateRun.Evaluate(ctx, runPath, bundle.BaseRef, bundle.HeadRef)
	if err != nil {
		return err
	}
	result.Gates = gates
	if err := s.store.Save(ctx, userID, *result); err != nil {
		return fmt.Errorf("save gated run: %w", err)
	}
	if _, err := s.emitEvent(ctx, userID, result.RunID, codeqa.RunEventGatesCompleted, mergePayload(extra, map[string]any{
		"gate_count": len(gates),
	}), onEvent); err != nil {
		return err
	}
	judges, err := s.judge.Evaluate(ctx, bundle, gates)
	if err != nil {
		return err
	}
	result.Judges = judges
	if err := s.store.Save(ctx, userID, *result); err != nil {
		return fmt.Errorf("save judged run: %w", err)
	}
	if _, err := s.emitEvent(ctx, userID, result.RunID, codeqa.RunEventJudgesCompleted, mergePayload(extra, map[string]any{
		"judge_count": len(judges),
	}), onEvent); err != nil {
		return err
	}
	result.Aggregate = codeqa.Decide(codeqa.DecisionInput{
		Mode:            mode,
		Bundle:          bundle,
		Gates:           gates,
		Judges:          judges,
		AcceptThreshold: s.opts.EffectiveAcceptThreshold(req.AcceptThreshold),
		MinConfidence:   s.opts.EffectiveMinConfidence(req.MinConfidence),
		HighRiskGlobs:   s.opts.HighRiskGlobs,
	})
	return nil
}

func (s *Service) saveOptimizeArtifacts(ctx context.Context, runID string, iteration int, candidate evolve.Candidate, repoPath string, artifacts map[string]string) error {
	patch, err := s.runner.Run(ctx, repoPath, codeqa.CommandRequest{Command: "git", Args: []string{"format-patch", "--stdout", "-1", "HEAD"}})
	if err != nil {
		return fmt.Errorf("capture optimizer patch: %w", err)
	}
	entries := map[string][]byte{
		fmt.Sprintf("patches/iteration-%03d.patch", iteration):           []byte(patch.Stdout),
		fmt.Sprintf("optimizer/iteration-%03d-prompt.txt", iteration):    []byte(candidate.Prompt),
		fmt.Sprintf("optimizer/iteration-%03d-response.json", iteration): []byte(candidate.Response),
	}
	for name, payload := range entries {
		path, saveErr := s.artifacts.Save(ctx, runID, playartifacts.Artifact{Name: name, ContentType: "application/octet-stream", Bytes: payload})
		if saveErr != nil {
			return saveErr
		}
		artifacts[name] = path
	}
	return nil
}

func mergePayload(base map[string]any, extra map[string]any) map[string]any {
	if len(base) == 0 {
		return extra
	}
	out := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func cloneRunResult(in codeqa.RunResult) codeqa.RunResult {
	b, _ := json.Marshal(in)
	var out codeqa.RunResult
	_ = json.Unmarshal(b, &out)
	if out.Artifacts == nil {
		out.Artifacts = map[string]string{}
	}
	return out
}

func (s *Service) acquireRunSlot(ctx context.Context) error {
	select {
	case s.runSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) releaseRunSlot() {
	select {
	case <-s.runSlots:
	default:
	}
}

func normalizeMode(mode codeqa.RunMode) codeqa.RunMode {
	switch mode {
	case codeqa.ModeGate, codeqa.ModeOptimize:
		return mode
	default:
		return codeqa.ModeJudge
	}
}

func (s *Service) failRun(ctx context.Context, userID int64, result *codeqa.RunResult, runErr error, onEvent func(codeqa.RunEvent)) (codeqa.RunResult, error) {
	result.Status = codeqa.StatusFailed
	result.Error = runErr.Error()
	result.CompletedAt = time.Now().UTC()
	if result.Aggregate.Action == "" {
		result.Aggregate.Action = codeqa.ActionHumanReview
		result.Aggregate.Rationale = runErr.Error()
	}
	if saveErr := s.store.Save(ctx, userID, *result); saveErr != nil {
		return codeqa.RunResult{}, fmt.Errorf("save failed run: %w", saveErr)
	}
	if _, eventErr := s.emitEvent(ctx, userID, result.RunID, codeqa.RunEventFailed, map[string]any{"error": runErr.Error()}, onEvent); eventErr != nil {
		return codeqa.RunResult{}, eventErr
	}
	return *result, nil
}

func (s *Service) emitEvent(ctx context.Context, userID int64, runID string, typ codeqa.RunEventType, payload map[string]any, onEvent func(codeqa.RunEvent)) (codeqa.RunEvent, error) {
	event, err := s.store.AppendEvent(ctx, userID, codeqa.RunEvent{
		RunID:      runID,
		Type:       typ,
		Payload:    payload,
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("append run event: %w", err)
	}
	if onEvent != nil {
		onEvent(event)
	}
	return event, nil
}

func (s *Service) persistArtifacts(ctx context.Context, req codeqa.RunRequest, result *codeqa.RunResult) error {
	requestBytes, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	report := renderMarkdown(*result)
	artifacts := map[string][]byte{
		"request.json": requestBytes,
		"result.json":  resultBytes,
		"report.md":    []byte(report),
	}
	for name, payload := range artifacts {
		path, saveErr := s.artifacts.Save(ctx, result.RunID, playartifacts.Artifact{Name: name, ContentType: "application/octet-stream", Bytes: payload})
		if saveErr != nil {
			return fmt.Errorf("save %s: %w", name, saveErr)
		}
		result.Artifacts[name] = path
	}
	return nil
}
