package ack

import (
	"os"
	"slices"
	"testing"
)

func TestExamplesFormValidInterconnectedWorkflow(t *testing.T) {
	t.Parallel()

	profile, err := LoadProfile("examples/ack.yaml")
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	changesetContents, err := os.ReadFile("examples/changeset.yaml")
	if err != nil {
		t.Fatalf("ReadFile(changeset) error = %v", err)
	}
	changeset, err := ParseIXSChangeset(changesetContents)
	if err != nil {
		t.Fatalf("ParseIXSChangeset() error = %v", err)
	}
	if err := ValidateIXSChangeset(changeset, profile); err != nil {
		t.Fatalf("ValidateIXSChangeset() error = %v", err)
	}
	recordContents, err := os.ReadFile("examples/changelog.yaml")
	if err != nil {
		t.Fatalf("ReadFile(changelog) error = %v", err)
	}
	record, err := ParseICLSRecord(recordContents)
	if err != nil {
		t.Fatalf("ParseICLSRecord() error = %v", err)
	}
	if err := ValidateICLSRecordProfile(record, profile); err != nil {
		t.Fatalf("ValidateICLSRecordProfile() error = %v", err)
	}
	if record.Entries[0].ID != changeset.ID || !slices.Contains(record.Entries[0].Provenance["changesets"], changeset.ID) {
		t.Errorf("changelog entry does not retain changeset identity")
	}
	manifestContents, err := os.ReadFile("examples/release.yaml")
	if err != nil {
		t.Fatalf("ReadFile(release) error = %v", err)
	}
	manifest, err := ParseReleaseManifest(manifestContents)
	if err != nil {
		t.Fatalf("ParseReleaseManifest() error = %v", err)
	}
	if err := ValidateReleaseManifestProfile(manifest, profile); err != nil {
		t.Fatalf("ValidateReleaseManifestProfile() error = %v", err)
	}
}
