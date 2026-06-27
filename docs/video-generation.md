# Video Generation

Manifold supports text-to-video generation through the specialist media-generation pipeline. Video generation reuses the same specialist configuration and chat execution path as image generation — no separate tool surface is needed.

> **Prerequisites**
>
> - Manifold is running with database access.
> - The web UI is accessible at `http://localhost:32180`.
> - You have an API key for a video-capable provider (e.g., OpenRouter with access to Seedance or similar text-to-video models).

## How It Works

Video generation follows the same architecture as image generation, with a different provider-level execution path:

1. **Specialist configuration** — A specialist is configured with `videoGeneration: true` in its config. This tells Manifold to route chat requests through the video generation path instead of standard chat completions.
2. **Context injection** — When a chat request targets a video-generation specialist, `applyBuildImagePrompt` detects `build.VideoGeneration` and calls `llm.WithVideoPrompt(ctx, ...)` to annotate the request context.
3. **Provider routing** — The OpenAI provider client checks for the video prompt context value in `Chat()` and `ChatStream()`. When present, it routes to `chatWithVideoGeneration()` instead of the standard chat completion path.
4. **Async submit-and-poll** — The provider submits the video generation request to the provider's video API endpoint, then polls until the job reaches a terminal state.
5. **Result delivery** — The generated video is returned via `llm.Message.Videos` (non-streaming) or streamed via `llm.StreamHandler.OnVideo()` (streaming). The video is also saved to disk and served via the project file API.

### Provider Routing Flow

```
Chat request
  → applyBuildImagePrompt(ctx, build)
    → build.VideoGeneration == true
      → llm.WithVideoPrompt(ctx, VideoPromptOptions{})
  → OpenAI Client.Chat(ctx, ...)
    → llm.VideoPromptFromContext(ctx) present?
      → chatWithVideoGeneration(ctx, msgs, model)
        → submitAndPollVideo(ctx, model, prompt)
          → POST {baseURL}/videos  (submit job)
          → GET  {polling_url}     (poll until complete)
          → download video from result URL
        → return llm.Message{Videos: [GeneratedVideo{...}]}
```

### Streaming Path

For streaming chat, the flow is identical, but results are delivered through the stream handler:

```
ChatStream request
  → llm.VideoPromptFromContext(ctx) present?
    → streamVideoChatResult(ctx, msgs, model, handler)
      → chatWithVideoGeneration(ctx, msgs, model)
      → for each video in msg.Videos:
          handler.OnVideo(video)
```

The streaming SSE channel emits events with `"type": "video"` containing the video metadata:

```json
{
  "type": "video",
  "name": "generated_video_1719421234567890.mp4",
  "mime": "video/mp4",
  "data_url": "data:video/mp4;base64,...",
  "url": "/api/projects/{projectId}/files?path=videos/generated_video_1719421234567890.mp4",
  "rel_path": "videos/generated_video_1719421234567890.mp4",
  "file_path": "/absolute/path/to/videos/generated_video_1719421234567890.mp4"
}
```

For non-streaming (JSON) responses, videos are included in the response payload under the `"videos"` key.

## Configuring a Video Generation Specialist

To enable video generation, create or update a specialist with the following settings:

| Field | Description | Example |
|---|---|---|
| `videoGeneration` | Must be `true` to route through the video generation path | `true` |
| `baseURL` | The provider's API base URL (must include `/v1` suffix) | `https://openrouter.ai/api/v1` |
| `model` | The video-capable model identifier | `bytedance/seedance-2.0` |
| `apiKey` | API key for the provider | (your OpenRouter key) |

### Polling Configuration (Extra Params)

Video generation is asynchronous. The provider returns a `polling_url` after job submission, and Manifold polls until the job completes. Configure polling behavior via the specialist's extra params:

| Extra Param | Description | Default |
|---|---|---|
| `max_poll_attempts` | Maximum number of polling attempts before timeout | `300` |
| `poll_interval_ms` | Milliseconds to wait between poll requests | `2000` (2 seconds) |

With the defaults (300 attempts × 2s interval), Manifold will poll for up to 10 minutes before timing out. For providers with longer generation times, increase `max_poll_attempts` accordingly.

### Example: Seedance 2 via OpenRouter

