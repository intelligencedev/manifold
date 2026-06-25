package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

func (c *Client) chatWithImageGeneration(ctx context.Context, msgs []llm.Message, model string, opts llm.ImagePromptOptions) (llm.Message, error) {
	prompt := lastUserPrompt(msgs)
	if strings.TrimSpace(prompt) == "" {
		return llm.Message{}, fmt.Errorf("image generation requires a user prompt")
	}

	imgModel := c.imageModel(model)
	size := normalizeImageSize(opts.Size)

	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ImageGen", imgModel, 0, len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	params := sdk.ImageGenerateParams{
		Prompt: prompt,
		Model:  sdk.ImageModel(imgModel),
		N:      param.NewOpt[int64](1),
		Size:   sdk.ImageGenerateParamsSize(size),
	}
	applyImageExtraParams(&params, c.extra)

	start := time.Now()
	resp, err := c.sdk.Images.Generate(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", imgModel).Dur("duration", dur).Msg("image_generation_error")
		span.RecordError(err)
		return llm.Message{}, err
	}
	images := make([]llm.GeneratedImage, 0, len(resp.Data))
	for _, img := range resp.Data {
		if strings.TrimSpace(img.B64JSON) == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(img.B64JSON)
		if err != nil {
			log.Warn().Err(err).Msg("decode_generated_image")
			continue
		}
		images = append(images, llm.GeneratedImage{
			Data:     data,
			MIMEType: "image/png",
		})
	}
	log.Debug().Str("model", imgModel).Dur("duration", dur).Int("images", len(images)).Msg("image_generation_ok")

	content := "Generated image"
	if len(images) > 1 {
		content = fmt.Sprintf("Generated %d images", len(images))
	}
	return llm.Message{Role: "assistant", Content: content, Images: images}, nil
}

func (c *Client) chatWithVideoGeneration(ctx context.Context, msgs []llm.Message, model string) (llm.Message, error) {
	prompt := lastUserPrompt(msgs)
	if strings.TrimSpace(prompt) == "" {
		return llm.Message{}, fmt.Errorf("video generation requires a user prompt")
	}
	videoModel := strings.TrimSpace(firstNonEmpty(model, c.model))
	if videoModel == "" {
		return llm.Message{}, fmt.Errorf("video generation requires a model")
	}

	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI VideoGen", videoModel, 0, len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	start := time.Now()
	video, err := c.submitAndPollVideo(ctx, videoModel, prompt)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", videoModel).Dur("duration", dur).Msg("video_generation_error")
		span.RecordError(err)
		return llm.Message{}, err
	}
	logEvent := log.Debug().Str("model", videoModel).Dur("duration", dur).Str("mime", video.MIMEType)
	if video.URL != "" {
		logEvent = logEvent.Str("url", video.URL)
	}
	logEvent.Msg("video_generation_ok")
	return llm.Message{Role: "assistant", Content: "Generated video", Videos: []llm.GeneratedVideo{video}}, nil
}

func (c *Client) submitAndPollVideo(ctx context.Context, model, prompt string) (llm.GeneratedVideo, error) {
	payload := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range c.extra {
		payload[k] = v
	}
	delete(payload, "poll_interval_ms")
	delete(payload, "pollIntervalMs")
	delete(payload, "max_poll_attempts")
	delete(payload, "maxPollAttempts")

	resp, err := c.doVideoJSON(ctx, http.MethodPost, c.videoEndpointURL("videos"), payload)
	if err != nil {
		return llm.GeneratedVideo{}, err
	}
	if videoURL := videoResultURL(resp); videoURL != "" {
		return c.downloadGeneratedVideo(ctx, videoURL, "")
	}
	pollingURL := strings.TrimSpace(firstString(resp, "polling_url", "pollingUrl", "poll_url", "pollUrl"))
	if pollingURL == "" {
		return llm.GeneratedVideo{}, fmt.Errorf("video generation response missing polling_url or video URL")
	}

	interval := c.videoPollInterval()
	attempts := c.videoMaxPollAttempts()
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, interval); err != nil {
				return llm.GeneratedVideo{}, err
			}
		}
		status, err := c.doVideoJSON(ctx, http.MethodGet, pollingURL, nil)
		if err != nil {
			return llm.GeneratedVideo{}, err
		}
		if videoURL := videoResultURL(status); videoURL != "" {
			return c.downloadGeneratedVideo(ctx, videoURL, "")
		}
		state := strings.ToLower(strings.TrimSpace(firstString(status, "status", "state")))
		if state == "failed" || state == "error" || state == "cancelled" || state == "canceled" {
			if msg := strings.TrimSpace(firstString(status, "error", "message", "reason")); msg != "" {
				return llm.GeneratedVideo{}, fmt.Errorf("video generation %s: %s", state, msg)
			}
			return llm.GeneratedVideo{}, fmt.Errorf("video generation %s", state)
		}
	}
	return llm.GeneratedVideo{}, fmt.Errorf("video generation polling timed out after %d attempts", attempts)
}

