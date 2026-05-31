package belief

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// EvidenceSource produces auxiliary belief-shaped search results from sources
// outside the belief store (for example RAG retrieval). Implementations must
// enforce tenant/project/objective filters at the storage layer before
// returning results so the blended router never leaks cross-tenant evidence.
type EvidenceSource interface {
	Retrieve(ctx context.Context, request RetrievalRequest) ([]SearchResult, error)
}

// RAGRetrieveFunc is a narrow callback invoked by RAGEvidenceSource. It is
// modelled after rag/service.Retrieve but kept dependency-free so tests can
// substitute deterministic fakes.
//
// The callback MUST honor the tenant/filter values passed in to maintain the
// security boundary required by the shared belief memory plan
// (docs/shared_belief_memory_implementation_plan.md §13).
type RAGRetrieveFunc func(ctx context.Context, query, tenant string, filter map[string]string, k int) ([]RAGEvidenceItem, error)

// RAGEvidenceItem is the source-agnostic shape returned by the retrieve
// callback. It carries only the fields needed to render and rank a piece of
// retrieved evidence inside the belief router.
type RAGEvidenceItem struct {
	ID      string
	DocID   string
	Score   float64
	Title   string
	URL     string
	Snippet string
	Text    string
}

// RAGEvidenceSource adapts a RAG retrieval callback into the EvidenceSource
// contract used by BlendedRetriever.
type RAGEvidenceSource struct {
	Retriever RAGRetrieveFunc
	K         int
	MinScore  float64
	MaxItems  int
}

// Retrieve fulfils EvidenceSource. Returns nil when the source is unconfigured
// or the request lacks the query needed to scope a useful RAG query.
func (r RAGEvidenceSource) Retrieve(ctx context.Context, request RetrievalRequest) ([]SearchResult, error) {
	if r.Retriever == nil {
		return nil, nil
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, nil
	}
	k := r.K
	if k <= 0 {
		k = 8
	}
	tenant := tenantToken(request.TenantID)
	filter := buildRAGFilter(request)
	items, err := r.Retriever(ctx, query, tenant, filter, k)
	if err != nil {
		return nil, err
	}
	maxItems := r.MaxItems
	if maxItems <= 0 {
		maxItems = k
	}
	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		if r.MinScore > 0 && item.Score < r.MinScore {
			continue
		}
		statement := strings.TrimSpace(item.Snippet)
		if statement == "" {
			statement = strings.TrimSpace(item.Text)
		}
		if statement == "" && strings.TrimSpace(item.Title) != "" {
			statement = strings.TrimSpace(item.Title)
		}
		if statement == "" {
			continue
		}
		now := time.Now().UTC()
		metadata := map[string]any{
			"source":      "rag",
			"source_kind": string(SourceKindRAGChunk),
			"source_id":   item.ID,
			"doc_id":      item.DocID,
			"rag_score":   item.Score,
		}
		if item.Title != "" {
			metadata["title"] = item.Title
		}
		if item.URL != "" {
			metadata["url"] = item.URL
		}
		results = append(results, SearchResult{
			Belief: Belief{
				ID:         "rag:" + item.ID,
				TenantID:   request.TenantID,
				Statement:  statement,
				Confidence: clamp01(item.Score),
				Status:     BeliefStatusActive,
				UpdatedAt:  now,
				Metadata:   metadata,
			},
			Score:  item.Score,
			Reason: "rag_evidence doc=" + item.DocID,
		})
		if len(results) >= maxItems {
			break
		}
	}
	return results, nil
}

// BlendedRetriever composes a primary belief Retriever with one or more
// auxiliary EvidenceSource implementations (for example RAG). Beliefs always
// outrank evidence per the precedence model in
// docs/shared_belief_memory_implementation_plan.md §12. RAG items are appended
// after belief results, capped by MaxEvidence, and attributed via metadata so
// downstream prompt rendering can delimit untrusted evidence.
type BlendedRetriever struct {
	Inner       Retriever
	Evidence    []EvidenceSource
	MaxEvidence int
}

// NewBlendedRetriever builds a BlendedRetriever, falling back to inner when no
// active evidence sources are configured.
func NewBlendedRetriever(inner Retriever, maxEvidence int, sources ...EvidenceSource) Retriever {
	if inner == nil {
		inner = NoopRetriever{}
	}
	active := make([]EvidenceSource, 0, len(sources))
	for _, src := range sources {
		if src == nil {
			continue
		}
		active = append(active, src)
	}
	if len(active) == 0 {
		return inner
	}
	if maxEvidence <= 0 {
		maxEvidence = 3
	}
	return BlendedRetriever{Inner: inner, Evidence: active, MaxEvidence: maxEvidence}
}

// Retrieve gathers belief results first, then appends evidence items capped by
// MaxEvidence. Belief results retain their relative ranking; evidence items are
// appended in the order returned by sources and de-duplicated by source id.
func (b BlendedRetriever) Retrieve(ctx context.Context, request RetrievalRequest) ([]SearchResult, error) {
	inner := b.Inner
	if inner == nil {
		inner = NoopRetriever{}
	}
	beliefs, err := inner.Retrieve(ctx, request)
	if err != nil {
		return beliefs, err
	}
	if len(b.Evidence) == 0 || b.MaxEvidence <= 0 {
		return beliefs, nil
	}
	annotateSource(beliefs, "belief")
	combined := make([]SearchResult, 0, len(beliefs)+b.MaxEvidence)
	combined = append(combined, beliefs...)
	seen := make(map[string]struct{}, b.MaxEvidence)
	added := 0
	for _, source := range b.Evidence {
		if added >= b.MaxEvidence {
			break
		}
		items, srcErr := source.Retrieve(ctx, request)
		if srcErr != nil {
			// Evidence is supplemental — never fail the primary belief
			// retrieval because of an auxiliary source.
			continue
		}
		for _, item := range items {
			if added >= b.MaxEvidence {
				break
			}
			key := item.Belief.ID
			if key == "" {
				if md, ok := item.Belief.Metadata["source_id"].(string); ok {
					key = md
				}
			}
			if key != "" {
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
			}
			ensureMetadata(&item)
			if _, ok := item.Belief.Metadata["source"]; !ok {
				item.Belief.Metadata["source"] = "rag"
			}
			combined = append(combined, item)
			added++
		}
	}
	return combined, nil
}

func annotateSource(results []SearchResult, label string) {
	for i := range results {
		ensureMetadata(&results[i])
		if _, ok := results[i].Belief.Metadata["source"]; !ok {
			results[i].Belief.Metadata["source"] = label
		}
	}
}

func ensureMetadata(result *SearchResult) {
	if result.Belief.Metadata == nil {
		result.Belief.Metadata = map[string]any{}
	}
}

func tenantToken(tenantID int64) string {
	return formatInt(tenantID)
}

func buildRAGFilter(request RetrievalRequest) map[string]string {
	filter := map[string]string{"tenant_id": formatInt(request.TenantID)}
	if pid := strings.TrimSpace(request.ProjectID); pid != "" {
		filter["project_id"] = pid
	}
	if oid := strings.TrimSpace(request.ObjectiveID); oid != "" {
		filter["objective_id"] = oid
	}
	if uid := request.UserID; uid != 0 {
		filter["user_id"] = formatInt(uid)
	}
	return filter
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func formatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}
