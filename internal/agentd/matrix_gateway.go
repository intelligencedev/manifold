package agentd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/matrixgw"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

const defaultMatrixMessageRetention = 100

func (a *app) handleMatrixMessage(ctx context.Context, message matrixgw.InboundMessage) error {
	if a.matrixMessageStore != nil {
		if _, err := a.matrixMessageStore.Append(ctx, persist.MatrixMessage{
			RoomID:    message.RoomID,
			EventID:   message.EventID,
			Direction: "inbound",
			Sender:    message.Sender,
			Target:    message.Target,
			Body:      message.Body,
			MsgType:   "m.text",
			CreatedAt: time.Now().UTC(),
		}, matrixMessageRetention(a.cfg.Matrix, message.RoomID)); err != nil {
			return fmt.Errorf("persist matrix inbound message: %w", err)
		}
	}
	result, images, matrixMessages, err := a.executeMatrixScopedRun(ctx, chatRunRequest{
		Prompt:      message.Prompt,
		SessionID:   matrixSessionID(message.RoomID),
		RoomID:      message.RoomID,
		RouteTarget: message.Target,
	}, message.Target, true)
	if err != nil {
		return err
	}
	if result != "" && a.matrixGateway != nil {
		if err := a.matrixGateway.SendAttributed(ctx, message.RoomID, message.Target, result); err != nil {
			return err
		}
	}
	if len(images) > 0 && a.matrixGateway != nil {
		if err := a.sendMatrixGeneratedImages(ctx, message.RoomID, images); err != nil {
			log.Warn().Err(err).Str("room_id", message.RoomID).Int("image_count", len(images)).Msg("matrix_gateway_image_delivery_failed")
		}
	}

	log.Info().
		Str("room_id", message.RoomID).
		Str("event_id", message.EventID).
		Str("target", message.Target).
		Str("sender", message.Sender).
		Int("image_count", len(images)).
		Int("matrix_messages", len(matrixMessages)).
		Int("result_len", len(result)).
		Msg("matrix_gateway_message_processed")
	return nil
}

func (a *app) handlePulseRoom(ctx context.Context, roomID, target, projectID, prompt string) (string, error) {
	result, _, matrixMessages, err := a.executeMatrixScopedRun(ctx, chatRunRequest{
		Prompt:      prompt,
		SessionID:   matrixSessionID(roomID),
		RoomID:      roomID,
		RouteTarget: target,
		ProjectID:   projectID,
	}, target, false)
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

func (a *app) executeMatrixScopedRun(ctx context.Context, req chatRunRequest, targetName string, includeHistory bool) (string, []savedImage, []sandbox.MatrixMessage, error) {
	request, err := a.prepareMatrixScopedRequest(ctx, req)
	if err != nil {
		return "", nil, nil, err
	}
	httpReq, checkedOutWorkspace, err := a.prepareMatrixHTTPRun(ctx, request)
	if err != nil {
		return "", nil, nil, err
	}

	owner := int64(systemUserID)
	descriptor := a.describeMatrixChatTarget(httpReq.Context(), request, checkedOutWorkspace, targetName, owner)
	build, history, err := a.buildMatrixChatEngine(httpReq.Context(), descriptor, request, includeHistory)
	if err != nil {
		return "", nil, nil, err
	}

	runID := a.runs.create(request.Prompt).ID
	payload, err := a.executeInternalJSONChat(httpReq.Context(), chatExecutionRequest{
		RunContext:          matrixRunContext(descriptor.RunContext, request, build),
		Engine:              build.Engine,
		RunRequest:          request,
		History:             history,
		RunID:               runID,
		CheckedOutWorkspace: checkedOutWorkspace,
	}, matrixJSONOptions(descriptor.JSON, build))
	if err != nil {
		return "", nil, nil, err
	}
	return matrixPayloadResult(payload)
}

func (a *app) prepareMatrixScopedRequest(ctx context.Context, req chatRunRequest) (chatRunRequest, error) {
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
			return request, fmt.Errorf("ensure matrix room project: %w", err)
		}
		request.ProjectID = projectID
	}
	return request, nil
}

