package agentd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	agentpkg "manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/auth"
	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
	anthropicllm "manifold/internal/llm/anthropic"
	googlellm "manifold/internal/llm/google"
	openaillm "manifold/internal/llm/openai"
	persist "manifold/internal/persistence"
	"manifold/internal/specialists"
)

type visionClientSelection struct {
	Provider  string
	Model     string
	OpenAI    *openaillm.Client
	Anthropic *anthropicllm.Client
	Google    *googlellm.Client
}

func (v visionClientSelection) supportsCompaction() bool {
	return providerSupportsCompaction(v.provider())
}

func (v visionClientSelection) provider() llmpkg.Provider {
	switch {
	case v.OpenAI != nil:
		return v.OpenAI
	case v.Anthropic != nil:
		return v.Anthropic
	case v.Google != nil:
		return v.Google
	default:
		return nil
	}
}

type visionImageAttachment struct {
	mime string
	b64  string
}

type visionRequest struct {
	UserID        *int64
	Prompt        string
	SessionID     string
	Files         []*multipart.FileHeader
	Specialist    string
	Team          string
	Owner         int64
	Client        visionClientSelection
	History       []llmpkg.Message
	Images        []visionImageAttachment
	LLMMessages   []llmpkg.Message
	AcceptsStream bool
}

func (a *app) agentVisionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		visionReq, ok := a.prepareVisionRequest(w, r)
		if !ok {
			return
		}
		if a.writeMockVisionResponseIfNeeded(w, r, visionReq) {
			return
		}

		ctx, cancel, vDur := withMaybeTimeout(r.Context(), a.cfg.AgentRunTimeoutSeconds)
		defer cancel()
		if vDur > 0 {
			log.Debug().Dur("timeout", vDur).Str("endpoint", "/agent/vision").Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/vision").Msg("no timeout configured; running until completion")
		}

		vrun := a.runs.create("[vision] " + visionReq.Prompt)
		out, callErr := callVisionProvider(ctx, visionReq)
		if callErr != nil {
			log.Error().Err(callErr).Msg("vision chat error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			a.runs.updateStatus(vrun.ID, "failed", 0)
			return
		}

		a.writeVisionResponse(w, r, visionReq, out.Content, vrun.ID)
	}
}

func (a *app) prepareVisionRequest(w http.ResponseWriter, r *http.Request) (visionRequest, bool) {
	userID, ok := a.resolveVisionUser(w, r)
	if !ok {
		return visionRequest{}, false
	}
	prompt, sessionID, files, ok := a.parseVisionForm(w, r, userID)
	if !ok {
		return visionRequest{}, false
	}
	req := visionRequest{
		UserID:        userID,
		Prompt:        prompt,
		SessionID:     sessionID,
		Files:         files,
		Specialist:    strings.TrimSpace(r.URL.Query().Get("specialist")),
		Team:          visionTeamName(r),
		AcceptsStream: r.Header.Get("Accept") == "text/event-stream",
	}
	req.Owner = systemUserID
	if userID != nil {
		req.Owner = *userID
	}
	return a.prepareVisionClientData(w, r, req)
}

func (a *app) resolveVisionUser(w http.ResponseWriter, r *http.Request) (*int64, bool) {
	if !a.cfg.Auth.Enabled {
		return nil, true
	}
	u, ok := auth.CurrentUser(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
	if err != nil {
		log.Error().Err(err).Msg("resolve_chat_access")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, false
	}
	return id, true
}

func (a *app) parseVisionForm(w http.ResponseWriter, r *http.Request, userID *int64) (string, string, []*multipart.FileHeader, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return "", "", nil, false
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return "", "", nil, false
	}
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return "", "", nil, false
	}
	sessionID := normalizeClientChatSessionID(strings.TrimSpace(r.FormValue("session_id")))
	if !a.ensureVisionChatSession(w, r, userID, sessionID) {
		return "", "", nil, false
	}
	files := visionImageFiles(r.MultipartForm)
	if len(files) == 0 {
		http.Error(w, "no images provided", http.StatusBadRequest)
		return "", "", nil, false
	}
	return prompt, sessionID, files, true
}

