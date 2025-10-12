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
//   - Sentence/Paragraph/Hyrbid boundary grouping
//     Diagram: [Sentence][Sentence] | [Paragraph]
//     Pros: Natural boundaries; variable size with target.
//   - Markdown-aware
//     Diagram: # H1 -> chunk(s); ## H2 -> chunk(s)
//   - Code-aware
//     Diagram: fn a(){...} | class C{...}
//   - Semantic breakpoints
//     Diagram: ... high sim ... | low sim | ...
//   - TextTiling-style lexical segmentation
//   - Rolling n-sentence windows
//   - Layout-aware pages/tables (heuristic)
//   - Recursive hierarchical splitting
package textsplitters
