package main

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("warning: no .env file found, relying on shell environment")
	}

	client := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)

	// 1. PERCEIVE — the broken pod the agent must diagnose
	podLog := `Name:          payment-service-7d9f8b6c4-xk2lp
               Namespace:     production
               Status:        Running
               Restart Count: 5
               Last State:    Terminated
               Reason:      OOMKilled
               Exit Code:   137
               Events: Warning  BackOff  Back-off restarting failed container payment-service`

	// 2. Build the prompt: role + the log + what we want back
	prompt := fmt.Sprintf(`You are an experienced Site Reliability Engineer.

                           Here is the status of a failing Kubernetes pod:

                           %s

                           In plain English, explain:
                           1. What went wrong
                           2. The most likely cause
                           3. What to check or try first

                           Keep it concise.`, podLog)

	// 3. REASON — ask Claude to diagnose
	message, err := client.Messages.New(context.TODO(), anthropic.MessageNewParams{
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
		Model: anthropic.ModelClaudeSonnet4_5_20250929,
	})
	if err != nil {
		panic(err.Error())
	}

	// Print the diagnosis
	fmt.Println("=== Sentinel diagnosis ===")
	for _, block := range message.Content {
		if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
			fmt.Println(textBlock.Text)
		}
	}
}
