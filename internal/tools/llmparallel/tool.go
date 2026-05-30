package llmparallel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/tools"
)

const (
	toolName                = "llm_parallel_completions"
	defaultParallelRequests = 3
	maxParallelRequests     = 3
	defaultTimeoutMS        = 45000
	fixedMaxTokens          = 16000
	defaultMaxTokens        = fixedMaxTokens
	defaultAggMaxTokens     = fixedMaxTokens
)

type Tool struct {
	httpClient     *http.Client
	defaultBaseURL string
	defaultModel   string
	defaultAPIKey  string
}

func New(httpClient *http.Client, baseURL, model, apiKey string) tools.Tool {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Tool{
		httpClient:     httpClient,
		defaultBaseURL: strings.TrimSpace(baseURL),
		defaultModel:   strings.TrimSpace(model),
		defaultAPIKey:  strings.TrimSpace(apiKey),
	}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        toolName,
		"description": "Run the same prompt in parallel against an OpenAI-compatible completions endpoint, then aggregate candidates into one final answer.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":                  map[string]any{"type": "string", "description": "Prompt to send to each parallel completion call."},
				"model":                   map[string]any{"type": "string", "description": "Model name for the completions endpoint. Defaults to configured model."},
				"endpoint":                map[string]any{"type": "string", "description": "Full completions URL (for example http://host:port/v1/completions). Overrides base_url."},
				"base_url":                map[string]any{"type": "string", "description": "Base URL for an OpenAI-compatible endpoint. /v1/completions is appended automatically."},
				"api_key":                 map[string]any{"type": "string", "description": "Optional bearer token. Leave empty for local endpoints that require no key."},
				"parallel_requests":       map[string]any{"type": "integer", "minimum": 1, "maximum": maxParallelRequests, "description": "Total number of same-prompt completions to run."},
				"batch_size":              map[string]any{"type": "integer", "minimum": 1, "maximum": maxParallelRequests, "description": "Maximum in-flight completion calls (concurrency limit)."},
				"max_tokens":              map[string]any{"type": "integer", "minimum": 1, "description": "Ignored by this tool; max_tokens is fixed at 16000 for every completion request."},
				"temperature":             map[string]any{"type": "number", "description": "Sampling temperature per candidate completion."},
				"top_p":                   map[string]any{"type": "number", "description": "Top-p nucleus sampling parameter."},
				"stop":                    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional stop sequences."},
				"timeout_ms":              map[string]any{"type": "integer", "minimum": 1, "description": "Per-request timeout for each completion call."},
				"aggregate":               map[string]any{"type": "boolean", "description": "When true (default), run a final synthesis pass to combine candidates."},
				"aggregation_model":       map[string]any{"type": "string", "description": "Optional model override for the final synthesis pass."},
				"aggregation_max_tokens":  map[string]any{"type": "integer", "minimum": 1, "description": "Ignored by this tool; synthesis max_tokens is fixed at 16000."},
				"aggregation_temperature": map[string]any{"type": "number", "description": "Temperature for synthesis pass. Defaults to 0.2."},
				"extra_params":            map[string]any{"type": "object", "description": "Additional JSON fields merged into each completion request body."},
			},
			"required": []string{"prompt"},
		},
	}
}

type callArgs struct {
	Prompt                 string         `json:"prompt"`
	Model                  string         `json:"model"`
	Endpoint               string         `json:"endpoint"`
	BaseURL                string         `json:"base_url"`
	APIKey                 string         `json:"api_key"`
	ParallelRequests       int            `json:"parallel_requests"`
	BatchSize              int            `json:"batch_size"`
	MaxTokens              int            `json:"max_tokens"`
	Temperature            *float64       `json:"temperature"`
	TopP                   *float64       `json:"top_p"`
	Stop                   []string       `json:"stop"`
	TimeoutMS              int            `json:"timeout_ms"`
	Aggregate              *bool          `json:"aggregate"`
	AggregationModel       string         `json:"aggregation_model"`
	AggregationMaxTokens   int            `json:"aggregation_max_tokens"`
	AggregationTemperature *float64       `json:"aggregation_temperature"`
	ExtraParams            map[string]any `json:"extra_params"`
}

type candidate struct {
	Index        int     `json:"index"`
	Text         string  `json:"text"`
	FinishReason string  `json:"finish_reason,omitempty"`
	DurationMS   int64   `json:"duration_ms"`
	Score        float64 `json:"score,omitempty"`
}

