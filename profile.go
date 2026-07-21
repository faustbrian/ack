package ack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

func LoadProfile(path string) (Profile, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read profile: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Profile{}, fmt.Errorf("decode profile: expected exactly one YAML document")
		}
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Profile{}, fmt.Errorf("decode profile: root must be a mapping")
	}
	if err := validateIntentDocument(contents, &document); err != nil {
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}

	var profile Profile
	decoder = yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&profile); err != nil {
		return Profile{}, fmt.Errorf("decode profile: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}

	return profile, nil
}

func (profile Profile) Validate() error {
	if profile.ProfileVersion != "" && profile.ProfileVersion != "0.1.0" && profile.ProfileVersion != "1.0.0" {
		return fmt.Errorf("profile: unsupported project profile specification version %q", profile.ProfileVersion)
	}
	if profile.ProfileVersion != "" {
		if err := validateAbsoluteURI(profile.Repository); err != nil {
			return fmt.Errorf("profile: repository %w", err)
		}
	}
	commitsVersion := profile.commitsVersion()
	changesetsVersion := profile.changesetsVersion()
	changelogVersion := profile.changelogVersion()
	manifestVersion := profile.releaseManifestVersion()
	pullRequestsVersion := profile.Specifications.PullRequests
	if profile.ReleasePattern != "" {
		if _, err := regexp.Compile(profile.ReleasePattern); err != nil {
			return fmt.Errorf("profile: invalid release-pattern: %w", err)
		}
	}
	if profile.ProfileVersion != "" && profile.ReleasePattern == "" {
		return fmt.Errorf("profile: project profile requires release-pattern")
	}
	if commitsVersion != "0.1.0" && commitsVersion != "0.2.0" && commitsVersion != "1.0.0" {
		return fmt.Errorf("profile: unsupported Intent Commits version %q", commitsVersion)
	}
	if profile.ProfileVersion == "0.1.0" && commitsVersion != "0.2.0" {
		return fmt.Errorf("profile: project profile 0.1.0 requires Intent Commits 0.2.0")
	}
	if profile.ProfileVersion == "1.0.0" && commitsVersion != "1.0.0" {
		return fmt.Errorf("profile: project profile 1.0.0 requires Intent Commits 1.0.0")
	}
	for changeType, policy := range profile.Types {
		if !typePattern.MatchString(changeType) {
			return fmt.Errorf("profile: invalid type %q", changeType)
		}
		if policy.DefaultImpact != "" && !validImpact(policy.DefaultImpact) {
			return fmt.Errorf("profile: type %q has invalid default impact %q", changeType, policy.DefaultImpact)
		}
	}
	for scope, policy := range profile.Scopes {
		if !scopePattern.MatchString(scope) {
			return fmt.Errorf("profile: invalid scope %q", scope)
		}
		if policy.ReleaseUnit != "" {
			if _, ok := profile.ReleaseUnits[policy.ReleaseUnit]; !ok {
				return fmt.Errorf("profile: scope %q maps to unknown release unit %q", scope, policy.ReleaseUnit)
			}
		}
		if policy.ReleaseUnit == "" && !policy.RequiresAffects {
			return fmt.Errorf("profile: scope %q must map to a release unit or require affected units", scope)
		}
	}
	if changesetsVersion != "" && changesetsVersion != "0.1.0" && changesetsVersion != "0.2.0" && changesetsVersion != "1.0.0" {
		return fmt.Errorf("profile: unsupported Intent Changesets version %q", changesetsVersion)
	}
	if changelogVersion != "" && changelogVersion != "0.1.0" && changelogVersion != "0.2.0" && changelogVersion != "1.0.0" {
		return fmt.Errorf("profile: unsupported Intent Changelog version %q", changelogVersion)
	}
	if manifestVersion != "" && manifestVersion != "0.1.0" && manifestVersion != "1.0.0" {
		return fmt.Errorf("profile: unsupported Release Manifest version %q", manifestVersion)
	}
	if pullRequestsVersion != "" && pullRequestsVersion != "1.0.0" {
		return fmt.Errorf("profile: unsupported Intent Pull Requests version %q", pullRequestsVersion)
	}
	if pullRequestsVersion != "" {
		if err := validateRepositoryPath(profile.PullRequests.Directory); err != nil {
			return fmt.Errorf("profile: pull-request directory: %w", err)
		}
		if !slices.Contains([]string{"merge", "squash", "rebase"}, profile.PullRequests.MergeStrategy) {
			return fmt.Errorf("profile: pull-request merge-strategy must be merge, squash, or rebase")
		}
		if profile.PullRequests.MaxTitleLength <= 0 || profile.PullRequests.MaxBodyLength <= 0 {
			return fmt.Errorf("profile: pull-request length limits must be positive")
		}
		for _, status := range profile.PullRequests.VerificationStatuses {
			if !slices.Contains([]string{"passed", "failed", "skipped", "unavailable", "not-applicable"}, status) {
				return fmt.Errorf("profile: unsupported pull-request verification status %q", status)
			}
		}
	}
	if profile.ProfileVersion == "0.1.0" && (changesetsVersion != "0.2.0" || changelogVersion != "0.2.0" || manifestVersion != "0.1.0") {
		return fmt.Errorf("profile: project profile 0.1.0 requires Intent Changesets 0.2.0, Intent Changelog 0.2.0, and Release Manifest 0.1.0")
	}
	if profile.ProfileVersion == "1.0.0" && (changesetsVersion != "1.0.0" || changelogVersion != "1.0.0" || manifestVersion != "1.0.0") {
		return fmt.Errorf("profile: project profile 1.0.0 requires Intent Changesets 1.0.0, Intent Changelog 1.0.0, and Release Manifest 1.0.0")
	}
	for releaseUnit, policy := range profile.ReleaseUnits {
		if !scopePattern.MatchString(releaseUnit) {
			return fmt.Errorf("profile: invalid release unit %q", releaseUnit)
		}
		if len(policy.Streams) == 0 {
			if err := validateRepositoryPath(policy.Changelog); err != nil {
				return fmt.Errorf("profile: release unit %q changelog: %w", releaseUnit, err)
			}
			continue
		}
		for stream, streamPolicy := range policy.Streams {
			if !scopePattern.MatchString(stream) {
				return fmt.Errorf("profile: release unit %q has invalid stream %q", releaseUnit, stream)
			}
			if strings.TrimSpace(streamPolicy.ReleaseLine) == "" {
				return fmt.Errorf("profile: release unit %q stream %q requires release-line", releaseUnit, stream)
			}
			if !slices.Contains(profile.Channels, streamPolicy.Channel) {
				return fmt.Errorf("profile: release unit %q stream %q has unknown channel %q", releaseUnit, stream, streamPolicy.Channel)
			}
			if err := validateRepositoryPath(streamPolicy.Changelog); err != nil {
				return fmt.Errorf("profile: release unit %q stream %q changelog: %w", releaseUnit, stream, err)
			}
			if manifestVersion != "" {
				if err := validateRepositoryPath(streamPolicy.Manifests); err != nil {
					return fmt.Errorf("profile: release unit %q stream %q manifests: %w", releaseUnit, stream, err)
				}
			}
		}
	}
	for _, releaseType := range profile.ReleaseTypes {
		if !typePattern.MatchString(releaseType) {
			return fmt.Errorf("profile: invalid release type %q", releaseType)
		}
	}
	for _, impact := range profile.ReleaseImpacts {
		if !validImpact(impact) {
			return fmt.Errorf("profile: invalid release impact %q", impact)
		}
	}
	for _, impact := range profile.Changesets.RequiredImpacts {
		if !validImpact(impact) {
			return fmt.Errorf("profile: invalid required changeset impact %q", impact)
		}
	}
	for _, changeType := range profile.Changesets.RequiredCommitTypes {
		if _, ok := profile.Types[changeType]; !ok {
			return fmt.Errorf("profile: required changeset commit type %q is not an accepted Intent Commits type", changeType)
		}
	}
	if changesetsVersion != "" {
		if len(profile.ReleaseUnits) == 0 || len(profile.ReleaseTypes) == 0 || len(profile.ReleaseImpacts) == 0 || len(profile.Audiences) == 0 || len(profile.Disclosures) == 0 {
			return fmt.Errorf("profile: Intent Changesets requires release-units, release-types, release-impacts, audiences, and disclosures")
		}
		if err := validateRepositoryPath(profile.Changesets.Directory); err != nil {
			return fmt.Errorf("profile: changeset directory: %w", err)
		}
		if profile.Changesets.ArchiveDirectory != "" {
			if err := validateRepositoryPath(profile.Changesets.ArchiveDirectory); err != nil {
				return fmt.Errorf("profile: changeset archive directory: %w", err)
			}
		}
		if !slices.Contains([]string{"keep", "archive", "delete"}, profile.Changesets.AfterConsumption) {
			return fmt.Errorf("profile: after-consumption must be keep, archive, or delete")
		}
		if profile.Changesets.ConflictPolicy != "preserve" {
			return fmt.Errorf("profile: conflict-policy must be preserve")
		}
		if profile.Changesets.IDPattern != "" {
			if _, err := regexp.Compile(profile.Changesets.IDPattern); err != nil {
				return fmt.Errorf("profile: invalid changeset id-pattern: %w", err)
			}
		}
		if profile.Changesets.AfterConsumption == "archive" && profile.Changesets.ArchiveDirectory == "" {
			return fmt.Errorf("profile: archive after-consumption requires archive-directory")
		}
	}
	if changelogVersion != "" && len(profile.Channels) == 0 {
		return fmt.Errorf("profile: Intent Changelog requires channels")
	}
	if changelogVersion != "" {
		if err := validateRepositoryPath(profile.LedgerDirectory); err != nil {
			return fmt.Errorf("profile: ledger-directory: %w", err)
		}
		ledger := filepath.Clean(profile.LedgerDirectory)
		for releaseUnit, policy := range profile.ReleaseUnits {
			for stream, streamPolicy := range profile.streamsForPolicy(policy) {
				relative, err := filepath.Rel(ledger, filepath.Clean(streamPolicy.Changelog))
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					return fmt.Errorf("profile: release unit %q stream %q changelog must be inside ledger-directory", releaseUnit, stream)
				}
			}
		}
	}
	if manifestVersion != "" {
		if err := validateRepositoryPath(profile.ManifestDirectory); err != nil {
			return fmt.Errorf("profile: release-manifest-directory: %w", err)
		}
		manifestRoot := filepath.Clean(profile.ManifestDirectory)
		for releaseUnit, policy := range profile.ReleaseUnits {
			for stream, streamPolicy := range profile.streamsForPolicy(policy) {
				relative, err := filepath.Rel(manifestRoot, filepath.Clean(streamPolicy.Manifests))
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					return fmt.Errorf("profile: release unit %q stream %q manifests must be inside release-manifest-directory", releaseUnit, stream)
				}
			}
		}
	}
	if profile.MaxHeaderLength < 0 || profile.MaxBodyLineLength < 0 || profile.LargeDiffThreshold < 0 {
		return fmt.Errorf("profile: length and diff thresholds must not be negative")
	}

	return nil
}

