package databases

import (
    "context"
    "math"
    "sort"
    "sync"
)

type memoryVector struct {
    mu      sync.RWMutex
    vectors map[string]vec
}

type vec struct {
    v        []float32
    metadata map[string]string
}

func NewMemoryVector() VectorStore { return &memoryVector{vectors: make(map[string]vec)} }

func (m *memoryVector) Upsert(_ context.Context, id string, vector []float32, metadata map[string]string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    cp := make([]float32, len(vector))
    copy(cp, vector)
    md := copyMap(metadata)
    m.vectors[id] = vec{v: cp, metadata: md}
    return nil
}

func (m *memoryVector) Delete(_ context.Context, id string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    delete(m.vectors, id)
    return nil
}

func (m *memoryVector) SimilaritySearch(_ context.Context, vector []float32, k int, filter map[string]string) ([]VectorResult, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    if k <= 0 {
        k = 10
    }
    // Precompute norm for cosine similarity
    qnorm := norm(vector)
    scores := make([]VectorResult, 0, len(m.vectors))
    for id, v := range m.vectors {
        if !matchesFilter(v.metadata, filter) {
            continue
        }
        s := cosine(vector, v.v, qnorm)
        scores = append(scores, VectorResult{ID: id, Score: s, Metadata: copyMap(v.metadata)})
    }
    sort.Slice(scores, func(i, j int) bool { return scores[i].Score > scores[j].Score })
    if len(scores) > k {
        scores = scores[:k]
    }
    return scores, nil
}

func matchesFilter(md map[string]string, f map[string]string) bool {
    if len(f) == 0 {
        return true
    }
    for k, v := range f {
        if md[k] != v {
            return false
        }
    }
    return true
}

func norm(a []float32) float64 {
    var s float64
    for _, x := range a {
        s += float64(x) * float64(x)
    }
    return math.Sqrt(s)
}

func dot(a, b []float32) float64 {
    n := len(a)
    if len(b) < n {
        n = len(b)
    }
    var s float64
    for i := 0; i < n; i++ {
        s += float64(a[i]) * float64(b[i])
    }
    return s
}

func cosine(a, b []float32, anorm float64) float64 {
    if anorm == 0 {
        anorm = norm(a)
    }
    bnorm := norm(b)
    if anorm == 0 || bnorm == 0 {
        return 0
    }
    return dot(a, b) / (anorm * bnorm)
}
