package service

import (
	"context"
	"maps"

	"manifold/internal/embedding"
	"manifold/internal/persistence/databases"
	"manifold/internal/rag/retrieve"
)

func (s *Service) retrieveQueryVector(ctx context.Context, plan retrieve.QueryPlan, opt retrieve.RetrieveOptions) ([]float32, embedding.InstructionResult, bool, error) {
	instruction := embedding.FormatQueryInput(s.embCfg, embedding.UseCaseRAGQuery, plan.Query, opt.Instruction)
	if s.vector == nil || s.emb == nil || plan.VecK <= 0 {
		return nil, instruction, false, nil
	}
	emb, err := s.emb.EmbedBatch(ctx, []string{instruction.Input})
	if err != nil {
		return nil, instruction, true, err
	}
	if len(emb) == 0 {
		return nil, instruction, true, nil
	}
	return emb[0], instruction, true, nil
}

func (s *Service) retrieveCandidates(ctx context.Context, plan retrieve.QueryPlan, qvec []float32) ([]databases.SearchResult, []databases.VectorResult, retrieve.SourceDiagnostics, error) {
	ftRes, vecRes, diag, err := retrieve.ParallelCandidates(ctx, s.search, s.vector, plan, qvec)
	if err != nil {
		return nil, nil, retrieve.SourceDiagnostics{}, err
	}
	s.recordCandidateMetrics(plan.Tenant, diag)
	return ftRes, vecRes, diag, nil
}

func (s *Service) recordCandidateMetrics(tenant string, diag retrieve.SourceDiagnostics) {
	s.metrics.ObserveHistogram("retrieval_stage_ms", float64(ms(diag.FtLatency)), map[string]string{"stage": "fts", "tenant": tenant})
	s.metrics.ObserveHistogram("retrieval_stage_ms", float64(ms(diag.VecLatency)), map[string]string{"stage": "vec", "tenant": tenant})
	for i := 0; i < diag.FtCount; i++ {
		s.metrics.IncCounter("retrieval_candidates", map[string]string{"type": "fts", "tenant": tenant})
	}
	for i := 0; i < diag.VecCount; i++ {
		s.metrics.IncCounter("retrieval_candidates", map[string]string{"type": "vec", "tenant": tenant})
	}
}

func (s *Service) fuseRetrieveItems(ftRes []databases.SearchResult, vecRes []databases.VectorResult, plan retrieve.QueryPlan, opt retrieve.RetrieveOptions) ([]retrieve.RetrievedItem, int64) {
	if opt.UseRRF {
		t0 := s.clock.Now()
		items := retrieve.FuseAndDiversify(ftRes, vecRes, plan, opt)
		fusionMS := ms(s.clock.Now().Sub(t0))
		s.metrics.ObserveHistogram("retrieval_stage_ms", float64(fusionMS), map[string]string{"stage": "fusion", "tenant": plan.Tenant})
		return items, fusionMS
	}
	return concatRetrieveItems(ftRes, vecRes, opt.K), 0
}

func concatRetrieveItems(ftRes []databases.SearchResult, vecRes []databases.VectorResult, k int) []retrieve.RetrievedItem {
	items := make([]retrieve.RetrievedItem, 0, len(ftRes)+len(vecRes))
	for _, r := range ftRes {
		items = append(items, retrieve.RetrievedItem{ID: r.ID, Score: r.Score, Snippet: r.Snippet, Text: r.Text, Metadata: r.Metadata})
	}
	for _, r := range vecRes {
		items = append(items, retrieve.RetrievedItem{ID: r.ID, Score: r.Score, Metadata: r.Metadata})
	}
	if k <= 0 {
		k = 10
	}
	if len(items) > k {
		items = items[:k]
	}
	return items
}

func (s *Service) augmentAndAssembleRetrieve(ctx context.Context, q string, opt retrieve.RetrieveOptions, plan retrieve.QueryPlan, items []retrieve.RetrievedItem) ([]retrieve.RetrievedItem, map[string]any, map[string]any, error) {
	if opt.Rerank && s.rerank != nil {
		items = s.hydrateRerankText(ctx, items)
	}
	items, magmaGraphDbg, err := s.maybeAugmentMagmaGraph(ctx, q, opt, plan, items)
	if err != nil {
		return nil, nil, nil, err
	}
	items, addDbg, err := retrieve.AssembleResults(ctx, s.graph, s.rerank, plan, opt, items)
	if err != nil {
		return nil, nil, nil, err
	}
	s.recordAssembleMetrics(plan.Tenant, addDbg)
	return items, magmaGraphDbg, addDbg, nil
}

