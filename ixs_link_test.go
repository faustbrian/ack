package ack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateChangesetLinksAcceptsMatchingICSCommit(t *testing.T) {
	t.Parallel()

	repository := createLinkedCommitRepository(t, "patch")
	if _, err := CreateIXSChangeset(repository, testWorkflowProfile(), []byte(validIXSChangeset)); err != nil {
		t.Fatalf("CreateIXSChangeset() error = %v", err)
	}

	report, err := ValidateChangesetLinks(context.Background(), repository, "HEAD", testWorkflowProfile())
	if err != nil {
		t.Fatalf("ValidateChangesetLinks() error = %v", err)
	}
	if report.HasErrors() {
		t.Errorf("diagnostics = %#v, want no errors", report.Diagnostics)
	}
}

func TestValidateChangesetLinksReportsMissingAndConflictingDecision(t *testing.T) {
	t.Parallel()

	t.Run("missing", func(t *testing.T) {
		repository := createLinkedCommitRepository(t, "patch")
		report, err := ValidateChangesetLinks(context.Background(), repository, "HEAD", testWorkflowProfile())
		if err != nil {
			t.Fatalf("ValidateChangesetLinks() error = %v", err)
		}
		if !report.HasCode("unknown-changeset") {
			t.Errorf("diagnostics = %#v, want unknown-changeset", report.Diagnostics)
		}
	})

	t.Run("impact", func(t *testing.T) {
		repository := createLinkedCommitRepository(t, "minor")
		if _, err := CreateIXSChangeset(repository, testWorkflowProfile(), []byte(validIXSChangeset)); err != nil {
			t.Fatalf("CreateIXSChangeset() error = %v", err)
		}
		report, err := ValidateChangesetLinks(context.Background(), repository, "HEAD", testWorkflowProfile())
		if err != nil {
			t.Fatalf("ValidateChangesetLinks() error = %v", err)
		}
		if !report.HasCode("impact-conflict") {
			t.Errorf("diagnostics = %#v, want impact-conflict", report.Diagnostics)
		}
	})

	t.Run("release unit", func(t *testing.T) {
		repository := createLinkedCommitRepository(t, "patch")
		profile := testWorkflowProfile()
		profile.ReleaseUnits["apps/gateway"] = ReleaseUnitPolicy{Changelog: ".ack/changelog/apps-gateway.yaml"}
		profile.Scopes["apps/gateway"] = ScopePolicy{ReleaseUnit: "apps/gateway"}
		gateway := strings.Replace(validIXSChangeset, "apps/worker", "apps/gateway", 1)
		if _, err := CreateIXSChangeset(repository, profile, []byte(gateway)); err != nil {
			t.Fatalf("CreateIXSChangeset() error = %v", err)
		}
		report, err := ValidateChangesetLinks(context.Background(), repository, "HEAD", profile)
		if err != nil {
			t.Fatalf("ValidateChangesetLinks() error = %v", err)
		}
		if !report.HasCode("release-unit-conflict") {
			t.Errorf("diagnostics = %#v, want release-unit-conflict", report.Diagnostics)
		}
	})

	t.Run("migration", func(t *testing.T) {
		repository := createLinkedCommitRepository(t, "patch", "Migration: restart workers")
		if _, err := CreateIXSChangeset(repository, testWorkflowProfile(), []byte(validIXSChangeset)); err != nil {
			t.Fatalf("CreateIXSChangeset() error = %v", err)
		}
		report, err := ValidateChangesetLinks(context.Background(), repository, "HEAD", testWorkflowProfile())
		if err != nil {
			t.Fatalf("ValidateChangesetLinks() error = %v", err)
		}
		if !report.HasCode("migration-conflict") {
			t.Errorf("diagnostics = %#v, want migration-conflict", report.Diagnostics)
		}
	})

	t.Run("invalid Intent Commits metadata", func(t *testing.T) {
		repository := createLinkedCommitRepository(t, "major")
		major := strings.Replace(validIXSChangeset, "impact: patch", "impact: major", 1)
		major = strings.Replace(major, "migration: null", "migration: restart workers after deployment", 1)
		if _, err := CreateIXSChangeset(repository, testWorkflowProfile(), []byte(major)); err != nil {
			t.Fatalf("CreateIXSChangeset() error = %v", err)
		}
		report, err := ValidateChangesetLinks(context.Background(), repository, "HEAD", testWorkflowProfile())
		if err != nil {
			t.Fatalf("ValidateChangesetLinks() error = %v", err)
		}
		if !report.HasCode("missing-migration") {
			t.Errorf("diagnostics = %#v, want missing-migration", report.Diagnostics)
		}
	})
}

func TestValidateLinkedDecisionUsesQualifiedStreamImpacts(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(apps/worker): correct callbacks\n\nImpact: patch\nTarget-Impact: apps/worker@next-v2=minor\nChangeset: correct-callbacks\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := Profile{
		Scopes: map[string]ScopePolicy{"apps/worker": {ReleaseUnit: "apps/worker"}},
		ReleaseUnits: map[string]ReleaseUnitPolicy{
			"apps/worker": {Streams: map[string]ReleaseStreamPolicy{
				"stable-v1": {ReleaseLine: "1", Channel: "stable", Changelog: ".ack/changelog/stable.yaml"},
				"next-v2":   {ReleaseLine: "2", Channel: "preview", Changelog: ".ack/changelog/next.yaml"},
			}},
		},
	}
	validation := Validate(message, profile)
	if validation.HasErrors() {
		t.Fatalf("Validate() diagnostics = %#v", validation.Diagnostics)
	}
	commit := Commit{Hash: "abc1234", Message: message}
	targets := []IXSTarget{
		{ReleaseUnit: "apps/worker", Stream: "stable-v1", Impact: "patch"},
		{ReleaseUnit: "apps/worker", Stream: "next-v2", Impact: "minor"},
	}
	var report LinkReport
	validateLinkedDecision(commit, "correct-callbacks", targets, profile, validation, &report)
	if report.HasErrors() {
		t.Fatalf("validateLinkedDecision() diagnostics = %#v", report.Diagnostics)
	}
}

func createLinkedCommitRepository(t *testing.T, impact string, extraTrailers ...string) string {
	t.Helper()
	repository := t.TempDir()
	runStoreGit(t, repository, "init", "--quiet")
	runStoreGit(t, repository, "config", "user.name", "ACK Test")
	runStoreGit(t, repository, "config", "user.email", "ack@example.test")
	if err := os.WriteFile(filepath.Join(repository, "worker.go"), []byte("package worker\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runStoreGit(t, repository, "add", "worker.go")
	trailers := "Impact: " + impact + "\nChangeset: resume-settlement-jobs"
	if len(extraTrailers) > 0 {
		trailers += "\n" + strings.Join(extraTrailers, "\n")
	}
	runStoreGit(t, repository, "commit", "--quiet", "-m", "fix(apps/worker): resume interrupted jobs", "-m", trailers)
	return repository
}

func runStoreGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}
