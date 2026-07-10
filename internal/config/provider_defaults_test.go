package config

import (
	"reflect"
	"testing"
)

func TestKnownProvidersOrder(t *testing.T) {
	got := KnownProviders()
	want := []string{"openai", "anthropic", "google", "openrouter", "llamacpp", "local"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownProviders() = %v, want %v", got, want)
	}
}

func TestProviderDefaultsShape(t *testing.T) {
	cases := []struct {
		provider     string
		backend      string
		api          string
		baseURL      string
		wantParamKey string
		wantParamVal any
	}{
		{"openai", "openai", "responses", "https://api.openai.com/v1", "reasoning_effort", "medium"},
		{"anthropic", "anthropic", "", "https://api.anthropic.com", "max_tokens", 16384},
		{"google", "google", "", "https://generativelanguage.googleapis.com/", "generation_config", nil},
		{"openrouter", "anthropic", "", "https://openrouter.ai/api", "max_tokens", 16384},
		{"llamacpp", "openai", "responses", "", "cache_prompt", true},
		{"local", "openai", "completions", "", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			pd, ok := ProviderDefaults(tc.provider)
			if !ok {
				t.Fatalf("ProviderDefaults(%q) not found", tc.provider)
			}
			if pd.Backend != tc.backend {
				t.Errorf("Backend = %q, want %q", pd.Backend, tc.backend)
			}
			if pd.API != tc.api {
				t.Errorf("API = %q, want %q", pd.API, tc.api)
			}
			if pd.BaseURL != tc.baseURL {
				t.Errorf("BaseURL = %q, want %q", pd.BaseURL, tc.baseURL)
			}
			if tc.wantParamKey != "" {
				v, present := pd.ExtraParams[tc.wantParamKey]
				if !present {
					t.Fatalf("ExtraParams missing key %q; got %v", tc.wantParamKey, pd.ExtraParams)
				}
				if tc.wantParamVal != nil && !reflect.DeepEqual(v, tc.wantParamVal) {
					t.Errorf("ExtraParams[%q] = %v (%T), want %v", tc.wantParamKey, v, v, tc.wantParamVal)
				}
			}
		})
	}
}

func TestProviderDefaultsUnknown(t *testing.T) {
	if _, ok := ProviderDefaults("does-not-exist"); ok {
		t.Fatalf("expected unknown provider to return ok=false")
	}
}

func TestProviderDefaultsCaseInsensitiveAndTrimmed(t *testing.T) {
	if _, ok := ProviderDefaults("  OpenRouter "); !ok {
		t.Fatalf("expected provider lookup to normalize case/whitespace")
	}
}

func TestProviderDefaultsReturnsCopy(t *testing.T) {
	first, _ := ProviderDefaults("anthropic")
	first.ExtraParams["max_tokens"] = 1 // mutate the returned map
	second, _ := ProviderDefaults("anthropic")
	if second.ExtraParams["max_tokens"] != 16384 {
		t.Fatalf("ProviderDefaults returned a shared map; mutation leaked: %v", second.ExtraParams["max_tokens"])
	}
}
