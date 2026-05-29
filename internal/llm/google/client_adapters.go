package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	genai "google.golang.org/genai"

	"manifold/internal/llm"
)

func (c *Client) pickModel(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return c.model
	}
	return m
}

func (c *Client) buildContentConfig(ctx context.Context, model string, tools []*genai.Tool, toolCfg *genai.ToolConfig) *genai.GenerateContentConfig {
	httpOpts := c.httpOptions
	if extraBody := c.buildExtraBody(); extraBody != nil {
		if httpOpts.ExtraBody != nil {
			httpOpts.ExtraBody = mergeAnyMap(httpOpts.ExtraBody, extraBody)
		} else {
			httpOpts.ExtraBody = extraBody
		}
	}

	cfg := &genai.GenerateContentConfig{
		HTTPOptions: &httpOpts,
		Tools:       tools,
		ToolConfig:  toolCfg,
	}
	if shouldIncludeThoughtSummaries(model) {
		cfg.ThinkingConfig = &genai.ThinkingConfig{IncludeThoughts: true}
	}
	if opts, ok := llm.ImagePromptFromContext(ctx); ok {
		size := strings.TrimSpace(opts.Size)
		if size == "" {
			size = "1K"
		}
		cfg.ResponseModalities = []string{"IMAGE", "TEXT"}
		cfg.ImageConfig = &genai.ImageConfig{
			ImageSize: size,
		}
	}
	return cfg
}

func (c *Client) buildExtraBody() map[string]any {
	if len(c.extra) == 0 {
		return nil
	}

	body := map[string]any{}
	genCfg := map[string]any{}

	for rawKey, val := range c.extra {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		norm := normalizeExtraKey(key)
		switch norm {
		case "generationconfig":
			if m, ok := val.(map[string]any); ok {
				maps.Copy(genCfg, m)
			} else {
				body[key] = val
			}
		case "temperature":
			genCfg["temperature"] = val
		case "topp":
			genCfg["topP"] = val
		case "topk":
			genCfg["topK"] = val
		case "candidatecount":
			genCfg["candidateCount"] = val
		case "maxoutputtokens":
			genCfg["maxOutputTokens"] = val
		case "stopsequences":
			genCfg["stopSequences"] = val
		case "responsemimetype":
			genCfg["responseMimeType"] = val
		case "responseschema":
			genCfg["responseSchema"] = val
		case "responsejsonschema":
			genCfg["responseJsonSchema"] = val
		case "responselogprobs":
			genCfg["responseLogprobs"] = val
		case "logprobs":
			genCfg["logprobs"] = val
		case "presencepenalty":
			genCfg["presencePenalty"] = val
		case "frequencypenalty":
			genCfg["frequencyPenalty"] = val
		case "seed":
			genCfg["seed"] = val
		default:
			body[key] = val
		}
	}

	if len(genCfg) > 0 {
		body["generationConfig"] = genCfg
	}
	if len(body) == 0 {
		return nil
	}
	return body
}

func normalizeExtraKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return strings.ToLower(key)
}

func mergeAnyMap(base, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(override))
	maps.Copy(out, base)
	maps.Copy(out, override)
	return out
}

func shouldIncludeThoughtSummaries(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if idx := strings.LastIndex(m, "/"); idx != -1 {
		m = m[idx+1:]
	}
	return strings.Contains(m, "gemini-2.5") || strings.Contains(m, "gemini-3")
}

func toContents(msgs []llm.Message) ([]*genai.Content, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("messages required")
	}
	state := googleContentState{toolNamesByID: map[string]string{}}
	contents := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		content, ok, err := contentFromMessage(m, &state)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		contents = append(contents, content)
	}
	return contents, nil
}

type googleContentState struct {
	toolNamesByID map[string]string
	lastFuncName  string
}

func contentFromMessage(m llm.Message, state *googleContentState) (*genai.Content, bool, error) {
	role, err := googleMessageRole(m, state)
	if err != nil {
		return nil, false, err
	}
	if role == "tool" {
		return toolResponseContent(m, state), true, nil
	}
	parts := googleMessageParts(m, role)
	if len(parts) == 0 {
		return nil, false, nil
	}
	return &genai.Content{Role: role, Parts: parts}, true, nil
}

func googleMessageRole(m llm.Message, state *googleContentState) (string, error) {
	switch strings.ToLower(strings.TrimSpace(m.Role)) {
	case "", "user", "system":
		return genai.RoleUser, nil
	case "assistant":
		for _, tc := range m.ToolCalls {
			if tc.ID != "" && tc.Name != "" {
				state.toolNamesByID[tc.ID] = tc.Name
			}
			if strings.TrimSpace(tc.Name) != "" {
				state.lastFuncName = tc.Name
			}
		}
		return genai.RoleModel, nil
	case "tool":
		return "tool", nil
	default:
		return "", fmt.Errorf("unsupported role for google provider: %s", m.Role)
	}
}

