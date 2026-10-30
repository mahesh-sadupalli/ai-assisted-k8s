// Command agent is Sentinel's AI reliability agent.
//
//	agent summarize <logfile>   diagnose a log file ("-" reads stdin)
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/joho/godotenv"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: agent summarize <logfile>")
	os.Exit(2)
}

func readInput(path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

func printDiagnosis(d Diagnosis) {
	fmt.Println("=== Sentinel diagnosis ===")
	fmt.Println("Severity:        ", d.Severity)
	fmt.Println("What broke:      ", d.WhatBroke)
	fmt.Println("Likely cause:    ", d.LikelyCause)
	fmt.Println("Suggested action:", d.SuggestedAction)
	if d.Pages() {
		fmt.Println(">> This would page an on-call engineer.")
	}
}

func main() {
	godotenv.Load() // optional: .env with ANTHROPIC_API_KEY
	if len(os.Args) < 2 {
		usage()
	}
	client := anthropic.NewClient() // reads ANTHROPIC_API_KEY
	ctx := context.Background()

	switch os.Args[1] {
	case "summarize":
		if len(os.Args) != 3 {
			usage()
		}
		evidence, err := readInput(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "read log:", err)
			os.Exit(1)
		}
		d, err := Diagnose(ctx, client, evidence)
		if err != nil {
			fmt.Fprintln(os.Stderr, "diagnose:", err)
			os.Exit(1)
		}
		printDiagnosis(d)
	default:
		usage()
	}
}
