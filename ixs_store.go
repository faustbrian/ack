package ack

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

func CreateIXSChangeset(repository string, profile Profile, contents []byte) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	changeset, err := ParseIXSChangeset(contents)
	if err != nil {
		return "", err
	}
	if err := ValidateIXSChangeset(changeset, profile); err != nil {
		return "", err
	}
	locations, err := changesetIdentifierLocations(repository, profile, changeset.ID)
	if err != nil {
		return "", err
	}
	if len(locations) > 0 {
		return "", fmt.Errorf("create Intent Changesets record: identifier %q already exists in %s", changeset.ID, strings.Join(locations, ", "))
	}

	directory, err := repositoryPath(repository, profile.Changesets.Directory)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create Intent Changesets record: create directory: %w", err)
	}
	path := filepath.Join(directory, changeset.ID+".yaml")
	if err := writeExclusive(path, contents); err != nil {
		return "", fmt.Errorf("create Intent Changesets record: %w", err)
	}
	return path, nil
}

func InitializeICLSRecord(repository string, profile Profile, releaseUnit, release, channel string) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	if profile.changelogVersion() != "0.1.0" {
		return "", errors.New("initialize Intent Changelog record: profile does not enable Intent Changelog 0.1.0")
	}
	policy, ok := profile.ReleaseUnits[releaseUnit]
	if !ok {
		return "", fmt.Errorf("initialize Intent Changelog record: unknown release unit %q", releaseUnit)
	}
	if strings.TrimSpace(release) == "" {
		return "", errors.New("initialize Intent Changelog record: release must not be empty")
	}
	if len(profile.Channels) > 0 && !slices.Contains(profile.Channels, channel) {
		return "", fmt.Errorf("initialize Intent Changelog record: channel %q is not accepted by the project profile", channel)
	}

	path, err := repositoryPath(repository, policy.Changelog)
	if err != nil {
		return "", err
	}
	record := ICLSRecord{
		Version: profile.ICLSVersion, ReleaseUnit: releaseUnit, Release: release,
		Channel: channel, Entries: []ICLSEntry{},
	}
	contents, err := yaml.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("initialize Intent Changelog record: encode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("initialize Intent Changelog record: create directory: %w", err)
	}
	if err := writeExclusive(path, contents); err != nil {
		return "", fmt.Errorf("initialize Intent Changelog record: %w", err)
	}
	return path, nil
}

func InitializeICLSStreamRecord(repository string, profile Profile, releaseUnit, stream, release string) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	if !isStructuredIntentVersion(profile.changelogVersion()) {
		return "", errors.New("initialize Intent Changelog record: profile does not enable a structured Intent Changelog version")
	}
	streamPolicy, err := profile.releaseStream(releaseUnit, stream)
	if err != nil {
		return "", fmt.Errorf("initialize Intent Changelog record: %w", err)
	}
	if strings.TrimSpace(release) == "" {
		return "", errors.New("initialize Intent Changelog record: release must not be empty")
	}
	path, err := repositoryPath(repository, streamPolicy.Changelog)
	if err != nil {
		return "", err
	}
	record := ICLSRecord{
		Version: profile.changelogVersion(), ReleaseUnit: releaseUnit, Release: release,
		SourceRepository: profile.Repository,
		Stream:           stream, ReleaseLine: streamPolicy.ReleaseLine, Channel: streamPolicy.Channel,
		Entries: []ICLSEntry{}, Amendments: []ICLSAmendment{},
	}
	contents, err := yaml.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("initialize Intent Changelog record: encode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("initialize Intent Changelog record: create directory: %w", err)
	}
	if err := writeExclusive(path, contents); err != nil {
		return "", fmt.Errorf("initialize Intent Changelog record: %w", err)
	}
	return path, nil
}

func changesetIdentifierLocations(repository string, profile Profile, id string) ([]string, error) {
	var locations []string
	seenDirectories := make(map[string]struct{})
	for _, configured := range []string{profile.Changesets.Directory, profile.Changesets.ArchiveDirectory} {
		if configured == "" {
			continue
		}
		directory, err := repositoryPath(repository, configured)
		if err != nil {
			return nil, err
		}
		if _, ok := seenDirectories[directory]; ok {
			continue
		}
		seenDirectories[directory] = struct{}{}
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
				locations = append(locations, path)
			}
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("scan changesets: %w", err)
		}
	}

	recordPaths, err := iclsRecordPaths(repository, profile)
	if err != nil {
		return nil, err
	}
	for _, path := range recordPaths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read Intent Changelog record %s: %w", path, err)
		}
		record, err := ParseICLSRecord(contents)
		if err != nil {
			return nil, err
		}
		for _, entry := range record.Entries {
			if slices.Contains(entry.Provenance["changesets"], id) {
				locations = append(locations, path+"#"+entry.ID)
			}
		}
	}
	return locations, nil
}

func iclsRecordPaths(repository string, profile Profile) ([]string, error) {
	directory, err := repositoryPath(repository, profile.LedgerDirectory)
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
		return nil, fmt.Errorf("scan Intent Changelog ledger: %w", err)
	}
	slices.Sort(paths)
	return paths, nil
}

func repositoryPath(repository, configured string) (string, error) {
	if err := validateRepositoryPath(configured); err != nil {
		return "", err
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	path := filepath.Join(root, configured)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repository", configured)
	}
	return path, nil
}

func writeExclusive(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(contents); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	remove = false
	return nil
}
