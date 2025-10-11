// Package textsplitters provides strategies to split text for RAG ingestion.
//
// Extensibility
//
//	The package exposes a simple Splitter interface and a factory to construct
//	concrete implementations by type, allowing new methods to be added without
//	affecting callers.
//
// Implemented strategies
//   - Fixed-length (chars/tokens)
//     Diagram: |====100====||====100====||====100====|
//     Pros: Simple, fast, predictable.
//     Cons: Cuts mid-sentence; semantic drift; brittle across formats.
//     Sources: Inspired by LangChain text splitters.
package textsplitters
