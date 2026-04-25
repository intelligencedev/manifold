package codeqa

import (
	"path/filepath"
	"strings"

	appconfig "manifold/internal/config"
)

type Options struct {
	Enabled                bool
	ArtifactDir            string
	MaxConcurrentRuns      int
	MaxGateParallelism     int
	MaxJudgeParallelism    int
	DefaultMaxDiffBytes    int
	DefaultMaxChangedFiles int
	AcceptThreshold        float64
	MinConfidence          float64
	JudgeModel             string
	ProposerModel          string
	AllowedCommands        []string
	HighRiskGlobs          []string
	ForbiddenGlobs         []string
	AllowAutoApply         bool
	AllowCommitAccepted    bool
	Workdir                string
}

func OptionsFromConfig(cfg appconfig.CodeQAConfig, workdir string) Options {
	artifactDir := strings.TrimSpace(cfg.ArtifactDir)
	if artifactDir == "" {
		artifactDir = "codeqa-artifacts"
	}
	if !filepath.IsAbs(artifactDir) {
		artifactDir = filepath.Join(workdir, artifactDir)
	}
	allowed := append([]string(nil), cfg.AllowedCommands...)
	highRisk := append([]string(nil), cfg.HighRiskGlobs...)
	forbidden := append([]string(nil), cfg.ForbiddenGlobs...)
	return Options{
		Enabled:                cfg.Enabled,
		ArtifactDir:            artifactDir,
		MaxConcurrentRuns:      cfg.MaxConcurrentRuns,
		MaxGateParallelism:     cfg.MaxGateParallelism,
		MaxJudgeParallelism:    cfg.MaxJudgeParallelism,
		DefaultMaxDiffBytes:    cfg.DefaultMaxDiffBytes,
		DefaultMaxChangedFiles: cfg.DefaultMaxChangedFiles,
		AcceptThreshold:        cfg.AcceptThreshold,
		MinConfidence:          cfg.MinConfidence,
		JudgeModel:             cfg.JudgeModel,
		ProposerModel:          cfg.ProposerModel,
		AllowedCommands:        allowed,
		HighRiskGlobs:          highRisk,
		ForbiddenGlobs:         forbidden,
		AllowAutoApply:         cfg.AllowAutoApply,
		AllowCommitAccepted:    cfg.AllowCommitAccepted,
		Workdir:                workdir,
	}
}

func (o Options) EffectiveAcceptThreshold(override float64) float64 {
	if override != 0 {
		return override
	}
	return o.AcceptThreshold
}

func (o Options) EffectiveMinConfidence(override float64) float64 {
	if override != 0 {
		return override
	}
	return o.MinConfidence
}
