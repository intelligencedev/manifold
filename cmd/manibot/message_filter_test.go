package main

import (
	"os"
	"testing"
)

func TestPromptFromMessage_WithPrefix(t *testing.T) {
	prompt, matched := promptFromMessage("!manibot status", "!manibot")
	if !matched {
		t.Fatalf("expected prefix match")
	}
	if prompt != "status" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestPromptFromMessage_MatrixMentionMatchesAnywhere(t *testing.T) {
	prompt, matched := promptFromMessage("Hello @gpt_bot and @razer_bot. What are you up to?", "@gpt_bot")
	if !matched {
		t.Fatalf("expected inline mention match")
	}
	if prompt != "Hello @gpt_bot and @razer_bot. What are you up to?" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestPromptFromMessage_MatrixMentionRequiresStandaloneTag(t *testing.T) {
	_, matched := promptFromMessage("Hello @gpt_bot_ops, are you there?", "@gpt_bot")
	if matched {
		t.Fatalf("expected partial mention to be ignored")
	}
}

func TestPromptFromMessage_CommandPrefixStillRequiresStart(t *testing.T) {
	_, matched := promptFromMessage("hello !manibot status", "!manibot")
	if matched {
		t.Fatalf("expected command prefix in the middle to be ignored")
	}
}

func TestPromptFromMessage_WithoutPrefixConfiguredMatchesAll(t *testing.T) {
	prompt, matched := promptFromMessage("what changed today?", "")
	if !matched {
		t.Fatalf("expected empty prefix to match all messages")
	}
	if prompt != "what changed today?" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestPromptFromMessage_NoPrefixMatch(t *testing.T) {
	_, matched := promptFromMessage("hello there", "!manibot")
	if matched {
		t.Fatalf("expected message without prefix to be ignored")
	}
}

func TestLoadConfig_DoesNotDefaultBotPrefix(t *testing.T) {
	keys := map[string]string{
		"MATRIX_HOMESERVER_URL":       os.Getenv("MATRIX_HOMESERVER_URL"),
		"MATRIX_BOT_USER_ID":          os.Getenv("MATRIX_BOT_USER_ID"),
		"MATRIX_ACCESS_TOKEN":         os.Getenv("MATRIX_ACCESS_TOKEN"),
		"BOT_PREFIX":                  os.Getenv("BOT_PREFIX"),
		"MANIFOLD_SYSTEM_PROMPT":      os.Getenv("MANIFOLD_SYSTEM_PROMPT"),
		"MANIFOLD_SYSTEM_PROMPT_FILE": os.Getenv("MANIFOLD_SYSTEM_PROMPT_FILE"),
	}
	defer func() {
		for key, value := range keys {
			if value == "" {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, value)
		}
	}()

	_ = os.Setenv("MATRIX_HOMESERVER_URL", "https://matrix.example.com")
	_ = os.Setenv("MATRIX_BOT_USER_ID", "@manibot:example.com")
	_ = os.Setenv("MATRIX_ACCESS_TOKEN", "token")
	_ = os.Unsetenv("BOT_PREFIX")
	_ = os.Setenv("MANIFOLD_SYSTEM_PROMPT", "test prompt")
	_ = os.Unsetenv("MANIFOLD_SYSTEM_PROMPT_FILE")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if cfg.BotPrefix != "" {
		t.Fatalf("expected empty BOT_PREFIX when unset, got %q", cfg.BotPrefix)
	}
}
