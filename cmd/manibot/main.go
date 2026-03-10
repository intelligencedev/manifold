package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/matrix-org/gomatrix"
	"github.com/yuin/goldmark"

	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	pulsecore "manifold/internal/pulse"
)

type config struct {
	MatrixHomeserverURL string
	MatrixBotUserID     string
	MatrixAccessToken   string
	BotPrefix           string
	ProcessBacklog      bool

	ManifoldBaseURL           string
	ManifoldPromptPath        string
	ManifoldProjectID         string
	ManifoldSpecialist        string
	ManifoldSystemPrompt      string
	ManifoldSessionPrefix     string
	ManifoldSessionCookie     string
	ManifoldSessionCookieName string
	ManifoldAuthBearerToken   string

	SyncTimeoutSeconds       int
	SyncRetryDelaySeconds    int
	RequestTimeoutSeconds    int
	PulseEnabled             bool
	PulsePollIntervalSeconds int
	PulseLeaseSeconds        int
	ReactiveLeaseEnabled     bool
	ReactiveLeaseSeconds     int
	ReactiveWaitSeconds      int
	ReactiveContextMessages  int
	PulseDatabaseDSN         string
}

type manifoldPromptRequest struct {
	Prompt       string `json:"prompt"`
	RoomID       string `json:"room_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

type manifoldPromptResponse struct {
	Result         string                  `json:"result"`
	Error          string                  `json:"error,omitempty"`
	MatrixMessages []manifoldMatrixMessage `json:"matrix_messages,omitempty"`
}

type manifoldMatrixMessage struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

type matrixMessageSender interface {
	SendText(roomID, text string) (*gomatrix.RespSendEvent, error)
	SendFormattedText(roomID, text, formattedText string) (*gomatrix.RespSendEvent, error)
}

func renderMatrixHTML(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(text), &buf); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func sendMatrixMessage(client matrixMessageSender, roomID, text string) error {
	formatted, err := renderMatrixHTML(text)
	if err == nil && formatted != "" {
		_, err = client.SendFormattedText(roomID, text, formatted)
		return err
	}
	_, err = client.SendText(roomID, text)
	return err
}

func main() {
	_ = godotenv.Load()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	matrixClient, err := gomatrix.NewClient(cfg.MatrixHomeserverURL, cfg.MatrixBotUserID, cfg.MatrixAccessToken)
	if err != nil {
		log.Fatalf("failed to create matrix client: %v", err)
	}

	httpClient := &http.Client{Timeout: time.Duration(cfg.RequestTimeoutSeconds) * time.Second}

	var pulseStore persist.PulseStore
	var reactiveClaimStore persist.ReactiveClaimStore
	roomHistory := newRoomEventHistory(32)
	if cfg.PulseEnabled || cfg.ReactiveLeaseEnabled {
		pool, err := databases.OpenPool(context.Background(), cfg.PulseDatabaseDSN)
		if err != nil {
			log.Fatalf("failed to connect pulse store: %v", err)
		}
		defer pool.Close()
		if cfg.PulseEnabled {
			pulseStore = databases.NewPulseStore(pool)
			if err := pulseStore.Init(context.Background()); err != nil {
				log.Fatalf("failed to init pulse store: %v", err)
			}
			go runPulseLoop(matrixClient, httpClient, cfg, pulseStore, pulsecore.NewService())
		}
		if cfg.ReactiveLeaseEnabled {
			reactiveClaimStore = databases.NewReactiveClaimStore(pool)
			if err := reactiveClaimStore.Init(context.Background()); err != nil {
				log.Fatalf("failed to init reactive claim store: %v", err)
			}
		}
	}

	log.Printf("manibot started as %s, homeserver=%s, manifold=%s%s", cfg.MatrixBotUserID, cfg.MatrixHomeserverURL, cfg.ManifoldBaseURL, cfg.ManifoldPromptPath)

	seen := map[string]struct{}{}
	var since string
	initialized := false

	for {
		resp, err := matrixClient.SyncRequest(cfg.SyncTimeoutSeconds*1000, since, "", false, "online")
		if err != nil {
			log.Printf("sync error: %v", err)
			time.Sleep(time.Duration(cfg.SyncRetryDelaySeconds) * time.Second)
			continue
		}

		since = resp.NextBatch

		for roomID := range resp.Rooms.Invite {
			if _, err := matrixClient.JoinRoom(roomID, "", nil); err != nil {
				log.Printf("failed to join room %s: %v", roomID, err)
			} else {
				log.Printf("joined room %s", roomID)
			}
		}

		if !initialized {
			initialized = true
			if !cfg.ProcessBacklog {
				log.Printf("startup sync complete; ignoring backlog and waiting for new events")
				continue
			}
			log.Printf("startup sync complete; processing backlog is enabled")
		}

		for roomID, joined := range resp.Rooms.Join {
			for _, ev := range joined.Timeline.Events {
				if ev.Type != "m.room.message" {
					continue
				}
				if ev.ID != "" {
					if _, ok := seen[ev.ID]; ok {
						continue
					}
					seen[ev.ID] = struct{}{}
					if len(seen) > 10000 {
						seen = map[string]struct{}{}
					}
				}
				roomHistory.Add(roomID, ev)
				if ev.Sender == cfg.MatrixBotUserID {
					continue
				}

				body, ok := ev.Content["body"].(string)
				if !ok || strings.TrimSpace(body) == "" {
					continue
				}

				prompt, matched := promptFromMessage(body, cfg.BotPrefix)
				if !matched {
					continue
				}
				if strings.TrimSpace(cfg.BotPrefix) != "" && prompt == "" {
					_ = sendMatrixMessage(matrixClient, roomID, fmt.Sprintf("Usage: %s <your question>", cfg.BotPrefix))
					continue
				}
				if cfg.ReactiveLeaseEnabled && reactiveClaimStore != nil {
					go handleReactiveMessage(matrixClient, httpClient, cfg, pulseStore, reactiveClaimStore, roomHistory, roomID, ev, body, prompt)
					continue
				}

				handleDirectMessage(matrixClient, httpClient, cfg, pulseStore, roomID, prompt)
			}
		}
	}
}

func loadConfig() (config, error) {
	systemPrompt, err := loadMatrixSystemPrompt()
	if err != nil {
		return config{}, err
	}

	c := config{
		MatrixHomeserverURL: strings.TrimSpace(os.Getenv("MATRIX_HOMESERVER_URL")),
		MatrixBotUserID:     strings.TrimSpace(os.Getenv("MATRIX_BOT_USER_ID")),
		MatrixAccessToken:   strings.TrimSpace(os.Getenv("MATRIX_ACCESS_TOKEN")),
		BotPrefix:           strings.TrimSpace(os.Getenv("BOT_PREFIX")),
		ProcessBacklog:      boolEnv("MATRIX_PROCESS_BACKLOG", false),

		ManifoldBaseURL:           strings.TrimSpace(os.Getenv("MANIFOLD_BASE_URL")),
		ManifoldPromptPath:        strings.TrimSpace(os.Getenv("MANIFOLD_PROMPT_PATH")),
		ManifoldProjectID:         strings.TrimSpace(os.Getenv("MANIFOLD_PROJECT_ID")),
		ManifoldSpecialist:        strings.TrimSpace(os.Getenv("MANIFOLD_SPECIALIST")),
		ManifoldSystemPrompt:      systemPrompt,
		ManifoldSessionPrefix:     strings.TrimSpace(os.Getenv("MANIFOLD_SESSION_PREFIX")),
		ManifoldSessionCookie:     strings.TrimSpace(os.Getenv("MANIFOLD_SESSION_COOKIE")),
		ManifoldSessionCookieName: strings.TrimSpace(os.Getenv("MANIFOLD_SESSION_COOKIE_NAME")),
		ManifoldAuthBearerToken:   strings.TrimSpace(os.Getenv("MANIFOLD_AUTH_BEARER_TOKEN")),

		SyncTimeoutSeconds:       intEnv("MATRIX_SYNC_TIMEOUT_SECONDS", 30),
		SyncRetryDelaySeconds:    intEnv("MATRIX_SYNC_RETRY_DELAY_SECONDS", 3),
		RequestTimeoutSeconds:    intEnv("MANIFOLD_REQUEST_TIMEOUT_SECONDS", 180),
		PulseEnabled:             boolEnv("MATRIX_PULSE_ENABLED", false),
		PulsePollIntervalSeconds: intEnv("MATRIX_PULSE_POLL_INTERVAL_SECONDS", 300),
		PulseLeaseSeconds:        intEnv("MATRIX_PULSE_LEASE_SECONDS", 240),
		ReactiveLeaseEnabled:     boolEnv("MATRIX_REACTIVE_LEASE_ENABLED", false),
		ReactiveLeaseSeconds:     intEnv("MATRIX_REACTIVE_LEASE_SECONDS", 90),
		ReactiveWaitSeconds:      intEnv("MATRIX_REACTIVE_WAIT_TIMEOUT_SECONDS", 180),
		ReactiveContextMessages:  intEnv("MATRIX_REACTIVE_CONTEXT_MESSAGES", 12),
		PulseDatabaseDSN: strings.TrimSpace(firstNonEmpty(
			os.Getenv("PULSE_DATABASE_DSN"),
			os.Getenv("DATABASE_URL"),
			os.Getenv("DB_URL"),
			os.Getenv("POSTGRES_DSN"),
		)),
	}

	if c.ManifoldBaseURL == "" {
		c.ManifoldBaseURL = "http://localhost:32180"
	}
	if c.ManifoldPromptPath == "" {
		c.ManifoldPromptPath = "/api/prompt"
	}
	if c.ManifoldSessionPrefix == "" {
		c.ManifoldSessionPrefix = "matrix"
	}
	if c.ManifoldSessionCookieName == "" {
		c.ManifoldSessionCookieName = "sio_session"
	}
	if c.PulseLeaseSeconds <= 0 {
		c.PulseLeaseSeconds = c.RequestTimeoutSeconds + 60
	}
	if c.ReactiveLeaseSeconds <= 0 {
		c.ReactiveLeaseSeconds = 90
	}
	if c.ReactiveWaitSeconds <= 0 {
		c.ReactiveWaitSeconds = c.ReactiveLeaseSeconds * 2
	}
	if c.ReactiveContextMessages <= 0 {
		c.ReactiveContextMessages = 12
	}

	if c.MatrixHomeserverURL == "" || c.MatrixBotUserID == "" || c.MatrixAccessToken == "" {
		return c, errors.New("missing required env vars: MATRIX_HOMESERVER_URL, MATRIX_BOT_USER_ID, MATRIX_ACCESS_TOKEN")
	}
	if (c.PulseEnabled || c.ReactiveLeaseEnabled) && strings.TrimSpace(c.PulseDatabaseDSN) == "" {
		return c, errors.New("MATRIX_PULSE_ENABLED or MATRIX_REACTIVE_LEASE_ENABLED requires PULSE_DATABASE_DSN or DATABASE_URL/DB_URL/POSTGRES_DSN")
	}

	return c, nil
}

func callManifold(httpClient *http.Client, cfg config, roomID, sessionID, projectID, prompt string) (manifoldPromptResponse, error) {
	reqBody := manifoldPromptRequest{
		Prompt:       prompt,
		RoomID:       roomID,
		SessionID:    sessionID,
		ProjectID:    firstNonEmpty(strings.TrimSpace(projectID), cfg.ManifoldProjectID),
		SystemPrompt: strings.TrimSpace(cfg.ManifoldSystemPrompt),
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return manifoldPromptResponse{}, err
	}

	endpoint, err := url.Parse(strings.TrimRight(cfg.ManifoldBaseURL, "/") + "/" + strings.TrimLeft(cfg.ManifoldPromptPath, "/"))
	if err != nil {
		return manifoldPromptResponse{}, err
	}
	if specialist := strings.TrimSpace(cfg.ManifoldSpecialist); specialist != "" {
		query := endpoint.Query()
		query.Set("specialist", specialist)
		endpoint.RawQuery = query.Encode()
	}
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(b))
	if err != nil {
		return manifoldPromptResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	if cfg.ManifoldSessionCookie != "" {
		req.Header.Set("Cookie", cfg.ManifoldSessionCookieName+"="+cfg.ManifoldSessionCookie)
	}
	if cfg.ManifoldAuthBearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.ManifoldAuthBearerToken)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return manifoldPromptResponse{}, err
	}
	defer resp.Body.Close()

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return decodeSSEPromptResponse(resp)
	}

	return decodeJSONPromptResponse(resp)
}

func decodeSSEPromptResponse(resp *http.Response) (manifoldPromptResponse, error) {
	reader := bufio.NewReader(resp.Body)
	out := manifoldPromptResponse{}
	var lastError string

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return manifoldPromptResponse{}, err
		}

		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "data:"))
			if payload != "" {
				var event map[string]any
				if json.Unmarshal([]byte(payload), &event) == nil {
					eventType, _ := event["type"].(string)
					switch eventType {
					case "final":
						if data, ok := event["data"].(string); ok {
							out.Result = strings.TrimSpace(data)
						}
						if rawMessages, ok := event["matrix_messages"].([]any); ok {
							out.MatrixMessages = decodeMatrixMessages(rawMessages)
						}
					case "error":
						if data, ok := event["data"].(string); ok {
							lastError = strings.TrimSpace(data)
						}
					}
				} else {
					var plain string
					if json.Unmarshal([]byte(payload), &plain) == nil && strings.TrimSpace(plain) != "" {
						lastError = strings.TrimSpace(plain)
					}
				}
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}
	}

	if resp.StatusCode >= 300 {
		if lastError != "" {
			return manifoldPromptResponse{}, fmt.Errorf("manifold status %d: %s", resp.StatusCode, lastError)
		}
		return manifoldPromptResponse{}, fmt.Errorf("manifold returned status %d", resp.StatusCode)
	}

	if out.Result != "" || len(out.MatrixMessages) > 0 {
		return out, nil
	}

	if lastError != "" {
		return manifoldPromptResponse{}, fmt.Errorf("manifold stream error: %s", lastError)
	}

	return manifoldPromptResponse{}, errors.New("empty response from manifold stream")
}

func decodeJSONPromptResponse(resp *http.Response) (manifoldPromptResponse, error) {

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return manifoldPromptResponse{}, err
	}

	var decoded manifoldPromptResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		if resp.StatusCode >= 300 {
			trimmed := strings.TrimSpace(string(body))
			if trimmed == "" {
				trimmed = http.StatusText(resp.StatusCode)
			}
			return manifoldPromptResponse{}, fmt.Errorf("manifold status %d: %s", resp.StatusCode, trimmed)
		}
		return manifoldPromptResponse{}, fmt.Errorf("failed to decode manifold response (status=%d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode >= 300 {
		if strings.TrimSpace(decoded.Error) != "" {
			return manifoldPromptResponse{}, fmt.Errorf("manifold status %d: %s", resp.StatusCode, decoded.Error)
		}
		return manifoldPromptResponse{}, fmt.Errorf("manifold returned status %d", resp.StatusCode)
	}

	decoded.Result = strings.TrimSpace(decoded.Result)
	decoded.MatrixMessages = pulseMessages(decoded.MatrixMessages, "")
	if decoded.Result == "" && len(decoded.MatrixMessages) == 0 {
		return manifoldPromptResponse{}, errors.New("empty response from manifold")
	}

	return decoded, nil
}

func decodeMatrixMessages(raw []any) []manifoldMatrixMessage {
	out := make([]manifoldMatrixMessage, 0, len(raw))
	for _, item := range raw {
		decoded, ok := item.(map[string]any)
		if !ok {
			continue
		}
		roomID, _ := decoded["room_id"].(string)
		text, _ := decoded["text"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, manifoldMatrixMessage{RoomID: strings.TrimSpace(roomID), Text: strings.TrimSpace(text)})
	}
	return out
}

func pulseMessages(messages []manifoldMatrixMessage, fallbackRoomID string) []manifoldMatrixMessage {
	out := make([]manifoldMatrixMessage, 0, len(messages))
	for _, message := range messages {
		roomID := strings.TrimSpace(message.RoomID)
		text := strings.TrimSpace(message.Text)
		if roomID == "" {
			roomID = strings.TrimSpace(fallbackRoomID)
		}
		if roomID == "" || text == "" {
			continue
		}
		out = append(out, manifoldMatrixMessage{RoomID: roomID, Text: text})
	}
	return out
}

func sessionIDForRoom(prefix, roomID string) string {
	cleaned := strings.TrimSpace(roomID)
	if cleaned == "" {
		cleaned = "default"
	}
	namespaceSeed := strings.TrimSpace(prefix) + ":" + cleaned
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(namespaceSeed)).String()
}

func runPulseLoop(matrixClient *gomatrix.Client, httpClient *http.Client, cfg config, store persist.PulseStore, service *pulsecore.Service) {
	if store == nil || service == nil {
		return
	}
	interval := time.Duration(cfg.PulsePollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runPulseIteration(matrixClient, httpClient, cfg, store, service)
	for range ticker.C {
		runPulseIteration(matrixClient, httpClient, cfg, store, service)
	}
}

func runPulseIteration(matrixClient *gomatrix.Client, httpClient *http.Client, cfg config, store persist.PulseStore, service *pulsecore.Service) {
	ctx := context.Background()
	rooms, err := store.ListRooms(ctx)
	if err != nil {
		log.Printf("pulse list rooms error: %v", err)
		return
	}
	now := time.Now().UTC()
	for _, room := range rooms {
		if !room.Enabled {
			continue
		}
		tasks, err := store.ListTasks(ctx, room.RoomID)
		if err != nil {
			log.Printf("pulse list tasks error (room=%s): %v", room.RoomID, err)
			continue
		}
		plan := service.EvaluateRoom(now, room, tasks)
		if len(plan.DueTasks) == 0 {
			continue
		}
		claimToken := uuid.NewString()
		claimed, err := store.ClaimRoom(ctx, room.RoomID, claimToken, now.Add(time.Duration(cfg.PulseLeaseSeconds)*time.Second))
		if err != nil {
			log.Printf("pulse claim error (room=%s): %v", room.RoomID, err)
			continue
		}
		if !claimed {
			continue
		}

		prompt := service.BuildPrompt(now, plan, time.Duration(cfg.PulsePollIntervalSeconds)*time.Second)
		sessionID := pulsecore.PulseSessionID(cfg.ManifoldSessionPrefix, room.RoomID)
		projectID := firstNonEmpty(strings.TrimSpace(room.ProjectID), cfg.ManifoldProjectID)
		response, runErr := callManifold(httpClient, cfg, room.RoomID, sessionID, projectID, prompt)
		pulseErr := ""
		dueTaskIDs := []string{}
		if runErr != nil {
			pulseErr = runErr.Error()
			log.Printf("pulse manifold error (room=%s session=%s): %v", room.RoomID, sessionID, runErr)
		} else {
			for _, task := range plan.DueTasks {
				dueTaskIDs = append(dueTaskIDs, task.ID)
			}
			for _, message := range pulseMessages(response.MatrixMessages, room.RoomID) {
				if err := sendMatrixMessage(matrixClient, message.RoomID, message.Text); err != nil {
					pulseErr = err.Error()
					log.Printf("pulse send message error (room=%s): %v", message.RoomID, err)
					break
				}
			}
		}
		if err := store.CompleteRoomPulse(ctx, room.RoomID, claimToken, time.Now().UTC(), response.Result, pulseErr, dueTaskIDs); err != nil {
			log.Printf("pulse completion error (room=%s): %v", room.RoomID, err)
		}
	}
}

func resolveRoomProjectID(ctx context.Context, store persist.PulseStore, roomID, fallback string) string {
	if store == nil {
		return fallback
	}
	room, err := store.GetRoom(ctx, roomID)
	if err != nil {
		return fallback
	}
	return firstNonEmpty(strings.TrimSpace(room.ProjectID), fallback)
}

type roomEventHistory struct {
	mu    sync.RWMutex
	limit int
	rooms map[string][]gomatrix.Event
}

func newRoomEventHistory(limit int) *roomEventHistory {
	if limit <= 0 {
		limit = 32
	}
	return &roomEventHistory{
		limit: limit,
		rooms: map[string][]gomatrix.Event{},
	}
}

func (h *roomEventHistory) Add(roomID string, event gomatrix.Event) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	events := append(h.rooms[roomID], event)
	if len(events) > h.limit {
		events = append([]gomatrix.Event(nil), events[len(events)-h.limit:]...)
	} else {
		events = append([]gomatrix.Event(nil), events...)
	}
	h.rooms[roomID] = events
}

func (h *roomEventHistory) Snapshot(roomID string, limit int) []gomatrix.Event {
	h.mu.RLock()
	defer h.mu.RUnlock()

	events := h.rooms[strings.TrimSpace(roomID)]
	if len(events) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(events) {
		limit = len(events)
	}
	return append([]gomatrix.Event(nil), events[len(events)-limit:]...)
}

func intEnv(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(v, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func boolEnv(name string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
