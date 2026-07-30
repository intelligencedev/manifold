package config

import "testing"

func TestLexMinifyConfig_DefaultDisabled(t *testing.T) {
	t.Parallel()
	var cfg Config
	applyDefaults(&cfg)
	if cfg.LexMinify.Enabled {
		t.Fatalf("expected lexMinify disabled by default, got %+v", cfg.LexMinify)
	}
	if got := cfg.LexMinify.EffectiveLevel(); got != 0 {
		t.Fatalf("expected effective level 0, got %d", got)
	}
}

func TestLexMinifyConfig_EnabledFillsRecommendedLevel(t *testing.T) {
	t.Parallel()
	cfg := Config{LexMinify: LexMinifyConfig{Enabled: true}}
	applyLexMinifyDefaults(&cfg)
	if cfg.LexMinify.Level != RecommendedLexMinifyLevel {
		t.Fatalf("expected recommended level %d, got %d", RecommendedLexMinifyLevel, cfg.LexMinify.Level)
	}
	if got := cfg.LexMinify.EffectiveLevel(); got != RecommendedLexMinifyLevel {
		t.Fatalf("expected effective %d, got %d", RecommendedLexMinifyLevel, got)
	}
}

func TestLexMinifyConfig_EngineSettingsOffWhenDisabled(t *testing.T) {
	t.Parallel()
	cfg := LexMinifyConfig{Enabled: false, Level: 6, Zones: 7, CurrentRequestMaxLevel: 2}
	level, zones, current := cfg.EngineSettings()
	if level != 0 || zones != 0 || current != 0 {
		t.Fatalf("expected zero settings when disabled, got %d %d %d", level, zones, current)
	}
}

func TestValidateLexMinifyConfig_RejectsOutOfRange(t *testing.T) {
	t.Parallel()
	if err := validateLexMinifyConfig(LexMinifyConfig{Enabled: true, Level: 9}); err == nil {
		t.Fatal("expected invalid level error")
	}
	if err := validateLexMinifyConfig(LexMinifyConfig{Enabled: true, Level: 0}); err == nil {
		t.Fatal("expected enabled-with-zero level error after defaults not applied")
	}
}
