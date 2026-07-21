package ack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testWorkflowProfile() Profile {
	return Profile{
		ICSVersion:      "0.1.0",
		IXSVersion:      "0.1.0",
		ICLSVersion:     "0.1.0",
		Types:           map[string]TypePolicy{"fix": {DefaultImpact: ImpactPatch}},
		Scopes:          map[string]ScopePolicy{"apps/worker": {ReleaseUnit: "apps/worker"}},
		ReleaseUnits:    map[string]ReleaseUnitPolicy{"apps/worker": {Changelog: ".ack/changelog/apps-worker.yaml"}},
		LedgerDirectory: ".ack/changelog",
		ReleaseTypes:    []string{"reliability"},
		ReleaseImpacts:  []Impact{ImpactNone, ImpactPatch, ImpactMinor, ImpactMajor},
		Audiences:       []string{"operators"},
		Disclosures:     []string{"public"},
		Channels:        []string{"stable"},
		Changesets: ChangesetPolicy{
			Directory:        ".ack/changes",
			ArchiveDirectory: ".ack/archive/changes",
			AfterConsumption: "archive",
			ConflictPolicy:   "preserve",
			RequiredImpacts:  []Impact{ImpactMinor, ImpactMajor},
		},
	}
}

func TestCreateIXSChangesetRejectsIdentifierInHistoricalLedgerRecord(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	historicalPath := filepath.Join(repository, ".ack/changelog/history/apps-worker-1.18.3.yaml")
	if err := os.MkdirAll(filepath.Dir(historicalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	record := strings.Replace(validICLSRecord, "commits: [9af23e771b40]", "changesets: [resume-settlement-jobs]", 1)
	if err := os.WriteFile(historicalPath, []byte(record), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateIXSChangeset() error = %v, want historical duplicate", err)
	}
}

func TestCreateIXSChangesetUsesCanonicalDirectoryAndPreservesSource(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	input := strings.Replace(validIXSChangeset, "summary:", "x-owner: release-team\nsummary:", 1)
	path, err := CreateIXSChangeset(repository, testWorkflowProfile(), []byte(input))
	if err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}
	wantPath := filepath.Join(repository, ".ack/changes/resume-settlement-jobs.yaml")
	if path != wantPath {
		t.Errorf("path = %q, want %q", path, wantPath)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != input {
		t.Errorf("created contents changed:\n%s", contents)
	}
}

func TestCreateIXSChangesetRejectsIdentifierAlreadyInLedger(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	recordPath := filepath.Join(repository, profile.ReleaseUnits["apps/worker"].Changelog)
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	record := strings.Replace(validICLSRecord, "commits: [9af23e771b40]", "changesets: [resume-settlement-jobs]", 1)
	if err := os.WriteFile(recordPath, []byte(record), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateIXSChangeset() error = %v, want duplicate identifier", err)
	}
}

func TestInitializeICLSRecordCreatesMappedUnreleasedLedger(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	path, err := InitializeICLSRecord(repository, testWorkflowProfile(), "apps/worker", "1.18.3", "stable")
	if err != nil {
		t.Fatalf("InitializeICLSRecord() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	record, err := ParseICLSRecord(contents)
	if err != nil {
		t.Fatalf("ParseICLSRecord() error = %v", err)
	}
	if record.Date != nil || len(record.Entries) != 0 {
		t.Errorf("record = %#v, want empty unreleased record", record)
	}
}