func (a *app) prepareMatrixHTTPRun(ctx context.Context, request chatRunRequest) (*http.Request, *workspaces.Workspace, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "/matrix-gateway", nil)
	if err != nil {
		return nil, nil, err
	}
	httpReq, checkedOutWorkspace, statusCode, err := a.prepareChatRunRequest(httpReq, nil, request)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare matrix chat request (status=%d): %w", statusCode, err)
	}
	if _, err := ensureMatrixChatSession(httpReq.Context(), a.chatStore, request.RoomID, request.SessionID); err != nil {
		return nil, nil, fmt.Errorf("ensure matrix chat session: %w", err)
	}
	return a.withMatrixGatewayOutbox(httpReq, request.RoomID), checkedOutWorkspace, nil
}

func (a *app) withMatrixGatewayOutbox(r *http.Request, roomID string) *http.Request {
	if roomID == "" {
		return r
	}
	gatewayOutbox := sandbox.NewMatrixOutbox(sandbox.WithMatrixDispatchHandler(func(runCtx context.Context, msg sandbox.MatrixMessage) error {
		if a.matrixGateway == nil {
			return nil
		}
		return a.matrixGateway.Send(runCtx, msg)
	}))
	return r.WithContext(sandbox.WithMatrixOutbox(r.Context(), gatewayOutbox))
}

func (a *app) describeMatrixChatTarget(ctx context.Context, request chatRunRequest, workspace *workspaces.Workspace, targetName string, owner int64) chatTargetDescriptor {
	target := chatDispatchTarget{}
	if targetName != "" {
		target.SpecialistName = targetName
	}
	descriptor, ok := a.describeChatTarget(chatTargetDescribeRequest{
		Target:               target,
		SessionID:            request.SessionID,
		ProjectID:            request.ProjectID,
		ObjectiveID:          request.ObjectiveID,
		SystemPromptOverride: request.SystemPrompt,
		Owner:                owner,
		MemorySettings:       chatMemorySettingsFromRunRequest(request),
	})
	if !ok {
		descriptor = a.promptOrchestratorDescriptor(ctx, owner, request, workspace)
	}
	if descriptor.RunContext == nil {
		descriptor.RunContext = llm.WithUserID(ctx, owner)
	}
	return descriptor
}

func (a *app) buildMatrixChatEngine(ctx context.Context, descriptor chatTargetDescriptor, request chatRunRequest, includeHistory bool) (chatEngineBuildResult, []llm.Message, error) {
	build := descriptor.Build(ctx)
	if build.Err != nil {
		return build, nil, build.Err
	}
	build = sanitizeImageGenerationBuild(build)
	history, err := a.matrixChatHistory(ctx, request, build, includeHistory)
	if err != nil {
		return build, nil, err
	}
	return build, history, nil
}

func (a *app) matrixChatHistory(ctx context.Context, request chatRunRequest, build chatEngineBuildResult, includeHistory bool) ([]llm.Message, error) {
	if !includeHistory || build.ImageGeneration || build.VideoGeneration {
		return nil, nil
	}
	history, summary, err := a.chatMemory.BuildContextForProvider(ctx, nil, request.SessionID, build.Engine.LLM, build.Engine.Model, memory.SummaryPolicy{
		TargetContextWindowTokens:    build.Engine.ContextWindowTokens,
		PlainTextContextWindowTokens: a.cfg.Summary.PlainTextContextWindowTokens,
	})
	if err != nil {
		return nil, err
	}
	build.Engine.SkipInitialSummarization = summary != nil && summary.Triggered
	return history, nil
}

