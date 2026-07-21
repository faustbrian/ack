package ack

import (
	"strings"
	"testing"
)

const validIXSChangeset = `intent-changeset: 0.1.0
id: resume-settlement-jobs
summary: Resume settlement jobs interrupted during shutdown
rationale: Avoid manual recovery when deployments interrupt long jobs
targets:
  - release-unit: apps/worker
    type: reliability
    impact: patch
    audiences: [operators]
    migration: null
provenance:
  issues: [DEVELOP-219]
relations:
  reverts: []
  supersedes: []
disclosure: public
`

const validIntentChangesetV1 = `intent-changeset: 1.0.0
id: resume-settlement-jobs
source-repository: https://github.com/example/platform
summary: Resume settlement jobs interrupted during shutdown
rationale: Avoid manual recovery when deployments interrupt long jobs
targets:
  - release-unit: apps/worker
    stream: stable-v1
    type: reliability
    impact: patch
    audiences: [operators]
    migration: null
  - release-unit: apps/worker
    stream: lts-v1
    type: reliability
    impact: patch
    audiences: [operators]
    migration: null
provenance:
  issues: [DEVELOP-219]
relations:
  reverts: []
  supersedes: []
disclosure:
  state: embargoed
  not-before: 2026-08-14
  policy: SECURITY-EMBARGO
`

func TestParseIntentChangesetV2DistinguishesReleaseStreams(t *testing.T) {
	t.Parallel()

	changeset, err := ParseIXSChangeset([]byte(validIntentChangesetV1))
	if err != nil {
		t.Fatalf("ParseIXSChangeset() error = %v", err)
	}
	if changeset.Targets[0].Stream != "stable-v1" || changeset.Targets[1].Stream != "lts-v1" {
		t.Fatalf("targets = %#v", changeset.Targets)
	}
	if changeset.SourceRepository != "https://github.com/example/platform" {
		t.Fatalf("source repository = %q", changeset.SourceRepository)
	}
	if changeset.Disclosure.State != "embargoed" || changeset.Disclosure.NotBefore == nil {
		t.Fatalf("disclosure = %#v", changeset.Disclosure)
	}
}

func TestParseIXSChangesetAcceptsCompleteDecision(t *testing.T) {
	t.Parallel()

	changeset, err := ParseIXSChangeset([]byte(validIXSChangeset))
	if err != nil {
		t.Fatalf("ParseIXSChangeset() error = %v", err)
	}
	if changeset.ID != "resume-settlement-jobs" {
		t.Errorf("ID = %q", changeset.ID)
	}
}

func TestParseIXSChangesetRejectsDuplicateReleaseUnit(t *testing.T) {
	t.Parallel()

	duplicate := strings.Replace(validIXSChangeset, "provenance:\n", `  - release-unit: apps/worker
    type: reliability
    impact: minor
    audiences: [operators]
    migration: null
provenance:
`, 1)
	_, err := ParseIXSChangeset([]byte(duplicate))
	if err == nil || !strings.Contains(err.Error(), "duplicate release unit") {
		t.Fatalf("ParseIXSChangeset() error = %v", err)
	}
}

func TestParseIXSChangesetRequiresMigrationFieldForEveryTarget(t *testing.T) {
	t.Parallel()

	missing := strings.Replace(validIXSChangeset, "    migration: null\n", "", 1)
	_, err := ParseIXSChangeset([]byte(missing))
	if err == nil || !strings.Contains(err.Error(), "missing migration") {
		t.Fatalf("ParseIXSChangeset() error = %v, want missing migration", err)
	}
}

func TestMarshalIXSChangesetPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	input := strings.Replace(validIXSChangeset, "summary:", "x-owner: release-team\nsummary:", 1)
	changeset, err := ParseIXSChangeset([]byte(input))
	if err != nil {
		t.Fatalf("ParseIXSChangeset() error = %v", err)
	}

	output, err := MarshalIXSChangeset(changeset)
	if err != nil {
		t.Fatalf("MarshalIXSChangeset() error = %v", err)
	}
	if !strings.Contains(string(output), "x-owner: release-team") {
		t.Errorf("MarshalIXSChangeset() = %q, want unknown field", output)
	}
}

func TestValidateIXSChangesetAppliesProjectProfile(t *testing.T) {
	t.Parallel()

	changeset, err := ParseIXSChangeset([]byte(validIXSChangeset))
	if err != nil {
		t.Fatalf("ParseIXSChangeset() error = %v", err)
	}
	profile := Profile{
		IXSVersion:   "0.1.0",
		ReleaseUnits: map[string]ReleaseUnitPolicy{"apps/gateway": {Changelog: ".ack/changelog/apps-gateway.yaml"}},
		ReleaseTypes: []string{"feature"},
		Audiences:    []string{"end-users"},
		Disclosures:  []string{"public"},
	}

	err = ValidateIXSChangeset(changeset, profile)
	if err == nil || !strings.Contains(err.Error(), "release unit") {
		t.Fatalf("ValidateIXSChangeset() error = %v, want release unit error", err)
	}
}

func TestParseIXSChangesetRejectsEquivalentRationaleAndInvalidRelations(t *testing.T) {
	t.Parallel()

	t.Run("equivalent rationale", func(t *testing.T) {
		input := strings.Replace(validIXSChangeset,
			"rationale: Avoid manual recovery when deployments interrupt long jobs",
			"rationale: '  RESUME settlement jobs interrupted during shutdown  '", 1)
		_, err := ParseIXSChangeset([]byte(input))
		if err == nil || !strings.Contains(err.Error(), "rationale must not repeat summary") {
			t.Fatalf("ParseIXSChangeset() error = %v", err)
		}
	})

	t.Run("invalid relation", func(t *testing.T) {
		input := strings.Replace(validIXSChangeset, "reverts: []", "reverts: [NOT_VALID]", 1)
		_, err := ParseIXSChangeset([]byte(input))
		if err == nil || !strings.Contains(err.Error(), "invalid relation") {
			t.Fatalf("ParseIXSChangeset() error = %v", err)
		}
	})
}

func TestParseIntentRecordsRejectsNonPortableYAMLFeatures(t *testing.T) {
	t.Parallel()

	anchored := strings.Replace(validIntentChangesetV1,
		"summary: Resume settlement jobs interrupted during shutdown",
		"summary: &generated Resume settlement jobs interrupted during shutdown", 1)
	if _, err := ParseIXSChangeset([]byte(anchored)); err == nil || !strings.Contains(err.Error(), "anchors") {
		t.Fatalf("ParseIXSChangeset() error = %v, want anchors rejection", err)
	}

	tagged := strings.Replace(validIntentChangelogV1,
		"summary: Resume settlement jobs interrupted during shutdown",
		"summary: !generated Resume settlement jobs interrupted during shutdown", 1)
	if _, err := ParseICLSRecord([]byte(tagged)); err == nil || !strings.Contains(err.Error(), "custom YAML tag") {
		t.Fatalf("ParseICLSRecord() error = %v, want custom tag rejection", err)
	}
}
