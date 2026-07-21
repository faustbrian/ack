package ack

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

type ConsumptionResult struct {
	ChangesetID string   `json:"changeset_id"`
	Records     []string `json:"records"`
	ArchivedTo  string   `json:"archived_to,omitempty"`
	Deleted     bool     `json:"deleted"`
}

type GateDiagnostic struct {
	Changeset   string `json:"changeset"`
	ReleaseUnit string `json:"release_unit"`
	Message     string `json:"message"`
}

type GateReport struct {
	Diagnostics []GateDiagnostic `json:"diagnostics"`
}

func GenerateICLSRecords(ctx context.Context, repository, revisionRange string, profile Profile) ([]ConsumptionResult, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	paths, err := pendingChangesetPaths(repository, profile)
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	linkReport, err := ValidateChangesetLinks(ctx, repository, revisionRange, profile)
	if err != nil {
		return nil, err
	}
	if linkReport.HasErrors() {
		return nil, fmt.Errorf("generate Intent Changelog records: linked commit conflict: %s", linkReport.Diagnostics[0].Message)
	}
	commits, err := ReadHistory(ctx, repository, revisionRange)
	if err != nil {
		return nil, err
	}
	results := make([]ConsumptionResult, 0, len(paths))
	for _, path := range paths {
		result, err := ConsumeIXSChangeset(repository, profile, path, commits)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (report GateReport) HasErrors() bool {
	return len(report.Diagnostics) > 0
}

func ConsumeIXSChangeset(repository string, profile Profile, changesetPath string, commits []Commit) (ConsumptionResult, error) {
	if err := profile.Validate(); err != nil {
		return ConsumptionResult{}, err
	}
	canonicalDirectory, err := repositoryPath(repository, profile.Changesets.Directory)
	if err != nil {
		return ConsumptionResult{}, err
	}
	resolvedPath := changesetPath
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(repository, resolvedPath)
	}
	absolutePath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: resolve path: %w", err)
	}
	if filepath.Dir(absolutePath) != canonicalDirectory {
		return ConsumptionResult{}, errors.New("consume Intent Changesets record: record is not in the configured pending directory")
	}
	contents, err := os.ReadFile(absolutePath)
	if err != nil {
		return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: read: %w", err)
	}
	changeset, err := ParseIXSChangeset(contents)
	if err != nil {
		return ConsumptionResult{}, err
	}
	if err := ValidateIXSChangeset(changeset, profile); err != nil {
		return ConsumptionResult{}, err
	}
	if filepath.Base(absolutePath) != changeset.ID+".yaml" {
		return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: filename must be %s.yaml", changeset.ID)
	}

	linkedCommits, err := validateConsumptionCommits(changeset, commits, profile)
	if err != nil {
		return ConsumptionResult{}, err
	}
	archivePath, err := preflightDisposition(repository, profile, changeset.ID)
	if err != nil {
		return ConsumptionResult{}, err
	}

	type plannedWrite struct {
		path     string
		contents []byte
		changed  bool
	}
	plans := make([]plannedWrite, 0, len(changeset.Targets))
	result := ConsumptionResult{ChangesetID: changeset.ID}
	for _, target := range changeset.Targets {
		streamPolicy, err := profile.releaseStream(target.ReleaseUnit, target.Stream)
		if err != nil {
			return ConsumptionResult{}, err
		}
		path, err := repositoryPath(repository, streamPolicy.Changelog)
		if err != nil {
			return ConsumptionResult{}, err
		}
		recordContents, err := os.ReadFile(path)
		if err != nil {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: release unit %s: read Intent Changelog record: %w", target.ReleaseUnit, err)
		}
		record, err := ParseICLSRecord(recordContents)
		if err != nil {
			return ConsumptionResult{}, err
		}
		if err := ValidateICLSRecordProfile(record, profile); err != nil {
			return ConsumptionResult{}, err
		}
		if record.ReleaseUnit != target.ReleaseUnit {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: mapped record owns %q, want %q", record.ReleaseUnit, target.ReleaseUnit)
		}
		if target.Stream != "" && record.Stream != target.Stream {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: mapped record owns stream %q, want %q", record.Stream, target.Stream)
		}
		if record.Date != nil {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: release unit %s record is already published", target.ReleaseUnit)
		}

		entry := projectIXSTarget(changeset, target, linkedCommits)
		changed, err := appendICLSEntry(&record, entry)
		if err != nil {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: release unit %s: %w", target.ReleaseUnit, err)
		}
		encoded := recordContents
		if changed {
			encoded, err = MarshalICLSRecord(record)
			if err != nil {
				return ConsumptionResult{}, err
			}
			verified, err := ParseICLSRecord(encoded)
			if err != nil {
				return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: generated Intent Changelog record: %w", err)
			}
			if err := ValidateICLSRecordProfile(verified, profile); err != nil {
				return ConsumptionResult{}, err
			}
		}
		plans = append(plans, plannedWrite{path: path, contents: encoded, changed: changed})
		result.Records = append(result.Records, path)
	}

	for _, plan := range plans {
		if plan.changed {
			if err := atomicReplace(plan.path, plan.contents); err != nil {
				return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: write %s: %w", plan.path, err)
			}
		}
	}
	if err := verifyConsumedTargets(repository, profile, changeset); err != nil {
		return ConsumptionResult{}, err
	}

	switch profile.Changesets.AfterConsumption {
	case "archive":
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: create archive: %w", err)
		}
		if err := os.Rename(absolutePath, archivePath); err != nil {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: archive: %w", err)
		}
		result.ArchivedTo = archivePath
	case "delete":
		if err := os.Remove(absolutePath); err != nil {
			return ConsumptionResult{}, fmt.Errorf("consume Intent Changesets record: delete: %w", err)
		}
		result.Deleted = true
	}
	return result, nil
}