type completionResponse struct {
	Choices []struct {
		Text         string `json:"text"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type runConfig struct {
	args      callArgs
	endpoint  string
	model     string
	apiKey    string
	total     int
	batchSize int
	timeoutMS int
	aggregate bool
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	cfg, err := t.prepareRunConfig(raw)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}

	successful, errorsOut := t.runParallelCompletions(ctx, cfg)
	if len(successful) == 0 {
		return allFailedResponse(cfg.total, errorsOut), nil
	}

	finalText, method, errorsOut := t.finalResponse(ctx, cfg, successful, errorsOut)
	scoreCandidates(successful)
	sort.Slice(successful, func(i, j int) bool { return successful[i].Index < successful[j].Index })

	return successResponse(finalText, method, successful, errorsOut, cfg.total), nil
}

func (t *Tool) prepareRunConfig(raw json.RawMessage) (runConfig, error) {
	var args callArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return runConfig{}, fmt.Errorf("invalid arguments: %v", err)
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return runConfig{}, fmt.Errorf("prompt is required")
	}

	endpoint := t.resolveEndpoint(args)
	if endpoint == "" {
		return runConfig{}, fmt.Errorf("endpoint is required (or configure openai.baseURL)")
	}
	model := strings.TrimSpace(args.Model)
	if model == "" {
		model = t.defaultModel
	}
	if model == "" {
		return runConfig{}, fmt.Errorf("model is required (or configure openai.model)")
	}

	apiKey := strings.TrimSpace(args.APIKey)
	if apiKey == "" {
		apiKey = t.defaultAPIKey
	}

	total := args.ParallelRequests
	if total <= 0 {
		total = defaultParallelRequests
	}
	if total > maxParallelRequests {
		total = maxParallelRequests
	}

	aggregate := true
	if args.Aggregate != nil {
		aggregate = *args.Aggregate
	}
	if aggregate && total >= maxParallelRequests {
		total = maxParallelRequests - 1
	}
	if total <= 0 {
		total = 1
	}

	batchSize := args.BatchSize
	if batchSize <= 0 {
		batchSize = total
	}
	if batchSize > total {
		batchSize = total
	}

	timeoutMS := args.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultTimeoutMS
	}
	args.MaxTokens = fixedMaxTokens

	return runConfig{
		args:      args,
		endpoint:  endpoint,
		model:     model,
		apiKey:    apiKey,
		total:     total,
		batchSize: batchSize,
		timeoutMS: timeoutMS,
		aggregate: aggregate,
	}, nil
}

func (t *Tool) runParallelCompletions(ctx context.Context, cfg runConfig) ([]candidate, []string) {
	cands := make([]candidate, cfg.total)
	errorsOut := make([]string, 0)
	errMu := sync.Mutex{}
	wg := sync.WaitGroup{}
	sem := make(chan struct{}, cfg.batchSize)

	for i := 0; i < cfg.total; i++ {
		i := i
		wg.Go(func() {
			cand, err := t.runCompletion(ctx, cfg, sem, i)
			if err != nil {
				errMu.Lock()
				errorsOut = append(errorsOut, err.Error())
				errMu.Unlock()
				return
			}
			cands[i] = cand
		})
	}
	wg.Wait()

	successful := make([]candidate, 0, cfg.total)
	for _, c := range cands {
		if strings.TrimSpace(c.Text) != "" {
			successful = append(successful, c)
		}
	}
	return successful, errorsOut
}

func (t *Tool) runCompletion(ctx context.Context, cfg runConfig, sem chan struct{}, index int) (candidate, error) {
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return candidate{}, fmt.Errorf("request %d canceled: %v", index, ctx.Err())
	}
	defer func() { <-sem }()

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.timeoutMS)*time.Millisecond)
	defer cancel()

	started := time.Now()
	body := requestBody(cfg.args, cfg.model, cfg.args.Prompt, 1)
	text, finish, err := t.callCompletion(callCtx, cfg.endpoint, cfg.apiKey, body)
	if err != nil {
		return candidate{}, fmt.Errorf("request %d failed: %v", index, err)
	}
	return candidate{
		Index:        index,
		Text:         text,
		FinishReason: finish,
		DurationMS:   time.Since(started).Milliseconds(),
	}, nil
}

func (t *Tool) finalResponse(ctx context.Context, cfg runConfig, successful []candidate, errorsOut []string) (string, string, []string) {
	finalText := successful[0].Text
	aggregationMethod := "first_success"

	if cfg.aggregate && len(successful) > 1 {
		aggModel := strings.TrimSpace(cfg.args.AggregationModel)
		if aggModel == "" {
			aggModel = cfg.model
		}
		aggMaxTokens := cfg.args.AggregationMaxTokens
		if aggMaxTokens <= 0 {
			aggMaxTokens = max(cfg.args.MaxTokens, defaultAggMaxTokens)
		}
		aggTemp := 0.2
		if cfg.args.AggregationTemperature != nil {
			aggTemp = *cfg.args.AggregationTemperature
		}

		synthesisPrompt := buildSynthesisPrompt(cfg.args.Prompt, successful)
		synthArgs := cfg.args
		synthArgs.MaxTokens = aggMaxTokens
		synthArgs.Temperature = &aggTemp
		synthArgs.Stop = nil
		synthBody := requestBody(synthArgs, aggModel, synthesisPrompt, 1)

		if synthesis, _, err := t.callCompletion(ctx, cfg.endpoint, cfg.apiKey, synthBody); err == nil && strings.TrimSpace(synthesis) != "" {
			finalText = synthesis
			aggregationMethod = "synthesis"
		} else {
			best := bestCandidate(successful)
			finalText = best.Text
			aggregationMethod = "best_candidate"
			if err != nil {
				errorsOut = append(errorsOut, fmt.Sprintf("aggregation synthesis failed: %v", err))
			}
		}
	} else if len(successful) > 1 {
		best := bestCandidate(successful)
		finalText = best.Text
		aggregationMethod = "best_candidate"
	}
	return finalText, aggregationMethod, errorsOut
}

func scoreCandidates(successful []candidate) {
	for i := range successful {
		successful[i].Score = scoreCandidate(successful[i].Text)
	}
}

func allFailedResponse(total int, errorsOut []string) map[string]any {
	return map[string]any{
		"ok":     false,
		"error":  "all parallel completion requests failed",
		"errors": errorsOut,
		"stats": map[string]any{
			"requested":  total,
			"successful": 0,
			"failed":     total,
		},
	}
}

func successResponse(finalText, method string, candidates []candidate, errorsOut []string, total int) map[string]any {
	return map[string]any{
		"ok":                 true,
		"final_response":     finalText,
		"aggregation_method": method,
		"candidates":         candidates,
		"errors":             errorsOut,
		"stats": map[string]any{
			"requested":  total,
			"successful": len(candidates),
			"failed":     total - len(candidates),
		},
	}
}

func (t *Tool) resolveEndpoint(args callArgs) string {
	if ep := strings.TrimSpace(args.Endpoint); ep != "" {
		return ep
	}
	base := strings.TrimSpace(args.BaseURL)
	if base == "" {
		base = t.defaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, "/v1/completions") {
		return base
	}
	return base + "/v1/completions"
}

func requestBody(args callArgs, model, prompt string, n int) map[string]any {
	body := map[string]any{
		"model":  model,
		"prompt": prompt,
		"n":      n,
		"stream": false,
	}
	if args.MaxTokens > 0 {
		body["max_tokens"] = fixedMaxTokens
	}
	if args.Temperature != nil {
		body["temperature"] = *args.Temperature
	}
	if args.TopP != nil {
		body["top_p"] = *args.TopP
	}
	if len(args.Stop) > 0 {
		body["stop"] = args.Stop
	}
	for k, v := range args.ExtraParams {
		if _, exists := body[k]; !exists {
			body[k] = v
		}
	}
	return body
}

func (t *Tool) callCompletion(ctx context.Context, endpoint, apiKey string, body map[string]any) (string, string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out completionResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", "", fmt.Errorf("endpoint returned no choices")
	}
	text := strings.TrimSpace(out.Choices[0].Text)
	if text == "" {
		text = strings.TrimSpace(out.Choices[0].Message.Content)
	}
	if text == "" {
		return "", "", fmt.Errorf("choice text is empty")
	}
	return text, out.Choices[0].FinishReason, nil
}

func buildSynthesisPrompt(prompt string, candidates []candidate) string {
	var b strings.Builder
	b.WriteString("You are an expert response aggregator. Combine multiple candidate answers to the same prompt into one best final answer.\n")
	b.WriteString("Rules: keep the answer accurate, concise, and complete; resolve conflicts; do not mention candidates or voting.\n\n")
	b.WriteString("Original prompt:\n")
	b.WriteString(prompt)
	b.WriteString("\n\nCandidates:\n")
	for i, c := range candidates {
		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, strings.TrimSpace(c.Text)))
	}
	b.WriteString("\nFinal best answer:\n")
	return b.String()
}

func bestCandidate(candidates []candidate) candidate {
	if len(candidates) == 0 {
		return candidate{}
	}
	best := candidates[0]
	bestScore := scoreCandidate(best.Text)
	for _, c := range candidates[1:] {
		s := scoreCandidate(c.Text)
		if s > bestScore {
			best = c
			bestScore = s
		}
	}
	best.Score = bestScore
	return best
}

func scoreCandidate(text string) float64 {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return -1
	}
	words := strings.Fields(strings.ToLower(trimmed))
	if len(words) == 0 {
		return -1
	}
	unique := make(map[string]struct{}, len(words))
	for _, w := range words {
		unique[w] = struct{}{}
	}
	uniqueRatio := float64(len(unique)) / float64(len(words))
	lenScore := math.Log1p(float64(len(trimmed)))
	return lenScore + (2.0 * uniqueRatio)
}
