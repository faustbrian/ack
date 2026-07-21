package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const validChangelogRecord = `intent-changelog: 0.1.0
release-unit: apps/worker
release: 1.18.3
channel: stable
date: null
entries:
  - id: worker-219
    type: reliability
    summary: Resume interrupted settlement jobs
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

const validChangeset = `intent-changeset: 0.1.0
id: resume-settlement-jobs
summary: Resume interrupted settlement jobs
rationale: Avoid manual recovery after deployments
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

const workflowProfile = `ics: 0.1.0
ixs: 0.1.0
icls: 0.1.0
types:
  fix:
    default-impact: patch
scopes:
  apps/worker:
    release-unit: apps/worker
release-units:
  apps/worker:
    changelog: .ack/changelog/apps-worker.yaml
ledger-directory: .ack/changelog
release-types: [reliability]
release-impacts: [none, patch, minor, major]
audiences: [operators]
disclosures: [public]
channels: [stable]
changesets:
  directory: .ack/changes
  archive-directory: .ack/archive/changes
  after-consumption: archive
  conflict-policy: preserve
  required-impacts: [minor, major]
`

func TestCommitCheckReportsICSViolation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "message.txt")
	if err := os.WriteFile(path, []byte("fix(api): reject ambiguous payloads\n\nImpact: major\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"commit", "check", path}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), "missing-migration") {
		t.Errorf("stdout = %q, want missing-migration", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestLintReportsLargeDiffWarningWithoutFailing(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "config", "user.name", "ACK Test")
	runGit(t, repository, "config", "user.email", "ack@example.test")
	if err := os.WriteFile(filepath.Join(repository, "api.go"), []byte("package api\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, repository, "add", "api.go")
	runGit(t, repository, "commit", "--quiet", "-m", "fix(api): add package", "-m", "Impact: patch")

	profile := filepath.Join(repository, "ack.yaml")
	if err := os.WriteFile(profile, []byte("ics: 0.1.0\nlarge-diff-threshold: 1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"commit", "lint", "--repo", repository, "--profile", profile, "HEAD"}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "large-diff-low-impact") {
		t.Errorf("stdout = %q, want large-diff-low-impact", stdout.String())
	}
}

func TestChangelogPreservesProjectDefinedTypes(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "config", "user.name", "ACK Test")
	runGit(t, repository, "config", "user.email", "ack@example.test")
	if err := os.WriteFile(filepath.Join(repository, "policy.md"), []byte("rotated\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, repository, "add", "policy.md")
	runGit(t, repository, "commit", "--quiet", "-m", "security(auth): rotate leaked key", "-m", "Impact: patch")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"changelog", "source", "--repo", repository, "HEAD"}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "**security**") {
		t.Errorf("stdout = %q, want security type", stdout.String())
	}
}

func TestCLIConnectsICSIXSAndICLSIntoCanonicalChangelog(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "config", "user.name", "ACK Test")
	runGit(t, repository, "config", "user.email", "ack@example.test")
	if err := os.WriteFile(filepath.Join(repository, "worker.go"), []byte("package worker\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, repository, "add", "worker.go")
	runGit(t, repository, "commit", "--quiet", "-m", "fix(apps/worker): resume interrupted jobs", "-m", "Impact: patch\nChangeset: resume-settlement-jobs")
	profilePath := filepath.Join(repository, "ack.yaml")
	if err := os.WriteFile(profilePath, []byte(workflowProfile), 0o600); err != nil {
		t.Fatalf("WriteFile(profile) error = %v", err)
	}

	runCLI := func(args []string, input string) (int, string, string) {
		t.Helper()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(args, strings.NewReader(input), &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	if code, _, stderr := runCLI([]string{"changelog", "init", "--repo", repository, "--profile", profilePath, "--release-unit", "apps/worker", "--release", "1.18.3", "--channel", "stable"}, ""); code != 0 {
		t.Fatalf("changelog init exit = %d; stderr = %q", code, stderr)
	}
	if code, _, stderr := runCLI([]string{"changeset", "create", "--repo", repository, "--profile", profilePath, "-"}, validChangeset); code != 0 {
		t.Fatalf("changeset create exit = %d; stderr = %q", code, stderr)
	}
	if code, _, stderr := runCLI([]string{"changeset", "links", "--repo", repository, "--profile", profilePath, "HEAD"}, ""); code != 0 {
		t.Fatalf("changeset links exit = %d; stderr = %q", code, stderr)
	}
	if code, _, _ := runCLI([]string{"changeset", "gate", "--repo", repository, "--profile", profilePath}, ""); code != 1 {
		t.Fatalf("changeset gate before generation exit = %d, want 1", code)
	}
	if code, stdout, stderr := runCLI([]string{"changelog", "generate", "--repo", repository, "--profile", profilePath, "HEAD"}, ""); code != 0 {
		t.Fatalf("changelog generate exit = %d; stderr = %q", code, stderr)
	} else if !strings.Contains(stdout, "consumed 1 pending changesets") {
		t.Errorf("changelog generate stdout = %q", stdout)
	}
	if code, _, stderr := runCLI([]string{"changeset", "gate", "--repo", repository, "--profile", profilePath}, ""); code != 0 {
		t.Fatalf("changeset gate after generation exit = %d; stderr = %q", code, stderr)
	}

	recordContents, err := os.ReadFile(filepath.Join(repository, ".ack/changelog/apps-worker.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(changelog) error = %v", err)
	}
	if !strings.Contains(string(recordContents), "changesets:") || !strings.Contains(string(recordContents), "resume-settlement-jobs") || !strings.Contains(string(recordContents), "commits:") {
		t.Errorf("canonical changelog lacks linked provenance:\n%s", recordContents)
	}
}

func TestChangelogCheckValidatesICLSRecord(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "release.yaml")
	if err := os.WriteFile(path, []byte(validChangelogRecord), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"changelog", "check", path}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid Intent Changelog record") {
		t.Errorf("stdout = %q, want validation result", stdout.String())
	}
}

func TestChangelogRenderProducesPublicMarkdown(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "release.yaml")
	if err := os.WriteFile(path, []byte(validChangelogRecord), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"changelog", "render", path}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Why: Avoid manual recovery") {
		t.Errorf("stdout = %q, want rationale", stdout.String())
	}
}

func TestChangelogPublishChangesOnlyReleaseDate(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "release.yaml")
	if err := os.WriteFile(path, []byte(validChangelogRecord), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"changelog", "publish", "--date", "2026-07-20", path}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Count(string(contents), "date: 2026-07-20") != 1 || strings.Contains(string(contents), "date: null") {
		t.Errorf("published contents = %q", contents)
	}
}

func TestChangelogPublishRefusesUnconsumedRequiredChangeset(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	profilePath := filepath.Join(repository, "ack.yaml")
	if err := os.WriteFile(profilePath, []byte(workflowProfile), 0o600); err != nil {
		t.Fatalf("WriteFile(profile) error = %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run([]string{"changelog", "init", "--repo", repository, "--profile", profilePath, "--release-unit", "apps/worker", "--release", "1.18.3", "--channel", "stable"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("changelog init exit = %d; stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"changeset", "create", "--repo", repository, "--profile", profilePath, "-"}, strings.NewReader(validChangeset), &stdout, &stderr); code != 0 {
		t.Fatalf("changeset create exit = %d; stderr = %q", code, stderr.String())
	}
	path := filepath.Join(repository, ".ack/changelog/apps-worker.yaml")
	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"changelog", "publish", "--repo", repository, "--profile", profilePath, "--date", "2026-07-20", path}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("changelog publish exit = %d, want 1; stderr = %q", code, stderr.String())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(contents), "date: null") {
		t.Errorf("record was published despite pending changeset:\n%s", contents)
	}
}

