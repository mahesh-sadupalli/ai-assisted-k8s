package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Diffs bigger than this are never "low" risk: more change, more ways to break.
const largeDiffLines = 500

// DeployReview is the agent's read on one deploy.
type DeployReview struct {
	Risk     string   `json:"risk"` // low | medium | high
	Rollback bool     `json:"rollback"`
	Summary  string   `json:"summary"`
	Reasons  []string `json:"reasons"`
}

var deployTool = anthropic.ToolParam{
	Name:        "report_deploy_review",
	Description: anthropic.String("Report the risk assessment of this deploy. Call this exactly once."),
	Strict:      anthropic.Bool(true),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"risk":     map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"rollback": map[string]any{"type": "boolean", "description": "true if the release should be (or stay) rolled back"},
			"summary":  map[string]any{"type": "string", "description": "Two sentences max for the on-call engineer."},
			"reasons":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		Required:    []string{"risk", "rollback", "summary", "reasons"},
		ExtraFields: map[string]any{"additionalProperties": false},
	},
}

const deployPrompt = `You are the release engineer on call. You get the report of one deploy
(rollout status, smoke-test results, any rollback) and the size of the code change.
Assess the risk of this release and whether it should be rolled back. Be concrete about
which check failed and what that implies. Always answer by calling report_deploy_review.`

var shortstat = regexp.MustCompile(`(\d+) insertions?\(\+\)|(\d+) deletions?\(-\)`)

// diffLines sums insertions and deletions from `git diff --shortstat` output.
func diffLines(stat string) int {
	n := 0
	for _, m := range shortstat.FindAllStringSubmatch(stat, -1) {
		for _, g := range m[1:] {
			if v, err := strconv.Atoi(g); err == nil {
				n += v
			}
		}
	}
	return n
}

func deployFailed(report string) bool { return strings.Contains(report, "result: FAILED") }

// enforceDeployRules: the facts (failed checks, diff size) set a floor the model can't go below.
func enforceDeployRules(r DeployReview, report, stat string) DeployReview {
	if deployFailed(report) {
		r.Rollback = true
		r.Risk = "high"
	}
	if diffLines(stat) > largeDiffLines && r.Risk == "low" {
		r.Risk = "medium"
		r.Reasons = append(r.Reasons, fmt.Sprintf("large change: %d lines (> %d)", diffLines(stat), largeDiffLines))
	}
	return r
}

func ReviewDeploy(ctx context.Context, client anthropic.Client, report, stat string) (DeployReview, error) {
	input := "Deploy report:\n" + report + "\nCode change (git diff --shortstat):\n" + stat
	raw, err := callTool(ctx, client, deployPrompt, deployTool, input)
	if err != nil {
		return DeployReview{}, err
	}
	var r DeployReview
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return DeployReview{}, fmt.Errorf("parse deploy review: %w", err)
	}
	return enforceDeployRules(r, report, stat), nil
}

// Markdown renders the review for $GITHUB_STEP_SUMMARY.
func (r DeployReview) Markdown() string {
	icon := map[string]string{"low": "🟢", "medium": "🟡", "high": "🔴"}[r.Risk]
	var b strings.Builder
	fmt.Fprintf(&b, "## Sentinel deploy review\n\n**Risk:** %s %s · **Rollback:** %v\n\n%s\n\n", icon, r.Risk, r.Rollback, r.Summary)
	for _, reason := range r.Reasons {
		fmt.Fprintf(&b, "- %s\n", reason)
	}
	return b.String()
}
