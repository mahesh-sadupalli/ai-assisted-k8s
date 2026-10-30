package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// PlanReview is the agent's verdict on a terraform plan.
type PlanReview struct {
	Verdict  string     `json:"verdict"` // approve | needs_review | block
	Summary  string     `json:"summary"`
	Risks    []PlanRisk `json:"risks"`
	Findings []Finding  `json:"findings"` // deterministic rule hits, filled in by code, not the model
}

type PlanRisk struct {
	Address        string `json:"address"`
	Severity       string `json:"severity"`
	Risk           string `json:"risk"`
	Recommendation string `json:"recommendation"`
}

var reviewTool = anthropic.ToolParam{
	Name:        "report_plan_review",
	Description: anthropic.String("Report the change-safety review of the terraform plan. Call this exactly once."),
	Strict:      anthropic.Bool(true),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"verdict": map[string]any{"type": "string", "enum": []string{"approve", "needs_review", "block"}},
			"summary": map[string]any{"type": "string", "description": "Two sentences max: what the plan does and whether it is safe to merge."},
			"risks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"address":        map[string]any{"type": "string"},
						"severity":       map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
						"risk":           map[string]any{"type": "string"},
						"recommendation": map[string]any{"type": "string"},
					},
					"required":             []string{"address", "severity", "risk", "recommendation"},
					"additionalProperties": false,
				},
			},
		},
		Required:    []string{"verdict", "summary", "risks"},
		ExtraFields: map[string]any{"additionalProperties": false},
	},
}

const reviewPrompt = `You are a senior cloud/platform engineer reviewing a Terraform plan before merge.
Judge change safety: data loss, downtime, security exposure, privilege escalation, cost spikes.
Routine changes (tags, scaling, new stateless resources) are fine; do not invent risks.
"block" = must not merge as-is. "needs_review" = a human should look. "approve" = safe.
Always answer by calling the report_plan_review tool.`

// ReviewPlan runs the deterministic rules, then asks Claude for the full review.
// A rule hit always overrides an "approve": the model can add risks, never remove them.
func ReviewPlan(ctx context.Context, client anthropic.Client, p Plan) (PlanReview, error) {
	findings := Scan(p)
	var b strings.Builder
	b.WriteString("Changed resources:\n")
	b.WriteString(p.Summary())
	if len(findings) > 0 {
		b.WriteString("\nDeterministic policy checks already flagged:\n")
		for _, f := range findings {
			fmt.Fprintf(&b, "- %s: %s (%s)\n", f.Address, f.Detail, f.Rule)
		}
	}

	raw, err := callTool(ctx, client, reviewPrompt, reviewTool, b.String())
	if err != nil {
		return PlanReview{}, err
	}
	var r PlanReview
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return PlanReview{}, fmt.Errorf("parse review: %w", err)
	}
	r.Findings = findings
	return enforceFindings(r), nil
}

func enforceFindings(r PlanReview) PlanReview {
	if len(r.Findings) > 0 && r.Verdict == "approve" {
		r.Verdict = "needs_review"
	}
	return r
}