func toolResponseContent(m llm.Message, state *googleContentState) *genai.Content {
	name := state.toolNamesByID[m.ToolID]
	if name == "" {
		name = state.lastFuncName
		if name == "" {
			name = "tool_response"
		}
	}
	respMap := map[string]any{}
	if trimmed := strings.TrimSpace(m.Content); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &respMap); err != nil {
			respMap = map[string]any{"output": m.Content}
		}
	}
	part := genai.NewPartFromFunctionResponse(name, respMap)
	part.FunctionResponse.ID = m.ToolID
	return genai.NewContentFromParts([]*genai.Part{part}, genai.RoleUser)
}

func googleMessageParts(m llm.Message, role string) []*genai.Part {
	parts := make([]*genai.Part, 0, 1+len(m.ToolCalls))
	text := m.Content
	if role == genai.RoleUser && strings.ToLower(strings.TrimSpace(m.Role)) == "system" {
		text = "[system] " + text
	}
	if strings.TrimSpace(text) != "" {
		textPart := &genai.Part{Text: text}
		if role == genai.RoleModel {
			if sigBytes, ok := decodeThoughtSignature(m.ThoughtSignature); ok {
				textPart.ThoughtSignature = sigBytes
			}
		}
		parts = append(parts, textPart)
	}
	if role == genai.RoleModel {
		parts = append(parts, googleToolCallParts(m.ToolCalls)...)
	}
	return parts
}

func googleToolCallParts(calls []llm.ToolCall) []*genai.Part {
	parts := make([]*genai.Part, 0, len(calls))
	for _, tc := range calls {
		args := map[string]any{}
		if len(tc.Args) > 0 {
			_ = json.Unmarshal(tc.Args, &args)
		}
		if len(args) == 0 && len(tc.Args) > 0 {
			args = map[string]any{"input": string(tc.Args)}
		}
		part := genai.NewPartFromFunctionCall(tc.Name, args)
		part.FunctionCall.ID = tc.ID
		if sigBytes, ok := decodeThoughtSignature(tc.ThoughtSignature); ok {
			part.ThoughtSignature = sigBytes
		}
		parts = append(parts, part)
	}
	return parts
}

func decodeThoughtSignature(sig string) ([]byte, bool) {
	s := strings.TrimSpace(sig)
	if s == "" || strings.ContainsRune(s, '\uFFFD') {
		return nil, false
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, true
	}
	return []byte(s), true
}

// messageFromStreamResponse parses a streaming response chunk. It returns:
// - (msg, false, nil) when the chunk contains actionable content
// - (empty, true, nil) when the chunk should be skipped (empty/intermediate)
// - (empty, false, err) when the chunk contains an error condition (safety block, etc.)
//
// This is more lenient than messageFromResponse because streaming can produce
// intermediate chunks with empty candidates or nil content, which is normal.
func messageFromStreamResponse(resp *genai.GenerateContentResponse) (llm.Message, string, bool, error) {
	candidate, skip, err := streamCandidate(resp)
	if err != nil || skip {
		return llm.Message{}, "", skip, err
	}
	parsed := parseGoogleContentParts(candidate.Content, true)
	if parsed.empty() {
		return llm.Message{}, parsed.summary, true, nil
	}
	return parsed.message(), parsed.summary, false, nil
}

func messageFromResponse(resp *genai.GenerateContentResponse) (llm.Message, error) {
	candidate, err := responseCandidate(resp)
	if err != nil {
		return llm.Message{}, err
	}
	if candidate.Content == nil {
		return llm.Message{Role: "assistant"}, nil
	}
	return parseGoogleContentParts(candidate.Content, false).message(), nil
}

func streamCandidate(resp *genai.GenerateContentResponse) (*genai.Candidate, bool, error) {
	if resp == nil {
		return nil, true, nil
	}
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return nil, false, fmt.Errorf("request blocked by google: %s", resp.PromptFeedback.BlockReason)
	}
	if len(resp.Candidates) == 0 {
		return nil, true, nil
	}
	candidate := resp.Candidates[0]
	if err := validateGoogleFinishReason(candidate.FinishReason); err != nil {
		return nil, false, err
	}
	if candidate.Content == nil {
		return nil, true, nil
	}
	return candidate, false, nil
}

