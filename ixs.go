package ack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

type IXSChangeset struct {
	Version          string              `json:"intent_changeset" yaml:"intent-changeset"`
	ID               string              `json:"id" yaml:"id"`
	SourceRepository string              `json:"source_repository,omitempty" yaml:"source-repository,omitempty"`
	Summary          string              `json:"summary" yaml:"summary"`
	Rationale        string              `json:"rationale" yaml:"rationale"`
	Targets          []IXSTarget         `json:"targets" yaml:"targets"`
	Provenance       map[string][]string `json:"provenance" yaml:"provenance"`
	Relations        ICLSRelations       `json:"relations" yaml:"relations"`
	Disclosure       Disclosure          `json:"disclosure" yaml:"disclosure"`

	node *yaml.Node
}

type IXSTarget struct {
	ReleaseUnit string   `json:"release_unit" yaml:"release-unit"`
	Stream      string   `json:"stream,omitempty" yaml:"stream,omitempty"`
	Type        string   `json:"type" yaml:"type"`
	Impact      string   `json:"impact" yaml:"impact"`
	Audiences   []string `json:"audiences" yaml:"audiences"`
	Migration   *string  `json:"migration" yaml:"migration"`
}

var changesetIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func ParseIXSChangeset(contents []byte) (IXSChangeset, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return IXSChangeset{}, fmt.Errorf("decode Intent Changesets record: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return IXSChangeset{}, errors.New("decode Intent Changesets record: expected exactly one YAML document")
		}
		return IXSChangeset{}, fmt.Errorf("decode Intent Changesets record: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return IXSChangeset{}, errors.New("decode Intent Changesets record: root must be a mapping")
	}
	if err := validateIntentDocument(contents, &document); err != nil {
		return IXSChangeset{}, fmt.Errorf("decode Intent Changesets record: %w", err)
	}

	var changeset IXSChangeset
	if err := document.Decode(&changeset); err != nil {
		return IXSChangeset{}, fmt.Errorf("decode Intent Changesets record: %w", err)
	}
	changeset.node = &document
	if err := validateIXSChangeset(changeset, document.Content[0]); err != nil {
		return IXSChangeset{}, err
	}
	return changeset, nil
}

