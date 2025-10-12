package textsplitters

import (
    "math"
    "strings"
)

// SemanticConfig configures semantic/lexical segmentation.
type SemanticConfig struct {
    // Window number of sentences for local similarity context (>=1)
    Window int
    // Threshold below which we consider a boundary (0..1). Lower -> more splits.
    Threshold float64
    // After determining boundaries, group by BoundaryConfig (e.g., sentences with target size)
    Within BoundaryConfig
}

// simple sentence embedding: bag of lowercased words hashed into a map; cosine on counts
func sentVec(s string) map[string]float64 {
    m := map[string]float64{}
    for _, w := range strings.Fields(strings.ToLower(s)) {
        if w == "" {
            continue
        }
        m[w] += 1
    }
    return m
}

func cosine(a, b map[string]float64) float64 {
    if len(a) == 0 || len(b) == 0 {
        return 0
    }
    var dot, na, nb float64
    for k, va := range a {
        na += va * va
        if vb, ok := b[k]; ok {
            dot += va * vb
        }
    }
    for _, vb := range b {
        nb += vb * vb
    }
    if na == 0 || nb == 0 {
        return 0
    }
    return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

type semanticSplitter struct{ cfg SemanticConfig }

func newSemanticSplitter(cfg SemanticConfig) (Splitter, error) { return &semanticSplitter{cfg: cfg}, nil }

func (s *semanticSplitter) Split(text string) []string {
    ss := sentencesOf(text)
    if len(ss) == 0 {
        return nil
    }
    w := s.cfg.Window
    if w <= 0 {
        w = 1
    }
    thr := s.cfg.Threshold
    if thr <= 0 {
        thr = 0.15
    }
    // compute pairwise similarity between adjacent sentences using simple cos
    vecs := make([]map[string]float64, len(ss))
    for i, s := range ss {
        vecs[i] = sentVec(s)
    }
    // boundaries: where similarity dips below threshold
    var segments []string
    var cur []string
    sim := func(i int, j int) float64 { return cosine(vecs[i], vecs[j]) }
    push := func() {
        if len(cur) == 0 {
            return
        }
        segments = append(segments, strings.Join(cur, " "))
        cur = cur[:0]
    }
    cur = append(cur, ss[0])
    for i := 1; i < len(ss); i++ {
        // measure similarity to previous w sentences (average)
        start := i - w
        if start < 0 {
            start = 0
        }
        var total float64
        var cnt int
        for k := start; k < i; k++ {
            total += sim(k, i)
            cnt++
        }
        avg := 0.0
        if cnt > 0 {
            avg = total / float64(cnt)
        }
        if avg < thr {
            push()
        }
        cur = append(cur, ss[i])
    }
    push()

    // Now group segments using target size
    if s.cfg.Within.Size > 0 {
        return groupByTarget(segments, s.cfg.Within)
    }
    return segments
}

// TextTiling simplified: create similarity scores over fixed-size blocks of sentences
type TextTilingConfig struct {
    BlockSize int     // number of sentences per block
    Threshold float64 // depth threshold
    Within    BoundaryConfig
}

type textTilingSplitter struct{ cfg TextTilingConfig }

func newTextTilingSplitter(cfg TextTilingConfig) (Splitter, error) { return &textTilingSplitter{cfg: cfg}, nil }

func (t *textTilingSplitter) Split(text string) []string {
    ss := sentencesOf(text)
    if len(ss) == 0 {
        return nil
    }
    b := t.cfg.BlockSize
    if b <= 0 {
        b = 3
    }
    // compute block similarities between adjacent blocks
    // block i: sentences [i*b:(i+1)*b)
    blocks := [][]string{}
    for i := 0; i < len(ss); i += b {
        j := i + b
        if j > len(ss) {
            j = len(ss)
        }
        blocks = append(blocks, ss[i:j])
    }
    if len(blocks) == 1 {
        if t.cfg.Within.Size > 0 {
            return groupByTarget([]string{strings.Join(ss, " ")}, t.cfg.Within)
        }
        return []string{strings.Join(ss, " ")}
    }
    sims := make([]float64, len(blocks)-1)
    bvec := func(bl []string) map[string]float64 {
        m := map[string]float64{}
        for _, s := range bl {
            for k, v := range sentVec(s) {
                m[k] += v
            }
        }
        return m
    }
    bvs := make([]map[string]float64, len(blocks))
    for i := range blocks {
        bvs[i] = bvec(blocks[i])
    }
    for i := 0; i < len(blocks)-1; i++ {
        sims[i] = cosine(bvs[i], bvs[i+1])
    }
    thr := t.cfg.Threshold
    if thr <= 0 {
        thr = 0.2
    }
    // split where similarity < thr
    var segments []string
    var cur []string
    addBlock := func(i int) {
        cur = append(cur, strings.Join(blocks[i], " "))
    }
    addBlock(0)
    for i := 1; i < len(blocks); i++ {
        if sims[i-1] < thr {
            segments = append(segments, strings.Join(cur, " "))
            cur = cur[:0]
        }
        addBlock(i)
    }
    if len(cur) > 0 {
        segments = append(segments, strings.Join(cur, " "))
    }
    if t.cfg.Within.Size > 0 {
        return groupByTarget(segments, t.cfg.Within)
    }
    return segments
}

