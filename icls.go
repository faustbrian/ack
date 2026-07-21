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
	"time"

	"go.yaml.in/yaml/v3"
)

type ICLSRecord struct {
	Version          string          `json:"intent_changelog" yaml:"intent-changelog"`
	ReleaseUnit      string          `json:"release_unit" yaml:"release-unit"`
	Release          string          `json:"release" yaml:"release"`
	SourceRepository string          `json:"source_repository,omitempty" yaml:"source-repository,omitempty"`
	Stream           string          `json:"stream,omitempty" yaml:"stream,omitempty"`
	ReleaseLine      string          `json:"release_line,omitempty" yaml:"release-line,omitempty"`
	Channel          string          `json:"channel" yaml:"channel"`
	Date             *string         `json:"date" yaml:"date"`
	Entries          []ICLSEntry     `json:"entries" yaml:"entries"`
	Amendments       []ICLSAmendment `json:"amendments" yaml:"amendments"`

	node *yaml.Node
}

type ICLSAmendment struct {
	ID         string              `json:"id" yaml:"id"`
	Date       string              `json:"date" yaml:"date"`
	Summary    string              `json:"summary" yaml:"summary"`
	Provenance map[string][]string `json:"provenance" yaml:"provenance"`
}

type ICLSEntry struct {
	ID         string              `json:"id" yaml:"id"`
	Type       string              `json:"type" yaml:"type"`
	Summary    string              `json:"summary" yaml:"summary"`
	Rationale  string              `json:"rationale" yaml:"rationale"`
	Impact     string              `json:"impact" yaml:"impact"`
	Audiences  []string            `json:"audiences" yaml:"audiences"`
	Migration  *string             `json:"migration" yaml:"migration"`
	Affects    []string            `json:"affects" yaml:"affects"`
	Provenance map[string][]string `json:"provenance" yaml:"provenance"`
	Relations  ICLSRelations       `json:"relations" yaml:"relations"`
	Disclosure Disclosure          `json:"disclosure" yaml:"disclosure"`
}

type ICLSRelations struct {
	Reverts    []string `json:"reverts" yaml:"reverts"`
	Supersedes []string `json:"supersedes" yaml:"supersedes"`
}

func ParseICLSRecord(contents []byte) (ICLSRecord, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return ICLSRecord{}, fmt.Errorf("decode Intent Changelog record: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ICLSRecord{}, errors.New("decode Intent Changelog record: expected exactly one YAML document")
		}
		return ICLSRecord{}, fmt.Errorf("decode Intent Changelog record: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return ICLSRecord{}, errors.New("decode Intent Changelog record: root must be a mapping")
	}
	if err := validateIntentDocument(contents, &document); err != nil {
		return ICLSRecord{}, fmt.Errorf("decode Intent Changelog record: %w", err)
	}

	var record ICLSRecord
	if err := document.Decode(&record); err != nil {
		return ICLSRecord{}, fmt.Errorf("decode Intent Changelog record: %w", err)
	}
	record.node = &document
	if err := validateICLSRecord(record, document.Content[0]); err != nil {
		return ICLSRecord{}, err
	}

	return record, nil
}

func MarshalICLSRecord(record ICLSRecord) ([]byte, error) {
	if record.node == nil {
		return nil, errors.New("marshal Intent Changelog record: source document is unavailable")
	}
	contents, err := yaml.Marshal(record.node)
	if err != nil {
		return nil, fmt.Errorf("marshal Intent Changelog record: %w", err)
	}
	return contents, nil
}

func ValidateICLSRecordProfile(record ICLSRecord, profile Profile) error {
	if profile.changelogVersion() != record.Version {
		return fmt.Errorf("invalid Intent Changelog record: profile does not enable Intent Changelog %s", record.Version)
	}
	if _, ok := profile.ReleaseUnits[record.ReleaseUnit]; !ok {
		return fmt.Errorf("invalid Intent Changelog record: release unit %q is not accepted by the project profile", record.ReleaseUnit)
	}
	if len(profile.Channels) > 0 && !slices.Contains(profile.Channels, record.Channel) {
		return fmt.Errorf("invalid Intent Changelog record: channel %q is not accepted by the project profile", record.Channel)
	}
	if isStructuredIntentVersion(record.Version) {
		if record.SourceRepository != profile.Repository {
			return fmt.Errorf("invalid Intent Changelog record: source-repository conflicts with the project profile")
		}
		unit := profile.ReleaseUnits[record.ReleaseUnit]
		stream, ok := unit.Streams[record.Stream]
		if !ok {
			return fmt.Errorf("invalid Intent Changelog record: stream %q is not accepted for release unit %q", record.Stream, record.ReleaseUnit)
		}
		if stream.Channel != record.Channel || stream.ReleaseLine != record.ReleaseLine {
			return fmt.Errorf("invalid Intent Changelog record: stream %q metadata conflicts with the project profile", record.Stream)
		}
	}
	for _, entry := range record.Entries {
		if len(profile.ReleaseTypes) > 0 && !slices.Contains(profile.ReleaseTypes, entry.Type) {
			return fmt.Errorf("invalid Intent Changelog record: entry %q type %q is not accepted by the project profile", entry.ID, entry.Type)
		}
		for _, audience := range entry.Audiences {
			if len(profile.Audiences) > 0 && !slices.Contains(profile.Audiences, audience) {
				return fmt.Errorf("invalid Intent Changelog record: entry %q audience %q is not accepted by the project profile", entry.ID, audience)
			}
		}
		if err := validateDisclosure(entry.Disclosure, record.Version, profile.Disclosures); err != nil {
			return fmt.Errorf("invalid Intent Changelog record: entry %q: %w", entry.ID, err)
		}
	}
	return nil
}