This example configures a specialist named `seedance2` for text-to-video generation using ByteDance's Seedance 2.0 model through OpenRouter:

```yaml
# In specialists.yaml or via the UI specialist editor
- name: seedance2
  description: "Text-to-video generation using Seedance 2.0"
  baseURL: "https://openrouter.ai/api/v1"
  model: "bytedance/seedance-2.0"
  videoGeneration: true
  apiKey: "${OPENROUTER_API_KEY}"
  extraParams:
    max_poll_attempts: "180"
    poll_interval_ms: "2000"
```

> **Important:** The `baseURL` must include the `/v1` suffix. Without it, the computed video endpoint URL will be incorrect (e.g., `/api/videos` instead of `/api/v1/videos`), resulting in 404 errors.

You can also configure the specialist via the Manifold API:

```bash
curl -X PUT http://localhost:32180/api/v1/specialists/{id} \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "seedance2",
    "description": "Text-to-video generation using Seedance 2.0",
    "baseURL": "https://openrouter.ai/api/v1",
    "model": "bytedance/seedance-2.0",
    "videoGeneration": true,
    "extraParams": {
      "max_poll_attempts": "180",
      "poll_interval_ms": "2000"
    }
  }'
```

## Generating a Video

Once the specialist is configured, generate a video by sending a chat message to the specialist:

### Via the UI

1. Open the chat interface.
2. Select the video-generation specialist (e.g., `seedance2`) from the specialist dropdown.
3. Enter your text prompt describing the video you want.
4. Send the message. The specialist will process the request — this may take several minutes as the video is generated asynchronously.
5. The generated video appears in the chat as a video attachment. It is also saved to the project's `videos/` directory.

### Via the API (Streaming)

```bash
curl -N http://localhost:32180/api/v1/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "specialist": "seedance2",
    "session": "my-video-session",
    "prompt": "A serene mountain landscape at sunrise with mist rolling through the valleys"
  }'
```

The SSE stream will emit:
1. `delta` events with any text content from the specialist.
2. `video` events with the generated video metadata and download URL.
3. A `final` event marking the end of the stream.

### Via the API (JSON)

```bash
curl http://localhost:32180/api/v1/chat/json \
  -H 'Content-Type: application/json' \
  -d '{
    "specialist": "seedance2",
    "session": "my-video-session",
    "prompt": "A serene mountain landscape at sunrise with mist rolling through the valleys"
  }'
```

The JSON response includes a `videos` array:

```json
{
  "result": "Generated video\n\nGenerated videos:\n- /api/projects/{projectId}/files?path=videos/generated_video_1719421234567890.mp4",
  "videos": [
    {
      "Name": "generated_video_1719421234567890.mp4",
      "MIME": "video/mp4",
      "URL": "/api/projects/{projectId}/files?path=videos/generated_video_1719421234567890.mp4",
      "RelPath": "videos/generated_video_1719421234567890.mp4",
      "FullPath": "/absolute/path/to/videos/generated_video_1719421234567890.mp4"
    }
  ]
}
```

## How the Async Video API Works

Video generation providers (such as OpenRouter with Seedance) use an asynchronous submit-and-poll workflow. Manifold's implementation handles this transparently:

### 1. Submit

Manifold sends a `POST` request to `{baseURL}/videos` with the model and prompt:

```
POST https://openrouter.ai/api/v1/videos
Content-Type: application/json
Authorization: Bearer {apiKey}

{
  "model": "bytedance/seedance-2.0",
  "prompt": "A serene mountain landscape at sunrise..."
}
```

The response contains either:
- A direct video URL (if generation completes synchronously), or
- A `polling_url` for asynchronous polling

If the response includes extra params from the specialist config (except polling-related keys), they are included in the submission payload. The polling keys (`poll_interval_ms`, `max_poll_attempts`) are stripped from the submission payload and used only for polling control.

### 2. Poll

If a `polling_url` is returned, Manifold polls it with `GET` requests at the configured interval:

```
GET {polling_url}
Authorization: Bearer {apiKey}
```

Polling continues until:
- **Success:** The response contains a video URL (checked via `videoResultURL()`)
- **Failure:** The response status is `failed`, `error`, `cancelled`, or `canceled`
- **Timeout:** The maximum number of poll attempts is reached

