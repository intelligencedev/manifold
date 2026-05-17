package inputrequest

import (
	"context"
	"testing"
)

type testRequester struct{}

func (testRequester) RequestInfo(ctx context.Context, req Request) (Response, error) {
	return Response{RequestID: req.ID}, nil
}

func TestRequesterContextRoundTrip(t *testing.T) {
	t.Parallel()

	requester := testRequester{}
	ctx := WithRequester(context.Background(), requester)

	got := RequesterFromContext(ctx)
	if got == nil {
		t.Fatal("expected requester from context")
	}
}

func TestRequesterContextIgnoresNil(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if WithRequester(ctx, nil) != ctx {
		t.Fatal("expected nil requester to leave context unchanged")
	}
	if RequesterFromContext(nil) != nil {
		t.Fatal("expected nil context to return no requester")
	}
}

func TestRunMetadataContextRoundTrip(t *testing.T) {
	t.Parallel()

	want := RunMetadata{
		Agent:        "planner",
		Model:        "gpt-test",
		CallID:       "call-1",
		ParentCallID: "parent-1",
		Depth:        2,
	}
	ctx := WithRunMetadata(context.Background(), want)

	if got := RunMetadataFromContext(ctx); got != want {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if got := RunMetadataFromContext(nil); got != (RunMetadata{}) {
		t.Fatalf("expected empty metadata for nil context, got %+v", got)
	}
}
