package google

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
	genai "google.golang.org/genai"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

type Client struct {
	client      *genai.Client
	model       string
	httpOptions genai.HTTPOptions
	extra       map[string]any
}

type ImageAttachment struct {
	MimeType   string
	Base64Data string
}

func New(cfg config.GoogleConfig, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gemini-1.5-flash"
	}

	httpOpts := genai.HTTPOptions{}
	if cfg.Timeout > 0 {
		t := time.Duration(cfg.Timeout) * time.Second
		httpOpts.Timeout = &t
	}
	if base := strings.TrimSpace(cfg.BaseURL); base != "" {
		httpOpts.BaseURL = strings.TrimSuffix(base, "/") + "/"
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      strings.TrimSpace(cfg.APIKey),
		HTTPClient:  httpClient,
		HTTPOptions: httpOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("init google client: %w", err)
	}

	return &Client{
		client:      client,
		model:       model,
		httpOptions: httpOpts,
		extra:       llm.NormalizeExtraParams(cfg.ExtraParams),
	}, nil
}

func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	effectiveModel := c.pickModel(model)

	// Add observability like OpenAI/Anthropic clients
	ctx, span := llm.StartRequestSpan(ctx, "Google Chat", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	log := observability.LoggerWithTrace(ctx)

	contents, err := toContents(msgs)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_chat_toContents_error")
		return llm.Message{}, err
	}

	toolDecls, toolCfg, err := adaptTools(tools)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_chat_adaptTools_error")
		return llm.Message{}, err
	}

	log.Debug().Str("model", effectiveModel).Int("tools", len(tools)).Int("contents", len(contents)).Msg("google_chat_api_call_start")

	start := time.Now()
	resp, err := c.client.Models.GenerateContent(ctx, effectiveModel, contents, c.buildContentConfig(ctx, effectiveModel, toolDecls, toolCfg))
	dur := time.Since(start)

	log.Debug().Dur("duration", dur).Bool("has_response", resp != nil).Bool("has_error", err != nil).Msg("google_chat_api_call_complete")

	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Str("model", effectiveModel).Dur("duration", dur).Msg("google_chat_error")
		return llm.Message{}, err
	}

	msg, err := messageFromResponse(resp)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Dur("duration", dur).Msg("google_chat_response_parse_error")
		return llm.Message{}, err
	}

	llm.LogRedactedResponse(ctx, resp)
	log.Debug().Str("model", effectiveModel).Int("tools", len(tools)).Dur("duration", dur).Int("tool_calls", len(msg.ToolCalls)).Msg("google_chat_ok")

	return msg, nil
}

func (c *Client) ChatWithImageAttachments(ctx context.Context, msgs []llm.Message, images []ImageAttachment, tools []llm.ToolSchema, model string) (llm.Message, error) {
	effectiveModel := c.pickModel(model)

	ctx, span := llm.StartRequestSpan(ctx, "Google ChatWithImageAttachments", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	log := observability.LoggerWithTrace(ctx)

	contents, err := toContents(msgs)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_chat_with_images_toContents_error")
		return llm.Message{}, err
	}
	contents, err = appendGoogleImageParts(contents, images)
	if err != nil {
		return llm.Message{}, err
	}

	toolDecls, toolCfg, err := adaptTools(tools)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_chat_with_images_adaptTools_error")
		return llm.Message{}, err
	}

	start := time.Now()
	resp, err := c.client.Models.GenerateContent(ctx, effectiveModel, contents, c.buildVisionContentConfig(effectiveModel, toolDecls, toolCfg))
	dur := time.Since(start)
	if err != nil {
		log.Warn().Err(err).Str("model", effectiveModel).Dur("duration", dur).Msg("google_chat_with_images_non_stream_error_retrying_stream")
		return c.streamGoogleImageFallback(ctx, effectiveModel, contents, toolDecls, toolCfg, span, log, start)
	}

	msg, err := messageFromResponse(resp)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Dur("duration", dur).Msg("google_chat_with_images_response_parse_error")
		return llm.Message{}, err
	}

	llm.LogRedactedResponse(ctx, resp)
	log.Debug().Str("model", effectiveModel).Int("tools", len(tools)).Dur("duration", dur).Int("tool_calls", len(msg.ToolCalls)).Msg("google_chat_with_images_ok")

	return msg, nil
}