func matrixRunContext(runCtx context.Context, request chatRunRequest, build chatEngineBuildResult) context.Context {
	if request.ObjectiveID != "" {
		runCtx = sandbox.WithObjectiveID(runCtx, request.ObjectiveID)
	}
	return applyBuildImagePrompt(runCtx, build)
}

func matrixJSONOptions(jsonOpts chatJSONOptions, build chatEngineBuildResult) chatJSONOptions {
	if jsonOpts.StoreModel == "" {
		jsonOpts.StoreModel = build.ModelLabel
	}
	return jsonOpts
}

func matrixPayloadResult(payload map[string]any) (string, []savedImage, []sandbox.MatrixMessage, error) {
	result, _ := payload["result"].(string)
	var images []savedImage
	if payloadImages, ok := payload["images"].([]savedImage); ok {
		images = payloadImages
	}
	var matrixMessages []sandbox.MatrixMessage
	if messages, ok := payload["matrix_messages"].([]sandbox.MatrixMessage); ok {
		matrixMessages = messages
	}
	return result, images, matrixMessages, nil
}

func (a *app) sendMatrixGeneratedImages(ctx context.Context, roomID string, images []savedImage) error {
	var firstErr error
	for _, img := range images {
		upload, err := matrixUploadImage(img)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			log.Warn().Err(err).Str("name", img.Name).Msg("matrix_gateway_prepare_image_failed")
			continue
		}
		if err := a.matrixGateway.SendImage(ctx, roomID, upload); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			log.Warn().Err(err).Str("name", upload.Body).Msg("matrix_gateway_send_image_failed")
		}
	}
	return firstErr
}

func matrixUploadImage(img savedImage) (matrixgw.UploadImage, error) {
	body := strings.TrimSpace(img.Name)
	if body == "" && strings.TrimSpace(img.RelPath) != "" {
		body = filepath.Base(img.RelPath)
	}
	if body == "" {
		body = "image"
	}
	mimeType := strings.TrimSpace(img.MIME)
	if mimeType == "" {
		mimeType = "image/png"
	}
	content, err := savedImageContent(img)
	if err != nil {
		return matrixgw.UploadImage{}, err
	}
	return matrixgw.UploadImage{Body: body, Content: content, MIMEType: mimeType}, nil
}

func savedImageContent(img savedImage) ([]byte, error) {
	if fullPath := strings.TrimSpace(img.FullPath); fullPath != "" {
		content, err := os.ReadFile(fullPath)
		if err == nil {
			return content, nil
		}
	}
	dataURL := strings.TrimSpace(img.DataURL)
	if dataURL == "" {
		return nil, fmt.Errorf("saved image has no readable content")
	}
	comma := strings.IndexByte(dataURL, ',')
	if comma <= 0 || comma == len(dataURL)-1 {
		return nil, fmt.Errorf("invalid image data url")
	}
	encoded := dataURL[comma+1:]
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode image data url: %w", err)
	}
	return content, nil
}

func ensureMatrixChatSession(ctx context.Context, store persist.ChatStore, roomID, sessionID string) (persist.ChatSession, error) {
	name := "Matrix Conversation"
	if strings.TrimSpace(roomID) != "" {
		name = matrixRoomProjectName(roomID)
	}
	return store.EnsureSessionKind(ctx, nil, sessionID, name, persist.ChatSessionKindMatrix)
}

func matrixSessionID(roomID string) string {
	namespaceSeed := "matrix:" + roomID
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(namespaceSeed)).String()
}

func matrixMessageRetention(cfg config.MatrixConfig, roomID string) int {
	for _, room := range cfg.Rooms {
		if strings.TrimSpace(room.RoomID) != strings.TrimSpace(roomID) {
			continue
		}
		if room.MessageRetention > 0 {
			return room.MessageRetention
		}
		break
	}
	if cfg.MessageRetention > 0 {
		return cfg.MessageRetention
	}
	return defaultMatrixMessageRetention
}
