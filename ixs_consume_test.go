package ack

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGenerateICLSRecordsConsumesLinkedCommitsAndChangesets(t *testing.T) {
	t.Parallel()

	repository := createLinkedCommitRepository(t, "patch")
	profile := testWorkflowProfile()
	if _, err := InitializeICLSRecord(repository, profile, "apps/worker", "1.18.3", "stable"); err != nil {
		t.Fatalf("InitializeICLSRecord() error = %v", err)
	}
	if _, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset)); err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}

	results, err := GenerateICLSRecords(context.Background(), repository, "HEAD", profile)
	if err != nil {
		t.Fatalf("GenerateICLSRecords() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	record := readTestICLSRecord(t, repository, profile, "apps/worker")
	if len(record.Entries) != 1 || len(record.Entries[0].Provenance["commits"]) != 1 {
		t.Errorf("entries = %#v, want linked commit provenance", record.Entries)
	}
}

func TestConsumeIXSChangesetCreatesCanonicalICLSEntryAndArchivesDecision(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	if _, err := InitializeICLSRecord(repository, profile, "apps/worker", "1.18.3", "stable"); err != nil {
		t.Fatalf("InitializeICLSRecord() error = %v", err)
	}
	changesetPath, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset))
	if err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}

	result, err := ConsumeIXSChangeset(repository, profile, changesetPath, nil)
	if err != nil {
		t.Fatalf("ConsumeIXSChangeset() error = %v", err)
	}
	if result.ArchivedTo == "" {
		t.Errorf("ArchivedTo is empty")
	}
	if _, err := os.Stat(changesetPath); !os.IsNotExist(err) {
		t.Errorf("pending changeset still exists: %v", err)
	}
	archived, err := os.ReadFile(result.ArchivedTo)
	if err != nil {
		t.Fatalf("ReadFile(archive) error = %v", err)
	}
	if string(archived) != validIXSChangeset {
		t.Errorf("archived changeset changed:\n%s", archived)
	}

	record := readTestICLSRecord(t, repository, profile, "apps/worker")
	if len(record.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(record.Entries))
	}
	entry := record.Entries[0]
	if entry.ID != "resume-settlement-jobs" || entry.Rationale != "Avoid manual recovery when deployments interrupt long jobs" {
		t.Errorf("entry = %#v", entry)
	}
	if !slices.Contains(entry.Provenance["changesets"], "resume-settlement-jobs") {
		t.Errorf("provenance = %#v, want changeset identifier", entry.Provenance)
	}
}

func TestConsumeIXSChangesetResolvesRelativePathInsideRepository(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	if _, err := InitializeICLSRecord(repository, profile, "apps/worker", "1.18.3", "stable"); err != nil {
		t.Fatalf("InitializeICLSRecord() error = %v", err)
	}
	if _, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset)); err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}
	relative := filepath.Join(".ack", "changes", "resume-settlement-jobs.yaml")
	if _, err := ConsumeIXSChangeset(repository, profile, relative, nil); err != nil {
		t.Fatalf("ConsumeIXSChangeset() error = %v", err)
	}
}

func TestConsumeIXSChangesetAppliesDeleteDispositionAfterVerification(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	profile.Changesets.AfterConsumption = "delete"
	if _, err := InitializeICLSRecord(repository, profile, "apps/worker", "1.18.3", "stable"); err != nil {
		t.Fatalf("InitializeICLSRecord() error = %v", err)
	}
	path, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset))
	if err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}
	result, err := ConsumeIXSChangeset(repository, profile, path, nil)
	if err != nil {
		t.Fatalf("ConsumeIXSChangeset() error = %v", err)
	}
	if !result.Deleted {
		t.Errorf("Deleted = false, want true")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("pending record still exists: %v", err)
	}
}

func TestConsumeIXSChangesetProtectsEditedICLSEntry(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	changesetPath, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset))
	if err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}
	path := filepath.Join(repository, profile.ReleaseUnits["apps/worker"].Changelog)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	edited := strings.Replace(validICLSRecord, "id: worker-219", "id: resume-settlement-jobs", 1)
	edited = strings.Replace(edited, "summary: Resume interrupted settlement jobs during shutdown", "summary: Editorial wording", 1)
	edited = strings.Replace(edited, "commits: [9af23e771b40]", "changesets: [resume-settlement-jobs]", 1)
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err = ConsumeIXSChangeset(repository, profile, changesetPath, nil)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("ConsumeIXSChangeset() error = %v, want conflict", err)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(contents) != edited {
		t.Errorf("edited record was overwritten:\n%s", contents)
	}
	if _, statErr := os.Stat(changesetPath); statErr != nil {
		t.Errorf("pending changeset was removed after conflict: %v", statErr)
	}
}

