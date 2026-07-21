package ack

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func VerifyPullRequest(ctx context.Context, repository, head string, record PullRequest, profile Profile) error {
	if err := ValidatePullRequest(record, profile); err != nil {
		return err
	}
	resolved, err := runGitCommand(ctx, repository, "rev-parse", "--verify", head+"^{commit}")
	if err != nil {
		return fmt.Errorf("verify Intent Pull Requests evidence revision: %w", err)
	}
	if strings.TrimSpace(string(resolved)) != record.EvidenceRevision {
		return fmt.Errorf("verify Intent Pull Requests evidence revision: record is stale for %s", head)
	}
	base, err := runGitCommand(ctx, repository, "rev-parse", "--verify", record.BaseRevision+"^{commit}")
	if err != nil || strings.TrimSpace(string(base)) != record.BaseRevision {
		return fmt.Errorf("verify Intent Pull Requests base revision: record does not identify an available base commit")
	}
	if _, err := runGitCommand(ctx, repository, "merge-base", "--is-ancestor", record.BaseRevision, record.EvidenceRevision); err != nil {
		return fmt.Errorf("verify Intent Pull Requests revisions: base is not an ancestor of the evidence revision")
	}
	for _, id := range record.Provenance["changesets"] {
		locations, err := changesetIdentifierLocations(repository, profile, id)
		if err != nil {
			return err
		}
		if len(locations) == 0 {
			return fmt.Errorf("verify Intent Pull Requests changeset: identifier %q was not found", id)
		}
		for _, location := range locations {
			if strings.Contains(location, "#") {
				continue
			}
			contents, err := os.ReadFile(location)
			if err != nil {
				return fmt.Errorf("verify Intent Pull Requests changeset %q: %w", id, err)
			}
			changeset, err := ParseIXSChangeset(contents)
			if err != nil {
				return err
			}
			if err := verifyPullRequestChangesetTargets(record, changeset); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyPullRequestChangesetTargets(record PullRequest, changeset IXSChangeset) error {
	for _, target := range changeset.Targets {
		matched := false
		for _, pullRequestTarget := range record.Targets {
			if target.ReleaseUnit != pullRequestTarget.ReleaseUnit || target.Stream != pullRequestTarget.Stream {
				continue
			}
			matched = true
			if target.Impact != string(pullRequestTarget.Impact) {
				return fmt.Errorf("verify Intent Pull Requests changeset %q: target %s@%s has conflicting impact", changeset.ID, target.ReleaseUnit, target.Stream)
			}
			if target.Impact == "major" && strings.TrimSpace(stringValue(target.Migration)) != strings.TrimSpace(stringValue(pullRequestTarget.Migration)) {
				return fmt.Errorf("verify Intent Pull Requests changeset %q: target %s@%s has conflicting migration", changeset.ID, target.ReleaseUnit, target.Stream)
			}
		}
		if !matched {
			return fmt.Errorf("verify Intent Pull Requests changeset %q: target %s@%s is missing from the pull request", changeset.ID, target.ReleaseUnit, target.Stream)
		}
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
