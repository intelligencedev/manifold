package documents

import (
	"context"
	"io"
	"sync"
)

// Options configure the ingestion pipeline.
type Options struct {
	Splitter    Splitter
	MaxWorkers  int
	BatchSize   int
	Storage     VectorStore
	EmbedFn     func(string) []float32
	SummariseFn func(string) string
	Logger      func(string)
}

// Ingest processes text from r and stores chunks using opt.Storage.
func Ingest(ctx context.Context, r io.Reader, opt Options) error {
	if opt.MaxWorkers <= 0 {
		opt.MaxWorkers = 1
	}
	if opt.BatchSize <= 0 {
		opt.BatchSize = 1
	}
	if opt.EmbedFn == nil {
		opt.EmbedFn = func(s string) []float32 { return []float32{float32(len(s))} }
	}
	if opt.SummariseFn == nil {
		opt.SummariseFn = func(string) string { return "" }
	}
	if opt.Logger == nil {
		opt.Logger = func(string) {}
	}

	jobs := make(chan Chunk)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		opt.Splitter.Stream(r, func(c Chunk) error {
			jobs <- c
			return nil
		})
		close(jobs)
	}()

	for i := 0; i < opt.MaxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch := make([]PersistReq, 0, opt.BatchSize)
			for c := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				emb := opt.EmbedFn(c.Text)
				c.Text = opt.SummariseFn(c.Text)
				batch = append(batch, PersistReq{Chunk: c, Embedding: emb})
				if len(batch) >= opt.BatchSize {
					opt.Storage.UpsertChunks(ctx, batch)
					batch = batch[:0]
				}
			}
			if len(batch) > 0 {
				opt.Storage.UpsertChunks(ctx, batch)
			}
		}()
	}

	wg.Wait()
	return ctx.Err()
}