### 3. Extract Video URL

When the job completes, Manifold extracts the video URL from the response. The `videoResultURL()` function checks multiple possible fields in priority order:

- Top-level: `url`, `video_url`, `videoUrl`, `output_url`, `outputUrl`
- Nested arrays/objects: `unsigned_urls`, `unsignedUrls`, `urls`, `data`, `output`, `result`, `video`

This flexible extraction supports different provider response formats.

### 4. Download

Once the video URL is extracted, Manifold downloads the video content and returns it as a `GeneratedVideo` with:
- `Data` — raw video bytes (when downloadable)
- `MIMEType` — detected from the response `Content-Type` header (defaults to `video/mp4`)
- `URL` — the original remote video URL

The video is saved to the project workspace under `videos/generated_video_{timestamp}.mp4` and served via the project file API.

## Contracts and Types

### `llm.GeneratedVideo`

```go
type GeneratedVideo struct {
    Data     []byte  // raw video bytes when available
    MIMEType string  // e.g., "video/mp4"
    URL      string  // remote URL when the provider hosts the video
}
```

### `llm.Message.Videos`

Video results are attached to the assistant message via the `Videos` field:

```go
type Message struct {
    Role    string
    Content string
    Videos  []GeneratedVideo
    // ... other fields
}
```

### `llm.StreamHandler.OnVideo`

For streaming responses, each generated video is delivered through the stream handler's `OnVideo` method:

```go
type StreamHandler interface {
    OnVideo(video GeneratedVideo)
    // ... other methods
}
```

### Context Injection

Video generation is triggered by annotating the request context with `llm.WithVideoPrompt()`. The provider checks for this context value to route to the video generation path:

```go
// Set by applyBuildImagePrompt when build.VideoGeneration is true
ctx = llm.WithVideoPrompt(ctx, llm.VideoPromptOptions{})

// Checked by the provider in Chat() and ChatStream()
if _, ok := llm.VideoPromptFromContext(ctx); ok {
    return c.chatWithVideoGeneration(ctx, msgs, model)
}
```

## Troubleshooting

- **404 error on video submission**: Ensure the specialist's `baseURL` includes the `/v1` suffix (e.g., `https://openrouter.ai/api/v1`, not `https://openrouter.ai/api`). Without it, the computed endpoint URL will be incorrect.

- **Polling timeout**: If video generation consistently times out, increase `max_poll_attempts` in the specialist's extra params. Video generation can take several minutes depending on the model and provider load.

- **No video in response**: Verify that `videoGeneration: true` is set on the specialist. Without this flag, the chat request routes through the standard text completion path instead of the video generation path.

- **Video URL not found in response**: The `videoResultURL()` function checks many common field names. If your provider uses an unconventional response format, the URL may not be extracted. Check the provider's API documentation for the response structure and verify it matches one of the supported field names.

- **Image generation endpoint called instead of video**: This indicates `applyBuildImagePrompt` is not correctly detecting `build.VideoGeneration`. Ensure the specialist config properly sets `videoGeneration: true` and that the specialist is being used for the chat request.

## Relevant Source Files

| File | Purpose |
|---|---|
| `internal/llm/image_prompt.go` | Context helpers: `WithVideoPrompt`, `VideoPromptFromContext`, `VideoPromptOptions` |
| `internal/llm/provider.go` | Types: `GeneratedVideo`, `Message.Videos`, `StreamHandler.OnVideo` |
| `internal/llm/openai/chat.go` | Provider routing: `Chat()` and `ChatStream()` dispatch to video generation |
| `internal/llm/openai/images.go` | Video generation implementation: `chatWithVideoGeneration`, `submitAndPollVideo`, `videoResultURL`, `downloadGeneratedVideo` |
| `internal/agentd/chat_execution.go` | HTTP chat execution: `applyBuildImagePrompt` injects video prompt context; `chatTurnCollector` saves and streams video results |
| `internal/agentd/images.go` | Video persistence: `saveGeneratedVideos` writes video files to the project workspace |
| `internal/config/config.go` | Specialist config: `VideoGeneration` field |
| `internal/specialists/registry.go` | Specialist registry: propagates `VideoGeneration` to engine build |
