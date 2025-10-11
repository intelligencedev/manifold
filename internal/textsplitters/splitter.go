package textsplitters

// Splitter splits text into chunks appropriate for RAG ingestion.
// Implementations should be stateless or concurrency-safe after construction.
type Splitter interface {
	// Split yields non-empty chunks for the input text.
	Split(text string) []string
}