func validateICLSRecord(record ICLSRecord, root *yaml.Node) error {
	for _, field := range []string{"intent-changelog", "release-unit", "release", "channel", "date", "entries"} {
		if mappingValue(root, field) == nil {
			return fmt.Errorf("invalid Intent Changelog record: missing %s", field)
		}
	}
	if record.Version != "0.1.0" && !isStructuredIntentVersion(record.Version) {
		return fmt.Errorf("invalid Intent Changelog record: unsupported specification version %q", record.Version)
	}
	if isStructuredIntentVersion(record.Version) {
		for _, field := range []string{"source-repository", "stream", "release-line", "amendments"} {
			if mappingValue(root, field) == nil {
				return fmt.Errorf("invalid Intent Changelog record: missing %s", field)
			}
		}
		if strings.TrimSpace(record.Stream) == "" || strings.TrimSpace(record.ReleaseLine) == "" {
			return errors.New("invalid Intent Changelog record: stream and release-line must not be empty")
		}
		if err := validateAbsoluteURI(record.SourceRepository); err != nil {
			return fmt.Errorf("invalid Intent Changelog record: source-repository %w", err)
		}
	}
	if strings.TrimSpace(record.ReleaseUnit) == "" || strings.TrimSpace(record.Release) == "" || strings.TrimSpace(record.Channel) == "" {
		return errors.New("invalid Intent Changelog record: release-unit, release, and channel must not be empty")
	}
	if record.Date != nil {
		if _, err := time.Parse(time.DateOnly, *record.Date); err != nil {
			return fmt.Errorf("invalid Intent Changelog record: date must use YYYY-MM-DD: %w", err)
		}
	}

	entriesNode := mappingValue(root, "entries")
	if entriesNode.Kind != yaml.SequenceNode {
		return errors.New("invalid Intent Changelog record: entries must be a sequence")
	}
	if len(entriesNode.Content) != len(record.Entries) {
		return errors.New("invalid Intent Changelog record: entries could not be decoded")
	}
	seen := make(map[string]struct{}, len(record.Entries))
	for index, entry := range record.Entries {
		node := entriesNode.Content[index]
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("invalid Intent Changelog record: entry %d must be a mapping", index+1)
		}
		if err := validateICLSEntry(record.Version, record.ReleaseUnit, entry, node); err != nil {
			return fmt.Errorf("invalid Intent Changelog record: entry %d: %w", index+1, err)
		}
		if _, ok := seen[entry.ID]; ok {
			return fmt.Errorf("invalid Intent Changelog record: duplicate entry id %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
	}
	seenAmendments := make(map[string]struct{}, len(record.Amendments))
	for index, amendment := range record.Amendments {
		if !changesetIDPattern.MatchString(amendment.ID) {
			return fmt.Errorf("invalid Intent Changelog record: amendment %d has invalid id %q", index+1, amendment.ID)
		}
		if _, ok := seenAmendments[amendment.ID]; ok {
			return fmt.Errorf("invalid Intent Changelog record: duplicate amendment id %q", amendment.ID)
		}
		seenAmendments[amendment.ID] = struct{}{}
		if _, err := time.Parse(time.DateOnly, amendment.Date); err != nil {
			return fmt.Errorf("invalid Intent Changelog record: amendment %q date must use YYYY-MM-DD", amendment.ID)
		}
		if strings.TrimSpace(amendment.Summary) == "" || !hasProvenance(amendment.Provenance) {
			return fmt.Errorf("invalid Intent Changelog record: amendment %q requires summary and provenance", amendment.ID)
		}
	}

	return nil
}

func validateICLSEntry(version, releaseUnit string, entry ICLSEntry, node *yaml.Node) error {
	required := []string{"id", "type", "summary", "rationale", "impact", "audiences", "migration", "affects", "provenance", "relations", "disclosure"}
	for _, field := range required {
		if mappingValue(node, field) == nil {
			return fmt.Errorf("missing %s", field)
		}
	}
	for field, value := range map[string]string{
		"id": entry.ID, "type": entry.Type, "summary": entry.Summary,
		"rationale": entry.Rationale, "impact": entry.Impact,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must not be empty", field)
		}
	}
	if entry.Rationale == "self-evident" {
		return errors.New("rationale self-evident requires a project profile")
	}
	if len(entry.Audiences) == 0 {
		return errors.New("audiences must not be empty")
	}
	if !slices.Contains(entry.Affects, releaseUnit) {
		return fmt.Errorf("affects must include release unit %q", releaseUnit)
	}
	if entry.Impact == "major" && (entry.Migration == nil || strings.TrimSpace(*entry.Migration) == "") {
		return errors.New("major impact requires migration guidance")
	}
	if err := validateDisclosure(entry.Disclosure, version, nil); err != nil {
		return err
	}
	if !hasProvenance(entry.Provenance) {
		return errors.New("provenance must contain at least one stable reference")
	}
	relations := mappingValue(node, "relations")
	if relations == nil || relations.Kind != yaml.MappingNode || mappingValue(relations, "reverts") == nil || mappingValue(relations, "supersedes") == nil {
		return errors.New("relations must contain reverts and supersedes")
	}

	return nil
}

