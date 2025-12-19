# Image Generation (OpenAI & Google)

This project supports first‑class image generation for both OpenAI and Google providers. The two providers have different models, defaults, and request shapes—use the sections below to configure and trigger generation correctly.

## OpenAI (gpt-image-1.5)

**Model**: Set the orchestrator/specialist model to `gpt-image-1.5` (or another image-capable OpenAI model).  
**API surface**: Even if the provider is configured with `api: responses`, image runs are executed via the OpenAI Images API under the hood.  
**How to trigger**:
- In the UI: toggle **Request image output** before sending your prompt.  
- Via HTTP: include `"image": true` (optional `"image_size"` is currently ignored by the API) in `/agent/run` or `/api/prompt` requests.  
- In code: wrap the request context with `llm.WithImagePrompt(...)` so the provider routes to image generation.

**Behavior**:
- The prompt text is sent to the Images API; the response includes base64 image data that is decoded and surfaced as attachments.
- Assistant text is a short summary (“Generated image”), while the UI shows thumbnails and emits `image` SSE events when streaming.

## Google (gemini-3-pro-image-preview)

**Model**: Set the orchestrator/specialist model to `gemini-3-pro-image-preview` (or another Gemini image-capable preview model).  
**API surface**: Uses Google GenAI `GenerateContent` with `ResponseModalities: ["IMAGE","TEXT"]` and `ImageConfig.ImageSize` (default `1K` unless overridden).  
**How to trigger**:
- In the UI: toggle **Request image output** before sending your prompt.  
- Via HTTP: include `"image": true` (and optionally `"image_size"` like `"1K"`) in `/agent/run` or `/api/prompt`.  
- In code: use `llm.WithImagePrompt(...)` so the Google client requests image + text modalities.

**Behavior**:
- The model returns inline `inlineData` parts containing image bytes; these are saved under the current project path (e.g., `images/generated_image_*.png`) and exposed as attachments plus `image` SSE events.
- Assistant text is the textual part of the response; saved paths/URLs are appended for reference.

## Notes & Tips
- Ensure your API keys are valid and the selected model supports image generation; otherwise requests will fail.
- The UI and streaming SSE emit `type: "image"` events; attachments in chat carry thumbnails and saved paths.
- For project-scoped runs, images are saved under the project base directory; make sure a project is selected.  
