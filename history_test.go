package ack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadHistoryIncludesMessageAndDiffStats(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "config", "user.name", "ACK Test")
	runGit(t, repository, "config", "user.email", "ack@example.test")
	if err := os.WriteFile(filepath.Join(repository, "api.go"), []byte("package api\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, repository, "add", "api.go")
	runGit(t, repository, "commit", "--quiet", "-m", "feat(api): add package", "-m", "Impact: minor")

	commits, err := ReadHistory(context.Background(), repository, "HEAD")
	if err != nil {
		t.Fatalf("ReadHistory() error = %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("len(commits) = %d, want 1", len(commits))
	}
	if commits[0].Message.Type != "feat" {
		t.Errorf("Type = %q, want feat", commits[0].Message.Type)
	}
	if commits[0].Stats.Added != 1 || commits[0].Stats.Files != 1 {
		t.Errorf("Stats = %#v, want one added line in one file", commits[0].Stats)
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
