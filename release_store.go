package ack

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var releaseFilenamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func CreateReleaseManifest(repository string, profile Profile, contents []byte) (string, error) {
	manifest, err := ParseReleaseManifest(contents)
	if err != nil {
		return "", err
	}
	if err := VerifyReleaseManifest(repository, profile, manifest); err != nil {
		return "", err
	}
	if !releaseFilenamePattern.MatchString(manifest.Release) {
		return "", fmt.Errorf("create release manifest: release %q is not safe as a canonical filename", manifest.Release)
	}
	stream, err := profile.releaseStream(manifest.ReleaseUnit, manifest.Stream)
	if err != nil {
		return "", err
	}
	directory, err := repositoryPath(repository, stream.Manifests)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create release manifest: create directory: %w", err)
	}
	path := filepath.Join(directory, manifest.Release+".yaml")
	if err := writeExclusive(path, contents); err != nil {
		return "", fmt.Errorf("create release manifest: %w", err)
	}
	return path, nil
}
