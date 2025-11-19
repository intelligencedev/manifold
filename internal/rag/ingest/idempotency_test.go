package ingest

import (
	"context"
	"testing"
)

type mockLookup struct {
	id  string
	ver int
	ok  bool
	err error
}

func (m mockLookup) LookupByHash(ctx context.Context, hash string, tenant string) (string, int, bool, error) {
	return m.id, m.ver, m.ok, m.err
}

func TestResolveIdempotency_SkipOverwriteNewVersion(t *testing.T) {
	req := IngestRequest{ID: "doc:ns:1", Options: IngestOptions{ReingestPolicy: ReingestSkipIfUnchanged}}
	dec, err := ResolveIdempotency(context.Background(), mockLookup{"doc:ns:1", 2, true, nil}, "t1", req, PreprocessedDoc{Hash: "h"})
	if err != nil || dec.Action != "skip" || dec.Version != 2 {
		t.Fatalf("skip_if_unchanged wrong: %+v err=%v", dec, err)
	}

	req.Options.ReingestPolicy = ReingestOverwrite
	dec, err = ResolveIdempotency(context.Background(), mockLookup{"doc:ns:1", 3, true, nil}, "t1", req, PreprocessedDoc{Hash: "h"})
	if err != nil || dec.Action != "overwrite" || dec.Version != 3 {
		t.Fatalf("overwrite wrong: %+v err=%v", dec, err)
	}

	req.Options.ReingestPolicy = ReingestNewVersion
	dec, err = ResolveIdempotency(context.Background(), mockLookup{"doc:ns:1", 5, true, nil}, "t1", req, PreprocessedDoc{Hash: "h"})
	if err != nil || dec.Action != "new_version" || dec.Version != 6 {
		t.Fatalf("new_version wrong: %+v err=%v", dec, err)
	}
}
