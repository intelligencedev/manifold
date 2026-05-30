package embedding

import (
	"strings"

	"manifold/internal/config"
)

const (
	InstructionModeAuto     = "auto"
	InstructionModeEnabled  = "enabled"
	InstructionModeDisabled = "disabled"

	InstructionFormatQwen = "qwen"

	UseCaseGenericQuery        = "query"
	UseCaseRAGQuery            = "rag.query"
	UseCaseEvolvingMemoryQuery = "evolving_memory.query"
	UseCaseTransitQuery        = "transit.query"
)

const (
	DefaultGenericQueryInstruction = "Given a search query, retrieve relevant text that answers the query."
	DefaultRAGQueryInstruction     = "Given a search query, retrieve relevant passages that answer the query."
	DefaultMemoryQueryInstruction  = "Given the current task, retrieve past experiences, lessons, and strategies relevant to the current task."
	DefaultTransitQueryInstruction = "Given a search query, retrieve relevant stored shared-memory records."
)

// InstructionResult describes the effective text used for an embedding call.
type InstructionResult struct {
	Input       string
	Applied     bool
	Instruction string
	UseCase     string
	Format      string
	Mode        string
	Source      string
}

// FormatQueryInput applies the configured query-side embedding instruction for
// retrieval-style inputs. Explicit instructions are always honored, while
// configured/default instructions depend on the instruction mode.
func FormatQueryInput(cfg config.EmbeddingConfig, useCase, query, explicitInstruction string) InstructionResult {
	result := InstructionResult{
		Input:   query,
		UseCase: useCase,
		Format:  normalizedFormat(cfg.Instructions.Format),
		Mode:    normalizedMode(cfg.Instructions.Mode),
	}

	instruction := strings.TrimSpace(explicitInstruction)
	if instruction != "" {
		result.Source = "explicit"
		return applyInstruction(result, instruction)
	}

	if result.Mode == InstructionModeDisabled {
		result.Source = "disabled"
		return result
	}
	if result.Mode == InstructionModeAuto && !looksLikeQwen3Embedding(cfg.Model) {
		result.Source = "auto_not_matched"
		return result
	}

	instruction = configuredInstruction(cfg.Instructions, useCase)
	source := "configured"
	if instruction == "" {
		instruction = builtInInstruction(useCase)
		source = "builtin"
	}
	result.Source = source
	if instruction == "" {
		return result
	}
	return applyInstruction(result, instruction)
}

func applyInstruction(result InstructionResult, instruction string) InstructionResult {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return result
	}
	switch result.Format {
	case InstructionFormatQwen:
		result.Input = "Instruct: " + instruction + "\nQuery: " + result.Input
		result.Applied = true
		result.Instruction = instruction
	}
	return result
}

func configuredInstruction(cfg config.EmbeddingInstructionConfig, useCase string) string {
	switch useCase {
	case UseCaseRAGQuery:
		if strings.TrimSpace(cfg.RAGQuery) != "" {
			return strings.TrimSpace(cfg.RAGQuery)
		}
	case UseCaseEvolvingMemoryQuery:
		if strings.TrimSpace(cfg.EvolvingMemoryQuery) != "" {
			return strings.TrimSpace(cfg.EvolvingMemoryQuery)
		}
	case UseCaseTransitQuery:
		if strings.TrimSpace(cfg.TransitQuery) != "" {
			return strings.TrimSpace(cfg.TransitQuery)
		}
	}
	return strings.TrimSpace(cfg.DefaultQuery)
}

func builtInInstruction(useCase string) string {
	switch useCase {
	case UseCaseRAGQuery:
		return DefaultRAGQueryInstruction
	case UseCaseEvolvingMemoryQuery:
		return DefaultMemoryQueryInstruction
	case UseCaseTransitQuery:
		return DefaultTransitQueryInstruction
	default:
		return DefaultGenericQueryInstruction
	}
}

func normalizedMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case InstructionModeEnabled:
		return InstructionModeEnabled
	case InstructionModeDisabled:
		return InstructionModeDisabled
	default:
		return InstructionModeAuto
	}
}

func normalizedFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case InstructionFormatQwen:
		return InstructionFormatQwen
	default:
		return InstructionFormatQwen
	}
}

func looksLikeQwen3Embedding(model string) bool {
	return strings.Contains(strings.ToLower(model), "qwen3-embedding")
}