func (a *app) ensureVisionChatSession(w http.ResponseWriter, r *http.Request, userID *int64, sessionID string) bool {
	if _, err := ensureChatSession(r.Context(), a.chatStore, userID, sessionID); err != nil {
		if errors.Is(err, persist.ErrForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return false
		}
		log.Error().Err(err).Str("session", sessionID).Msg("ensure_chat_session")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return false
	}
	return true
}

func visionImageFiles(form *multipart.Form) []*multipart.FileHeader {
	var files []*multipart.FileHeader
	if form == nil {
		return files
	}
	files = append(files, form.File["images"]...)
	files = append(files, form.File["image"]...)
	return files
}

func visionTeamName(r *http.Request) string {
	teamName := strings.TrimSpace(r.URL.Query().Get("team"))
	if teamName == "" {
		teamName = strings.TrimSpace(r.URL.Query().Get("group"))
	}
	return teamName
}

func (a *app) prepareVisionClientData(w http.ResponseWriter, r *http.Request, req visionRequest) (visionRequest, bool) {
	visionSel, statusCode, err := a.resolveVisionClientAndModel(r.Context(), req.Owner, req.Specialist, req.Team)
	if err != nil {
		http.Error(w, err.Error(), statusCode)
		return req, false
	}
	if strings.TrimSpace(visionSel.Model) == "" {
		visionSel.Model = strings.TrimSpace(a.cfg.LLMClient.OpenAI.Model)
	}
	req.Client = visionSel
	if a.isMockVisionRequest(req) {
		return req, true
	}
	history, ok := a.loadVisionHistory(w, r, req)
	if !ok {
		return req, false
	}
	images, ok := readVisionImages(w, req.Files)
	if !ok {
		return req, false
	}
	req.History = history
	req.Images = images
	req.LLMMessages = agentpkg.BuildInitialLLMMessages("", req.Prompt, history)
	return req, true
}

func (a *app) loadVisionHistory(w http.ResponseWriter, r *http.Request, req visionRequest) ([]llmpkg.Message, bool) {
	history, _, err := a.chatMemory.BuildContextForProvider(r.Context(), req.UserID, req.SessionID, req.Client.provider(), req.Client.Model, memory.SummaryPolicy{
		TargetContextWindowTokens:    a.chatSummaryContextSize(0, req.Client.Model),
		PlainTextContextWindowTokens: a.cfg.Summary.PlainTextContextWindowTokens,
	})
	if err != nil {
		if errors.Is(err, persist.ErrForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil, false
		}
		log.Error().Err(err).Str("session", req.SessionID).Msg("load_chat_history")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, false
	}
	return history, true
}

func readVisionImages(w http.ResponseWriter, files []*multipart.FileHeader) ([]visionImageAttachment, bool) {
	atts := make([]visionImageAttachment, 0, len(files))
	for _, fh := range files {
		att, ok := readVisionImage(w, fh)
		if !ok {
			return nil, false
		}
		atts = append(atts, att)
	}
	return atts, true
}

func readVisionImage(w http.ResponseWriter, fh *multipart.FileHeader) (visionImageAttachment, bool) {
	f, err := fh.Open()
	if err != nil {
		http.Error(w, "file open", http.StatusBadRequest)
		return visionImageAttachment{}, false
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		http.Error(w, "file read", http.StatusBadRequest)
		return visionImageAttachment{}, false
	}
	mt := http.DetectContentType(data)
	if mt != "image/png" && mt != "image/jpeg" && mt != "image/jpg" && mt != "image/webp" {
		http.Error(w, "unsupported image type", http.StatusBadRequest)
		return visionImageAttachment{}, false
	}
	if mt == "image/jpg" {
		mt = "image/jpeg"
	}
	return visionImageAttachment{mime: mt, b64: base64.StdEncoding.EncodeToString(data)}, true
}

func (a *app) isMockVisionRequest(req visionRequest) bool {
	return req.Specialist == "" && req.Team == "" && strings.EqualFold(req.Client.Provider, "openai") && a.cfg.LLMClient.OpenAI.APIKey == ""
}

