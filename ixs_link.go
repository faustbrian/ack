package ack

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type LinkDiagnostic struct {
	Commit    string   `json:"commit"`
	Changeset string   `json:"changeset,omitempty"`
	Code      string   `json:"code"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"`
}

type LinkReport struct {
	Diagnostics []LinkDiagnostic `json:"diagnostics"`
}

func (report LinkReport) HasErrors() bool {
	return slices.ContainsFunc(report.Diagnostics, func(diagnostic LinkDiagnostic) bool {
		return diagnostic.Severity == SeverityError
	})
}

func (report LinkReport) HasCode(code string) bool {
	return slices.ContainsFunc(report.Diagnostics, func(diagnostic LinkDiagnostic) bool {
		return diagnostic.Code == code
	})
}

func ValidateChangesetLinks(ctx context.Context, repository, revisionRange string, profile Profile) (LinkReport, error) {
	if err := profile.Validate(); err != nil {
		return LinkReport{}, err
	}
	commits, err := ReadHistory(ctx, repository, revisionRange)
	if err != nil {
		return LinkReport{}, err
	}
	var report LinkReport
	decisions := make(map[string][]IXSTarget)
	for _, commit := range commits {
		if commit.ParseError != "" {
			report.add(commit.Hash, "", "malformed-message", commit.ParseError)
			continue
		}
		validation := Validate(commit.Message, profile)
		for _, diagnostic := range validation.Diagnostics {
			if diagnostic.Severity == SeverityError {
				report.add(commit.Hash, "", diagnostic.Code, diagnostic.Message)
			}
		}
		for _, id := range commit.Message.TrailerValues("Changeset") {
			if !changesetIDPattern.MatchString(id) {
				continue
			}
			targets, ok := decisions[id]
			if !ok {
				targets, err = loadDecisionTargets(repository, profile, id)
				if err != nil {
					return LinkReport{}, err
				}
				decisions[id] = targets
			}
			if len(targets) == 0 {
				report.add(commit.Hash, id, "unknown-changeset", fmt.Sprintf("Changeset %q cannot be resolved", id))
				continue
			}
			validateLinkedDecision(commit, id, targets, profile, validation, &report)
		}
	}
	return report, nil
}

func validateLinkedDecision(commit Commit, id string, targets []IXSTarget, profile Profile, validation Report, report *LinkReport) {
	units := resolveCommitReleaseUnits(commit.Message, profile)
	if len(units) == 0 {
		report.add(commit.Hash, id, "unresolved-release-unit", "commit scope and Affects values do not resolve to a release unit")
		return
	}
	for _, unit := range units {
		if _, ok := targetForReleaseUnit(targets, unit); !ok {
			report.add(commit.Hash, id, "release-unit-conflict", fmt.Sprintf("commit affects %q but the changeset has no matching target", unit))
		}
	}
	for _, target := range targets {
		if !slices.Contains(units, target.ReleaseUnit) {
			continue
		}
		impact := validation.ImpactFor(target.ReleaseUnit, target.Stream)
		selector := target.ReleaseUnit
		if target.Stream != "" {
			selector += "@" + target.Stream
		}
		if impact != ImpactUnspecified && string(impact) != target.Impact {
			report.add(commit.Hash, id, "impact-conflict", fmt.Sprintf("commit impact %q conflicts with changeset target impact %q for %s", impact, target.Impact, selector))
		}
		migration := validation.MigrationForSelector(commit.Message, selector)
		if migration != "" {
			targetMigration := ""
			if target.Migration != nil {
				targetMigration = *target.Migration
			}
			if normalizeText(migration) != normalizeText(targetMigration) {
				report.add(commit.Hash, id, "migration-conflict", fmt.Sprintf("commit migration conflicts with changeset target migration for %s", selector))
			}
		}
	}
}

func (report *LinkReport) add(commit, changeset, code, message string) {
	report.Diagnostics = append(report.Diagnostics, LinkDiagnostic{
		Commit: commit, Changeset: changeset, Code: code,
		Severity: SeverityError, Message: message,
	})
}

func resolveCommitReleaseUnits(message Message, profile Profile) []string {
	var units []string
	for _, component := range append([]string{message.Scope}, message.TrailerValues("Affects")...) {
		if policy, ok := profile.Scopes[component]; ok && policy.ReleaseUnit != "" {
			if !slices.Contains(units, policy.ReleaseUnit) {
				units = append(units, policy.ReleaseUnit)
			}
			continue
		}
		best := ""
		for unit := range profile.ReleaseUnits {
			if component == unit || strings.HasPrefix(component, unit+"/") {
				if len(unit) > len(best) {
					best = unit
				}
			}
		}
		if best != "" && !slices.Contains(units, best) {
			units = append(units, best)
		}
	}
	return units
}

func targetForReleaseUnit(targets []IXSTarget, releaseUnit string) (IXSTarget, bool) {
	for _, target := range targets {
		if target.ReleaseUnit == releaseUnit {
			return target, true
		}
	}
	return IXSTarget{}, false
}

func loadDecisionTargets(repository string, profile Profile, id string) ([]IXSTarget, error) {
	var found []IXSChangeset
	for _, configured := range []string{profile.Changesets.Directory, profile.Changesets.ArchiveDirectory} {
		if configured == "" {
			continue
		}
		directory, err := repositoryPath(repository, configured)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !slices.Contains([]string{".yaml", ".yml"}, strings.ToLower(filepath.Ext(path))) {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			changeset, err := ParseIXSChangeset(contents)
			if err != nil {
				return fmt.Errorf("read changeset %s: %w", path, err)
			}
			if changeset.ID == id {
				found = append(found, changeset)
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	if len(found) > 1 {
		return nil, fmt.Errorf("changeset %q has multiple canonical records", id)
	}
	if len(found) == 1 {
		return found[0].Targets, nil
	}

	var targets []IXSTarget
	recordPaths, err := iclsRecordPaths(repository, profile)
	if err != nil {
		return nil, err
	}
	for _, path := range recordPaths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		record, err := ParseICLSRecord(contents)
		if err != nil {
			return nil, err
		}
		for _, entry := range record.Entries {
			if slices.Contains(entry.Provenance["changesets"], id) {
				targets = append(targets, IXSTarget{
					ReleaseUnit: record.ReleaseUnit, Type: entry.Type, Impact: entry.Impact,
					Audiences: entry.Audiences, Migration: entry.Migration,
				})
			}
		}
	}
	return targets, nil
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