func validateIXSChangeset(changeset IXSChangeset, root *yaml.Node) error {
	for _, field := range []string{"intent-changeset", "id", "summary", "rationale", "targets", "provenance", "relations", "disclosure"} {
		if mappingValue(root, field) == nil {
			return fmt.Errorf("invalid Intent Changesets record: missing %s", field)
		}
	}
	if changeset.Version != "0.1.0" && !isStructuredIntentVersion(changeset.Version) {
		return fmt.Errorf("invalid Intent Changesets record: unsupported specification version %q", changeset.Version)
	}
	if isStructuredIntentVersion(changeset.Version) {
		if mappingValue(root, "source-repository") == nil {
			return fmt.Errorf("invalid Intent Changesets record: missing source-repository")
		}
		if err := validateAbsoluteURI(changeset.SourceRepository); err != nil {
			return fmt.Errorf("invalid Intent Changesets record: source-repository %w", err)
		}
	}
	if !changesetIDPattern.MatchString(changeset.ID) {
		return fmt.Errorf("invalid Intent Changesets record: invalid id %q", changeset.ID)
	}
	if strings.TrimSpace(changeset.Summary) == "" || strings.TrimSpace(changeset.Rationale) == "" {
		return errors.New("invalid Intent Changesets record: summary and rationale must not be empty")
	}
	if strings.EqualFold(normalizeText(changeset.Summary), normalizeText(changeset.Rationale)) {
		return errors.New("invalid Intent Changesets record: rationale must not repeat summary")
	}
	if !hasProvenance(changeset.Provenance) {
		return errors.New("invalid Intent Changesets record: provenance must contain a stable reference")
	}
	if err := validateDisclosure(changeset.Disclosure, changeset.Version, nil); err != nil {
		return fmt.Errorf("invalid Intent Changesets record: %w", err)
	}
	relations := mappingValue(root, "relations")
	if relations == nil || mappingValue(relations, "reverts") == nil || mappingValue(relations, "supersedes") == nil {
		return errors.New("invalid Intent Changesets record: relations must contain reverts and supersedes")
	}
	for kind, identifiers := range map[string][]string{
		"reverts": changeset.Relations.Reverts, "supersedes": changeset.Relations.Supersedes,
	} {
		seenRelations := make(map[string]struct{}, len(identifiers))
		for _, identifier := range identifiers {
			if !changesetIDPattern.MatchString(identifier) || identifier == changeset.ID {
				return fmt.Errorf("invalid Intent Changesets record: invalid relation %s %q", kind, identifier)
			}
			if _, ok := seenRelations[identifier]; ok {
				return fmt.Errorf("invalid Intent Changesets record: duplicate relation %s %q", kind, identifier)
			}
			seenRelations[identifier] = struct{}{}
		}
	}
	if len(changeset.Targets) == 0 {
		return errors.New("invalid Intent Changesets record: targets must not be empty")
	}
	seen := make(map[string]struct{}, len(changeset.Targets))
	targetsNode := mappingValue(root, "targets")
	for index, target := range changeset.Targets {
		if targetsNode == nil || targetsNode.Kind != yaml.SequenceNode || index >= len(targetsNode.Content) {
			return errors.New("invalid Intent Changesets record: targets must be a sequence")
		}
		if mappingValue(targetsNode.Content[index], "migration") == nil {
			return fmt.Errorf("invalid Intent Changesets record: target %d: missing migration", index+1)
		}
		if err := validateIXSTarget(target); err != nil {
			return fmt.Errorf("invalid Intent Changesets record: target %d: %w", index+1, err)
		}
		if isStructuredIntentVersion(changeset.Version) && strings.TrimSpace(target.Stream) == "" {
			return fmt.Errorf("invalid Intent Changesets record: target %d: missing stream", index+1)
		}
		key := target.ReleaseUnit + "@" + target.Stream
		if _, ok := seen[key]; ok {
			if target.Stream == "" {
				return fmt.Errorf("invalid Intent Changesets record: duplicate release unit %q", target.ReleaseUnit)
			}
			return fmt.Errorf("invalid Intent Changesets record: duplicate target %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func MarshalIXSChangeset(changeset IXSChangeset) ([]byte, error) {
	if changeset.node == nil {
		return nil, errors.New("marshal Intent Changesets record: source document is unavailable")
	}
	contents, err := yaml.Marshal(changeset.node)
	if err != nil {
		return nil, fmt.Errorf("marshal Intent Changesets record: %w", err)
	}
	return contents, nil
}

func ValidateIXSChangeset(changeset IXSChangeset, profile Profile) error {
	if profile.changesetsVersion() != changeset.Version {
		return fmt.Errorf("invalid Intent Changesets record: profile does not enable Intent Changesets %s", changeset.Version)
	}
	if isStructuredIntentVersion(changeset.Version) && changeset.SourceRepository != profile.Repository {
		return fmt.Errorf("invalid Intent Changesets record: source-repository conflicts with the project profile")
	}
	if profile.Changesets.IDPattern != "" {
		pattern, err := regexp.Compile(profile.Changesets.IDPattern)
		if err != nil {
			return fmt.Errorf("invalid Intent Changesets record: project id-pattern: %w", err)
		}
		if !pattern.MatchString(changeset.ID) {
			return fmt.Errorf("invalid Intent Changesets record: id %q does not match the project id-pattern", changeset.ID)
		}
	}
	for _, target := range changeset.Targets {
		if len(profile.ReleaseUnits) > 0 {
			if _, ok := profile.ReleaseUnits[target.ReleaseUnit]; !ok {
				return fmt.Errorf("invalid Intent Changesets record: release unit %q is not accepted by the project profile", target.ReleaseUnit)
			}
		}
		if len(profile.ReleaseTypes) > 0 && !slices.Contains(profile.ReleaseTypes, target.Type) {
			return fmt.Errorf("invalid Intent Changesets record: type %q is not accepted by the project profile", target.Type)
		}
		if target.Stream != "" {
			policy := profile.ReleaseUnits[target.ReleaseUnit]
			if _, ok := policy.Streams[target.Stream]; !ok {
				return fmt.Errorf("invalid Intent Changesets record: stream %q is not accepted for release unit %q", target.Stream, target.ReleaseUnit)
			}
		}
		if !validImpact(Impact(target.Impact)) || len(profile.ReleaseImpacts) > 0 && !slices.Contains(profile.ReleaseImpacts, Impact(target.Impact)) {
			return fmt.Errorf("invalid Intent Changesets record: impact %q is not accepted by the project profile", target.Impact)
		}
		for _, audience := range target.Audiences {
			if len(profile.Audiences) > 0 && !slices.Contains(profile.Audiences, audience) {
				return fmt.Errorf("invalid Intent Changesets record: audience %q is not accepted by the project profile", audience)
			}
		}
	}
	if err := validateDisclosure(changeset.Disclosure, changeset.Version, profile.Disclosures); err != nil {
		return fmt.Errorf("invalid Intent Changesets record: %w", err)
	}
	return nil
}

func validateIXSTarget(target IXSTarget) error {
	if strings.TrimSpace(target.ReleaseUnit) == "" || strings.TrimSpace(target.Type) == "" || strings.TrimSpace(target.Impact) == "" {
		return errors.New("release-unit, type, and impact must not be empty")
	}
	if len(target.Audiences) == 0 {
		return errors.New("audiences must not be empty")
	}
	if target.Impact == "major" && (target.Migration == nil || strings.TrimSpace(*target.Migration) == "") {
		return errors.New("major impact requires migration guidance")
	}
	return nil
}