func (s *Service) maybeAugmentMagmaGraph(ctx context.Context, q string, opt retrieve.RetrieveOptions, plan retrieve.QueryPlan, items []retrieve.RetrievedItem) ([]retrieve.RetrievedItem, map[string]any, error) {
	if !opt.GraphAugment || opt.Magma.Enabled || s.magma == nil {
		return items, nil, nil
	}
	t0 := s.clock.Now()
	items, magmaGraphDbg, err := s.augmentWithMagmaGraph(ctx, q, opt, items)
	if err != nil {
		return nil, nil, err
	}
	if magmaGraphDbg != nil {
		magmaGraphDbg["ms"] = ms(s.clock.Now().Sub(t0))
		s.metrics.ObserveHistogram("retrieval_stage_ms", float64(magmaGraphDbg["ms"].(int64)), map[string]string{"stage": "magma_graph", "tenant": plan.Tenant})
	}
	return items, magmaGraphDbg, nil
}

func (s *Service) recordAssembleMetrics(tenant string, addDbg map[string]any) {
	if gv, ok := addDbg["graph"]; ok {
		if gmap, ok := gv.(map[string]any); ok {
			if msVal, ok := gmap["ms"].(int64); ok {
				s.metrics.ObserveHistogram("retrieval_stage_ms", float64(msVal), map[string]string{"stage": "graph", "tenant": tenant})
			}
		}
	}
	if rv, ok := addDbg["rerank_ms"].(int64); ok {
		s.metrics.ObserveHistogram("retrieval_stage_ms", float64(rv), map[string]string{"stage": "rerank", "tenant": tenant})
	}
}

func (s *Service) packageRetrieveItems(ctx context.Context, plan retrieve.QueryPlan, opt retrieve.RetrieveOptions, items []retrieve.RetrievedItem) ([]retrieve.RetrievedItem, int64) {
	pkgStart := s.clock.Now()
	if opt.IncludeSnippet {
		items = retrieve.GenerateSnippets(ctx, s.search, items, retrieve.SnippetOptions{Lang: plan.Lang, Query: plan.Query})
	}
	if opt.IncludeText && s.search != nil {
		items = s.fillRetrieveText(ctx, items)
	}
	items = retrieve.AttachDocMetadata(ctx, s.search, items)
	ensureRetrieveExplanations(items)
	pkgMS := ms(s.clock.Now().Sub(pkgStart))
	s.metrics.ObserveHistogram("retrieval_stage_ms", float64(pkgMS), map[string]string{"stage": "package", "tenant": plan.Tenant})
	for range items {
		s.metrics.IncCounter("retrieval_results_total", map[string]string{"tenant": plan.Tenant})
	}
	return items, pkgMS
}

func (s *Service) fillRetrieveText(ctx context.Context, items []retrieve.RetrievedItem) []retrieve.RetrievedItem {
	for i := range items {
		if items[i].Text != "" {
			continue
		}
		if doc, ok, _ := s.search.GetByID(ctx, items[i].ID); ok {
			items[i].Text = doc.Text
		}
	}
	return items
}

func ensureRetrieveExplanations(items []retrieve.RetrievedItem) {
	for i := range items {
		if items[i].Explanation == nil {
			items[i].Explanation = map[string]any{}
		}
		if items[i].DocID == "" {
			items[i].DocID = retrieve.DeriveDocIDPublic(items[i].ID, items[i].Metadata)
		}
	}
}

func retrieveDebug(
	plan retrieve.QueryPlan,
	diag retrieve.SourceDiagnostics,
	pkgMS int64,
	fusionMS int64,
	totalMS int64,
	instruction embedding.InstructionResult,
	instructionUsed bool,
	addDbg map[string]any,
	magmaGraphDbg map[string]any,
) map[string]any {
	debug := map[string]any{
		"plan":        map[string]any{"lang": plan.Lang, "ftK": plan.FtK, "vecK": plan.VecK},
		"diagnostics": map[string]any{"ft_ms": ms(diag.FtLatency), "vec_ms": ms(diag.VecLatency), "ft_n": diag.FtCount, "vec_n": diag.VecCount, "package_ms": pkgMS, "fusion_ms": fusionMS, "total_ms": totalMS},
		"embedding_instruction": map[string]any{
			"used":    instructionUsed,
			"applied": instruction.Applied,
			"useCase": instruction.UseCase,
			"format":  instruction.Format,
			"mode":    instruction.Mode,
			"source":  instruction.Source,
		},
	}
	addRetrieveDiagnostics(debug, addDbg, magmaGraphDbg)
	maps.Copy(debug, addDbg)
	if magmaGraphDbg != nil {
		debug["magma_graph"] = magmaGraphDbg
	}
	return debug
}

func addRetrieveDiagnostics(debug map[string]any, addDbg map[string]any, magmaGraphDbg map[string]any) {
	dm, ok := debug["diagnostics"].(map[string]any)
	if !ok {
		return
	}
	if magmaGraphDbg != nil {
		if msVal, ok := magmaGraphDbg["ms"]; ok {
			dm["magma_graph_ms"] = msVal
		}
	}
	if gv, ok := addDbg["graph"]; ok {
		if gmap, ok := gv.(map[string]any); ok {
			if msVal, ok := gmap["ms"]; ok {
				dm["graph_ms"] = msVal
			}
		}
	}
	if rv, ok := addDbg["rerank_ms"]; ok {
		dm["rerank_ms"] = rv
	}
}