func TestChangesetCheckValidatesIXSDecision(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "change.yaml")
	if err := os.WriteFile(path, []byte(validChangeset), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"changeset", "check", path}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid Intent Changesets record") {
		t.Errorf("stdout = %q, want validation result", stdout.String())
	}
}

func TestReleaseCheckValidatesManifest(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"release", "check", "../../examples/release.yaml"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid release manifest") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestLegacyTopLevelCommitCommandsAreRejected(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"check-message", "lint", "profile"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run([]string{command}, strings.NewReader(""), &stdout, &stderr)

		if exitCode != 2 {
			t.Errorf("Run(%q) exit code = %d, want 2", command, exitCode)
		}
	}
}

func TestVersionUsesAckProductName(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"version"}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if !strings.HasPrefix(stdout.String(), "ack ") {
		t.Errorf("stdout = %q, want ack product name", stdout.String())
	}
}

func TestCommandGroupsProvideFocusedHelp(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"commit":       "ack commit check",
		"changeset":    "ack changeset create",
		"changelog":    "ack changelog generate",
		"release":      "ack release verify",
		"pull-request": "ack pull-request check",
	}
	for group, expected := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run([]string{group, "help"}, strings.NewReader(""), &stdout, &stderr)

		if exitCode != 0 {
			t.Errorf("%s help exit code = %d, want 0", group, exitCode)
		}
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("%s help = %q, want %q", group, stdout.String(), expected)
		}
	}
}

func TestCommitProfileValidationUsesAckProductName(t *testing.T) {
	t.Parallel()

	profile := filepath.Join(t.TempDir(), "ack.yaml")
	if err := os.WriteFile(profile, []byte("ics: 0.1.0\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"commit", "profile", "validate", profile}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid ACK commit profile") {
		t.Errorf("stdout = %q, want ACK product name", stdout.String())
	}
}

func TestUnifiedProfileValidationCoversAllSpecifications(t *testing.T) {
	t.Parallel()

	profile := filepath.Join(t.TempDir(), "ack.yaml")
	if err := os.WriteFile(profile, []byte(workflowProfile), 0o600); err != nil {
		t.Fatalf("WriteFile(profile) error = %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"profile", "validate", profile}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid ACK project profile") {
		t.Errorf("stdout = %q, want project profile result", stdout.String())
	}
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}
