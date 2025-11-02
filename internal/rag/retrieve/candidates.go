package retrieve

import (
    "context"
    "time"

    "manifold/internal/persistence/databases"
)

// SourceDiagnostics carries per-source retrieval timings and counts.
type SourceDiagnostics struct {
    FtLatency  time.Duration
    VecLatency time.Duration
    FtCount    int
    VecCount   int
}

// ParallelCandidates queries FTS and vector stores in parallel according to the plan.
// It returns the raw candidates from each source and diagnostics.
func ParallelCandidates(ctx context.Context, search databases.FullTextSearch, vector databases.VectorStore, plan QueryPlan, embVec []float32) (fts []databases.SearchResult, vrs []databases.VectorResult, diag SourceDiagnostics, err error) {
    type ftOut struct {
        res []databases.SearchResult
        dur time.Duration
        err error
    }
    type vecOut struct {
        res []databases.VectorResult
        dur time.Duration
        err error
    }

    ftCh := make(chan ftOut, 1)
    vecCh := make(chan vecOut, 1)

    if plan.FtK > 0 && search != nil {
        go func() {
            t0 := time.Now()
            // Prefer chunk-aware search when available.
            type chunkSearcher interface {
                SearchChunks(ctx context.Context, query string, lang string, limit int, filter map[string]string) ([]databases.SearchResult, error)
            }
            var res []databases.SearchResult
            var e error
            if cs, ok := search.(chunkSearcher); ok {
                res, e = cs.SearchChunks(ctx, plan.Query, plan.Lang, plan.FtK, plan.Filters)
            } else {
                res, e = search.Search(ctx, plan.Query, plan.FtK)
            }
            ftCh <- ftOut{res: res, dur: time.Since(t0), err: e}
        }()
    } else {
        ftCh <- ftOut{}
    }

    if plan.VecK > 0 && vector != nil && len(embVec) > 0 {
        go func() {
            t0 := time.Now()
            res, e := vector.SimilaritySearch(ctx, embVec, plan.VecK, plan.Filters)
            vecCh <- vecOut{res: res, dur: time.Since(t0), err: e}
        }()
    } else {
        vecCh <- vecOut{}
    }

    fto := <-ftCh
    vco := <-vecCh

    if fto.err != nil {
        return nil, nil, SourceDiagnostics{}, fto.err
    }
    if vco.err != nil {
        return nil, nil, SourceDiagnostics{}, vco.err
    }
    diag = SourceDiagnostics{FtLatency: fto.dur, VecLatency: vco.dur, FtCount: len(fto.res), VecCount: len(vco.res)}
    return fto.res, vco.res, diag, nil
}

