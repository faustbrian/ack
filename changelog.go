package ack

import (
	"fmt"
	"strings"
)

func GenerateChangelog(commits []Commit, profile Profile) string {
	groups := map[Impact][]Commit{
		ImpactMajor:       {},
		ImpactMinor:       {},
		ImpactPatch:       {},
		ImpactNone:        {},
		ImpactUnspecified: {},
	}
	malformed := make([]Commit, 0)
	for _, commit := range commits {
		if commit.ParseError != "" {
			malformed = append(malformed, commit)
			continue
		}
		report := Validate(commit.Message, profile)
		groups[report.EffectiveImpact] = append(groups[report.EffectiveImpact], commit)
	}

	var output strings.Builder
	output.WriteString("# Changelog\n")
	writeChangelogGroup(&output, "Major", groups[ImpactMajor])
	writeChangelogGroup(&output, "Minor", groups[ImpactMinor])
	writeChangelogGroup(&output, "Patch", groups[ImpactPatch])
	writeChangelogGroup(&output, "No release impact", groups[ImpactNone])
	writeChangelogGroup(&output, "Unspecified impact", groups[ImpactUnspecified])
	writeMalformedGroup(&output, malformed)

	return output.String()
}

func writeChangelogGroup(output *strings.Builder, title string, commits []Commit) {
	if len(commits) == 0 {
		return
	}
	fmt.Fprintf(output, "\n## %s\n", title)
	for index := len(commits) - 1; index >= 0; index-- {
		commit := commits[index]
		scope := ""
		if commit.Message.Scope != "" {
			scope = " `" + commit.Message.Scope + "`"
		}
		fmt.Fprintf(output, "\n- **%s**%s: %s (`%s`)\n", commit.Message.Type, scope, commit.Message.Description, shortHash(commit.Hash))
		if affected := commit.Message.TrailerValues("Affects"); len(affected) > 0 {
			fmt.Fprintf(output, "  - Affects: %s\n", strings.Join(affected, ", "))
		}
		if migrations := commit.Message.TrailerValues("Migration"); len(migrations) > 0 {
			fmt.Fprintf(output, "  - Migration: %s\n", strings.ReplaceAll(migrations[0], "\n", " "))
		}
	}
}

func writeMalformedGroup(output *strings.Builder, commits []Commit) {
	if len(commits) == 0 {
		return
	}
	output.WriteString("\n## Unclassified\n")
	for index := len(commits) - 1; index >= 0; index-- {
		commit := commits[index]
		fmt.Fprintf(output, "\n- `%s`: %s\n", shortHash(commit.Hash), commit.ParseError)
	}
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}