func responseCandidate(resp *genai.GenerateContentResponse) (*genai.Candidate, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil response from google provider")
	}
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return nil, fmt.Errorf("request blocked by google: %s", resp.PromptFeedback.BlockReason)
	}
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in google response")
	}
	candidate := resp.Candidates[0]
	if err := validateGoogleFinishReason(candidate.FinishReason); err != nil {
		return nil, err
	}
	return candidate, nil
}

func validateGoogleFinishReason(reason genai.FinishReason) error {
	switch reason {
	case genai.FinishReasonSafety:
		return fmt.Errorf("response blocked by safety filters")
	case genai.FinishReasonRecitation:
		return fmt.Errorf("response blocked due to recitation")
	case genai.FinishReasonMalformedFunctionCall:
		return fmt.Errorf("malformed function call generated by model")
	default:
		return nil
	}
}

type parsedGoogleContent struct {
	content          strings.Builder
	summary          string
	toolCalls        []llm.ToolCall
	images           []llm.GeneratedImage
	thoughtSignature string
}

func (p parsedGoogleContent) empty() bool {
	return p.content.Len() == 0 && len(p.toolCalls) == 0 && len(p.images) == 0
}

func (p parsedGoogleContent) message() llm.Message {
	return llm.Message{
		Role:             "assistant",
		Content:          p.content.String(),
		ToolCalls:        nonEmptyToolCalls(p.toolCalls),
		Images:           nonEmptyImages(p.images),
		ThoughtSignature: p.thoughtSignature,
	}
}

func parseGoogleContentParts(content *genai.Content, includeSummary bool) parsedGoogleContent {
	parsed := parsedGoogleContent{}
	var summary strings.Builder
	callIdx := 0
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		parsed.captureThoughtSignature(part)
		parsed.captureImage(part)
		if part.Thought {
			if includeSummary {
				summary.WriteString(part.Text)
			}
			continue
		}
		parsed.content.WriteString(part.Text)
		if part.FunctionCall != nil {
			callIdx++
			parsed.toolCalls = append(parsed.toolCalls, googleToolCallFromPart(part, callIdx))
		}
	}
	parsed.summary = summary.String()
	return parsed
}

func (p *parsedGoogleContent) captureThoughtSignature(part *genai.Part) {
	if part.FunctionCall == nil && len(part.ThoughtSignature) > 0 && p.thoughtSignature == "" {
		p.thoughtSignature = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
	}
}

func (p *parsedGoogleContent) captureImage(part *genai.Part) {
	if part.InlineData == nil {
		return
	}
	p.images = append(p.images, llm.GeneratedImage{Data: part.InlineData.Data, MIMEType: part.InlineData.MIMEType})
}

func googleToolCallFromPart(part *genai.Part, callIdx int) llm.ToolCall {
	args, _ := json.Marshal(part.FunctionCall.Args)
	id := part.FunctionCall.ID
	if strings.TrimSpace(id) == "" {
		id = "call-" + strconv.Itoa(callIdx)
	}
	var sig string
	if len(part.ThoughtSignature) > 0 {
		sig = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
	}
	return llm.ToolCall{Name: part.FunctionCall.Name, Args: args, ID: id, ThoughtSignature: sig}
}

func nonEmptyToolCalls(calls []llm.ToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func nonEmptyImages(images []llm.GeneratedImage) []llm.GeneratedImage {
	if len(images) == 0 {
		return nil
	}
	return images
}

func adaptTools(schemas []llm.ToolSchema) ([]*genai.Tool, *genai.ToolConfig, error) {
	if len(schemas) == 0 {
		return nil, nil, nil
	}
	fd := make([]*genai.FunctionDeclaration, 0, len(schemas))
	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		if strings.TrimSpace(s.Name) == "" {
			return nil, nil, fmt.Errorf("google provider: tool name required")
		}
		names = append(names, s.Name)
		fd = append(fd, &genai.FunctionDeclaration{
			Name:                 s.Name,
			Description:          s.Description,
			ParametersJsonSchema: s.Parameters,
		})
	}
	sort.Strings(names)
	// Use AUTO mode to let the model decide whether to call a function or respond with text.
	// This prevents infinite loops where the model repeatedly calls the same function.
	// Note: AllowedFunctionNames should only be set when mode is ANY, not AUTO.
	// See: https://ai.google.dev/gemini-api/docs/function-calling#function-calling-modes
	cfg := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeAuto,
			// AllowedFunctionNames is intentionally omitted in AUTO mode per API requirements
		},
	}
	tool := &genai.Tool{FunctionDeclarations: fd}
	return []*genai.Tool{tool}, cfg, nil
}
