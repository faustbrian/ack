package ack

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type ReleaseManifest struct {
	Version     string              `json:"intent_release_manifest" yaml:"intent-release-manifest"`
	ReleaseUnit string              `json:"release_unit" yaml:"release-unit"`
	Release     string              `json:"release" yaml:"release"`
	Stream      string              `json:"stream" yaml:"stream"`
	ReleaseLine string              `json:"release_line" yaml:"release-line"`
	Channel     string              `json:"channel" yaml:"channel"`
	PublishedAt string              `json:"published_at" yaml:"published-at"`
	Source      ReleaseSource       `json:"source" yaml:"source"`
	Changelog   ReleaseChangelog    `json:"changelog" yaml:"changelog"`
	Artifacts   []ReleaseArtifact   `json:"artifacts" yaml:"artifacts"`
	Provenance  map[string][]string `json:"provenance" yaml:"provenance"`

	node *yaml.Node
}

func VerifyReleaseManifest(repository string, profile Profile, manifest ReleaseManifest) error {
	if err := ValidateReleaseManifestProfile(manifest, profile); err != nil {
		return fmt.Errorf("verify release manifest: %w", err)
	}
	stream, err := profile.releaseStream(manifest.ReleaseUnit, manifest.Stream)
	if err != nil {
		return fmt.Errorf("verify release manifest: %w", err)
	}
	if stream.ReleaseLine != manifest.ReleaseLine || stream.Channel != manifest.Channel {
		return fmt.Errorf("verify release manifest: release stream metadata conflicts with the project profile")
	}
	if stream.Changelog != manifest.Changelog.Path {
		return fmt.Errorf("verify release manifest: changelog path %q does not match stream path %q", manifest.Changelog.Path, stream.Changelog)
	}
	changelogPath, err := repositoryPath(repository, manifest.Changelog.Path)
	if err != nil {
		return fmt.Errorf("verify release manifest: %w", err)
	}
	changelogContents, err := os.ReadFile(changelogPath)
	if err != nil {
		return fmt.Errorf("verify release manifest: read changelog: %w", err)
	}
	if digestBytes(changelogContents) != manifest.Changelog.Digest {
		return fmt.Errorf("verify release manifest: changelog digest does not match %s", manifest.Changelog.Path)
	}
	record, err := ParseICLSRecord(changelogContents)
	if err != nil {
		return fmt.Errorf("verify release manifest: %w", err)
	}
	if err := ValidateICLSRecordProfile(record, profile); err != nil {
		return fmt.Errorf("verify release manifest: %w", err)
	}
	if record.Date == nil {
		return fmt.Errorf("verify release manifest: changelog record is unreleased")
	}
	if record.ReleaseUnit != manifest.ReleaseUnit || record.Release != manifest.Release ||
		record.Stream != manifest.Stream || record.ReleaseLine != manifest.ReleaseLine || record.Channel != manifest.Channel {
		return fmt.Errorf("verify release manifest: changelog release identity does not match manifest")
	}
	if record.SourceRepository != manifest.Source.Repository {
		return fmt.Errorf("verify release manifest: source repository does not match changelog")
	}
	publishedAt, _ := time.Parse(time.RFC3339, manifest.PublishedAt)
	if publishedAt.Format(time.DateOnly) != *record.Date {
		return fmt.Errorf("verify release manifest: published-at date does not match changelog date")
	}
	changesets := make([]string, 0)
	for _, entry := range record.Entries {
		for _, id := range entry.Provenance["changesets"] {
			if !slices.Contains(changesets, id) {
				changesets = append(changesets, id)
			}
		}
	}
	slices.Sort(changesets)
	declaredChangesets := slices.Clone(manifest.Changelog.Changesets)
	slices.Sort(declaredChangesets)
	if !slices.Equal(changesets, declaredChangesets) {
		return fmt.Errorf("verify release manifest: changeset provenance does not match changelog")
	}
	command := exec.Command("git", "cat-file", "-e", manifest.Source.Commit+"^{commit}")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("verify release manifest: source commit is not present: %s", strings.TrimSpace(string(output)))
	}
	for _, artifact := range manifest.Artifacts {
		parsed, err := url.Parse(artifact.URI)
		if err != nil {
			return fmt.Errorf("verify release manifest: artifact %q URI: %w", artifact.Name, err)
		}
		if parsed.IsAbs() {
			continue
		}
		path, err := repositoryPath(repository, artifact.URI)
		if err != nil {
			return fmt.Errorf("verify release manifest: artifact %q: %w", artifact.Name, err)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("verify release manifest: read artifact %q: %w", artifact.Name, err)
		}
		if digestBytes(contents) != artifact.Digest {
			return fmt.Errorf("verify release manifest: artifact digest for %q does not match", artifact.Name)
		}
	}
	return nil
}

func ValidateReleaseManifestProfile(manifest ReleaseManifest, profile Profile) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.releaseManifestVersion() != manifest.Version {
		return fmt.Errorf("profile does not enable Release Manifest %s", manifest.Version)
	}
	if manifest.Source.Repository != profile.Repository {
		return fmt.Errorf("source repository conflicts with the project profile")
	}
	stream, err := profile.releaseStream(manifest.ReleaseUnit, manifest.Stream)
	if err != nil {
		return err
	}
	if stream.ReleaseLine != manifest.ReleaseLine || stream.Channel != manifest.Channel {
		return fmt.Errorf("release stream metadata conflicts with the project profile")
	}
	if stream.Changelog != manifest.Changelog.Path {
		return fmt.Errorf("changelog path %q does not match stream path %q", manifest.Changelog.Path, stream.Changelog)
	}
	if profile.ReleasePattern != "" {
		pattern, _ := regexp.Compile(profile.ReleasePattern)
		if !pattern.MatchString(manifest.Release) {
			return fmt.Errorf("release %q does not match the project release-pattern", manifest.Release)
		}
	}
	return nil
}

func digestBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return fmt.Sprintf("sha256:%x", digest)
}

type ReleaseSource struct {
	Repository string `json:"repository" yaml:"repository"`
	Commit     string `json:"commit" yaml:"commit"`
	Tag        string `json:"tag" yaml:"tag"`
}

type ReleaseChangelog struct {
	Path       string   `json:"path" yaml:"path"`
	Digest     string   `json:"digest" yaml:"digest"`
	Changesets []string `json:"changesets" yaml:"changesets"`
}

type ReleaseArtifact struct {
	Name      string `json:"name" yaml:"name"`
	URI       string `json:"uri" yaml:"uri"`
	Digest    string `json:"digest" yaml:"digest"`
	MediaType string `json:"media_type" yaml:"media-type"`
}

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var fullObjectNamePattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func ParseReleaseManifest(contents []byte) (ReleaseManifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ReleaseManifest{}, errors.New("decode release manifest: expected exactly one YAML document")
		}
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return ReleaseManifest{}, errors.New("decode release manifest: root must be a mapping")
	}
	if err := validateIntentDocument(contents, &document); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	var manifest ReleaseManifest
	if err := document.Decode(&manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	manifest.node = &document
	if err := validateReleaseManifest(manifest, document.Content[0]); err != nil {
		return ReleaseManifest{}, err
	}
	return manifest, nil
}

func MarshalReleaseManifest(manifest ReleaseManifest) ([]byte, error) {
	if manifest.node == nil {
		return nil, errors.New("marshal release manifest: source document is unavailable")
	}
	contents, err := yaml.Marshal(manifest.node)
	if err != nil {
		return nil, fmt.Errorf("marshal release manifest: %w", err)
	}
	return contents, nil
}

func validateReleaseManifest(manifest ReleaseManifest, root *yaml.Node) error {
	for _, field := range []string{
		"intent-release-manifest", "release-unit", "release", "stream",
		"release-line", "channel", "published-at", "source", "changelog",
		"artifacts", "provenance",
	} {
		if mappingValue(root, field) == nil {
			return fmt.Errorf("invalid release manifest: missing %s", field)
		}
	}
	if manifest.Version != "0.1.0" && manifest.Version != "1.0.0" {
		return fmt.Errorf("invalid release manifest: unsupported specification version %q", manifest.Version)
	}
	for field, value := range map[string]string{
		"release-unit": manifest.ReleaseUnit, "release": manifest.Release,
		"stream": manifest.Stream, "release-line": manifest.ReleaseLine,
		"channel": manifest.Channel,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("invalid release manifest: %s must not be empty", field)
		}
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
		return fmt.Errorf("invalid release manifest: published-at must use RFC 3339: %w", err)
	}
	repository, err := url.Parse(manifest.Source.Repository)
	if err != nil || repository.Scheme == "" || repository.Host == "" {
		return fmt.Errorf("invalid release manifest: source repository must be an absolute URI")
	}
	if !fullObjectNamePattern.MatchString(manifest.Source.Commit) {
		return fmt.Errorf("invalid release manifest: source commit must be a full Git object name")
	}
	if strings.TrimSpace(manifest.Source.Tag) == "" {
		return fmt.Errorf("invalid release manifest: source tag must not be empty")
	}
	if err := validateRepositoryPath(manifest.Changelog.Path); err != nil {
		return fmt.Errorf("invalid release manifest: changelog path: %w", err)
	}
	if !sha256DigestPattern.MatchString(manifest.Changelog.Digest) {
		return fmt.Errorf("invalid release manifest: changelog digest must use sha256:<64 lowercase hex characters>")
	}
	seenChangesets := make(map[string]struct{}, len(manifest.Changelog.Changesets))
	for _, id := range manifest.Changelog.Changesets {
		if !changesetIDPattern.MatchString(id) {
			return fmt.Errorf("invalid release manifest: invalid changeset identifier %q", id)
		}
		if _, ok := seenChangesets[id]; ok {
			return fmt.Errorf("invalid release manifest: duplicate changeset identifier %q", id)
		}
		seenChangesets[id] = struct{}{}
	}
	if len(manifest.Artifacts) == 0 {
		return errors.New("invalid release manifest: artifacts must not be empty")
	}
	seenArtifacts := make(map[string]struct{}, len(manifest.Artifacts))
	for index, artifact := range manifest.Artifacts {
		if strings.TrimSpace(artifact.Name) == "" || strings.TrimSpace(artifact.URI) == "" || strings.TrimSpace(artifact.MediaType) == "" {
			return fmt.Errorf("invalid release manifest: artifact %d requires name, uri, and media-type", index+1)
		}
		if !sha256DigestPattern.MatchString(artifact.Digest) {
			return fmt.Errorf("invalid release manifest: artifact digest for %q must use sha256:<64 lowercase hex characters>", artifact.Name)
		}
		if _, ok := seenArtifacts[artifact.Name]; ok {
			return fmt.Errorf("invalid release manifest: duplicate artifact name %q", artifact.Name)
		}
		seenArtifacts[artifact.Name] = struct{}{}
	}
	return nil
}
