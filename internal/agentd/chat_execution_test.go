package agentd

import (
	"context"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/sandbox"
)

func TestBuildChatJSONPayloadIncludesMatrixMessages(t *testing.T) {
	t.Parallel()

	outbox := sandbox.NewMatrixOutbox()
	outbox.Add("room-1", "hello")
	ctx := sandbox.WithMatrixOutbox(context.Background(), outbox)

	payload := buildChatJSONPayload("done", nil, ctx, true)

	if payload["result"] != "done" {
		t.Fatalf("expected result payload, got %#v", payload["result"])
	}
	messages, ok := payload["matrix_messages"].([]sandbox.MatrixMessage)
	if !ok {
		t.Fatalf("expected matrix messages in payload, got %#v", payload["matrix_messages"])
	}
	if len(messages) != 1 || messages[0].RoomID != "room-1" || messages[0].Text != "hello" {
		t.Fatalf("unexpected matrix messages: %#v", messages)
	}
}

func TestBuildChatJSONPayloadIncludesImages(t *testing.T) {
	t.Parallel()

	images := []savedImage{{Name: "image-1.png", MIME: "image/png", URL: "/api/projects/p/files?path=images%2Fimage-1.png"}}
	payload := buildChatJSONPayload("done", images, context.Background(), false)

	got, ok := payload["images"].([]savedImage)
	if !ok {
		t.Fatalf("expected images in payload, got %#v", payload["images"])
	}
	if len(got) != 1 || got[0].Name != "image-1.png" || got[0].MIME != "image/png" {
		t.Fatalf("unexpected images payload: %#v", got)
	}
}

func TestBuildChatStreamFinalPayloadOmitsMatrixMessagesWhenDisabled(t *testing.T) {
	t.Parallel()

	outbox := sandbox.NewMatrixOutbox()
	outbox.Add("room-1", "hello")
	ctx := sandbox.WithMatrixOutbox(context.Background(), outbox)

	payload := buildChatStreamFinalPayload("done", ctx, false)

	if payload["type"] != "final" || payload["data"] != "done" {
		t.Fatalf("unexpected stream payload: %#v", payload)
	}
	if _, ok := payload["matrix_messages"]; ok {
		t.Fatalf("expected matrix messages to be omitted: %#v", payload)
	}
}

func TestBuildChatStreamFinalPayloadIncludesDuration(t *testing.T) {
	t.Parallel()

	payload := buildChatStreamFinalPayload("done", context.Background(), false, 12437)

	if payload["durationMs"] != int64(12437) {
		t.Fatalf("expected durationMs in payload, got %#v", payload)
	}
}

func TestChatTurnCollectorResultTextAppendsImageSummary(t *testing.T) {
	t.Parallel()

	collector := &chatTurnCollector{
		savedImages: []savedImage{{Name: "image-1", URL: "/audio/image-1"}},
	}

	result := collector.resultText("base output")
	if result == "base output" {
		t.Fatalf("expected image summary to be appended")
	}
	if want := "Generated images:"; !contains(result, want) {
		t.Fatalf("expected %q in result %q", want, result)
	}
	if want := "/audio/image-1"; !contains(result, want) {
		t.Fatalf("expected %q in result %q", want, result)
	}
}

func TestApplyChatImagePromptPrefersInheritedContext(t *testing.T) {
	t.Parallel()

	runCtx := llm.WithImagePrompt(context.Background(), llm.ImagePromptOptions{Size: "2K"})
	ctx := applyChatImagePrompt(context.Background(), runCtx, chatRunRequest{Image: true, ImageSize: "1K"}, true)

	opts, ok := llm.ImagePromptFromContext(ctx)
	if !ok {
		t.Fatal("expected image prompt options in context")
	}
	if opts.Size != "2K" {
		t.Fatalf("expected inherited image size, got %q", opts.Size)
	}
}

func TestChatStoreModelPrefersOverride(t *testing.T) {
	t.Parallel()

	if got := chatStoreModel(nil, "team:gpt-4.1"); got != "team:gpt-4.1" {
		t.Fatalf("expected override model label, got %q", got)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})())
}
