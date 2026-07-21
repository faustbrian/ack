package ack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

type PullRequest struct {
	Version          string              `json:"intent_pull_request" yaml:"intent-pull-request"`
	ID               string              `json:"id" yaml:"id"`
	Title            string              `json:"title" yaml:"title"`
	State            string              `json:"state" yaml:"state"`
	Summary          string              `json:"summary" yaml:"summary"`
	Rationale        string              `json:"rationale" yaml:"rationale"`
	Approach         string              `json:"approach" yaml:"approach"`
	TradeOffs        string              `json:"trade_offs" yaml:"trade-offs"`
	Targets          []PullRequestTarget `json:"targets" yaml:"targets"`
	Risks            []string            `json:"risks" yaml:"risks"`
	Rollout          *string             `json:"rollout" yaml:"rollout"`
	Rollback         *string             `json:"rollback" yaml:"rollback"`
	Verification     []VerificationClaim `json:"verification" yaml:"verification"`
	Provenance       map[string][]string `json:"provenance" yaml:"provenance"`
	Disclosure       Disclosure          `json:"disclosure" yaml:"disclosure"`
	BaseRevision     string              `json:"base_revision" yaml:"base-revision"`
	EvidenceRevision string              `json:"evidence_revision" yaml:"evidence-revision"`

	node *yaml.Node
}

type PullRequestTarget struct {
	ReleaseUnit string  `json:"release_unit" yaml:"release-unit"`
	Stream      string  `json:"stream" yaml:"stream"`
	Impact      Impact  `json:"impact" yaml:"impact"`
	Migration   *string `json:"migration" yaml:"migration"`
}

type VerificationClaim struct {
	Name     string  `json:"name" yaml:"name"`
	Status   string  `json:"status" yaml:"status"`
	Command  *string `json:"command" yaml:"command"`
	Evidence string  `json:"evidence" yaml:"evidence"`
}

func ParsePullRequest(contents []byte) (PullRequest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return PullRequest{}, fmt.Errorf("decode Intent Pull Requests record: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return PullRequest{}, errors.New("decode Intent Pull Requests record: expected exactly one YAML document")
		}
		return PullRequest{}, fmt.Errorf("decode Intent Pull Requests record: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return PullRequest{}, errors.New("decode Intent Pull Requests record: root must be a mapping")
	}
	if err := validateIntentDocument(contents, &document); err != nil {
		return PullRequest{}, fmt.Errorf("decode Intent Pull Requests record: %w", err)
	}
	var record PullRequest
	if err := document.Decode(&record); err != nil {
		return PullRequest{}, fmt.Errorf("decode Intent Pull Requests record: %w", err)
	}
	record.node = &document
	if err := validatePullRequest(record, document.Content[0]); err != nil {
		return PullRequest{}, err
	}
	return record, nil
}

func validatePullRequest(record PullRequest, root *yaml.Node) error {
	for _, field := range []string{"intent-pull-request", "id", "title", "state", "summary", "rationale", "approach", "trade-offs", "targets", "risks", "rollout", "rollback", "verification", "provenance", "disclosure", "base-revision", "evidence-revision"} {
		if mappingValue(root, field) == nil {
			return fmt.Errorf("invalid Intent Pull Requests record: missing %s", field)
		}
	}
	if record.Version != "1.0.0" {
		return fmt.Errorf("invalid Intent Pull Requests record: unsupported specification version %q", record.Version)
	}
	if !changesetIDPattern.MatchString(record.ID) {
		return fmt.Errorf("invalid Intent Pull Requests record: invalid id %q", record.ID)
	}
	if _, err := Parse(record.Title); err != nil {
		return fmt.Errorf("invalid Intent Pull Requests record: title: %w", err)
	}
	if !slices.Contains([]string{"draft", "ready", "merged", "closed"}, record.State) {
		return fmt.Errorf("invalid Intent Pull Requests record: unsupported state %q", record.State)
	}
	if record.State != "draft" {
		for field, value := range map[string]string{"summary": record.Summary, "rationale": record.Rationale, "approach": record.Approach, "trade-offs": record.TradeOffs} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("invalid Intent Pull Requests record: %s must not be empty when state is %s", field, record.State)
			}
		}
	}
	if strings.EqualFold(normalizeText(record.Summary), normalizeText(record.Rationale)) {
		return errors.New("invalid Intent Pull Requests record: rationale must not repeat summary")
	}
	if !hasProvenance(record.Provenance) {
		return errors.New("invalid Intent Pull Requests record: provenance must contain a stable reference")
	}
	if err := validateDisclosure(record.Disclosure, record.Version, nil); err != nil {
		return fmt.Errorf("invalid Intent Pull Requests record: %w", err)
	}
	if len(record.Targets) == 0 && record.State != "draft" {
		return errors.New("invalid Intent Pull Requests record: targets must not be empty when ready")
	}
	seen := map[string]struct{}{}
	for _, target := range record.Targets {
		key := target.ReleaseUnit + "@" + target.Stream
		if strings.TrimSpace(target.ReleaseUnit) == "" || strings.TrimSpace(target.Stream) == "" || !validImpact(target.Impact) {
			return fmt.Errorf("invalid Intent Pull Requests record: invalid target %q", key)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("invalid Intent Pull Requests record: duplicate target %q", key)
		}
		seen[key] = struct{}{}
		if target.Impact == ImpactMajor && (target.Migration == nil || strings.TrimSpace(*target.Migration) == "") {
			return fmt.Errorf("invalid Intent Pull Requests record: major target %q requires migration", key)
		}
	}
	for _, claim := range record.Verification {
		if strings.TrimSpace(claim.Name) == "" || !slices.Contains([]string{"passed", "failed", "skipped", "unavailable", "not-applicable"}, claim.Status) {
			return fmt.Errorf("invalid Intent Pull Requests record: invalid verification claim %q", claim.Name)
		}
		if strings.TrimSpace(claim.Evidence) == "" {
			return fmt.Errorf("invalid Intent Pull Requests record: verification %q requires evidence", claim.Name)
		}
		if claim.Status == "passed" && (claim.Command == nil || strings.TrimSpace(*claim.Command) == "") {
			return fmt.Errorf("invalid Intent Pull Requests record: passed verification %q requires command", claim.Name)
		}
	}
	if record.State != "draft" && (!fullObjectNamePattern.MatchString(record.BaseRevision) || !fullObjectNamePattern.MatchString(record.EvidenceRevision)) {
		return errors.New("invalid Intent Pull Requests record: ready record requires full Git base-revision and evidence-revision object names")
	}
	return nil
}

