// Command agent is Sentinel's AI reliability agent.
//
//	agent summarize <logfile>   diagnose a log file ("-" reads stdin)
//	agent pod <namespace> <pod> diagnose a live pod (status, events, logs)
//	agent scan <namespace>      find unhealthy pods in a namespace and diagnose each
//	agent plan <plan.json>      review `terraform show -json` output; exit 1 on block
//	agent deploy-review <report> [diffstat]  assess a deploy; prints Markdown for CI
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/joho/godotenv"
	"k8s.io/client-go/kubernetes"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: agent summarize <logfile> | pod <namespace> <pod> | scan <namespace> | plan <plan.json> | deploy-review <report> [diffstat]")
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

func printReview(r PlanReview) {
	fmt.Println("=== Sentinel plan review ===")
	fmt.Println("Verdict:", strings.ToUpper(r.Verdict))
	fmt.Println("Summary:", r.Summary)
	if len(r.Findings) > 0 {
		fmt.Println("\nPolicy rules (deterministic):")
		for _, f := range r.Findings {
			fmt.Printf("  - [%s] %s: %s\n", f.Rule, f.Address, f.Detail)
		}
	}
	if len(r.Risks) > 0 {
		fmt.Println("\nAgent review:")
		for _, k := range r.Risks {
			fmt.Printf("  - (%s) %s: %s\n    -> %s\n", k.Severity, k.Address, k.Risk, k.Recommendation)
		}
	}
}

func fail(what string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
	os.Exit(1)
}

func diagnosePod(ctx context.Context, client anthropic.Client, cs kubernetes.Interface, ns, pod string) {
	evidence, err := PodEvidence(ctx, cs, ns, pod)
	if err != nil {
		fail("read pod", err)
	}
	d, err := Diagnose(ctx, client, evidence)
	if err != nil {
		fail("diagnose", err)
	}
	fmt.Printf("\n[%s/%s]\n", ns, pod)
	printDiagnosis(d)
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
	case "pod", "scan":
		cs, err := kubeClient()
		if err != nil {
			fail("kubeconfig", err)
		}
		if os.Args[1] == "pod" {
			if len(os.Args) != 4 {
				usage()
			}
			diagnosePod(ctx, client, cs, os.Args[2], os.Args[3])
			return
		}
		if len(os.Args) != 3 {
			usage()
		}
		pods, err := UnhealthyPods(ctx, cs, os.Args[2])
		if err != nil {
			fail("list pods", err)
		}
		if len(pods) == 0 {
			fmt.Println("all pods in", os.Args[2], "look healthy")
		}
		for _, p := range pods {
			diagnosePod(ctx, client, cs, p.Namespace, p.Name)
		}
	case "plan":
		if len(os.Args) != 3 {
			usage()
		}
		data, err := readInput(os.Args[2])
		if err != nil {
			fail("read plan", err)
		}
		plan, err := ParsePlan([]byte(data))
		if err != nil {
			fail("plan", err)
		}
		r, err := ReviewPlan(ctx, client, plan)
		if err != nil {
			fail("review", err)
		}
		printReview(r)
		if r.Verdict == "block" {
			os.Exit(1) // lets CI fail the PR check
		}
	case "deploy-review":
		if len(os.Args) < 3 || len(os.Args) > 4 {
			usage()
		}
		report, err := readInput(os.Args[2])
		if err != nil {
			fail("read report", err)
		}
		var stat string
		if len(os.Args) == 4 {
			if stat, err = readInput(os.Args[3]); err != nil {
				fail("read diffstat", err)
			}
		}
		r, err := ReviewDeploy(ctx, client, report, stat)
		if err != nil {
			fail("deploy review", err)
		}
		fmt.Print(r.Markdown())
	default:
		usage()
	}
}
