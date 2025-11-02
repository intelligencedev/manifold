package retrieve

// RetrieveOptions configures a retrieval operation over hybrid backends.
type RetrieveOptions struct {
    // K is the desired total number of results after fusion/reranking.
    K int
    // FtK is the number of FTS candidates to pull pre-fusion.
    FtK int
    // VecK is the number of vector candidates to pull pre-fusion.
    VecK int
    // Alpha controls weighted fusion between FTS and vector scores (0..1).
    Alpha float64
    // UseRRF toggles Reciprocal Rank Fusion for combining candidate lists.
    UseRRF bool
    // RRFK is the standard RRF constant; when 0, a default is used.
    RRFK int
    // IncludeText requests full chunk text to be included in results.
    IncludeText bool
    // IncludeSnippet requests a highlighted snippet to be generated.
    IncludeSnippet bool
    // Diversify penalizes near-duplicates.
    Diversify bool
    // Rerank toggles an optional cross-encoder reranking stage.
    Rerank bool
    // GraphAugment toggles graph-based neighborhood expansion.
    GraphAugment bool
    // Tenant for multi-tenant isolation.
    Tenant string
    // Filter applies ACL and metadata constraints consistently across stores.
    Filter map[string]string
}

// RetrievedItem represents a fused retrieval hit.
type RetrievedItem struct {
    ID       string
    DocID    string
    Score    float64
    Snippet  string
    Text     string
    // Metadata surface; values should be strings for portability.
    Metadata map[string]string
    // Doc carries lightweight document metadata for citations.
    Doc DocumentMeta
    // Explanation contains per-item provenance such as ranks, fusion components, and boosts.
    Explanation map[string]any
}

// RetrieveResponse contains fused and optionally reranked results.
type RetrieveResponse struct {
    Query string
    Items []RetrievedItem
    // Debug optionally carries diagnostics and per-stage scores for evaluation.
    Debug map[string]any
}

// DocumentMeta is a portable subset of document fields for citation.
type DocumentMeta struct {
    Title string `json:"title,omitempty"`
    URL   string `json:"url,omitempty"`
}

