package cli

import (
	"strings"
	"testing"

	"github.com/faustbrian/ack"
)

func TestDraftMessageIncludesReleaseMetadata(t *testing.T) {
	t.Parallel()

	message := draftMessage(commitDraft{
		ChangeType:       "fix",
		Scope:            "apps/billing",
		Description:      "recalculate imported tax summaries",
		Body:             "Correct reports generated before the importer fix.",
		Impact:           ack.ImpactMajor,
		Migration:        "regenerate affected reports",
		Affects:          []string{"apps/admin"},
		TargetImpacts:    []string{"apps/admin@stable=patch"},
		TargetMigrations: []string{"apps/admin@stable=restart administrators"},
		Changesets:       []string{"recalculate-tax-summaries"},
	})
	output := message.String()

	for _, expected := range []string{
		"fix(apps/billing): recalculate imported tax summaries",
		"Impact: major",
		"Migration: regenerate affected reports",
		"Affects: apps/admin",
		"Target-Impact: apps/admin@stable=patch",
		"Target-Migration: apps/admin@stable=restart administrators",
		"Changeset: recalculate-tax-summaries",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("message = %q, want %q", output, expected)
		}
	}
}
