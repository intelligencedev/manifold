package judge

import (
	"context"
	"fmt"
	"sync"

	"manifold/internal/codeqa"
	"manifold/internal/llm"
)

type Engine struct {
	provider    llm.Provider
	model       string
	parallelism int
	profiles    []JudgeProfile
}

func NewEngine(provider llm.Provider, model string, parallelism int) *Engine {
	return &Engine{provider: provider, model: model, parallelism: parallelism, profiles: DefaultProfiles()}
}

func (e *Engine) Evaluate(ctx context.Context, bundle codeqa.DiffBundle, gates []codeqa.GateResult) ([]codeqa.JudgeVerdict, error) {
	if e.provider == nil {
		out := make([]codeqa.JudgeVerdict, 0, len(e.profiles))
		for _, profile := range e.profiles {
			out = append(out, codeqa.JudgeVerdict{
				JudgeID:          profile.ID,
				Verdict:          "insufficient",
				Confidence:       0,
				Scores:           zeroScores(),
				BlockingConcerns: []string{"no llm provider configured"},
			})
		}
		return out, nil
	}
	parallelism := e.parallelism
	if parallelism <= 0 || parallelism > len(e.profiles) {
		parallelism = len(e.profiles)
	}
	results := make([]codeqa.JudgeVerdict, len(e.profiles))
	errCh := make(chan error, len(e.profiles))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for idx := range e.profiles {
		idx := idx
		profile := e.profiles[idx]
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
			defer func() { <-sem }()
			verdict, err := e.evaluateProfile(ctx, profile, bundle, gates)
			if err != nil {
				errCh <- err
				return
			}
			results[idx] = verdict
		}()
	}
	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return nil, err
	}
	return results, nil
}

func (e *Engine) evaluateProfile(ctx context.Context, profile JudgeProfile, bundle codeqa.DiffBundle, gates []codeqa.GateResult) (codeqa.JudgeVerdict, error) {
	swap := SwapAppliedForJudge(bundle, profile.ID)
	prompt := BuildPrompt(profile, bundle, gates, swap)
	response, err := e.provider.Chat(ctx, []llm.Message{{Role: "user", Content: prompt}}, nil, e.model)
	if err != nil {
		return codeqa.JudgeVerdict{}, fmt.Errorf("judge request %s: %w", profile.ID, err)
	}
	verdict, err := ParseResponse(profile.ID, response.Content, swap, bundle, func(raw string) (string, error) {
		fixed, repairErr := e.provider.Chat(ctx, []llm.Message{{Role: "user", Content: BuildRepairPrompt(raw)}}, nil, e.model)
		if repairErr != nil {
			return "", repairErr
		}
		return fixed.Content, nil
	})
	if err != nil {
		return codeqa.JudgeVerdict{}, err
	}
	return verdict, nil
}

func zeroScores() map[string]float64 {
	out := make(map[string]float64, len(Dimensions))
	for _, dimension := range Dimensions {
		out[dimension] = 0
	}
	return out
}