func (a *app) writeMockVisionResponseIfNeeded(w http.ResponseWriter, r *http.Request, req visionRequest) bool {
	if !a.isMockVisionRequest(req) {
		return false
	}
	vrun := a.runs.create("[vision] " + req.Prompt)
	if req.AcceptsStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, _ := w.(http.Flusher)
		if b, err := json.Marshal("(dev) mock vision response: " + req.Prompt); err == nil {
			fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
		} else {
			fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock vision response")
		}
		fl.Flush()
		a.runs.updateStatus(vrun.ID, "completed", 0)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock vision response: " + req.Prompt})
	a.runs.updateStatus(vrun.ID, "completed", 0)
	return true
}

func callVisionProvider(ctx context.Context, req visionRequest) (llmpkg.Message, error) {
	switch {
	case req.Client.OpenAI != nil:
		return req.Client.OpenAI.ChatWithImageAttachments(ctx, req.LLMMessages, openAIVisionImages(req.Images), nil, req.Client.Model)
	case req.Client.Anthropic != nil:
		return req.Client.Anthropic.ChatWithImageAttachments(ctx, req.LLMMessages, anthropicVisionImages(req.Images), nil, req.Client.Model)
	case req.Client.Google != nil:
		return req.Client.Google.ChatWithImageAttachments(ctx, req.LLMMessages, googleVisionImages(req.Images), nil, req.Client.Model)
	default:
		return llmpkg.Message{}, errors.New("vision provider unavailable")
	}
}

func openAIVisionImages(atts []visionImageAttachment) []openaillm.ImageAttachment {
	images := make([]openaillm.ImageAttachment, 0, len(atts))
	for _, att := range atts {
		images = append(images, openaillm.ImageAttachment{MimeType: att.mime, Base64Data: att.b64})
	}
	return images
}

func anthropicVisionImages(atts []visionImageAttachment) []anthropicllm.ImageAttachment {
	images := make([]anthropicllm.ImageAttachment, 0, len(atts))
	for _, att := range atts {
		images = append(images, anthropicllm.ImageAttachment{MimeType: att.mime, Base64Data: att.b64})
	}
	return images
}

func googleVisionImages(atts []visionImageAttachment) []googlellm.ImageAttachment {
	images := make([]googlellm.ImageAttachment, 0, len(atts))
	for _, att := range atts {
		images = append(images, googlellm.ImageAttachment{MimeType: att.mime, Base64Data: att.b64})
	}
	return images
}