func ValidatePullRequest(record PullRequest, profile Profile) error {
	if profile.Specifications.PullRequests != "" && profile.Specifications.PullRequests != record.Version {
		return fmt.Errorf("invalid Intent Pull Requests record: profile does not enable Intent Pull Requests %s", record.Version)
	}
	if profile.PullRequests.RequireReady && record.State != "ready" && record.State != "merged" {
		return fmt.Errorf("invalid Intent Pull Requests record: state %q is not ready for integration", record.State)
	}
	for _, target := range record.Targets {
		unit, ok := profile.ReleaseUnits[target.ReleaseUnit]
		if !ok {
			return fmt.Errorf("invalid Intent Pull Requests record: release unit %q is not accepted by the project profile", target.ReleaseUnit)
		}
		if _, ok := unit.Streams[target.Stream]; !ok {
			return fmt.Errorf("invalid Intent Pull Requests record: stream %q is not accepted for release unit %q", target.Stream, target.ReleaseUnit)
		}
	}
	maxTitleLength := profile.PullRequests.MaxTitleLength
	if maxTitleLength == 0 {
		maxTitleLength = profile.MaxHeaderLength
	}
	if maxTitleLength > 0 && len([]rune(record.Title)) > maxTitleLength {
		return fmt.Errorf("invalid Intent Pull Requests record: title exceeds %d characters", maxTitleLength)
	}
	body := RenderPullRequestBody(record)
	if profile.PullRequests.MaxBodyLength > 0 && len([]rune(body)) > profile.PullRequests.MaxBodyLength {
		return fmt.Errorf("invalid Intent Pull Requests record: rendered body exceeds %d characters", profile.PullRequests.MaxBodyLength)
	}
	for _, claim := range record.Verification {
		if len(profile.PullRequests.VerificationStatuses) > 0 && !slices.Contains(profile.PullRequests.VerificationStatuses, claim.Status) {
			return fmt.Errorf("invalid Intent Pull Requests record: verification status %q is not accepted by the project profile", claim.Status)
		}
	}
	if err := validateDisclosure(record.Disclosure, record.Version, profile.Disclosures); err != nil {
		return fmt.Errorf("invalid Intent Pull Requests record: %w", err)
	}
	return nil
}

func RenderPullRequestBody(record PullRequest) string {
	var body strings.Builder
	fmt.Fprintf(&body, "## What\n\n%s\n\n## Why\n\n%s\n\n## Approach\n\n%s\n\n", record.Summary, record.Rationale, record.Approach)
	fmt.Fprintf(&body, "### Trade-offs\n\n%s\n\n## Impact\n\n", record.TradeOffs)
	for _, target := range record.Targets {
		fmt.Fprintf(&body, "- `%s@%s`: %s\n", target.ReleaseUnit, target.Stream, target.Impact)
	}
	fmt.Fprint(&body, "\n## Verification\n\n")
	for _, claim := range record.Verification {
		fmt.Fprintf(&body, "- **%s — %s:** %s\n", claim.Name, claim.Status, claim.Evidence)
	}
	return body.String()
}

func MarshalPullRequest(record PullRequest) ([]byte, error) {
	if record.node == nil {
		return nil, errors.New("marshal Intent Pull Requests record: source document is unavailable")
	}
	contents, err := yaml.Marshal(record.node)
	if err != nil {
		return nil, fmt.Errorf("marshal Intent Pull Requests record: %w", err)
	}
	return contents, nil
}

func CreatePullRequest(repository string, profile Profile, contents []byte) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	record, err := ParsePullRequest(contents)
	if err != nil {
		return "", err
	}
	if err := ValidatePullRequest(record, profile); err != nil {
		return "", err
	}
	directory, err := repositoryPath(repository, profile.PullRequests.Directory)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create Intent Pull Requests record: create directory: %w", err)
	}
	path := filepath.Join(directory, record.ID+".yaml")
	if err := writeExclusive(path, contents); err != nil {
		return "", fmt.Errorf("create Intent Pull Requests record: %w", err)
	}
	return path, nil
}
