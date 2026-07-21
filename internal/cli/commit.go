package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/faustbrian/ack"
)

type commitDraft struct {
	ChangeType       string
	Scope            string
	Description      string
	Body             string
	Impact           ack.Impact
	Migration        string
	Affects          []string
	TargetImpacts    []string
	TargetMigrations []string
	Changesets       []string
}

func draftMessage(draft commitDraft) ack.Message {
	message := ack.Message{
		Type:        draft.ChangeType,
		Scope:       draft.Scope,
		Description: draft.Description,
		Body:        draft.Body,
	}
	if draft.Impact != "" && draft.Impact != ack.ImpactUnspecified {
		message.Trailers = append(message.Trailers, ack.Trailer{Token: "Impact", Value: string(draft.Impact)})
	}
	if draft.Migration != "" {
		message.Trailers = append(message.Trailers, ack.Trailer{Token: "Migration", Value: draft.Migration})
	}
	for _, component := range draft.Affects {
		message.Trailers = append(message.Trailers, ack.Trailer{Token: "Affects", Value: component})
	}
	for _, impact := range draft.TargetImpacts {
		message.Trailers = append(message.Trailers, ack.Trailer{Token: "Target-Impact", Value: impact})
	}
	for _, migration := range draft.TargetMigrations {
		message.Trailers = append(message.Trailers, ack.Trailer{Token: "Target-Migration", Value: migration})
	}
	for _, changeset := range draft.Changesets {
		message.Trailers = append(message.Trailers, ack.Trailer{Token: "Changeset", Value: changeset})
	}

	return message
}

func runCommitCreate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("commit create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK commit profile")
	accessible := flags.Bool("accessible", os.Getenv("ACK_ACCESSIBLE") != "", "use screen-reader-friendly prompts")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "commit create does not accept positional arguments")
		return 2
	}
	profile, err := loadOptionalProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	draft := commitDraft{Impact: ack.ImpactUnspecified}
	changesets := ""
	targetImpacts := ""
	targetMigrations := ""
	fields := []huh.Field{
		huh.NewInput().
			Title("Type").
			Suggestions(profileTypes(profile)).
			Validate(validateType).
			Value(&draft.ChangeType),
		huh.NewInput().
			Title("Primary scope").
			Description("Use the release unit first, for example apps/billing/refunds").
			Suggestions(profileScopes(profile)).
			Validate(validateScope).
			Value(&draft.Scope),
		huh.NewInput().
			Title("Description").
			Validate(requireValue("description")).
			Value(&draft.Description),
		huh.NewText().
			Title("Body").
			Description("Explain context and reasoning that the diff cannot preserve").
			Value(&draft.Body),
		huh.NewSelect[ack.Impact]().
			Title("Version impact").
			Description("This is the explicit release bump and overrides the type default").
			Options(
				huh.NewOption("none", ack.ImpactNone),
				huh.NewOption("patch", ack.ImpactPatch),
				huh.NewOption("minor", ack.ImpactMinor),
				huh.NewOption("major", ack.ImpactMajor),
			).
			Value(&draft.Impact),
		huh.NewInput().
			Title("Migration").
			Description("Required for major impact; otherwise optional").
			Validate(func(value string) error {
				if draft.Impact == ack.ImpactMajor && strings.TrimSpace(value) == "" {
					return errors.New("migration is required for major impact")
				}
				return nil
			}).
			Value(&draft.Migration),
		huh.NewInput().
			Title("Target impacts").
			Description("Comma-separated release-unit[@stream]=impact overrides").
			Value(&targetImpacts),
		huh.NewInput().
			Title("Target migrations").
			Description("Comma-separated release-unit[@stream]=guidance overrides").
			Value(&targetMigrations),
		huh.NewInput().
			Title("Changeset IDs").
			Description("Comma-separated verified changeset identifiers; never invent one").
			Value(&changesets),
	}
	if len(profile.Scopes) > 0 {
		options := make([]huh.Option[string], 0, len(profile.Scopes))
		for _, scope := range profileScopes(profile) {
			options = append(options, huh.NewOption(scope, scope))
		}
		fields = append(fields, huh.NewMultiSelect[string]().
			Title("Additional affected release units").
			Options(options...).
			Value(&draft.Affects))
	}

	form := huh.NewForm(huh.NewGroup(fields...)).
		WithInput(stdin).
		WithOutput(stderr).
		WithAccessible(*accessible)
	if err := form.Run(); err != nil {
		fmt.Fprintf(stderr, "commit form: %v\n", err)
		return 1
	}
	draft.Changesets = splitCommaSeparated(changesets)
	draft.TargetImpacts = splitCommaSeparated(targetImpacts)
	draft.TargetMigrations = splitCommaSeparated(targetMigrations)

	message := draftMessage(draft)
	report := ack.Validate(message, profile)
	if report.HasErrors() {
		writeReport(stderr, "text", report)
		return 1
	}
	fmt.Fprint(stdout, message.String())
	return 0
}

func profileScopes(profile ack.Profile) []string {
	scopes := make([]string, 0, len(profile.Scopes))
	for scope := range profile.Scopes {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes
}

func splitCommaSeparated(value string) []string {
	var values []string
	for item := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func profileTypes(profile ack.Profile) []string {
	if len(profile.Types) == 0 {
		return []string{"feat", "fix", "revert", "docs", "refactor", "perf", "test", "build", "ci", "chore"}
	}
	types := make([]string, 0, len(profile.Types))
	for changeType := range profile.Types {
		types = append(types, changeType)
	}
	sort.Strings(types)
	return types
}

func validateType(value string) error {
	if _, err := ack.Parse(value + ": description"); err != nil {
		return errors.New("use lowercase letters, digits, and hyphens")
	}
	return nil
}

func validateScope(value string) error {
	if value == "" {
		return nil
	}
	if _, err := ack.Parse("fix(" + value + "): description"); err != nil {
		return errors.New("use lowercase letters, digits, dots, slashes, and hyphens")
	}
	return nil
}

func requireValue(name string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
}
