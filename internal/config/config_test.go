package config

import "testing"

func TestPackageBuilds(t *testing.T) {}

func TestLocalObsConfigIsEnabledDefaultsToTrue(t *testing.T) {
	cfg := LocalObsConfig{}
	if !cfg.IsEnabled() {
		t.Fatal("expected omitted local observability enabled flag to default to true")
	}
}

func TestLocalObsConfigIsEnabledHonorsExplicitFalse(t *testing.T) {
	enabled := false
	cfg := LocalObsConfig{Enabled: &enabled}
	if cfg.IsEnabled() {
		t.Fatal("expected explicit false local observability enabled flag to disable local telemetry")
	}
}
