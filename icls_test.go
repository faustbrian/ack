package ack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validICLSRecord = `intent-changelog: 0.1.0
release-unit: apps/worker
release: 1.18.3
channel: stable
date: null
entries:
  - id: worker-219
    type: reliability
    summary: Resume settlement jobs interrupted during shutdown
    rationale: Avoid manual recovery after deployments
    impact: patch
    audiences: [operators]
    migration: null
    affects: [apps/worker]
    provenance:
      commits: [9af23e771b40]
    relations:
      reverts: []
      supersedes: []
    disclosure: public
`

const validIntentChangelogV1 = `intent-changelog: 1.0.0
release-unit: apps/worker
release: 1.18.3
source-repository: https://github.com/example/platform
stream: stable-v1
release-line: "1"
channel: stable
date: null
entries:
  - id: resume-settlement-jobs
    type: reliability
    summary: Resume settlement jobs interrupted during shutdown
    rationale: Avoid manual recovery after deployments
    impact: patch
    audiences: [operators]
    migration: null
    affects: [apps/worker]
    provenance:
      commits: [9af23e771b40]
      changesets: [resume-settlement-jobs]
    relations:
      reverts: []
      supersedes: []
    disclosure:
      state: public
amendments:
  - id: clarify-worker-rationale
    date: 2026-08-15
    summary: Clarify why interrupted jobs are resumed
    provenance:
      issues: [DEVELOP-220]
`

func TestParseIntentChangelogV2PreservesStreamAndAmendments(t *testing.T) {
	t.Parallel()

	record, err := ParseICLSRecord([]byte(validIntentChangelogV1))
	if err != nil {
		t.Fatalf("ParseICLSRecord() error = %v", err)
	}
	if record.Stream != "stable-v1" || record.ReleaseLine != "1" {
		t.Fatalf("record stream = %q, release line = %q", record.Stream, record.ReleaseLine)
	}
	if record.SourceRepository != "https://github.com/example/platform" {
		t.Fatalf("source repository = %q", record.SourceRepository)
	}
	if len(record.Amendments) != 1 || record.Amendments[0].ID != "clarify-worker-rationale" {
		t.Fatalf("amendments = %#v", record.Amendments)
	}
}

func TestParseICLSRecordAcceptsReleaseReadyUnreleasedRecord(t *testing.T) {
	t.Parallel()

	record, err := ParseICLSRecord([]byte(validICLSRecord))
	if err != nil {
		t.Fatalf("ParseICLSRecord() error = %v", err)
	}
	if record.ReleaseUnit != "apps/worker" {
		t.Errorf("ReleaseUnit = %q, want apps/worker", record.ReleaseUnit)
	}
	if record.Date != nil {
		t.Errorf("Date = %v, want nil", record.Date)
	}
}

func TestParseICLSRecordRejectsMissingRationale(t *testing.T) {
	t.Parallel()

	input := strings.Replace(validICLSRecord, "    rationale: Avoid manual recovery after deployments\n", "", 1)
	_, err := ParseICLSRecord([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "rationale") {
		t.Fatalf("ParseICLSRecord() error = %v, want missing rationale", err)
	}
}

func TestPublishICLSRecordChangesOnlyDate(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "release.yaml")
	if err := os.WriteFile(path, []byte(validICLSRecord), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PublishICLSRecord(path, "2026-07-20"); err != nil {
		t.Fatalf("PublishICLSRecord() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := strings.Replace(validICLSRecord, "date: null", "date: 2026-07-20", 1)
	if string(contents) != want {
		t.Errorf("published record changed beyond date\n%s", contents)
	}
}

func TestRenderICLSRecordIncludesRationaleAndHidesEmbargoedEntries(t *testing.T) {
	t.Parallel()

	input := strings.Replace(validICLSRecord, "    disclosure: public\n", `    disclosure: public
  - id: worker-220
    type: security
    summary: Reject forged settlement callbacks
    rationale: Prevent unauthenticated settlement transitions
    impact: patch
    audiences: [security-teams]
    migration: null
    affects: [apps/worker]
    provenance:
      advisories: [PRIVATE-220]
    relations:
      reverts: []
      supersedes: []
    disclosure: embargoed
`, 1)
	record, err := ParseICLSRecord([]byte(input))
	if err != nil {
		t.Fatalf("ParseICLSRecord() error = %v", err)
	}

	output := RenderICLSRecord(record)
	if !strings.Contains(output, "Why: Avoid manual recovery after deployments") {
		t.Errorf("output missing rationale:\n%s", output)
	}
	if strings.Contains(output, "forged settlement callbacks") || strings.Contains(output, "PRIVATE-220") {
		t.Errorf("output disclosed embargoed entry:\n%s", output)
	}
}
