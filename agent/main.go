package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/joho/godotenv"
)

// Diagnosis is the structured result we want back from the agent.
type Diagnosis struct {
	Severity        string `json:"severity"`
	LikelyCause     string `json:"likely_cause"`
	SuggestedAction string `json:"suggested_action"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("warning: no .env file found, relying on shell environment")
	}

	client := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)

	// PERCEIVE — the broken pod
	podLog := `Name:          payment-service-7d9f8b6c4-xk2lp
Namespace:     production
Status:        Running
Restart Count: 5
Last State:    Terminated
Reason:      OOMKilled
Exit Code:   137
Events: Warning  BackOff  Back-off restarting failed container payment-service`

	// System prompt: role + strict output contract
	systemPrompt := `You are an experienced Site Reliability Engineer.
Diagnose the Kubernetes pod problem you are given.
Respond with ONLY a JSON object, no markdown, no extra text, in exactly this shape:
{
  "severity": "one of: low, medium, high, critical",
  "likely_cause": "one short sentence",
  "suggested_action": "one short sentence"
}`

	// REASON — system sets the rules, user message is just the log
	message, err := client.Messages.New(context.TODO(), anthropic.MessageNewParams{
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{{
			Text: systemPrompt,
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(podLog)),
		},
		Model: anthropic.ModelClaudeSonnet4_5_20250929,
	})
	if err != nil {
		panic(err.Error())
	}

	// Collect the reply text from the blocks
	var raw string
	for _, block := range message.Content {
		if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
			raw += textBlock.Text
		}
	}
	// Strip markdown fences if the model wrapped the JSON
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// Parse the JSON into our struct
	var diag Diagnosis
	if err := json.Unmarshal([]byte(raw), &diag); err != nil {
		fmt.Println("could not parse JSON from model:", err)
		fmt.Println("raw reply was:", raw)
		return
	}

	// Now it is real data the code can use
	fmt.Println("=== Sentinel diagnosis (structured) ===")
	fmt.Println("Severity:        ", diag.Severity)
	fmt.Println("Likely cause:    ", diag.LikelyCause)
	fmt.Println("Suggested action:", diag.SuggestedAction)

	// The payoff: code can act on the data
	if diag.Severity == "high" || diag.Severity == "critical" {
		fmt.Println(">> This would page an on-call engineer.")
	}
}
