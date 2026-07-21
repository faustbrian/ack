package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

const validPullRequestRecord = `intent-pull-request: 1.0.0
id: bind-service-token-audiences
title: "feat(apps/gateway/auth): bind service tokens to audiences"
state: ready
summary: Bind service tokens to the application that receives them
rationale: Prevent credentials issued for one service from being replayed elsewhere
approach: Validate the audience at the shared authentication boundary
trade-offs: Existing clients must add an audience before upgrading
targets:
  - release-unit: apps/gateway
    stream: stable
    impact: major
    migration: Add the gateway audience claim before upgrading
risks: [Existing clients without an audience will be rejected]
rollout: Deploy overlapping token issuers before enforcing audiences
rollback: Disable audience enforcement while retaining issued claims
verification:
  - name: unit-tests
    status: passed
    command: go test ./...
    evidence: CI run 481
provenance:
  issues: [SEC-241]
  changesets: [require-token-audiences]
disclosure:
  state: public
base-revision: fedcba9876543210fedcba9876543210fedcba98
evidence-revision: 0123456789abcdef0123456789abcdef01234567
`

func TestPullRequestCheckSupportsJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"pull-request", "check", "--format", "json", "-"}, strings.NewReader(validPullRequestRecord), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id":"bind-service-token-audiences"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestPullRequestCreateSupportsJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	repository := t.TempDir()
	profile := filepath.Join("..", "..", "examples", "ack.yaml")
	example := filepath.Join("..", "..", "examples", "pull-request.yaml")
	exitCode := Run([]string{
		"pull-request", "create", "--format", "json", "--repo", repository,
		"--profile", profile, example,
	}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id":"bind-service-token-audiences"`) ||
		!strings.Contains(stdout.String(), `"path":`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
