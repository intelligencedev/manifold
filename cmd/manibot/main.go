package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/matrix-org/gomatrix"
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
	ManifoldSessionPrefix     string
	ManifoldSessionCookie     string
	ManifoldSessionCookieName string
	ManifoldAuthBearerToken   string

	SyncTimeoutSeconds    int
	SyncRetryDelaySeconds int
	RequestTimeoutSeconds int
}

type manifoldPromptRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type manifoldPromptResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
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
				if ev.Sender == cfg.MatrixBotUserID {
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

				body, ok := ev.Content["body"].(string)
				if !ok || strings.TrimSpace(body) == "" {
					continue
				}

				trimmed := strings.TrimSpace(body)
				if !strings.HasPrefix(trimmed, cfg.BotPrefix) {
					continue
				}

				prompt := strings.TrimSpace(strings.TrimPrefix(trimmed, cfg.BotPrefix))
				if prompt == "" {
					_, _ = matrixClient.SendText(roomID, fmt.Sprintf("Usage: %s <your question>", cfg.BotPrefix))
					continue
				}

				sessionID := sessionIDForRoom(cfg.ManifoldSessionPrefix, roomID)
				answer, err := callManifold(httpClient, cfg, sessionID, prompt)
				if err != nil {
					log.Printf("manifold prompt error (room=%s session=%s): %v", roomID, sessionID, err)
					_, _ = matrixClient.SendText(roomID, "Sorry, I hit an upstream error talking to Manifold.")
					continue
				}

				if _, err := matrixClient.SendText(roomID, answer); err != nil {
					log.Printf("send message error (room=%s): %v", roomID, err)
				}
			}
		}
	}
}

func loadConfig() (config, error) {
	c := config{
		MatrixHomeserverURL: strings.TrimSpace(os.Getenv("MATRIX_HOMESERVER_URL")),
		MatrixBotUserID:     strings.TrimSpace(os.Getenv("MATRIX_BOT_USER_ID")),
		MatrixAccessToken:   strings.TrimSpace(os.Getenv("MATRIX_ACCESS_TOKEN")),
		BotPrefix:           strings.TrimSpace(os.Getenv("BOT_PREFIX")),
		ProcessBacklog:      boolEnv("MATRIX_PROCESS_BACKLOG", false),

		ManifoldBaseURL:           strings.TrimSpace(os.Getenv("MANIFOLD_BASE_URL")),
		ManifoldPromptPath:        strings.TrimSpace(os.Getenv("MANIFOLD_PROMPT_PATH")),
		ManifoldProjectID:         strings.TrimSpace(os.Getenv("MANIFOLD_PROJECT_ID")),
		ManifoldSessionPrefix:     strings.TrimSpace(os.Getenv("MANIFOLD_SESSION_PREFIX")),
		ManifoldSessionCookie:     strings.TrimSpace(os.Getenv("MANIFOLD_SESSION_COOKIE")),
		ManifoldSessionCookieName: strings.TrimSpace(os.Getenv("MANIFOLD_SESSION_COOKIE_NAME")),
		ManifoldAuthBearerToken:   strings.TrimSpace(os.Getenv("MANIFOLD_AUTH_BEARER_TOKEN")),

		SyncTimeoutSeconds:    intEnv("MATRIX_SYNC_TIMEOUT_SECONDS", 30),
		SyncRetryDelaySeconds: intEnv("MATRIX_SYNC_RETRY_DELAY_SECONDS", 3),
		RequestTimeoutSeconds: intEnv("MANIFOLD_REQUEST_TIMEOUT_SECONDS", 180),
	}

	if c.BotPrefix == "" {
		c.BotPrefix = "!bot"
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

	if c.MatrixHomeserverURL == "" || c.MatrixBotUserID == "" || c.MatrixAccessToken == "" {
		return c, errors.New("missing required env vars: MATRIX_HOMESERVER_URL, MATRIX_BOT_USER_ID, MATRIX_ACCESS_TOKEN")
	}

	return c, nil
}

func callManifold(httpClient *http.Client, cfg config, sessionID, prompt string) (string, error) {
	reqBody := manifoldPromptRequest{
		Prompt:    prompt,
		SessionID: sessionID,
		ProjectID: cfg.ManifoldProjectID,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(cfg.ManifoldBaseURL, "/") + "/" + strings.TrimLeft(cfg.ManifoldPromptPath, "/")
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
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
		return "", err
	}
	defer resp.Body.Close()

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return decodeSSEPromptResponse(resp)
	}

	return decodeJSONPromptResponse(resp)
}

func decodeSSEPromptResponse(resp *http.Response) (string, error) {
	reader := bufio.NewReader(resp.Body)
	var final string
	var lastError string

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
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
							final = strings.TrimSpace(data)
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
			return "", fmt.Errorf("manifold status %d: %s", resp.StatusCode, lastError)
		}
		return "", fmt.Errorf("manifold returned status %d", resp.StatusCode)
	}

	if final != "" {
		return final, nil
	}

	if lastError != "" {
		return "", fmt.Errorf("manifold stream error: %s", lastError)
	}

	return "", errors.New("empty response from manifold stream")
}

func decodeJSONPromptResponse(resp *http.Response) (string, error) {

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var decoded manifoldPromptResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		if resp.StatusCode >= 300 {
			trimmed := strings.TrimSpace(string(body))
			if trimmed == "" {
				trimmed = http.StatusText(resp.StatusCode)
			}
			return "", fmt.Errorf("manifold status %d: %s", resp.StatusCode, trimmed)
		}
		return "", fmt.Errorf("failed to decode manifold response (status=%d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode >= 300 {
		if strings.TrimSpace(decoded.Error) != "" {
			return "", fmt.Errorf("manifold status %d: %s", resp.StatusCode, decoded.Error)
		}
		return "", fmt.Errorf("manifold returned status %d", resp.StatusCode)
	}

	if strings.TrimSpace(decoded.Result) == "" {
		return "", errors.New("empty response from manifold")
	}

	return strings.TrimSpace(decoded.Result), nil
}

func sessionIDForRoom(prefix, roomID string) string {
	cleaned := strings.TrimSpace(roomID)
	if cleaned == "" {
		cleaned = "default"
	}
	namespaceSeed := strings.TrimSpace(prefix) + ":" + cleaned
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(namespaceSeed)).String()
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