func appendGoogleImageParts(contents []*genai.Content, images []ImageAttachment) ([]*genai.Content, error) {
	imageParts, err := googleImageParts(images)
	if err != nil || len(imageParts) == 0 {
		return contents, err
	}
	lastUserIdx := -1
	for i := len(contents) - 1; i >= 0; i-- {
		if contents[i] != nil && contents[i].Role == genai.RoleUser {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx >= 0 {
		contents[lastUserIdx].Parts = append(contents[lastUserIdx].Parts, imageParts...)
		return contents, nil
	}
	return append(contents, genai.NewContentFromParts(imageParts, genai.RoleUser)), nil
}

func googleImageParts(images []ImageAttachment) ([]*genai.Part, error) {
	imageParts := make([]*genai.Part, 0, len(images))
	for _, img := range images {
		mime := strings.ToLower(strings.TrimSpace(img.MimeType))
		if mime == "image/jpg" {
			mime = "image/jpeg"
		}
		if mime == "" || strings.TrimSpace(img.Base64Data) == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(img.Base64Data)
		if err != nil {
			return nil, fmt.Errorf("decode image attachment: %w", err)
		}
		imageParts = append(imageParts, &genai.Part{InlineData: &genai.Blob{Data: data, MIMEType: mime}})
	}
	return imageParts, nil
}

func (c *Client) streamGoogleImageFallback(
	ctx context.Context,
	model string,
	contents []*genai.Content,
	toolDecls []*genai.Tool,
	toolCfg *genai.ToolConfig,
	span trace.Span,
	log *zerolog.Logger,
	start time.Time,
) (llm.Message, error) {
	stream := c.client.Models.GenerateContentStream(ctx, model, contents, c.buildVisionContentConfig(model, toolDecls, toolCfg))
	acc := googleStreamAccumulator{}
	for chunk, streamErr := range stream {
		if streamErr != nil {
			span.RecordError(streamErr)
			log.Error().Err(streamErr).Str("model", model).Dur("duration", time.Since(start)).Msg("google_chat_with_images_error")
			return llm.Message{}, streamErr
		}
		if err := acc.add(chunk); err != nil {
			span.RecordError(err)
			log.Error().Err(err).Str("model", model).Dur("duration", time.Since(start)).Msg("google_chat_with_images_stream_parse_error")
			return llm.Message{}, err
		}
	}
	out := acc.message()
	log.Debug().Str("model", model).Dur("duration", time.Since(start)).Int("tool_calls", len(out.ToolCalls)).Msg("google_chat_with_images_stream_fallback_ok")
	return out, nil
}

type googleStreamAccumulator struct {
	text   strings.Builder
	images []llm.GeneratedImage
	calls  []llm.ToolCall
}

func (a *googleStreamAccumulator) add(chunk *genai.GenerateContentResponse) error {
	msg, _, skip, err := messageFromStreamResponse(chunk)
	if err != nil || skip {
		return err
	}
	a.text.WriteString(msg.Content)
	a.images = append(a.images, msg.Images...)
	a.calls = append(a.calls, msg.ToolCalls...)
	return nil
}

func (a *googleStreamAccumulator) message() llm.Message {
	out := llm.Message{Role: "assistant", Content: a.text.String()}
	if len(a.images) > 0 {
		out.Images = a.images
	}
	if len(a.calls) > 0 {
		out.ToolCalls = a.calls
	}
	return out
}

func (c *Client) buildVisionContentConfig(model string, tools []*genai.Tool, toolCfg *genai.ToolConfig) *genai.GenerateContentConfig {
	httpOpts := c.httpOptions
	if extraBody := c.buildExtraBody(); extraBody != nil {
		if httpOpts.ExtraBody != nil {
			httpOpts.ExtraBody = mergeAnyMap(httpOpts.ExtraBody, extraBody)
		} else {
			httpOpts.ExtraBody = extraBody
		}
	}

	// Vision understanding requests use a minimal config: no image-generation modalities
	// and no thinking flags to maximize compatibility across Gemini endpoints/models.
	return &genai.GenerateContentConfig{
		HTTPOptions: &httpOpts,
		Tools:       tools,
		ToolConfig:  toolCfg,
	}
}

func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	effectiveModel := c.pickModel(model)

	// Add observability like OpenAI/Anthropic clients
	ctx, span := llm.StartRequestSpan(ctx, "Google ChatStream", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	log := observability.LoggerWithTrace(ctx)

	contents, err := toContents(msgs)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_stream_toContents_error")
		return err
	}

	toolDecls, toolCfg, err := adaptTools(tools)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_stream_adaptTools_error")
		return err
	}

	start := time.Now()
	log.Debug().Str("model", effectiveModel).Int("tools", len(tools)).Int("msgs", len(msgs)).Msg("google_stream_start")

	stream := c.client.Models.GenerateContentStream(ctx, effectiveModel, contents, c.buildContentConfig(ctx, effectiveModel, toolDecls, toolCfg))

	hasContent := false
	var toolCallCount int
	var thoughtSummaryCount int
	var thoughtSummary strings.Builder
	for resp, err := range stream {
		if err != nil {
			dur := time.Since(start)
			span.RecordError(err)
			log.Error().Err(err).Dur("duration", dur).Msg("google_stream_error")
			return err
		}
		// Use streaming-aware response parser that tolerates intermediate chunks
		// with empty candidates or nil content (normal in streaming).
		msg, summaryDelta, skip, err := messageFromStreamResponse(resp)
		if err != nil {
			dur := time.Since(start)
			span.RecordError(err)
			log.Error().Err(err).Dur("duration", dur).Msg("google_stream_response_parse_error")
			return err
		}
		if summaryDelta != "" && h != nil {
			thoughtSummaryCount++
			thoughtSummary.WriteString(summaryDelta)
			log.Debug().Int("thought_count", thoughtSummaryCount).Int("summary_len", thoughtSummary.Len()).Msg("google_stream_thought_summary")
			h.OnThoughtSummary(thoughtSummary.String())
		}
		if skip {
			// Intermediate chunk with no actionable content - continue streaming
			continue
		}
		hasContent = true
		if h != nil {
			if msg.Content != "" {
				h.OnDelta(msg.Content)
			}
			for _, img := range msg.Images {
				h.OnImage(img)
			}
		}
		for _, tc := range msg.ToolCalls {
			toolCallCount++
			if h != nil {
				h.OnToolCall(tc)
			}
		}
	}

	dur := time.Since(start)
	if !hasContent {
		log.Warn().Dur("duration", dur).Int("thought_summaries", thoughtSummaryCount).Msg("google_stream_empty_response")
	} else {
		log.Debug().Dur("duration", dur).Int("tool_calls", toolCallCount).Int("thought_summaries", thoughtSummaryCount).Msg("google_stream_ok")
	}

	return nil
}
