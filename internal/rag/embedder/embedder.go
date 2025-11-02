package embedder

import (
    "context"
    "hash/fnv"
    "math"
)

// Embedder defines the interface for converting text to embedding vectors.
type Embedder interface {
    // EmbedBatch returns an embedding vector per input text.
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    // Name returns a model identifier string.
    Name() string
    // Dimension returns the embedding dimensionality (0 for variable/unknown).
    Dimension() int
}

// deterministicEmbedder is a lightweight, deterministic embedder suitable for tests.
// It hashes byte 3-grams into a fixed-size vector and optionally L2-normalizes.
type deterministicEmbedder struct {
    dim       int
    normalize bool
    seed      uint64
    name      string
}

// NewDeterministic constructs a deterministic embedder with the given dimension.
// If normalize is true, vectors are L2-normalized. Seed perturbs hashing.
func NewDeterministic(dim int, normalize bool, seed uint64) Embedder {
    if dim <= 0 {
        dim = 64
    }
    return &deterministicEmbedder{dim: dim, normalize: normalize, seed: seed, name: "deterministic"}
}

func (d *deterministicEmbedder) Name() string      { return d.name }
func (d *deterministicEmbedder) Dimension() int    { return d.dim }

func (d *deterministicEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
    out := make([][]float32, len(texts))
    for i, t := range texts {
        out[i] = d.embedOne(t)
    }
    return out, nil
}

func (d *deterministicEmbedder) embedOne(s string) []float32 {
    v := make([]float32, d.dim)
    if len(s) == 0 {
        return v
    }
    // 3-gram hashing over bytes
    b := []byte(s)
    if len(b) < 3 {
        add(d.seed, b, v)
    } else {
        for i := 0; i <= len(b)-3; i++ {
            add(d.seed, b[i:i+3], v)
        }
    }
    if d.normalize {
        var sum float64
        for _, x := range v {
            sum += float64(x) * float64(x)
        }
        if sum > 0 {
            inv := float32(1.0 / math.Sqrt(sum))
            for i := range v {
                v[i] *= inv
            }
        }
    }
    return v
}

func add(seed uint64, gram []byte, v []float32) {
    h := fnv.New64a()
    if seed != 0 {
        var tmp [8]byte
        for i := 0; i < 8; i++ { tmp[i] = byte(seed >> (8 * i)) }
        _, _ = h.Write(tmp[:])
    }
    _, _ = h.Write(gram)
    hv := h.Sum64()
    idx := int(hv % uint64(len(v)))
    // map hash to a signed weight in [-1, 1]
    w := float32(int32(hv>>32)) / float32(1<<31)
    v[idx] += w
}

