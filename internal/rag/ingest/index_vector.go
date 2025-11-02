package ingest

import (
    "context"
    "strconv"

    "manifold/internal/persistence/databases"
    "manifold/internal/rag/embedder"
)

// UpsertChunkEmbeddings embeds chunk texts and upserts vectors into the vector store.
// It returns the number of upserts performed. Metadata includes doc_id, tenant, lang,
// model, and version.
func UpsertChunkEmbeddings(ctx context.Context, vec databases.VectorStore, emb embedder.Embedder, docID string, lang string, chunks []ChunkRecord, in IngestRequest, version int) (int, error) {
    if vec == nil || emb == nil || len(chunks) == 0 {
        return 0, nil
    }
    texts := make([]string, len(chunks))
    ids := make([]string, len(chunks))
    for i, c := range chunks {
        texts[i] = c.Text
        ids[i] = chunkID(docID, c.Index)
    }
    embs, err := emb.EmbedBatch(ctx, texts)
    if err != nil {
        return 0, err
    }
    // Prepare shared metadata base
    base := map[string]string{
        "type":   "chunk",
        "doc_id": docID,
        "model":  emb.Name(),
    }
    if in.Tenant != "" { base["tenant"] = in.Tenant }
    if lang != "" { base["lang"] = lang }
    if version > 0 { base["version"] = strconv.Itoa(version) }
    // Upsert sequentially (simple path); can be batched later if backend supports
    upserts := 0
    for i, id := range ids {
        md := copyMap(base)
        if in.Source != "" { md["source"] = in.Source }
        if in.URL != "" { md["url"] = in.URL }
        if err := vec.Upsert(ctx, id, embs[i], md); err != nil {
            return upserts, err
        }
        upserts++
    }
    return upserts, nil
}

func chunkID(docID string, idx int) string { return "chunk:" + docID + ":" + strconv.Itoa(idx) }

// copyMap shallow-copies a string map.
func copyMap(m map[string]string) map[string]string {
    out := make(map[string]string, len(m))
    for k, v := range m { out[k] = v }
    return out
}