func GatePendingChangesets(repository string, profile Profile) (GateReport, error) {
	if err := profile.Validate(); err != nil {
		return GateReport{}, err
	}
	paths, err := pendingChangesetPaths(repository, profile)
	if err != nil {
		return GateReport{}, err
	}
	var report GateReport
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return GateReport{}, err
		}
		changeset, err := ParseIXSChangeset(contents)
		if err != nil {
			return GateReport{}, err
		}
		if err := ValidateIXSChangeset(changeset, profile); err != nil {
			return GateReport{}, err
		}
		for _, target := range changeset.Targets {
			streamPolicy, err := profile.releaseStream(target.ReleaseUnit, target.Stream)
			if err != nil {
				return GateReport{}, err
			}
			recordPath, err := repositoryPath(repository, streamPolicy.Changelog)
			if err != nil {
				return GateReport{}, err
			}
			recordContents, err := os.ReadFile(recordPath)
			if errors.Is(err, fs.ErrNotExist) {
				report.Diagnostics = append(report.Diagnostics, GateDiagnostic{Changeset: changeset.ID, ReleaseUnit: target.ReleaseUnit, Message: "mapped Intent Changelog record does not exist"})
				continue
			}
			if err != nil {
				return GateReport{}, err
			}
			record, err := ParseICLSRecord(recordContents)
			if err != nil {
				return GateReport{}, err
			}
			if !recordContainsConsumedTarget(record, changeset, target) {
				report.Diagnostics = append(report.Diagnostics, GateDiagnostic{Changeset: changeset.ID, ReleaseUnit: target.ReleaseUnit, Message: "target is not consumed into the mapped Intent Changelog record"})
			}
		}
	}
	return report, nil
}

func validateConsumptionCommits(changeset IXSChangeset, commits []Commit, profile Profile) ([]string, error) {
	var hashes []string
	for _, commit := range commits {
		if commit.ParseError != "" || !slices.Contains(commit.Message.TrailerValues("Changeset"), changeset.ID) {
			continue
		}
		validation := Validate(commit.Message, profile)
		if validation.HasErrors() {
			return nil, fmt.Errorf("consume Intent Changesets record: linked commit %s is not valid Intent Commits: %s", commit.Hash, validation.Diagnostics[0].Message)
		}
		var report LinkReport
		validateLinkedDecision(commit, changeset.ID, changeset.Targets, profile, validation, &report)
		if report.HasErrors() {
			return nil, fmt.Errorf("consume Intent Changesets record: linked commit %s conflicts: %s", commit.Hash, report.Diagnostics[0].Message)
		}
		if !slices.Contains(hashes, commit.Hash) {
			hashes = append(hashes, commit.Hash)
		}
	}
	return hashes, nil
}

