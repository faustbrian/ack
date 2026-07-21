package ack

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const validReleaseManifest = `intent-release-manifest: 1.0.0
release-unit: apps/worker
release: 1.18.3
stream: stable-v1
release-line: "1"
channel: stable
published-at: 2026-07-20T12:30:00Z
source:
  repository: https://github.com/example/platform
  commit: 9af23e771b409af23e771b409af23e771b409af2
  tag: worker/v1.18.3
changelog:
  path: .ack/changelog/apps-worker-stable-v1.yaml
  digest: sha256:ee22177d52d584c55b283a70f06f9f31eb3b72d45f775905e2c9c8bb99eecf90
  changesets: [resume-settlement-jobs]
artifacts:
  - name: worker-linux-arm64
    uri: dist/worker-linux-arm64
    digest: sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
    media-type: application/vnd.example.binary
provenance:
  sboms: [dist/worker-linux-arm64.spdx.json]
`

func TestParseReleaseManifestCapturesShippedEvidence(t *testing.T) {
	t.Parallel()

	manifest, err := ParseReleaseManifest([]byte(validReleaseManifest))
	if err != nil {
		t.Fatalf("ParseReleaseManifest() error = %v", err)
	}
	if manifest.Stream != "stable-v1" || manifest.Source.Tag != "worker/v1.18.3" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if len(manifest.Artifacts) != 1 || manifest.Artifacts[0].Name != "worker-linux-arm64" {
		t.Fatalf("artifacts = %#v", manifest.Artifacts)
	}
}

func TestParseReleaseManifestRejectsWeakOrMissingEvidence(t *testing.T) {
	t.Parallel()

	weakDigest := strings.Replace(validReleaseManifest,
		"sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"9f86d081", 1)
	if _, err := ParseReleaseManifest([]byte(weakDigest)); err == nil || !strings.Contains(err.Error(), "artifact digest") {
		t.Fatalf("ParseReleaseManifest() error = %v, want artifact digest error", err)
	}

	missingArtifacts := strings.Replace(validReleaseManifest, "artifacts:\n  - name: worker-linux-arm64\n    uri: dist/worker-linux-arm64\n    digest: sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n    media-type: application/vnd.example.binary\n", "artifacts: []\n", 1)
	if _, err := ParseReleaseManifest([]byte(missingArtifacts)); err == nil || !strings.Contains(err.Error(), "artifacts must not be empty") {
		t.Fatalf("ParseReleaseManifest() error = %v, want artifacts error", err)
	}
}

func TestVerifyReleaseManifestBindsSourceChangelogAndArtifacts(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "config", "user.name", "ACK Test")
	runGit(t, repository, "config", "user.email", "ack@example.test")
	if err := os.WriteFile(filepath.Join(repository, "worker.go"), []byte("package worker\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, repository, "add", "worker.go")
	runGit(t, repository, "commit", "--quiet", "-m", "fix(apps/worker): ship worker", "-m", "Impact: patch")
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	commit := strings.TrimSpace(string(output))

	profile := testStableIntentProfile()
	changelog := strings.Replace(validIntentChangelogV1, "date: null", "date: 2026-07-20", 1)
	changelogPath := filepath.Join(repository, ".ack/changelog/apps-worker-stable-v1.yaml")
	if err := os.MkdirAll(filepath.Dir(changelogPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(changelogPath, []byte(changelog), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	artifactPath := filepath.Join(repository, "dist/worker-linux-arm64")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("test"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	digest := sha256.Sum256([]byte(changelog))
	input := strings.Replace(validReleaseManifest,
		"sha256:ee22177d52d584c55b283a70f06f9f31eb3b72d45f775905e2c9c8bb99eecf90",
		fmt.Sprintf("sha256:%x", digest), 1)
	input = strings.Replace(input, "9af23e771b409af23e771b409af23e771b409af2", commit, 1)
	manifest, err := ParseReleaseManifest([]byte(input))
	if err != nil {
		t.Fatalf("ParseReleaseManifest() error = %v", err)
	}
	if err := VerifyReleaseManifest(repository, profile, manifest); err != nil {
		t.Fatalf("VerifyReleaseManifest() error = %v", err)
	}
	manifestPath, err := CreateReleaseManifest(repository, profile, []byte(input))
	if err != nil {
		t.Fatalf("CreateReleaseManifest() error = %v", err)
	}
	if manifestPath != filepath.Join(repository, ".ack/releases/apps-worker/stable-v1/1.18.3.yaml") {
		t.Fatalf("manifest path = %q", manifestPath)
	}

	if err := os.WriteFile(artifactPath, []byte("changed"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := VerifyReleaseManifest(repository, profile, manifest); err == nil || !strings.Contains(err.Error(), "artifact digest") {
		t.Fatalf("VerifyReleaseManifest() error = %v, want artifact digest error", err)
	}
}

func testStableIntentProfile() Profile {
	return Profile{
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
			}},
		},
		LedgerDirectory:   ".ack/changelog",
		ManifestDirectory: ".ack/releases",
		ReleasePattern:    `^[0-9]+\.[0-9]+\.[0-9]+$`,
		ReleaseTypes:      []string{"reliability"},
		ReleaseImpacts:    []Impact{ImpactNone, ImpactPatch, ImpactMinor, ImpactMajor},
		Audiences:         []string{"operators"},
		Disclosures:       []string{"public", "embargoed", "redacted"},
		Channels:          []string{"stable"},
		Changesets: ChangesetPolicy{
			Directory: ".ack/changes", ArchiveDirectory: ".ack/archive/changes",
			AfterConsumption: "archive", ConflictPolicy: "preserve",
		},
	}
}
