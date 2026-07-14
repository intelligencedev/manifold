package agent

import (
	"context"
	"reflect"
	"testing"
)

func TestPrepareRunMessagesIsSharedAcrossStreamModes(t *testing.T) {
	t.Parallel()

	eng := &Engine{
		System:            "system",
		UserPromptContext: "runtime context",
		SummaryEnabled:    false,
	}

	nonStream, err := eng.prepareRunMessages(context.Background(), "hello", nil, false)
	if err != nil {
		t.Fatalf("prepareRunMessages(non-stream): %v", err)
	}
	stream, err := eng.prepareRunMessages(context.Background(), "hello", nil, true)
	if err != nil {
		t.Fatalf("prepareRunMessages(stream): %v", err)
	}
	if !reflect.DeepEqual(nonStream, stream) {
		t.Fatalf("prepared messages differ by stream mode:\nnon-stream=%#v\nstream=%#v", nonStream, stream)
	}
}
