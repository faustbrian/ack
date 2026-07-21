package ack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const completeProfile = `ics: 0.1.0
ixs: 0.1.0
icls: 0.1.0
types:
  fix:
    default-impact: patch
  security:
    default-impact: patch
scopes:
  apps/worker:
    release-unit: apps/worker
release-units:
  apps/worker:
    changelog: .ack/changelog/apps-worker.yaml
ledger-directory: .ack/changelog
release-types: [reliability, security]
release-impacts: [none, patch, minor, major]
audiences: [operators, security-teams]
disclosures: [public, embargoed, redacted]
channels: [stable]
changesets:
  directory: .ack/changes
  archive-directory: .ack/archive/changes
  after-consumption: archive
  conflict-policy: preserve
  id-pattern: '^[a-z][a-z0-9-]+$'
  required-impacts: [minor, major]
  required-commit-types: [security]
`

func TestLoadProfileAcceptsInterconnectedSpecificationPolicy(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ack.yaml")
	if err := os.WriteFile(path, []byte(completeProfile), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	profile, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if profile.Changesets.Directory != ".ack/changes" {
		t.Errorf("Changesets.Directory = %q", profile.Changesets.Directory)
	}
	if profile.ReleaseUnits["apps/worker"].Changelog != ".ack/changelog/apps-worker.yaml" {
		t.Errorf("worker changelog mapping was not loaded")
	}
}

func TestLoadProfileAcceptsStandardIntentProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ack.yaml")
	contents := `intent-profile: 1.0.0
repository: https://github.com/example/platform
specifications:
  commits: 1.0.0
  pull-requests: 1.0.0
  changesets: 1.0.0
  changelog: 1.0.0
  release-manifest: 1.0.0
types:
  fix:
    default-impact: patch
scopes:
  apps/worker:
    release-unit: apps/worker
release-units:
  apps/worker:
    streams:
      stable-v1:
        release-line: "1"
        channel: stable
        changelog: .ack/changelog/apps-worker-stable-v1.yaml
        manifests: .ack/releases/apps-worker/stable-v1
ledger-directory: .ack/changelog
release-manifest-directory: .ack/releases
release-pattern: '^[0-9]+\.[0-9]+\.[0-9]+$'
release-types: [reliability]
release-impacts: [none, patch, minor, major]
audiences: [operators]
disclosures: [public, embargoed, redacted]
channels: [stable]
changesets:
  directory: .ack/changes
  archive-directory: .ack/archive/changes
  after-consumption: archive
  conflict-policy: preserve
pull-requests:
  directory: .ack/pull-requests
  merge-strategy: squash
  max-title-length: 72
  max-body-length: 20000
  require-ready: true
  verification-statuses: [passed, failed, skipped, unavailable, not-applicable]
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	profile, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if profile.ProfileVersion != "1.0.0" {
		t.Errorf("ProfileVersion = %q, want 1.0.0", profile.ProfileVersion)
	}
	if profile.PullRequests.MergeStrategy != "squash" {
		t.Errorf("PullRequests = %#v", profile.PullRequests)
	}
	stream := profile.ReleaseUnits["apps/worker"].Streams["stable-v1"]
	if stream.Channel != "stable" || stream.ReleaseLine != "1" {
		t.Errorf("stream = %#v", stream)
	}
}

func TestLoadProfileRejectsNonPortableYAML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ack.yaml")
	input := strings.Replace(completeProfile, "types:\n", "types: &shared-types\n", 1)
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadProfile(path); err == nil || !strings.Contains(err.Error(), "anchors") {
		t.Fatalf("LoadProfile() error = %v, want anchors rejection", err)
	}
}

func TestLoadProfileRequiresCompleteIXSPolicy(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ack.yaml")
	incomplete := strings.Replace(completeProfile, "release-impacts: [none, patch, minor, major]\n", "", 1)
	if err := os.WriteFile(path, []byte(incomplete), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadProfile(path)
	if err == nil || !strings.Contains(err.Error(), "release-impacts") {
		t.Fatalf("LoadProfile() error = %v, want release-impacts requirement", err)
	}
}

func TestLoadProfileRequiresLedgerDirectoryForICLS(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ack.yaml")
	incomplete := strings.Replace(completeProfile, "ledger-directory: .ack/changelog\n", "", 1)
	if err := os.WriteFile(path, []byte(incomplete), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := LoadProfile(path)
	if err == nil || !strings.Contains(err.Error(), "ledger-directory") {
		t.Fatalf("LoadProfile() error = %v, want ledger-directory requirement", err)
	}
}

func TestLoadProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ack.yaml")
	contents := []byte("ics: 0.1.0\n" +
		"types:\n" +
		"  fix:\n" +
		"    default-impact: patch\n" +
		"scopes:\n" +
		"  apps/billing:\n" +
		"    release-unit: apps/billing\n" +
		"release-units:\n" +
		"  apps/billing:\n" +
		"    changelog: .ack/changelog/apps-billing.yaml\n" +
		"require-release-ready: true\n")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	profile, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if profile.Types["fix"].DefaultImpact != ImpactPatch {
		t.Errorf("fix default = %q, want patch", profile.Types["fix"].DefaultImpact)
	}
	if !profile.RequireReleaseReady {
		t.Error("RequireReleaseReady = false, want true")
	}
}
