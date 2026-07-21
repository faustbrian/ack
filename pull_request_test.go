package ack

import (
	"strings"
	"testing"
)

const validPullRequest = `intent-pull-request: 1.0.0
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
risks:
  - Existing clients without an audience will be rejected
rollout: Deploy overlapping token issuers before enforcing audiences
rollback: Disable audience enforcement while retaining issued claims
verification:
  - name: unit-tests
    status: passed
    command: go test ./...
    evidence: CI run 481
  - name: production-smoke
    status: unavailable
    command: null
    evidence: No production access from pull-request CI
provenance:
  issues: [SEC-241]
  changesets: [require-token-audiences]
  decisions: [docs/decisions/0042-token-audiences.md]
disclosure:
  state: public
base-revision: fedcba9876543210fedcba9876543210fedcba98
evidence-revision: 0123456789abcdef0123456789abcdef01234567
`

func TestParsePullRequestAcceptsCompleteReadyRecord(t *testing.T) {
	t.Parallel()

	record, err := ParsePullRequest([]byte(validPullRequest))
	if err != nil {
		t.Fatalf("ParsePullRequest() error = %v", err)
	}
	if record.ID != "bind-service-token-audiences" || len(record.Targets) != 1 {
		t.Fatalf("record = %#v", record)
	}
	if body := RenderPullRequestBody(record); !strings.Contains(body, "## Why") || !strings.Contains(body, "CI run 481") {
		t.Fatalf("RenderPullRequestBody() = %q", body)
	}
}

func TestParsePullRequestRejectsReadyRecordWithoutRationale(t *testing.T) {
	t.Parallel()

	input := strings.Replace(validPullRequest,
		"rationale: Prevent credentials issued for one service from being replayed elsewhere\n", "", 1)
	_, err := ParsePullRequest([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "rationale") {
		t.Fatalf("ParsePullRequest() error = %v, want rationale error", err)
	}
}

func TestValidatePullRequestAppliesMonorepoProfile(t *testing.T) {
	t.Parallel()

	record, err := ParsePullRequest([]byte(validPullRequest))
	if err != nil {
		t.Fatalf("ParsePullRequest() error = %v", err)
	}
	profile := testStableIntentProfile()
	profile.ReleaseUnits = map[string]ReleaseUnitPolicy{
		"apps/worker": profile.ReleaseUnits["apps/worker"],
	}
	err = ValidatePullRequest(record, profile)
	if err == nil || !strings.Contains(err.Error(), "release unit") {
		t.Fatalf("ValidatePullRequest() error = %v, want release unit error", err)
	}
}

func TestParsePullRequestRequiresExactBaseAndDisclosure(t *testing.T) {
	t.Parallel()

	for name, removed := range map[string]string{
		"base revision": "base-revision: fedcba9876543210fedcba9876543210fedcba98\n",
		"disclosure":    "disclosure:\n  state: public\n",
	} {
		t.Run(name, func(t *testing.T) {
			input := strings.Replace(validPullRequest, removed, "", 1)
			_, err := ParsePullRequest([]byte(input))
			if err == nil {
				t.Fatal("ParsePullRequest() error = nil")
			}
		})
	}
}

func TestValidatePullRequestAppliesPullRequestPolicy(t *testing.T) {
	t.Parallel()

	record, err := ParsePullRequest([]byte(validPullRequest))
	if err != nil {
		t.Fatalf("ParsePullRequest() error = %v", err)
	}
	profile := testStableIntentProfile()
	profile.ReleaseUnits["apps/gateway"] = ReleaseUnitPolicy{Streams: map[string]ReleaseStreamPolicy{
		"stable": {ReleaseLine: "1", Channel: "stable"},
	}}
	profile.PullRequests.MaxTitleLength = 10
	err = ValidatePullRequest(record, profile)
	if err == nil || !strings.Contains(err.Error(), "title exceeds") {
		t.Fatalf("ValidatePullRequest() error = %v, want title limit error", err)
	}
}
