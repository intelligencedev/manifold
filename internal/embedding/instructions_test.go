package embedding

import (
	"strings"
	"testing"

	"manifold/internal/config"
)

func TestFormatQueryInput_AutoAppliesForQwen(t *testing.T) {
	t.Parallel()

	cfg := config.EmbeddingConfig{
		Model: "Qwen3-Embedding-0.6B-f16.gguf",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:   "auto",
			Format: "qwen",
		},
	}

	got := FormatQueryInput(cfg, UseCaseRAGQuery, "Where is the setting?", "")
	if !got.Applied {
		t.Fatalf("expected instruction to be applied")
	}
	if got.Source != "builtin" {
		t.Fatalf("expected builtin source, got %q", got.Source)
	}
	if !strings.HasPrefix(got.Input, "Instruct: "+DefaultRAGQueryInstruction+"\nQuery: ") {
		t.Fatalf("unexpected formatted input: %q", got.Input)
	}
}

func TestFormatQueryInput_AutoNoopsForNonQwen(t *testing.T) {
	t.Parallel()

	cfg := config.EmbeddingConfig{
		Model: "text-embedding-3-small",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:   "auto",
			Format: "qwen",
		},
	}

	got := FormatQueryInput(cfg, UseCaseRAGQuery, "hello", "")
	if got.Applied {
		t.Fatalf("expected instruction not to be applied")
	}
	if got.Input != "hello" {
		t.Fatalf("expected raw query, got %q", got.Input)
	}
}

func TestFormatQueryInput_EnabledAppliesForAnyModel(t *testing.T) {
	t.Parallel()

	cfg := config.EmbeddingConfig{
		Model: "text-embedding-3-small",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:         "enabled",
			Format:       "qwen",
			DefaultQuery: "Find matching content.",
		},
	}

	got := FormatQueryInput(cfg, UseCaseTransitQuery, "shared note", "")
	if !got.Applied || got.Instruction != "Find matching content." {
		t.Fatalf("expected configured instruction, got %#v", got)
	}
}

func TestFormatQueryInput_DisabledNoopsWithoutExplicitInstruction(t *testing.T) {
	t.Parallel()

	cfg := config.EmbeddingConfig{
		Model: "Qwen3-Embedding-0.6B",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:   "disabled",
			Format: "qwen",
		},
	}

	got := FormatQueryInput(cfg, UseCaseEvolvingMemoryQuery, "current task", "")
	if got.Applied || got.Input != "current task" {
		t.Fatalf("expected raw query, got %#v", got)
	}
}

func TestFormatQueryInput_ExplicitInstructionOverridesModeAndDefault(t *testing.T) {
	t.Parallel()

	cfg := config.EmbeddingConfig{
		Model: "text-embedding-3-small",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:         "disabled",
			Format:       "qwen",
			DefaultQuery: "default",
		},
	}

	got := FormatQueryInput(cfg, UseCaseRAGQuery, "question", "Retrieve API docs.")
	if !got.Applied || got.Source != "explicit" || got.Instruction != "Retrieve API docs." {
		t.Fatalf("expected explicit instruction, got %#v", got)
	}
}

func TestFormatQueryInput_BlankConfiguredInstructionFallsBackToBuiltIn(t *testing.T) {
	t.Parallel()

	cfg := config.EmbeddingConfig{
		Model: "Qwen3-Embedding-0.6B",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:     "auto",
			Format:   "qwen",
			RAGQuery: "   ",
		},
	}

	got := FormatQueryInput(cfg, UseCaseRAGQuery, "question", "")
	if got.Instruction != DefaultRAGQueryInstruction {
		t.Fatalf("expected built-in instruction, got %q", got.Instruction)
	}
}