func (profile Profile) commitsVersion() string {
	if profile.Specifications.Commits != "" {
		return profile.Specifications.Commits
	}
	return profile.ICSVersion
}

func (profile Profile) changesetsVersion() string {
	if profile.Specifications.Changesets != "" {
		return profile.Specifications.Changesets
	}
	return profile.IXSVersion
}

func (profile Profile) changelogVersion() string {
	if profile.Specifications.Changelog != "" {
		return profile.Specifications.Changelog
	}
	return profile.ICLSVersion
}

func (profile Profile) releaseManifestVersion() string {
	return profile.Specifications.ReleaseManifest
}

func (profile Profile) streamsForPolicy(policy ReleaseUnitPolicy) map[string]ReleaseStreamPolicy {
	if len(policy.Streams) > 0 {
		return policy.Streams
	}
	return map[string]ReleaseStreamPolicy{"default": {Changelog: policy.Changelog}}
}

func (profile Profile) releaseStream(releaseUnit, stream string) (ReleaseStreamPolicy, error) {
	unit, ok := profile.ReleaseUnits[releaseUnit]
	if !ok {
		return ReleaseStreamPolicy{}, fmt.Errorf("unknown release unit %q", releaseUnit)
	}
	if len(unit.Streams) == 0 {
		if stream != "" && stream != "default" {
			return ReleaseStreamPolicy{}, fmt.Errorf("unknown stream %q for release unit %q", stream, releaseUnit)
		}
		return ReleaseStreamPolicy{Channel: "", Changelog: unit.Changelog}, nil
	}
	policy, ok := unit.Streams[stream]
	if !ok {
		return ReleaseStreamPolicy{}, fmt.Errorf("unknown stream %q for release unit %q", stream, releaseUnit)
	}
	return policy, nil
}

func validateRepositoryPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path must not be empty")
	}
	if filepath.IsAbs(path) || path == "." || strings.HasPrefix(filepath.Clean(path), ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must remain inside the repository")
	}
	return nil
}
