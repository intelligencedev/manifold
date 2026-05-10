package agentd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/matrixgw"
	pulsecore "manifold/internal/pulse"
	"manifold/internal/sandbox"
)

func (a *app) handleMatrixMessage(ctx context.Context, message matrixgw.InboundMessage) error {
	result, matrixMessages, err := a.executeMatrixScopedRun(ctx, chatRunRequest{
		Prompt:      message.Prompt,
		SessionID:   matrixSessionID(message.RoomID, message.Target),
		RoomID:      message.RoomID,
		RouteTarget: message.Target,
	}, message.Target)
	if err != nil {
		return err
	}
	if result != "" && a.matrixGateway != nil {
		if err := a.matrixGateway.SendAttributed(ctx, message.RoomID, message.Target, result); err != nil {
			return err
		}
	}

	log.Info().
		Str("room_id", message.RoomID).
		Str("event_id", message.EventID).
		Str("target", message.Target).
		Str("sender", message.Sender).
		Int("matrix_messages", len(matrixMessages)).
		Int("result_len", len(result)).
		Msg("matrix_gateway_message_processed")
	return nil
}

func (a *app) handlePulseRoom(ctx context.Context, roomID, target, projectID, prompt string) (string, error) {
	result, matrixMessages, err := a.executeMatrixScopedRun(ctx, chatRunRequest{
		Prompt:      prompt,
		SessionID:   pulsecore.PulseSessionID("matrix:"+target, roomID),
		RoomID:      roomID,
		RouteTarget: target,
		ProjectID:   projectID,
	}, target)
	if err != nil {
		return "", err
	}
	log.Info().
		Str("room_id", roomID).
		Str("target", target).
		Int("matrix_messages", len(matrixMessages)).
		Int("result_len", len(result)).
		Msg("matrix_pulse_processed")
	return result, nil
}

func (a *app) executeMatrixScopedRun(ctx context.Context, req chatRunRequest, targetName string) (string, []sandbox.MatrixMessage, error) {
	request := chatRunRequest{
		Prompt:           req.Prompt,
		SessionID:        req.SessionID,
		EphemeralSession: req.EphemeralSession,
		ProjectID:        req.ProjectID,
		ObjectiveID:      req.ObjectiveID,
		RoomID:           req.RoomID,
		RouteTarget:      req.RouteTarget,
		SystemPrompt:     req.SystemPrompt,
		Image:            req.Image,
		ImageSize:        req.ImageSize,
	}
	if request.ProjectID == "" && request.RoomID != "" {
		projectID, err := a.ensureMatrixRoomProject(ctx, request.RoomID)
		if err != nil {
			return "", nil, fmt.Errorf("ensure matrix room project: %w", err)
		}
		request.ProjectID = projectID
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "/matrix-gateway", nil)
	if err != nil {
		return "", nil, err
	}
	httpReq, checkedOutWorkspace, statusCode, err := a.prepareChatRunRequest(httpReq, nil, request)
	if err != nil {
		return "", nil, fmt.Errorf("prepare matrix chat request (status=%d): %w", statusCode, err)
	}
	if request.RoomID != "" {
		gatewayOutbox := sandbox.NewMatrixOutbox(sandbox.WithMatrixDispatchHandler(func(runCtx context.Context, msg sandbox.MatrixMessage) error {
			if a.matrixGateway == nil {
				return nil
			}
			return a.matrixGateway.Send(runCtx, msg)
		}))
		httpReq = httpReq.WithContext(sandbox.WithMatrixOutbox(httpReq.Context(), gatewayOutbox))
	}

	owner := int64(systemUserID)
	target := chatDispatchTarget{}
	if targetName != "" {
		target.SpecialistName = targetName
	}
	descriptor, ok := a.describeChatTarget(target, request.SessionID, request.ProjectID, request.ObjectiveID, request.SystemPrompt, owner)
	if !ok {
		descriptor = a.promptOrchestratorDescriptor(httpReq.Context(), owner, request, checkedOutWorkspace)
	}
	if descriptor.RunContext == nil {
		descriptor.RunContext = llm.WithUserID(httpReq.Context(), owner)
	}

	build := descriptor.Build(httpReq.Context())
	if build.Err != nil {
		return "", nil, build.Err
	}

	history, summary, err := a.chatMemory.BuildContextForProvider(httpReq.Context(), nil, request.SessionID, build.Engine.LLM, build.Engine.Model, memory.SummaryPolicy{
		TargetContextWindowTokens:    build.Engine.ContextWindowTokens,
		PlainTextContextWindowTokens: a.cfg.Summary.PlainTextContextWindowTokens,
	})
	if err != nil {
		return "", nil, err
	}
	build.Engine.SkipInitialSummarization = summary != nil && summary.Triggered

	runCtx := descriptor.RunContext
	if request.ObjectiveID != "" {
		runCtx = sandbox.WithObjectiveID(runCtx, request.ObjectiveID)
	}
	jsonOpts := descriptor.JSON
	if jsonOpts.StoreModel == "" {
		jsonOpts.StoreModel = build.ModelLabel
	}
	runID := a.runs.create(request.Prompt).ID
	payload, err := a.executeInternalJSONChat(httpReq.Context(), runCtx, build.Engine, request, history, runID, nil, checkedOutWorkspace, jsonOpts)
	if err != nil {
		return "", nil, err
	}

	result, _ := payload["result"].(string)
	var matrixMessages []sandbox.MatrixMessage
	if messages, ok := payload["matrix_messages"].([]sandbox.MatrixMessage); ok {
		matrixMessages = messages
	}
	return result, matrixMessages, nil
}

func matrixSessionID(roomID, target string) string {
	namespaceSeed := "matrix:" + roomID + ":" + target
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(namespaceSeed)).String()
}