func (a *app) writeVisionResponse(w http.ResponseWriter, r *http.Request, req visionRequest, content string, runID string) {
	if req.AcceptsStream {
		if writeVisionStreamResponse(w, content) {
			a.runs.updateStatus(runID, "completed", 0)
			a.storeVisionTurn(r, req, content, "store_chat_turn_vision_stream")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": content})
	a.runs.updateStatus(runID, "completed", 0)
	a.storeVisionTurn(r, req, content, "store_chat_turn_vision")
}

func writeVisionStreamResponse(w http.ResponseWriter, content string) bool {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return false
	}
	payload := map[string]string{"type": "final", "data": content}
	b, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", b)
	fl.Flush()
	return true
}

func (a *app) storeVisionTurn(r *http.Request, req visionRequest, content, logMessage string) {
	if err := storeChatTurn(r.Context(), a.chatStore, chatTurnRecord{
		UserID:           req.UserID,
		SessionID:        req.SessionID,
		UserContent:      req.Prompt,
		AssistantContent: content,
		Model:            req.Client.Model,
	}); err != nil {
		log.Error().Err(err).Str("session", req.SessionID).Msg(logMessage)
	}
}

func (a *app) resolveVisionClientAndModel(ctx context.Context, owner int64, specialistName, teamName string) (visionClientSelection, int, error) {
	unsupportedErr := errors.New("vision requires an OpenAI-compatible, Anthropic, or Google provider")
	if teamName != "" {
		return a.resolveTeamVision(ctx, owner, teamName, unsupportedErr)
	}
	if specialistName != "" && !strings.EqualFold(specialistName, specialists.OrchestratorName) {
		return a.resolveSpecialistVision(ctx, owner, specialistName, unsupportedErr)
	}
	return a.resolveOrchestratorVision(ctx, owner, unsupportedErr)
}

func (a *app) resolveTeamVision(ctx context.Context, owner int64, teamName string, unsupportedErr error) (visionClientSelection, int, error) {
	if a.teamStore == nil {
		return visionClientSelection{}, http.StatusInternalServerError, errors.New("teams unavailable")
	}
	team, ok, err := a.teamStore.GetByName(ctx, owner, teamName)
	if err != nil {
		return visionClientSelection{}, http.StatusInternalServerError, errors.New("failed to load team")
	}
	if !ok {
		return visionClientSelection{}, http.StatusNotFound, errors.New("team not found")
	}
	orchestrator, statusCode, err := a.resolveTeamOrchestratorSpecialist(ctx, owner, team)
	if err != nil {
		return visionClientSelection{}, statusCode, err
	}
	return a.resolveSpecialistVision(ctx, owner, orchestrator.Name, unsupportedErr)
}

func (a *app) resolveSpecialistVision(ctx context.Context, owner int64, specialistName string, unsupportedErr error) (visionClientSelection, int, error) {
	reg, err := a.specialistsRegistryForUser(ctx, owner)
	if err != nil {
		return visionClientSelection{}, http.StatusInternalServerError, errors.New("specialist registry unavailable")
	}
	sp, ok := reg.Get(specialistName)
	if !ok || sp == nil {
		return visionClientSelection{}, http.StatusNotFound, errors.New("specialist not found")
	}
	switch client := sp.Provider().(type) {
	case *openaillm.Client:
		return visionClientSelection{Provider: "openai", Model: strings.TrimSpace(sp.Model), OpenAI: client}, 0, nil
	case *anthropicllm.Client:
		return visionClientSelection{Provider: "anthropic", Model: strings.TrimSpace(sp.Model), Anthropic: client}, 0, nil
	case *googlellm.Client:
		return visionClientSelection{Provider: "google", Model: strings.TrimSpace(sp.Model), Google: client}, 0, nil
	default:
		return visionClientSelection{}, http.StatusBadRequest, unsupportedErr
	}
}

func (a *app) resolveOrchestratorVision(ctx context.Context, owner int64, unsupportedErr error) (visionClientSelection, int, error) {
	llmCfg := a.cfg.LLMClient
	provider := strings.ToLower(strings.TrimSpace(llmCfg.Provider))
	if provider == "" {
		provider = "openai"
	}
	if a.specStore != nil {
		if orch, ok, _ := a.specStore.GetByName(ctx, owner, specialists.OrchestratorName); ok {
			var resolved string
			llmCfg, resolved = specialists.ApplyLLMClientOverride(llmCfg, orch)
			if strings.TrimSpace(resolved) != "" {
				provider = strings.ToLower(strings.TrimSpace(resolved))
			}
		}
	}
	return a.visionFromConfig(llmCfg, provider, "", "orchestrator not configured", unsupportedErr)
}

func (a *app) visionFromConfig(llmCfg config.LLMClientConfig, provider, modelOverride, googleConfigErr string, unsupportedErr error) (visionClientSelection, int, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(llmCfg.Provider))
	}
	switch config.ProviderBackend(provider) {
	case "anthropic":
		return visionClientSelection{Provider: "anthropic", Model: visionModel(modelOverride, llmCfg.Anthropic.Model), Anthropic: anthropicllm.New(llmCfg.Anthropic, a.httpClient)}, 0, nil
	case "google":
		client, err := googlellm.New(llmCfg.Google, a.httpClient)
		if err != nil {
			return visionClientSelection{}, http.StatusInternalServerError, errors.New(googleConfigErr)
		}
		return visionClientSelection{Provider: "google", Model: visionModel(modelOverride, llmCfg.Google.Model), Google: client}, 0, nil
	default:
		return visionClientSelection{Provider: "openai", Model: visionModel(modelOverride, llmCfg.OpenAI.Model), OpenAI: openaillm.New(llmCfg.OpenAI, a.httpClient)}, 0, nil
	}
}

func visionModel(override, fallback string) string {
	model := strings.TrimSpace(override)
	if model != "" {
		return model
	}
	return strings.TrimSpace(fallback)
}

func (a *app) audioServeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		filename := strings.TrimPrefix(r.URL.Path, "/audio/")
		if filename == "" {
			http.Error(w, "file not specified", http.StatusBadRequest)
			return
		}
		http.ServeFile(w, r, filename)
	}
}

