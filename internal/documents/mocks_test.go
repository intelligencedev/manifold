package documents

import (
	"bytes"
	"context"
	"testing"
)

func TestPipeline(t *testing.T) {
	store := newMockStore()
	opt := Options{Splitter: Splitter{MaxTokens: 3}, MaxWorkers: 1, BatchSize: 2, Storage: store}
	err := Ingest(context.Background(), bytes.NewBufferString("one two three four"), opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.data) == 0 {
		t.Fatalf("no data stored")
	}
}
