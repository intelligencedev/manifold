package tokenizer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatML_Template(t *testing.T) {
	template := NewChatMLTemplate()

	tests := []struct {
		name     string
		messages []ChatMessage
		want     string
	}{
		{
			name: "single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello!"},
			},
			want: "<|im_start|>user\nHello!<|im_end|>\n<|im_start|>assistant\n",
		},
		{
			name: "system and user",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hi there!"},
			},
			want: "<|im_start|>system\nYou are helpful.<|im_end|>\n" +
				"<|im_start|>user\nHi there!<|im_end|>\n" +
				"<|im_start|>assistant\n",
		},
		{
			name: "full conversation",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hello!"},
				{Role: "assistant", Content: "Hi! How can I help?"},
				{Role: "user", Content: "Tell me a joke."},
			},
			want: "<|im_start|>system\nYou are helpful.<|im_end|>\n" +
				"<|im_start|>user\nHello!<|im_end|>\n" +
				"<|im_start|>assistant\nHi! How can I help?<|im_end|>\n" +
				"<|im_start|>user\nTell me a joke.<|im_end|>\n" +
				"<|im_start|>assistant\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := template.Apply(tt.messages)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestLLaMA_Template(t *testing.T) {
	template := NewLLaMATemplate()

	tests := []struct {
		name         string
		messages     []ChatMessage
		wantContains []string
	}{
		{
			name: "single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello!"},
			},
			wantContains: []string{"<s>", "[INST]", "Hello!", "[/INST]"},
		},
		{
			name: "with system prompt",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hi!"},
			},
			wantContains: []string{"<s>", "[INST]", "<<SYS>>", "You are helpful.", "<</SYS>>", "Hi!", "[/INST]"},
		},
		{
			name: "multi-turn conversation",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello!"},
				{Role: "assistant", Content: "Hi there!"},
				{Role: "user", Content: "How are you?"},
			},
			wantContains: []string{"<s>", "[INST]", "Hello!", "[/INST]", "Hi there!", "</s>", "How are you?"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := template.Apply(tt.messages)
			for _, substr := range tt.wantContains {
				assert.Contains(t, result, substr)
			}
		})
	}
}

func TestMistral_Template(t *testing.T) {
	template := NewMistralTemplate()

	tests := []struct {
		name         string
		messages     []ChatMessage
		wantContains []string
	}{
		{
			name: "single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello!"},
			},
			wantContains: []string{"<s>", "[INST]", "Hello!", "[/INST]"},
		},
		{
			name: "multi-turn",
			messages: []ChatMessage{
				{Role: "user", Content: "Hi!"},
				{Role: "assistant", Content: "Hello!"},
				{Role: "user", Content: "How are you?"},
			},
			wantContains: []string{"<s>", "[INST]", "Hi!", "[/INST]", "Hello!", "</s>", "How are you?"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := template.Apply(tt.messages)
			for _, substr := range tt.wantContains {
				assert.Contains(t, result, substr)
			}
		})
	}
}

func TestChatTemplate_GetByName(t *testing.T) {
	tests := []struct {
		name         string
		templateName string
		wantErr      bool
		expectedName string
	}{
		{
			name:         "chatml",
			templateName: "chatml",
			wantErr:      false,
			expectedName: "ChatML",
		},
		{
			name:         "llama",
			templateName: "llama",
			wantErr:      false,
			expectedName: "LLaMA",
		},
		{
			name:         "mistral",
			templateName: "mistral",
			wantErr:      false,
			expectedName: "Mistral",
		},
		{
			name:         "case insensitive",
			templateName: "CHATML",
			wantErr:      false,
			expectedName: "ChatML",
		},
		{
			name:         "unknown template",
			templateName: "unknown",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template, err := GetChatTemplate(tt.templateName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, template)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, template)
			assert.Equal(t, tt.expectedName, template.Name())
		})
	}
}

func TestChatML_Name(t *testing.T) {
	template := NewChatMLTemplate()
	assert.Equal(t, "ChatML", template.Name())
}

func TestLLaMA_Name(t *testing.T) {
	template := NewLLaMATemplate()
	assert.Equal(t, "LLaMA", template.Name())
}

func TestMistral_Name(t *testing.T) {
	template := NewMistralTemplate()
	assert.Equal(t, "Mistral", template.Name())
}

func TestChatML_EmptyMessages(t *testing.T) {
	template := NewChatMLTemplate()
	result := template.Apply([]ChatMessage{})
	// Should still have assistant start.
	assert.Equal(t, "<|im_start|>assistant\n", result)
}

func TestLLaMA_MultipleSystemMessages(t *testing.T) {
	template := NewLLaMATemplate()

	messages := []ChatMessage{
		{Role: "system", Content: "First system."},
		{Role: "system", Content: "Second system."},
		{Role: "user", Content: "Hello!"},
	}

	result := template.Apply(messages)

	// Only the last system message should be used (simplified behavior).
	// The implementation uses the last system prompt it encounters.
	assert.Contains(t, result, "Hello!")
	assert.Contains(t, result, "[INST]")
}

func TestChatTemplate_PreservesContent(t *testing.T) {
	templates := []ChatTemplate{
		NewChatMLTemplate(),
		NewLLaMATemplate(),
		NewMistralTemplate(),
	}

	messages := []ChatMessage{
		{Role: "user", Content: "Special chars: <>[]{}!@#$%"},
	}

	for _, template := range templates {
		t.Run(template.Name(), func(t *testing.T) {
			result := template.Apply(messages)
			// All templates should preserve the content.
			assert.Contains(t, result, "Special chars: <>[]{}!@#$%")
		})
	}
}

func TestChatML_MultilineContent(t *testing.T) {
	template := NewChatMLTemplate()

	messages := []ChatMessage{
		{Role: "user", Content: "Line 1\nLine 2\nLine 3"},
	}

	result := template.Apply(messages)
	assert.Contains(t, result, "Line 1\nLine 2\nLine 3")
	assert.True(t, strings.Count(result, "\n") >= 3)
}
