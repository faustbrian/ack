package ack

import (
	"strings"
	"testing"
)

func TestGenerateChangelogGroupsByImpactWithoutDiscardingType(t *testing.T) {
	t.Parallel()

	fix, err := Parse("fix(apps/billing): correct tax totals\n\nImpact: minor\n")
	if err != nil {
		t.Fatalf("Parse(fix) error = %v", err)
	}
	security, err := Parse("security(shared/auth): rotate leaked key\n\nImpact: major\nMigration: replace deployed credentials\n")
	if err != nil {
		t.Fatalf("Parse(security) error = %v", err)
	}

	output := GenerateChangelog([]Commit{
		{Hash: "111111111111", Message: fix},
		{Hash: "222222222222", Message: security},
	}, Profile{})

	major := strings.Index(output, "## Major")
	minor := strings.Index(output, "## Minor")
	if major < 0 || minor < 0 || major > minor {
		t.Fatalf("output = %q, want Major before Minor", output)
	}
	if !strings.Contains(output, "**security**") || !strings.Contains(output, "**fix**") {
		t.Errorf("output = %q, want original types", output)
	}
	if !strings.Contains(output, "Migration: replace deployed credentials") {
		t.Errorf("output = %q, want migration guidance", output)
	}
}
