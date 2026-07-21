package ack

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Impact string

const (
	ImpactUnspecified Impact = "unspecified"
	ImpactNone        Impact = "none"
	ImpactPatch       Impact = "patch"
	ImpactMinor       Impact = "minor"
	ImpactMajor       Impact = "major"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

type Report struct {
	EffectiveImpact     Impact            `json:"effective_impact"`
	EffectiveImpacts    map[string]Impact `json:"effective_impacts,omitempty"`
	EffectiveMigrations map[string]string `json:"effective_migrations,omitempty"`
	Diagnostics         []Diagnostic      `json:"diagnostics"`
}

type ChangeStats struct {
	Added   int `json:"added"`
	Deleted int `json:"deleted"`
	Files   int `json:"files"`
}

type TypePolicy struct {
	DefaultImpact Impact `json:"default_impact" yaml:"default-impact"`
}

type SpecificationPolicy struct {
	Commits         string `json:"commits" yaml:"commits"`
	PullRequests    string `json:"pull_requests,omitempty" yaml:"pull-requests,omitempty"`
	Changesets      string `json:"changesets" yaml:"changesets"`
	Changelog       string `json:"changelog" yaml:"changelog"`
	ReleaseManifest string `json:"release_manifest" yaml:"release-manifest"`
}

type PullRequestPolicy struct {
	Directory            string   `json:"directory" yaml:"directory"`
	MergeStrategy        string   `json:"merge_strategy" yaml:"merge-strategy"`
	MaxTitleLength       int      `json:"max_title_length" yaml:"max-title-length"`
	MaxBodyLength        int      `json:"max_body_length" yaml:"max-body-length"`
	RequireReady         bool     `json:"require_ready" yaml:"require-ready"`
	VerificationStatuses []string `json:"verification_statuses" yaml:"verification-statuses"`
}

type ScopePolicy struct {
	ReleaseUnit     string `json:"release_unit,omitempty" yaml:"release-unit,omitempty"`
	RequiresAffects bool   `json:"requires_affects,omitempty" yaml:"requires-affects,omitempty"`
}

type ReleaseStreamPolicy struct {
	ReleaseLine string `json:"release_line" yaml:"release-line"`
	Channel     string `json:"channel" yaml:"channel"`
	Changelog   string `json:"changelog" yaml:"changelog"`
	Manifests   string `json:"manifests" yaml:"manifests"`
}

type ReleaseUnitPolicy struct {
	Changelog string                         `json:"changelog,omitempty" yaml:"changelog,omitempty"`
	Streams   map[string]ReleaseStreamPolicy `json:"streams,omitempty" yaml:"streams,omitempty"`
}

type ChangesetPolicy struct {
	Directory           string   `json:"directory" yaml:"directory"`
	ArchiveDirectory    string   `json:"archive_directory" yaml:"archive-directory"`
	AfterConsumption    string   `json:"after_consumption" yaml:"after-consumption"`
	ConflictPolicy      string   `json:"conflict_policy" yaml:"conflict-policy"`
	IDPattern           string   `json:"id_pattern" yaml:"id-pattern"`
	RequiredImpacts     []Impact `json:"required_impacts" yaml:"required-impacts"`
	RequiredCommitTypes []string `json:"required_commit_types" yaml:"required-commit-types"`
}

type Profile struct {
	ProfileVersion      string                       `json:"intent_profile,omitempty" yaml:"intent-profile,omitempty"`
	Repository          string                       `json:"repository,omitempty" yaml:"repository,omitempty"`
	Specifications      SpecificationPolicy          `json:"specifications,omitempty" yaml:"specifications,omitempty"`
	ICSVersion          string                       `json:"ics" yaml:"ics"`
	IXSVersion          string                       `json:"ixs" yaml:"ixs"`
	ICLSVersion         string                       `json:"icls" yaml:"icls"`
	Types               map[string]TypePolicy        `json:"types" yaml:"types"`
	Scopes              map[string]ScopePolicy       `json:"scopes" yaml:"scopes"`
	ReleaseUnits        map[string]ReleaseUnitPolicy `json:"release_units" yaml:"release-units"`
	LedgerDirectory     string                       `json:"ledger_directory" yaml:"ledger-directory"`
	ManifestDirectory   string                       `json:"release_manifest_directory" yaml:"release-manifest-directory"`
	ReleasePattern      string                       `json:"release_pattern" yaml:"release-pattern"`
	ReleaseTypes        []string                     `json:"release_types" yaml:"release-types"`
	ReleaseImpacts      []Impact                     `json:"release_impacts" yaml:"release-impacts"`
	Audiences           []string                     `json:"audiences" yaml:"audiences"`
	Disclosures         []string                     `json:"disclosures" yaml:"disclosures"`
	Channels            []string                     `json:"channels" yaml:"channels"`
	Changesets          ChangesetPolicy              `json:"changesets" yaml:"changesets"`
	PullRequests        PullRequestPolicy            `json:"pull_requests" yaml:"pull-requests"`
	RequireReleaseReady bool                         `json:"require_release_ready" yaml:"require-release-ready"`
	MaxHeaderLength     int                          `json:"max_header_length" yaml:"max-header-length"`
	MaxBodyLineLength   int                          `json:"max_body_line_length" yaml:"max-body-line-length"`
	LargeDiffThreshold  int                          `json:"large_diff_threshold" yaml:"large-diff-threshold"`
}

var objectNamePattern = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
var targetImpactPattern = regexp.MustCompile(`^([a-z0-9][a-z0-9./-]*)(?:@([a-z0-9][a-z0-9./-]*))?=(none|patch|minor|major)$`)
var targetMigrationPattern = regexp.MustCompile(`^([a-z0-9][a-z0-9./-]*)(?:@([a-z0-9][a-z0-9./-]*))?=(.+)$`)

func Validate(message Message, profile Profile) Report {
	report := Report{
		EffectiveImpact:     ImpactUnspecified,
		EffectiveImpacts:    make(map[string]Impact),
		EffectiveMigrations: make(map[string]string),
	}
	impactValues := message.TrailerValues("Impact")
	if len(impactValues) > 1 {
		report.addError("duplicate-impact", "Impact must not occur more than once")
	} else if len(impactValues) == 1 {
		report.EffectiveImpact = Impact(impactValues[0])
		if !validImpact(report.EffectiveImpact) {
			report.addError("invalid-impact", fmt.Sprintf("unsupported Impact value %q", impactValues[0]))
			report.EffectiveImpact = ImpactUnspecified
		}
	} else if policy, ok := profile.Types[message.Type]; ok && policy.DefaultImpact != "" {
		report.EffectiveImpact = policy.DefaultImpact
		if !validImpact(report.EffectiveImpact) {
			report.addError("invalid-default-impact", fmt.Sprintf("type %q has unsupported default impact %q", message.Type, policy.DefaultImpact))
			report.EffectiveImpact = ImpactUnspecified
		}
	}

	if report.EffectiveImpact == ImpactMajor {
		migrations := message.TrailerValues("Migration")
		if len(migrations) == 0 || strings.TrimSpace(migrations[0]) == "" {
			report.addError("missing-migration", "major impact requires one actionable Migration trailer")
		} else if len(migrations) > 1 {
			report.addError("duplicate-migration", "major impact requires exactly one Migration trailer")
		}
	}

	validateReverts(message, &report)
	validateAffects(message, &report)
	validateTargetImpacts(message, profile, &report)
	validateTargetMigrations(message, profile, &report)
	validateChangesets(message, profile, report.EffectiveImpact, &report)
	validateProfile(message, profile, &report)
	validateStyle(message, profile, &report)

	if profile.RequireReleaseReady && report.EffectiveImpact == ImpactUnspecified {
		report.addError("unspecified-impact", "project requires a known effective impact")
	}

	return report
}

func validateTargetMigrations(message Message, profile Profile, report *Report) {
	units := resolveCommitReleaseUnits(message, profile)
	seen := make(map[string]struct{})
	for _, value := range message.TrailerValues("Target-Migration") {
		matches := targetMigrationPattern.FindStringSubmatch(value)
		if matches == nil || strings.TrimSpace(matches[3]) == "" {
			report.addError("invalid-target-migration", fmt.Sprintf("Target-Migration value %q must use release-unit[@stream]=guidance", value))
			continue
		}
		selector := matches[1]
		if matches[2] != "" {
			selector += "@" + matches[2]
		}
		if _, ok := seen[selector]; ok {
			report.addError("duplicate-target-migration", fmt.Sprintf("Target-Migration selector %q occurs more than once", selector))
			continue
		}
		seen[selector] = struct{}{}
		if !slices.Contains(units, matches[1]) {
			report.addError("unknown-target-migration", fmt.Sprintf("Target-Migration selector %q is not named by the scope or Affects", selector))
			continue
		}
		if matches[2] != "" {
			policy, ok := profile.ReleaseUnits[matches[1]]
			if !ok {
				report.addError("unknown-target-stream", fmt.Sprintf("Target-Migration selector %q has no configured stream", selector))
				continue
			}
			if _, ok := policy.Streams[matches[2]]; !ok {
				report.addError("unknown-target-stream", fmt.Sprintf("Target-Migration selector %q has no configured stream", selector))
				continue
			}
		}
		report.EffectiveMigrations[selector] = strings.TrimSpace(matches[3])
	}

	for selector, impact := range report.EffectiveImpacts {
		if impact == ImpactMajor && report.MigrationForSelector(message, selector) == "" {
			report.addError("missing-target-migration", fmt.Sprintf("major impact for %s requires actionable migration guidance", selector))
		}
	}
}

func validateTargetImpacts(message Message, profile Profile, report *Report) {
	units := resolveCommitReleaseUnits(message, profile)
	for _, unit := range units {
		report.EffectiveImpacts[unit] = report.EffectiveImpact
	}
	seen := make(map[string]struct{})
	for _, value := range message.TrailerValues("Target-Impact") {
		matches := targetImpactPattern.FindStringSubmatch(value)
		if matches == nil {
			report.addError("invalid-target-impact", fmt.Sprintf("Target-Impact value %q must use release-unit[@stream]=impact", value))
			continue
		}
		selector := matches[1]
		if matches[2] != "" {
			selector += "@" + matches[2]
		}
		if _, ok := seen[selector]; ok {
			report.addError("duplicate-target-impact", fmt.Sprintf("Target-Impact selector %q occurs more than once", selector))
			continue
		}
		seen[selector] = struct{}{}
		if !slices.Contains(units, matches[1]) {
			report.addError("unknown-target-impact", fmt.Sprintf("Target-Impact selector %q is not named by the scope or Affects", selector))
			continue
		}
		if matches[2] != "" {
			policy, ok := profile.ReleaseUnits[matches[1]]
			if !ok || len(policy.Streams) == 0 {
				report.addError("unknown-target-stream", fmt.Sprintf("Target-Impact selector %q has no configured stream", selector))
				continue
			}
			if _, ok := policy.Streams[matches[2]]; !ok {
				report.addError("unknown-target-stream", fmt.Sprintf("Target-Impact selector %q has no configured stream", selector))
				continue
			}
		}
		report.EffectiveImpacts[selector] = Impact(matches[3])
		if matches[2] == "" {
			report.EffectiveImpacts[matches[1]] = Impact(matches[3])
		}
	}
}

func (report Report) ImpactFor(releaseUnit, stream string) Impact {
	if stream != "" {
		if impact, ok := report.EffectiveImpacts[releaseUnit+"@"+stream]; ok {
			return impact
		}
	}
	if impact, ok := report.EffectiveImpacts[releaseUnit]; ok {
		return impact
	}
	return report.EffectiveImpact
}

func (report Report) MigrationFor(releaseUnit, stream string) string {
	if stream != "" {
		if migration, ok := report.EffectiveMigrations[releaseUnit+"@"+stream]; ok {
			return migration
		}
	}
	return report.EffectiveMigrations[releaseUnit]
}

func (report Report) MigrationForSelector(message Message, selector string) string {
	if migration, ok := report.EffectiveMigrations[selector]; ok {
		return migration
	}
	if unit, _, found := strings.Cut(selector, "@"); found {
		if migration, ok := report.EffectiveMigrations[unit]; ok {
			return migration
		}
	}
	migrations := message.TrailerValues("Migration")
	if len(migrations) > 0 {
		return strings.TrimSpace(migrations[0])
	}
	return ""
}

func validateChangesets(message Message, profile Profile, impact Impact, report *Report) {
	seen := make(map[string]struct{})
	values := message.TrailerValues("Changeset")
	for _, id := range values {
		if !changesetIDPattern.MatchString(id) {
			report.addError("invalid-changeset", fmt.Sprintf("Changeset value %q is not a valid Intent Changesets identifier", id))
		}
		if _, ok := seen[id]; ok {
			report.addError("duplicate-changeset", fmt.Sprintf("Changeset value %q occurs more than once", id))
		}
		seen[id] = struct{}{}
	}
	if (slices.Contains(profile.Changesets.RequiredImpacts, impact) || slices.Contains(profile.Changesets.RequiredCommitTypes, message.Type)) && len(values) == 0 {
		report.addError("missing-changeset", fmt.Sprintf("project requires an Intent Changesets record for %s impact", impact))
	}
}

func Review(message Message, profile Profile, stats ChangeStats) Report {
	report := Validate(message, profile)
	changedLines := stats.Added + stats.Deleted
	if profile.LargeDiffThreshold > 0 && changedLines >= profile.LargeDiffThreshold &&
		(report.EffectiveImpact == ImpactNone || report.EffectiveImpact == ImpactPatch) {
		report.addWarning(
			"large-diff-low-impact",
			fmt.Sprintf("%d changed lines exceed the warning threshold of %d for %s impact", changedLines, profile.LargeDiffThreshold, report.EffectiveImpact),
		)
	}

	return report
}

func (report Report) HasErrors() bool {
	return slices.ContainsFunc(report.Diagnostics, func(diagnostic Diagnostic) bool {
		return diagnostic.Severity == SeverityError
	})
}

func (report Report) HasCode(code string) bool {
	return slices.ContainsFunc(report.Diagnostics, func(diagnostic Diagnostic) bool {
		return diagnostic.Code == code
	})
}

func (report *Report) addError(code, message string) {
	report.Diagnostics = append(report.Diagnostics, Diagnostic{
		Code: code, Severity: SeverityError, Message: message,
	})
}

func (report *Report) addWarning(code, message string) {
	report.Diagnostics = append(report.Diagnostics, Diagnostic{
		Code: code, Severity: SeverityWarning, Message: message,
	})
}

func validImpact(impact Impact) bool {
	return impact == ImpactNone || impact == ImpactPatch || impact == ImpactMinor || impact == ImpactMajor
}

func validateReverts(message Message, report *Report) {
	reverts := message.TrailerValues("Reverts")
	if message.Type == "revert" && len(reverts) == 0 {
		report.addError("missing-reverts", "revert commits require at least one Reverts trailer")
	}
	for _, objectName := range reverts {
		if !objectNamePattern.MatchString(objectName) {
			report.addError("invalid-reverts", fmt.Sprintf("Reverts value %q is not a 7 to 64 character hexadecimal object name", objectName))
		}
	}
}

func validateAffects(message Message, report *Report) {
	seen := make(map[string]struct{})
	for _, component := range message.TrailerValues("Affects") {
		if component == message.Scope {
			report.addError("primary-scope-in-affects", fmt.Sprintf("Affects value %q duplicates the primary scope", component))
		}
		if !scopePattern.MatchString(component) {
			report.addError("invalid-affects", fmt.Sprintf("Affects value %q is not a valid component", component))
		}
		if _, ok := seen[component]; ok {
			report.addError("duplicate-affects", fmt.Sprintf("Affects value %q occurs more than once", component))
		}
		seen[component] = struct{}{}
	}
}

func validateProfile(message Message, profile Profile, report *Report) {
	if len(profile.Types) > 0 {
		if _, ok := profile.Types[message.Type]; !ok {
			report.addError("unknown-type", fmt.Sprintf("type %q is not accepted by the project profile", message.Type))
		}
	}
	if len(profile.Scopes) > 0 {
		policy, ok := profile.Scopes[message.Scope]
		if !ok {
			report.addError("unknown-scope", fmt.Sprintf("scope %q is not accepted by the project profile", message.Scope))
		} else if policy.RequiresAffects && len(message.TrailerValues("Affects")) == 0 {
			report.addError("missing-affected-unit", fmt.Sprintf("scope %q requires at least one Affects trailer", message.Scope))
		}
	}
	for _, component := range message.TrailerValues("Affects") {
		if len(profile.Scopes) > 0 {
			if _, ok := profile.Scopes[component]; !ok {
				report.addError("unknown-component", fmt.Sprintf("affected component %q is not accepted by the project profile", component))
			}
		}
	}
}

func validateStyle(message Message, profile Profile, report *Report) {
	if profile.MaxHeaderLength > 0 && len([]rune(message.Header())) > profile.MaxHeaderLength {
		report.addError("header-too-long", fmt.Sprintf("header exceeds %d characters", profile.MaxHeaderLength))
	}
	if profile.MaxBodyLineLength > 0 {
		for index, line := range strings.Split(message.Body, "\n") {
			if len([]rune(line)) > profile.MaxBodyLineLength {
				report.addError("body-line-too-long", fmt.Sprintf("body line %d exceeds %d characters", index+1, profile.MaxBodyLineLength))
			}
		}
	}
}
