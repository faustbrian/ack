package ack

import "testing"

func TestValidateMajorImpactRequiresMigration(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(api): reject ambiguous payloads\n\nImpact: major\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := Validate(message, Profile{})
	if !report.HasCode("missing-migration") {
		t.Fatalf("diagnostics = %#v, want missing-migration", report.Diagnostics)
	}
	if !report.HasErrors() {
		t.Fatal("HasErrors() = false, want true")
	}
}

func TestReviewWarnsWhenLargeDiffDeclaresLowImpact(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(api): normalize generated clients\n\nImpact: patch\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := Review(message, Profile{LargeDiffThreshold: 500}, ChangeStats{Added: 400, Deleted: 200})
	if !report.HasCode("large-diff-low-impact") {
		t.Fatalf("diagnostics = %#v, want large-diff-low-impact", report.Diagnostics)
	}
	if report.HasErrors() {
		t.Fatal("HasErrors() = true, want warning only")
	}
}

func TestDeclaredImpactOverridesTypeDefault(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(api): announce corrected totals\n\nImpact: minor\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := Profile{Types: map[string]TypePolicy{"fix": {DefaultImpact: ImpactPatch}}}

	report := Validate(message, profile)
	if report.EffectiveImpact != ImpactMinor {
		t.Errorf("EffectiveImpact = %q, want minor", report.EffectiveImpact)
	}
}

func TestValidateMonorepoComponents(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(apps/billing): correct totals\n\nImpact: patch\nAffects: apps/worker\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := Profile{Scopes: map[string]ScopePolicy{
		"apps/billing": {ReleaseUnit: "apps/billing"},
		"apps/worker":  {ReleaseUnit: "apps/worker"},
	}}

	report := Validate(message, profile)
	if report.HasErrors() {
		t.Errorf("diagnostics = %#v, want no errors", report.Diagnostics)
	}
}

func TestValidateRejectsDuplicateAffectedComponent(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(shared/auth): rotate key\n\nImpact: patch\nAffects: apps/worker\nAffects: apps/worker\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := Validate(message, Profile{})
	if !report.HasCode("duplicate-affects") {
		t.Errorf("diagnostics = %#v, want duplicate-affects", report.Diagnostics)
	}
}

func TestValidateRejectsPrimaryScopeAsAffectedComponent(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(apps/worker): retry jobs\n\nImpact: patch\nAffects: apps/worker\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := Validate(message, Profile{})
	if !report.HasCode("primary-scope-in-affects") {
		t.Errorf("diagnostics = %#v, want primary-scope-in-affects", report.Diagnostics)
	}
}

func TestValidateRequiresChangesetForConfiguredImpact(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(apps/worker): change settlement behavior\n\nImpact: minor\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := Validate(message, testWorkflowProfile())
	if !report.HasCode("missing-changeset") {
		t.Errorf("diagnostics = %#v, want missing-changeset", report.Diagnostics)
	}
}

func TestValidateRejectsInvalidChangesetIdentifier(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(apps/worker): change settlement behavior\n\nImpact: patch\nChangeset: NOT VALID\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := Validate(message, testWorkflowProfile())
	if !report.HasCode("invalid-changeset") {
		t.Errorf("diagnostics = %#v, want invalid-changeset", report.Diagnostics)
	}
}

func TestValidateQualifiedTargetImpacts(t *testing.T) {
	t.Parallel()

	profile := Profile{
		Types: map[string]TypePolicy{"fix": {DefaultImpact: ImpactPatch}},
		Scopes: map[string]ScopePolicy{
			"apps/gateway": {ReleaseUnit: "apps/gateway"},
			"apps/worker":  {ReleaseUnit: "apps/worker"},
		},
		ReleaseUnits: map[string]ReleaseUnitPolicy{
			"apps/gateway": {Changelog: ".ack/changelog/apps-gateway.yaml"},
			"apps/worker":  {Changelog: ".ack/changelog/apps-worker.yaml"},
		},
	}

	valid, err := Parse("fix(apps/gateway): rotate credentials\n\nImpact: minor\nAffects: apps/worker\nTarget-Impact: apps/worker=patch\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := Validate(valid, profile)
	if report.HasErrors() {
		t.Fatalf("Validate() diagnostics = %#v", report.Diagnostics)
	}
	if impact := report.EffectiveImpacts["apps/worker"]; impact != ImpactPatch {
		t.Errorf("worker impact = %q, want patch", impact)
	}

	invalid, err := Parse("fix(apps/gateway): rotate credentials\n\nImpact: minor\nTarget-Impact: unknown=patch\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report = Validate(invalid, profile)
	if !report.HasCode("unknown-target-impact") {
		t.Fatalf("Validate() diagnostics = %#v, want unknown-target-impact", report.Diagnostics)
	}
}

func TestValidateQualifiedMajorImpactRequiresTargetMigration(t *testing.T) {
	t.Parallel()

	profile := Profile{
		Scopes: map[string]ScopePolicy{
			"apps/gateway": {ReleaseUnit: "apps/gateway"},
			"apps/worker":  {ReleaseUnit: "apps/worker"},
		},
		ReleaseUnits: map[string]ReleaseUnitPolicy{
			"apps/gateway": {Changelog: ".ack/changelog/apps-gateway.yaml"},
			"apps/worker":  {Changelog: ".ack/changelog/apps-worker.yaml"},
		},
	}

	missing, err := Parse("fix(apps/gateway): rotate credentials\n\nImpact: patch\nAffects: apps/worker\nTarget-Impact: apps/worker=major\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := Validate(missing, profile)
	if !report.HasCode("missing-target-migration") {
		t.Fatalf("Validate() diagnostics = %#v, want missing-target-migration", report.Diagnostics)
	}

	valid, err := Parse("fix(apps/gateway): rotate credentials\n\nImpact: patch\nAffects: apps/worker\nTarget-Impact: apps/worker=major\nTarget-Migration: apps/worker=restart workers after rotation\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report = Validate(valid, profile)
	if report.HasErrors() {
		t.Fatalf("Validate() diagnostics = %#v", report.Diagnostics)
	}
	if migration := report.MigrationFor("apps/worker", ""); migration != "restart workers after rotation" {
		t.Errorf("worker migration = %q", migration)
	}
}

func TestValidateScopeThatRequiresAffectedReleaseUnits(t *testing.T) {
	t.Parallel()

	message, err := Parse("fix(shared/auth): rotate credentials\n\nImpact: patch\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := Profile{Scopes: map[string]ScopePolicy{
		"shared/auth": {RequiresAffects: true},
	}}
	report := Validate(message, profile)
	if !report.HasCode("missing-affected-unit") {
		t.Fatalf("Validate() diagnostics = %#v, want missing-affected-unit", report.Diagnostics)
	}
}