func (a *app) sttHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, ok := a.sttUserID(w, r)
		if !ok {
			return
		}
		data, ok := readSTTAudio(w, r)
		if !ok {
			return
		}
		// In-process pure-Go engine: no proxy.
		if strings.EqualFold(strings.TrimSpace(a.cfg.STT.Engine), "moonshine") {
			a.sttMoonshineResponse(w, r, data)
			return
		}
		reqURL, model, apiKey := a.sttEndpoint(r, userID)
		log.Debug().Str("endpoint", reqURL).Str("model", model).Int64("user_id", userID).Msg("stt_request")
		body, contentType, ok := buildSTTRequestBody(w, data, model)
		if !ok {
			return
		}
		a.sendSTTRequest(w, r, reqURL, apiKey, body, contentType)
	}
}

func (a *app) sttUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if !a.cfg.Auth.Enabled {
		return 0, true
	}
	u, ok := auth.CurrentUser(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
	if err != nil {
		log.Error().Err(err).Msg("stt_resolve_chat_access")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return 0, false
	}
	if id == nil {
		return 0, true
	}
	return *id, true
}

func readSTTAudio(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return nil, false
	}
	file, _, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "missing audio", http.StatusBadRequest)
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return nil, false
	}
	return data, true
}

func (a *app) sttEndpoint(r *http.Request, userID int64) (string, string, string) {
	orch := a.orchestratorSpecialist(r.Context(), userID)
	model := strings.TrimSpace(a.cfg.STT.Model)
	if model == "" {
		model = "gpt-4o-mini-transcribe"
	}
	baseURL := strings.TrimSpace(a.cfg.STT.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(orch.BaseURL)
	}
	if baseURL == "" {
		baseURL = config.OpenAIAPIBaseURL
	}
	baseURL = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1"), "/")
	return baseURL + "/v1/audio/transcriptions", model, orch.APIKey
}

func buildSTTRequestBody(w http.ResponseWriter, data []byte, model string) (*bytes.Buffer, string, bool) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "prompt.wav")
	if err != nil {
		http.Error(w, "form error", http.StatusInternalServerError)
		return nil, "", false
	}
	if _, err := fw.Write(data); err != nil {
		http.Error(w, "form error", http.StatusInternalServerError)
		return nil, "", false
	}
	if err := mw.WriteField("model", model); err != nil {
		http.Error(w, "form error", http.StatusInternalServerError)
		return nil, "", false
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		http.Error(w, "form error", http.StatusInternalServerError)
		return nil, "", false
	}
	if err := mw.Close(); err != nil {
		http.Error(w, "form error", http.StatusInternalServerError)
		return nil, "", false
	}
	return &buf, mw.FormDataContentType(), true
}

func (a *app) sendSTTRequest(w http.ResponseWriter, r *http.Request, reqURL, apiKey string, body *bytes.Buffer, contentType string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		http.Error(w, "request error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Warn().Err(err).Str("endpoint", reqURL).Dur("elapsed", time.Since(started)).Msg("stt_request_failed")
		http.Error(w, "stt request failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if !writeSTTErrorIfNeeded(w, resp, reqURL, started) {
		return
	}
	writeSTTResponse(w, resp, reqURL, started)
}

func writeSTTErrorIfNeeded(w http.ResponseWriter, resp *http.Response, reqURL string, started time.Time) bool {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	log.Warn().Int("status", resp.StatusCode).Str("body", strings.TrimSpace(string(b))).Str("endpoint", reqURL).Dur("elapsed", time.Since(started)).Msg("stt_request_error")
	http.Error(w, strings.TrimSpace(string(b)), resp.StatusCode)
	return false
}

func writeSTTResponse(w http.ResponseWriter, resp *http.Response, reqURL string, started time.Time) {
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Warn().Err(err).Str("endpoint", reqURL).Dur("elapsed", time.Since(started)).Msg("stt_response_decode_failed")
		http.Error(w, "invalid stt response", http.StatusBadGateway)
		return
	}
	log.Debug().Str("endpoint", reqURL).Int("text_len", len(out.Text)).Dur("elapsed", time.Since(started)).Msg("stt_response")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"text": strings.TrimSpace(out.Text)})
}
