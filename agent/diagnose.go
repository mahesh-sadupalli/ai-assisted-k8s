package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

const model = "claude-opus-5-5"

// Diagnosis is the structured result we want back from the agent.
type Diagnosis struct {
	Severity        string `json:"severity"`
	WhatBroke       string `json:"what_broke"`
	LikelyCause     string `json:"likely_cause"`
	SuggestedAction string `json:"suggested_action"`
}

// The agent answers by calling this tool. strict=true makes the API guarantee the
// input matches the schema, so no more stripping ```json fences off free text.
var reportTool = anthropic.ToolParam{
	Name:        "report_diagnosis",
	Description: anthropic.String("Report the diagnosis of the Kubernetes problem. Call this exactly once."),
	Strict:      anthropic.Bool(true),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"severity":         map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
			"what_broke":       map[string]any{"type": "string", "description": "One short sentence: the symptom."},
			"likely_cause":     map[string]any{"type": "string", "description": "One short sentence: the root cause."},
			"suggested_action": map[string]any{"type": "string", "description": "One short sentence: the next step an engineer should take."},
		},
		Required:    []string{"severity", "what_broke", "likely_cause", "suggested_action"},
		ExtraFields: map[string]any{"additionalProperties": false},
	},
}

const systemPrompt = `You are an experienced Site Reliability Engineer.
Diagnose the Kubernetes problem in the logs or state you are given.
Always answer by calling the report_diagnosis tool.`

// Diagnose sends evidence (logs, pod state) to Claude and returns its structured diagnosis.
func Diagnose(ctx context.Context, client anthropic.Client, evidence string) (Diagnosis, error) {
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:        model,
		MaxTokens:    16000,
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
		System:       []anthropic.TextBlockParam{{Text: systemPrompt}},
		Tools:        []anthropic.ToolUnionParam{{OfTool: &reportTool}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(evidence))},
	})
	if err != nil {
		return Diagnosis{}, err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return Diagnosis{}, fmt.Errorf("model refused: %s", resp.StopDetails.Explanation)
	}
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == reportTool.Name {
			return parseDiagnosis(tu.JSON.Input.Raw())
		}
	}
	return Diagnosis{}, errors.New("model did not call report_diagnosis")
}

func parseDiagnosis(raw string) (Diagnosis, error) {
	var d Diagnosis
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return Diagnosis{}, fmt.Errorf("parse diagnosis: %w", err)
	}
	return d, nil
}

// Pages reports whether this diagnosis should wake someone up.
func (d Diagnosis) Pages() bool {
	return d.Severity == "high" || d.Severity == "critical"
}
