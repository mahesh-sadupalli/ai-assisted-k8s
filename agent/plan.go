package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Plan is the part of `terraform show -json <planfile>` the agent reads.
type Plan struct {
	ResourceChanges []ResourceChange `json:"resource_changes"`
}

type ResourceChange struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Change  struct {
		Actions []string       `json:"actions"`
		Before  map[string]any `json:"before"`
		After   map[string]any `json:"after"`
	} `json:"change"`
}

// Finding is a risky change caught by a deterministic rule. These are always reported,
// whatever the model says, so the review never depends on the LLM noticing them.
type Finding struct {
	Address string `json:"address"`
	Rule    string `json:"rule"`
	Detail  string `json:"detail"`
}

func ParsePlan(data []byte) (Plan, error) {
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return Plan{}, fmt.Errorf("parse plan JSON (use `terraform show -json plan.out`): %w", err)
	}
	return p, nil
}

// Resources whose deletion loses data or takes the platform down.
var stateful = []string{
	"aws_db_instance", "aws_rds_cluster", "aws_dynamodb_table", "aws_s3_bucket",
	"aws_ebs_volume", "aws_efs_file_system", "aws_elasticache_cluster",
	"aws_eks_cluster", "aws_ecr_repository", "aws_kms_key",
}

func (rc ResourceChange) deletes() bool { return slices.Contains(rc.Change.Actions, "delete") }

func (rc ResourceChange) changed() bool {
	return !slices.Equal(rc.Change.Actions, []string{"no-op"}) && !slices.Equal(rc.Change.Actions, []string{"read"})
}

// Scan applies the rules to every change in the plan.
func Scan(p Plan) []Finding {
	var out []Finding
	for _, rc := range p.ResourceChanges {
		if rc.deletes() && slices.Contains(stateful, rc.Type) {
			what := "destroyed"
			if slices.Contains(rc.Change.Actions, "create") {
				what = "replaced (destroy then create)"
			}
			out = append(out, Finding{rc.Address, "stateful-destroy", fmt.Sprintf("%s will be %s: data loss / outage risk", rc.Type, what)})
		}
		if openToWorld(rc) {
			out = append(out, Finding{rc.Address, "open-ingress", "ingress from 0.0.0.0/0 or ::/0 on a port other than 80/443"})
		}
		if wildcardIAM(rc) {
			out = append(out, Finding{rc.Address, "iam-wildcard", `IAM policy allows Action "*"`})
		}
	}
	return out
}

func openToWorld(rc ResourceChange) bool {
	a := rc.Change.After
	if a == nil || !strings.HasPrefix(rc.Type, "aws_security_group") || a["type"] == "egress" {
		return false
	}
	world := false
	for _, key := range []string{"cidr_blocks", "ipv6_cidr_blocks", "cidr_ipv4", "cidr_ipv6"} {
		s := fmt.Sprint(a[key])
		world = world || strings.Contains(s, "0.0.0.0/0") || strings.Contains(s, "::/0")
	}
	if !world {
		return false
	}
	from, _ := a["from_port"].(float64)
	to, _ := a["to_port"].(float64)
	webOnly := (from == 80 && to == 80) || (from == 443 && to == 443)
	return !webOnly
}

func wildcardIAM(rc ResourceChange) bool {
	if !strings.HasPrefix(rc.Type, "aws_iam_") || rc.Change.After == nil {
		return false
	}
	policy, _ := rc.Change.After["policy"].(string)
	var doc struct {
		Statement []struct {
			Effect string `json:"Effect"`
			Action any    `json:"Action"`
		} `json:"Statement"`
	}
	if json.Unmarshal([]byte(policy), &doc) != nil {
		return false
	}
	for _, s := range doc.Statement {
		if s.Effect != "Allow" {
			continue
		}
		switch act := s.Action.(type) {
		case string:
			if act == "*" {
				return true
			}
		case []any:
			if slices.Contains(act, any("*")) {
				return true
			}
		}
	}
	return false
}

// Summary renders the changed resources compactly for the model.
func (p Plan) Summary() string {
	var b strings.Builder
	for _, rc := range p.ResourceChanges {
		if !rc.changed() {
			continue
		}
		before, _ := json.Marshal(rc.Change.Before)
		after, _ := json.Marshal(rc.Change.After)
		fmt.Fprintf(&b, "%s [%s]\n  before: %s\n  after:  %s\n", rc.Address, strings.Join(rc.Change.Actions, ","), before, after)
	}
	return b.String()
}