func projectIXSTarget(changeset IXSChangeset, target IXSTarget, commits []string) ICLSEntry {
	provenance := make(map[string][]string, len(changeset.Provenance)+2)
	for kind, references := range changeset.Provenance {
		provenance[kind] = slices.Clone(references)
	}
	provenance["changesets"] = appendUnique(provenance["changesets"], changeset.ID)
	for _, hash := range commits {
		provenance["commits"] = appendUnique(provenance["commits"], hash)
	}
	return ICLSEntry{
		ID: changeset.ID, Type: target.Type, Summary: changeset.Summary,
		Rationale: changeset.Rationale, Impact: target.Impact,
		Audiences: slices.Clone(target.Audiences), Migration: target.Migration,
		Affects: []string{target.ReleaseUnit}, Provenance: provenance,
		Relations:  ICLSRelations{Reverts: slices.Clone(changeset.Relations.Reverts), Supersedes: slices.Clone(changeset.Relations.Supersedes)},
		Disclosure: changeset.Disclosure,
	}
}

func appendICLSEntry(record *ICLSRecord, entry ICLSEntry) (bool, error) {
	for _, existing := range record.Entries {
		if existing.ID != entry.ID {
			continue
		}
		if reflect.DeepEqual(existing, entry) {
			return false, nil
		}
		return false, fmt.Errorf("entry %q conflicts with existing editorial content", entry.ID)
	}
	root := record.node.Content[0]
	entries := mappingValue(root, "entries")
	if entries == nil || entries.Kind != yaml.SequenceNode {
		return false, errors.New("entries must be a sequence")
	}
	var node yaml.Node
	if err := node.Encode(entry); err != nil {
		return false, fmt.Errorf("encode entry: %w", err)
	}
	entries.Style = 0
	entries.Content = append(entries.Content, &node)
	record.Entries = append(record.Entries, entry)
	return true, nil
}

func verifyConsumedTargets(repository string, profile Profile, changeset IXSChangeset) error {
	for _, target := range changeset.Targets {
		streamPolicy, err := profile.releaseStream(target.ReleaseUnit, target.Stream)
		if err != nil {
			return err
		}
		path, err := repositoryPath(repository, streamPolicy.Changelog)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		record, err := ParseICLSRecord(contents)
		if err != nil {
			return err
		}
		if !recordContainsConsumedTarget(record, changeset, target) {
			return fmt.Errorf("consume Intent Changesets record: could not verify target %s", target.ReleaseUnit)
		}
	}
	return nil
}

func recordContainsConsumedTarget(record ICLSRecord, changeset IXSChangeset, target IXSTarget) bool {
	for _, entry := range record.Entries {
		if entry.ID == changeset.ID && entry.Type == target.Type && entry.Impact == target.Impact &&
			slices.Contains(entry.Affects, target.ReleaseUnit) && slices.Contains(entry.Provenance["changesets"], changeset.ID) {
			return true
		}
	}
	return false
}

func preflightDisposition(repository string, profile Profile, id string) (string, error) {
	if profile.Changesets.AfterConsumption != "archive" {
		return "", nil
	}
	directory, err := repositoryPath(repository, profile.Changesets.ArchiveDirectory)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, id+".yaml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("consume Intent Changesets record: archive %s already exists", path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return path, nil
}

func pendingChangesetPaths(repository string, profile Profile) ([]string, error) {
	directory, err := repositoryPath(repository, profile.Changesets.Directory)
	if err != nil {
		return nil, err
	}
	var paths []string
	err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && slices.Contains([]string{".yaml", ".yml"}, strings.ToLower(filepath.Ext(path))) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func atomicReplace(path string, contents []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ack-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
