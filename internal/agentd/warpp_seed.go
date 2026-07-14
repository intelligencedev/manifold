package agentd

import (
	"context"

	"github.com/rs/zerolog/log"

	"manifold/internal/warpp"
)

const exampleWarppWorkflowID = "example-intro"

// seedExampleWarppWorkflow installs a small demonstration workflow for the
// system user on first run (when no workflows exist yet), so the editor opens
// with a working example that shows literal value entry and data flow.
func (a *app) seedExampleWarppWorkflow(ctx context.Context) {
	rt := a.warppState()
	sums, err := rt.ListWorkflowSummaries(ctx, systemUserID)
	if err != nil {
		return
	}
	if len(sums) > 0 {
		return // never clobber existing workflows
	}
	if _, _, err := rt.UpsertWorkflow(ctx, systemUserID, exampleWarppDocument(), exampleWarppCanvas()); err != nil {
		log.Warn().Err(err).Msg("warpp_seed_example_failed")
		return
	}
	log.Info().Str("workflow", exampleWarppWorkflowID).Msg("warpp_example_seeded")
}

func lit(v any) warpp.Input {
	return warpp.Input{One: &warpp.Binding{Value: v, HasValue: true}}
}

func from(ref string) warpp.Input {
	return warpp.Input{One: &warpp.Binding{From: ref}}
}

func exampleWarppDocument() warpp.Document {
	return warpp.Document{
		ID:          exampleWarppWorkflowID,
		Name:        "Example: intro builder",
		Description: "Combines two literal values with a workflow input into a prompt. Click a node to edit its values in the inspector.",
		Inputs: []warpp.PortSpec{
			{Name: "audience", Type: "text", Required: true, Description: "Who the intro is for."},
		},
		Nodes: []warpp.Node{
			{
				ID:   "tone",
				Type: "data.constant",
				Inputs: map[string]warpp.Input{
					"value": lit("warm and concise"),
					"as":    lit("text"),
				},
			},
			{
				ID:   "bullets",
				Type: "data.constant",
				Inputs: map[string]warpp.Input{
					"value": lit(float64(3)),
					"as":    lit("number"),
				},
			},
			{
				ID:   "prompt",
				Type: "data.template",
				Inputs: map[string]warpp.Input{
					"template": lit("Write a {tone} intro for {audience}. Include {n} bullet points."),
					"vars": {Named: map[string]warpp.Binding{
						"tone":     {From: "tone.value"},
						"audience": {From: "in.audience"},
						"n":        {From: "bullets.value"},
					}},
				},
			},
		},
		Outputs: map[string]warpp.Binding{
			"prompt": {From: "prompt.text"},
		},
	}
}

func exampleWarppCanvas() warpp.Canvas {
	return warpp.Canvas{
		Nodes: map[string]warpp.CanvasNode{
			"tone":    {X: 80, Y: 80},
			"bullets": {X: 80, Y: 280},
			"prompt":  {X: 460, Y: 160},
		},
	}
}
