package main

import (
	"context"
	"encoding/json"
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

// callTool sends one user message and returns the raw JSON input of the named tool call.
// Every agent capability is this shape: evidence in, schema-checked tool input out.
func callTool(ctx context.Context, client anthropic.Client, system string, tool anthropic.ToolParam, user string) (string, error) {
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:        model,
		MaxTokens:    16000,
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
		System:       []anthropic.TextBlockParam{{Text: system}},
		Tools:        []anthropic.ToolUnionParam{{OfTool: &tool}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))},
	})
	if err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("model refused: %s", resp.StopDetails.Explanation)
	}
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == tool.Name {
			return tu.JSON.Input.Raw(), nil
		}
	}
	return "", fmt.Errorf("model did not call %s", tool.Name)
}

// Diagnose sends evidence (logs, pod state) to Claude and returns its structured diagnosis.
func Diagnose(ctx context.Context, client anthropic.Client, evidence string) (Diagnosis, error) {
	raw, err := callTool(ctx, client, systemPrompt, reportTool, evidence)
	if err != nil {
		return Diagnosis{}, err
	}
	return parseDiagnosis(raw)
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
