package retrieve

import (
    "context"

    "manifold/internal/persistence/databases"
)

// AttachDocMetadata fills per-item DocID and DocumentMeta from the documents store
// when present in metadata. It uses the existing FullTextSearch GetByID to fetch
// the doc row and copies title/url fields from metadata if available.
func AttachDocMetadata(ctx context.Context, search databases.FullTextSearch, items []RetrievedItem) []RetrievedItem {
    for i := range items {
        // DocID may be derivable from the chunk ID and metadata
        items[i].DocID = deriveDocID(items[i].ID, items[i].Metadata)
        // Populate doc meta from available metadata aready on the chunk
        if items[i].Metadata != nil {
            if t, ok := items[i].Metadata["title"]; ok { items[i].Doc.Title = t }
            if u, ok := items[i].Metadata["url"]; ok { items[i].Doc.URL = u }
        }
        // If still empty, try to load the doc record
        if search != nil && (items[i].Doc.Title == "" && items[i].Doc.URL == "") {
            // If we have a separate doc_id different from chunk id, prefer that
            docID := items[i].DocID
            if docID != "" {
                if doc, ok, _ := search.GetByID(ctx, docID); ok {
                    if doc.Metadata != nil {
                        if t, ok := doc.Metadata["title"]; ok { items[i].Doc.Title = t }
                        if u, ok := doc.Metadata["url"]; ok { items[i].Doc.URL = u }
                    }
                }
            }
        }
    }
    return items
}