func hasProvenance(provenance map[string][]string) bool {
	for _, references := range provenance {
		for _, reference := range references {
			if strings.TrimSpace(reference) != "" {
				return true
			}
		}
	}
	return false
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func RenderICLSRecord(record ICLSRecord) string {
	var output strings.Builder
	fmt.Fprintf(&output, "# %s %s\n\n", record.ReleaseUnit, record.Release)
	if record.Date == nil {
		output.WriteString("**Unreleased**")
	} else {
		fmt.Fprintf(&output, "Released %s", *record.Date)
	}
	fmt.Fprintf(&output, " on the `%s` channel.\n", record.Channel)

	for _, entry := range record.Entries {
		switch entry.Disclosure.State {
		case "embargoed":
			continue
		case "redacted":
			output.WriteString("\n- **redacted**: A change is not publicly disclosed.\n")
			continue
		}

		fmt.Fprintf(&output, "\n- **%s**: %s\n", entry.Type, entry.Summary)
		fmt.Fprintf(&output, "  - Why: %s\n", strings.ReplaceAll(entry.Rationale, "\n", " "))
		fmt.Fprintf(&output, "  - Impact: %s\n", entry.Impact)
		fmt.Fprintf(&output, "  - Audiences: %s\n", strings.Join(entry.Audiences, ", "))
		if entry.Migration != nil {
			fmt.Fprintf(&output, "  - Migration: %s\n", strings.ReplaceAll(*entry.Migration, "\n", " "))
		}
	}

	return output.String()
}

var unreleasedDateLine = regexp.MustCompile(`(?m)^date: null\r?$`)

func PublishICLSRecord(path, date string) error {
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return fmt.Errorf("publish Intent Changelog record: date must use YYYY-MM-DD: %w", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("publish Intent Changelog record: read: %w", err)
	}
	record, err := ParseICLSRecord(contents)
	if err != nil {
		return fmt.Errorf("publish Intent Changelog record: %w", err)
	}
	if record.Date != nil {
		return errors.New("publish Intent Changelog record: record is already released")
	}
	if matches := unreleasedDateLine.FindAllIndex(contents, -1); len(matches) != 1 {
		return errors.New("publish Intent Changelog record: canonical top-level date must be exactly 'date: null'")
	}
	updated := unreleasedDateLine.ReplaceAll(contents, []byte("date: "+date))
	if _, err := ParseICLSRecord(updated); err != nil {
		return fmt.Errorf("publish Intent Changelog record: validate result: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("publish Intent Changelog record: stat: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ack-publish-*")
	if err != nil {
		return fmt.Errorf("publish Intent Changelog record: create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		temporary.Close()
		return fmt.Errorf("publish Intent Changelog record: set permissions: %w", err)
	}
	if _, err := temporary.Write(updated); err != nil {
		temporary.Close()
		return fmt.Errorf("publish Intent Changelog record: write: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("publish Intent Changelog record: close: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish Intent Changelog record: replace: %w", err)
	}

	return nil
}