func TestConsumeIXSChangesetPreflightsEveryTargetBeforeWriting(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	profile.ReleaseUnits["apps/gateway"] = ReleaseUnitPolicy{Changelog: ".ack/changelog/apps-gateway.yaml"}
	profile.Scopes["apps/gateway"] = ScopePolicy{ReleaseUnit: "apps/gateway"}
	if _, err := InitializeICLSRecord(repository, profile, "apps/worker", "1.18.3", "stable"); err != nil {
		t.Fatalf("InitializeICLSRecord() error = %v", err)
	}
	multi := strings.Replace(validIXSChangeset, "provenance:\n", `  - release-unit: apps/gateway
    type: reliability
    impact: patch
    audiences: [operators]
    migration: null
provenance:
`, 1)
	changesetPath, err := CreateIXSChangeset(repository, profile, []byte(multi))
	if err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}

	_, err = ConsumeIXSChangeset(repository, profile, changesetPath, nil)
	if err == nil || !strings.Contains(err.Error(), "apps/gateway") {
		t.Fatalf("ConsumeIXSChangeset() error = %v, want missing gateway record", err)
	}
	record := readTestICLSRecord(t, repository, profile, "apps/worker")
	if len(record.Entries) != 0 {
		t.Errorf("worker record was modified before preflight completed")
	}
}

func TestGatePendingChangesetsRequiresEveryTargetToBeConsumed(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := testWorkflowProfile()
	profile.Changesets.AfterConsumption = "keep"
	if _, err := InitializeICLSRecord(repository, profile, "apps/worker", "1.18.3", "stable"); err != nil {
		t.Fatalf("InitializeICLSRecord() error = %v", err)
	}
	changesetPath, err := CreateIXSChangeset(repository, profile, []byte(validIXSChangeset))
	if err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}

	before, err := GatePendingChangesets(repository, profile)
	if err != nil {
		t.Fatalf("GatePendingChangesets() error = %v", err)
	}
	if !before.HasErrors() {
		t.Errorf("gate passed before consumption")
	}
	if _, err := ConsumeIXSChangeset(repository, profile, changesetPath, nil); err != nil {
		t.Fatalf("ConsumeIXSChangeset() error = %v", err)
	}
	after, err := GatePendingChangesets(repository, profile)
	if err != nil {
		t.Fatalf("GatePendingChangesets() error = %v", err)
	}
	if after.HasErrors() {
		t.Errorf("gate diagnostics = %#v, want pass", after.Diagnostics)
	}
}

func TestConsumeStableIntentChangesetRoutesEveryReleaseStream(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profile := Profile{
		ProfileVersion: "1.0.0",
		Repository:     "https://github.com/example/platform",
		Specifications: SpecificationPolicy{
			Commits: "1.0.0", Changesets: "1.0.0", Changelog: "1.0.0", ReleaseManifest: "1.0.0",
		},
		Types:  map[string]TypePolicy{"fix": {DefaultImpact: ImpactPatch}},
		Scopes: map[string]ScopePolicy{"apps/worker": {ReleaseUnit: "apps/worker"}},
		ReleaseUnits: map[string]ReleaseUnitPolicy{
			"apps/worker": {Streams: map[string]ReleaseStreamPolicy{
				"stable-v1": {
					ReleaseLine: "1", Channel: "stable",
					Changelog: ".ack/changelog/apps-worker-stable-v1.yaml",
					Manifests: ".ack/releases/apps-worker/stable-v1",
				},
				"lts-v1": {
					ReleaseLine: "1", Channel: "lts",
					Changelog: ".ack/changelog/apps-worker-lts-v1.yaml",
					Manifests: ".ack/releases/apps-worker/lts-v1",
				},
			}},
		},
		LedgerDirectory:   ".ack/changelog",
		ManifestDirectory: ".ack/releases",
		ReleasePattern:    `^[0-9]+\.[0-9]+\.[0-9]+$`,
		ReleaseTypes:      []string{"reliability"},
		ReleaseImpacts:    []Impact{ImpactNone, ImpactPatch, ImpactMinor, ImpactMajor},
		Audiences:         []string{"operators"},
		Disclosures:       []string{"public", "embargoed", "redacted"},
		Channels:          []string{"stable", "lts"},
		Changesets: ChangesetPolicy{
			Directory: ".ack/changes", ArchiveDirectory: ".ack/archive/changes",
			AfterConsumption: "archive", ConflictPolicy: "preserve",
		},
	}
	for stream, release := range map[string]string{"stable-v1": "1.19.0", "lts-v1": "1.18.4"} {
		if _, err := InitializeICLSStreamRecord(repository, profile, "apps/worker", stream, release); err != nil {
			t.Fatalf("InitializeICLSStreamRecord(%s) error = %v", stream, err)
		}
	}
	path, err := CreateIXSChangeset(repository, profile, []byte(validIntentChangesetV1))
	if err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}
	if _, err := ConsumeIXSChangeset(repository, profile, path, nil); err != nil {
		t.Fatalf("ConsumeIXSChangeset() error = %v", err)
	}
	for stream, policy := range profile.ReleaseUnits["apps/worker"].Streams {
		contents, err := os.ReadFile(filepath.Join(repository, policy.Changelog))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", stream, err)
		}
		record, err := ParseICLSRecord(contents)
		if err != nil || len(record.Entries) != 1 || record.Stream != stream {
			t.Fatalf("record %s = %#v, error = %v", stream, record, err)
		}
	}
}

func readTestICLSRecord(t *testing.T, repository string, profile Profile, releaseUnit string) ICLSRecord {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(repository, profile.ReleaseUnits[releaseUnit].Changelog))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	record, err := ParseICLSRecord(contents)
	if err != nil {
		t.Fatalf("ParseICLSRecord() error = %v", err)
	}
	return record
}
