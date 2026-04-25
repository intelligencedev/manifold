package service

import (
	"manifold/internal/codeqa"
	"manifold/internal/codeqa/diff"
	"manifold/internal/codeqa/evolve"
	"manifold/internal/codeqa/gates"
	"manifold/internal/codeqa/judge"
	"manifold/internal/codeqa/store"
	"manifold/internal/codeqa/workspace"
	"manifold/internal/llm"
	playartifacts "manifold/internal/playground/artifacts"
)

type Service struct {
	opts      codeqa.Options
	runner    codeqa.CommandRunner
	packager  *diff.Packager
	gateRun   *gates.Runner
	judge     *judge.Engine
	optimizer *evolve.Optimizer
	store     store.CodeQAStore
	artifacts playartifacts.Store
	workspace *workspace.Factory
	runSlots  chan struct{}
}

func New(opts codeqa.Options, runner codeqa.CommandRunner, provider llm.Provider, codeQAStore store.CodeQAStore) *Service {
	artifactStore := playartifacts.NewFilesystemStore(opts.ArtifactDir)
	runSlots := opts.MaxConcurrentRuns
	if runSlots <= 0 {
		runSlots = 1
	}
	workspaceFactory := workspace.NewFactory(opts)
	if codeQAStore == nil {
		codeQAStore = store.NewMemoryStore()
	}
	return &Service{
		opts:      opts,
		runner:    runner,
		packager:  diff.NewPackager(runner, opts),
		gateRun:   gates.NewRunner(runner, opts.MaxGateParallelism, workspaceFactory, gates.DefaultGoGates()...),
		judge:     judge.NewEngine(provider, opts.JudgeModel, opts.MaxJudgeParallelism),
		optimizer: evolve.NewOptimizer(opts, runner, provider),
		store:     codeQAStore,
		artifacts: artifactStore,
		workspace: workspaceFactory,
		runSlots:  make(chan struct{}, runSlots),
	}
}