func (c *Client) doVideoJSON(ctx context.Context, method, reqURL string, payload any) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyAuthHeader(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("video generation HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var out map[string]any
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) videoEndpointURL(path string) string {
	base := strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
	if base == "" {
		base = "https:/api.openai.com/v1"
	}
	if strings.HasSuffix(base, "/chat/completions") || strings.HasSuffix(base, "/responses") || strings.HasSuffix(base, "/images/generations") || strings.HasSuffix(base, "/videos") {
		base = strings.TrimRight(base[:strings.LastIndex(base, "/")], "/")
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func videoResultURL(data map[string]any) string {
	for _, key := range []string{"url", "video_url", "videoUrl", "output_url", "outputUrl"} {
		if s := strings.TrimSpace(firstString(data, key)); s != "" {
			return s
		}
	}
	for _, key := range []string{"unsigned_urls", "unsignedUrls", "urls", "data", "output", "result", "video"} {
		if v, ok := data[key]; ok {
			if s := videoResultURLFromAny(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func videoResultURLFromAny(v any) string {
	switch tv := v.(type) {
	case string:
		if isLikelyVideoURL(tv) {
			return strings.TrimSpace(tv)
		}
	case map[string]any:
		return videoResultURL(tv)
	case []any:
		for _, item := range tv {
			if s := videoResultURLFromAny(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func isLikelyVideoURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "data")
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := data[key]; ok {
			switch tv := v.(type) {
			case string:
				return tv
			case map[string]any:
				if s := firstString(tv, "message", "url"); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func (c *Client) videoPollInterval() time.Duration {
	if ms, ok := imageExtraInt64(c.extra["poll_interval_ms"]); ok && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	if ms, ok := imageExtraInt64(c.extra["pollIntervalMs"]); ok && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 2 * time.Second
}

func (c *Client) videoMaxPollAttempts() int {
	if n, ok := imageExtraInt64(c.extra["max_poll_attempts"]); ok && n > 0 {
		return int(n)
	}
	if n, ok := imageExtraInt64(c.extra["maxPollAttempts"]); ok && n > 0 {
		return int(n)
	}
	return 300
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) downloadGeneratedVideo(ctx context.Context, videoURL, mimeType string) (llm.GeneratedVideo, error) {
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		return llm.GeneratedVideo{}, fmt.Errorf("video generation missing content URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return llm.GeneratedVideo{}, err
	}
	c.applyAuthHeader(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return llm.GeneratedVideo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return llm.GeneratedVideo{}, fmt.Errorf("video download HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return llm.GeneratedVideo{}, err
	}
	resolvedMIME := strings.TrimSpace(mimeType)
	if resolvedMIME == "" {
		resolvedMIME = strings.TrimSpace(resp.Header.Get("Content-Type"))
	}
	if resolvedMIME == "" {
		resolvedMIME = "video/mp4"
	}
	return llm.GeneratedVideo{Data: data, MIMEType: resolvedMIME, URL: videoURL}, nil
}

func videoExtension(mimeType string) string {
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ".mp4"
}

func applyImageExtraParams(params *sdk.ImageGenerateParams, extra map[string]any) {
	if params == nil || len(extra) == 0 {
		return
	}
	remaining := make(map[string]any, len(extra))
	maps.Copy(remaining, extra)
	if v, ok := popStringImageExtraParam(remaining, "size"); ok {
		params.Size = sdk.ImageGenerateParamsSize(normalizeImageSize(v))
	}
	if v, ok := popStringImageExtraParam(remaining, "quality"); ok {
		params.Quality = sdk.ImageGenerateParamsQuality(v)
	}
	if v, ok := popStringImageExtraParam(remaining, "background"); ok {
		params.Background = sdk.ImageGenerateParamsBackground(v)
	}
	if v, ok := popStringImageExtraParam(remaining, "moderation"); ok {
		params.Moderation = sdk.ImageGenerateParamsModeration(v)
	}
	if v, ok := popStringImageExtraParam(remaining, "output_format", "outputFormat"); ok {
		params.OutputFormat = sdk.ImageGenerateParamsOutputFormat(v)
	}
	if v, ok := popStringImageExtraParam(remaining, "response_format", "responseFormat"); ok {
		params.ResponseFormat = sdk.ImageGenerateParamsResponseFormat(v)
	}
	if v, ok := popStringImageExtraParam(remaining, "style"); ok {
		params.Style = sdk.ImageGenerateParamsStyle(v)
	}
	if v, ok := popStringImageExtraParam(remaining, "user"); ok {
		params.User = param.NewOpt(v)
	}
	if v, ok := popIntImageExtraParam(remaining, "n"); ok {
		params.N = param.NewOpt(v)
	}
	if v, ok := popIntImageExtraParam(remaining, "output_compression", "outputCompression"); ok {
		params.OutputCompression = param.NewOpt(v)
	}
	if v, ok := popIntImageExtraParam(remaining, "partial_images", "partialImages"); ok {
		params.PartialImages = param.NewOpt(v)
	}
}

func popStringImageExtraParam(extra map[string]any, keys ...string) (string, bool) {
	keyset := imageExtraKeyset(keys...)
	for k, v := range extra {
		if _, ok := keyset[normalizeImageExtraKey(k)]; !ok {
			continue
		}
		delete(extra, k)
		s := strings.TrimSpace(fmt.Sprint(v))
		return s, s != ""
	}
	return "", false
}

func popIntImageExtraParam(extra map[string]any, keys ...string) (int64, bool) {
	keyset := imageExtraKeyset(keys...)
	for k, v := range extra {
		if _, ok := keyset[normalizeImageExtraKey(k)]; !ok {
			continue
		}
		delete(extra, k)
		if n, ok := imageExtraInt64(v); ok {
			return n, true
		}
		return 0, false
	}
	return 0, false
}

func imageExtraKeyset(keys ...string) map[string]struct{} {
	keyset := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if normalized := normalizeImageExtraKey(key); normalized != "" {
			keyset[normalized] = struct{}{}
		}
	}
	return keyset
}

func normalizeImageExtraKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return strings.ToLower(key)
}

func imageExtraInt64(v any) (int64, bool) {
	switch tv := v.(type) {
	case int:
		return int64(tv), true
	case int64:
		return tv, true
	case int32:
		return int64(tv), true
	case float64:
		return int64(tv), true
	case float32:
		return int64(tv), true
	case json.Number:
		if i, err := tv.Int64(); err == nil {
			return i, true
		}
		if f, err := tv.Float64(); err == nil {
			return int64(f), true
		}
		return 0, false
	case string:
		if s := strings.TrimSpace(tv); s != "" {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return i, true
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return int64(f), true
			}
		}
	}
	return 0, false
}

func lastUserPrompt(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.EqualFold(msgs[i].Role, "user") && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

func normalizeImageSize(raw string) string {
	r := strings.TrimSpace(raw)
	switch strings.ToUpper(r) {
	case "1K", "1024", "1024X1024":
		return "1024x1024"
	case "1024X1792", "PORTRAIT":
		return "1024x1792"
	case "1792X1024", "LANDSCAPE":
		return "1792x1024"
	default:
		if r == "" {
			return "1024x1024"
		}
		return r
	}
}

func (c *Client) imageModel(model string) string {
	m := strings.TrimSpace(firstNonEmpty(model, c.model))
	if m == "" {
		m = "gpt-image-1.5"
	}
	lower := strings.ToLower(m)
	if strings.Contains(lower, "gpt-image") || strings.Contains(lower, "dall-e") {
		return m
	}
	return "gpt-image-1.5"
}

// ChatWithImageAttachment sends a chat completion with an image attachment.
// This is a concrete method specific to the OpenAI provider.
func (c *Client) ChatWithImageAttachment(ctx context.Context, msgs []llm.Message, mimeType, base64Data string, tools []llm.ToolSchema, model string) (llm.Message, error) {
	tools = c.requestTools(tools)

	images := []ImageAttachment{{MimeType: mimeType, Base64Data: base64Data}}
	if strings.EqualFold(c.api, "responses") {
		return c.chatResponsesWithImages(ctx, msgs, images, tools, model)
	}
	return c.chatCompletionWithImages(ctx, msgs, images, tools, model, "OpenAI ChatWithImageAttachment", "chat_completion_with_image_error", "chat_completion_with_image_ok")
}

// ChatWithImageAttachments sends a chat completion with one or more image attachments.
// The images are included as content parts alongside the user's text.
func (c *Client) ChatWithImageAttachments(ctx context.Context, msgs []llm.Message, images []ImageAttachment, tools []llm.ToolSchema, model string) (llm.Message, error) {
	tools = c.requestTools(tools)

	if strings.EqualFold(c.api, "responses") {
		return c.chatResponsesWithImages(ctx, msgs, images, tools, model)
	}
	return c.chatCompletionWithImages(ctx, msgs, images, tools, model, "OpenAI ChatWithImageAttachments", "chat_completion_with_images_error", "chat_completion_with_images_ok")
}

func (c *Client) chatCompletionWithImages(
	ctx context.Context,
	msgs []llm.Message,
	images []ImageAttachment,
	tools []llm.ToolSchema,
	model string,
	spanName string,
	errorMsg string,
	okMsg string,
) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, spanName, firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	params := c.imageChatCompletionParams(msgs, images, tools, model)

	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg(errorMsg)
		span.RecordError(err)
		return llm.Message{}, err
	}

	log.Debug().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg(okMsg)
	out := c.messageFromChatCompletion(comp, string(params.Model))
	llm.LogRedactedResponse(ctx, comp.Choices)
	c.recordChatCompletionUsage(ctx, span, comp.Usage, string(params.Model), msgs, out.Content)
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	return out, nil
}

func (c *Client) imageChatCompletionParams(msgs []llm.Message, images []ImageAttachment, tools []llm.ToolSchema, model string) sdk.ChatCompletionNewParams {
	params := sdk.ChatCompletionNewParams{Model: sdk.ChatModel(firstNonEmpty(model, c.model))}
	adaptedMsgs := AdaptMessages(model, c.chatCompletionMessages(msgs))
	params.Messages = imageChatMessages(adaptedMsgs, images)
	actualTools := configureChatCompletionTools(&params, tools, c.isSelfHosted())
	if len(c.extra) > 0 {
		if !actualTools {
			tmp := make(map[string]any, len(c.extra))
			maps.Copy(tmp, c.extra)
			delete(tmp, "parallel_tool_calls")
			params.SetExtraFields(sanitizeExtraFields(tmp))
		} else {
			params.SetExtraFields(sanitizeExtraFields(c.extra))
		}
	}
	return params
}

func imageChatMessages(adaptedMsgs []sdk.ChatCompletionMessageParamUnion, images []ImageAttachment) []sdk.ChatCompletionMessageParamUnion {
	for i := len(adaptedMsgs) - 1; i >= 0; i-- {
		if adaptedMsgs[i].OfUser == nil {
			continue
		}
		adaptedMsgs[i] = sdk.ChatCompletionMessageParamUnion{
			OfUser: &sdk.ChatCompletionUserMessageParam{
				Content: sdk.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: imageChatContentParts(adaptedMsgs[i].OfUser, images),
				},
			},
		}
		break
	}
	return adaptedMsgs
}

func imageChatContentParts(userMsg *sdk.ChatCompletionUserMessageParam, images []ImageAttachment) []sdk.ChatCompletionContentPartUnionParam {
	var contentParts []sdk.ChatCompletionContentPartUnionParam
	if userMsg.Content.OfString.Valid() && userMsg.Content.OfString.Value != "" {
		contentParts = append(contentParts, sdk.ChatCompletionContentPartUnionParam{
			OfText: &sdk.ChatCompletionContentPartTextParam{Text: userMsg.Content.OfString.Value},
		})
	}
	for _, img := range images {
		if part, ok := imageChatContentPart(img); ok {
			contentParts = append(contentParts, part)
		}
	}
	return contentParts
}

func imageChatContentPart(img ImageAttachment) (sdk.ChatCompletionContentPartUnionParam, bool) {
	if strings.TrimSpace(img.MimeType) == "" || strings.TrimSpace(img.Base64Data) == "" {
		return sdk.ChatCompletionContentPartUnionParam{}, false
	}
	dataURL := "data:" + img.MimeType + ";base64," + img.Base64Data
	return sdk.ChatCompletionContentPartUnionParam{
		OfImageURL: &sdk.ChatCompletionContentPartImageParam{
			ImageURL: sdk.ChatCompletionContentPartImageImageURLParam{
				URL:    dataURL,
				Detail: "auto",
			},
		},
	}, true
}
