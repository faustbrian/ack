package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/faustbrian/ack"
)

func runChangesetCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changeset check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "changeset check requires exactly one record or -")
		return 2
	}
	contents, err := readInput(flags.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	changeset, err := ack.ParseIXSChangeset(contents)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *profilePath != "" {
		profile, err := ack.LoadProfile(*profilePath)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if err := ack.ValidateIXSChangeset(changeset, profile); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "valid Intent Changesets record: %s (%d targets)\n", changeset.ID, len(changeset.Targets))
	return 0
}

func runChangesetCreate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changeset create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() != 1 {
		fmt.Fprintln(stderr, "changeset create requires --profile and one record or -")
		return 2
	}
	profile, err := ack.LoadProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	contents, err := readInput(flags.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	path, err := ack.CreateIXSChangeset(*repository, profile, contents)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "created Intent Changesets record: %s\n", path)
	return 0
}

func runChangesetLinks(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changeset links", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "Git repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() > 1 {
		fmt.Fprintln(stderr, "changeset links requires --profile and accepts at most one revision range")
		return 2
	}
	profile, err := ack.LoadProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	revisionRange := "HEAD"
	if flags.NArg() == 1 {
		revisionRange = flags.Arg(0)
	}
	report, err := ack.ValidateChangesetLinks(context.Background(), *repository, revisionRange, profile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintf(stdout, "ERROR %s %s %s: %s\n", shortHash(diagnostic.Commit), diagnostic.Changeset, diagnostic.Code, diagnostic.Message)
	}
	if report.HasErrors() {
		return 1
	}
	fmt.Fprintln(stdout, "valid Intent Commits to Intent Changesets links")
	return 0
}

func runChangesetConsume(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changeset consume", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "Git repository")
	revisionRange := flags.String("revision-range", "HEAD", "linked commit revision range")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() != 1 || flags.Arg(0) == "-" {
		fmt.Fprintln(stderr, "changeset consume requires --profile and one pending record file")
		return 2
	}
	profile, err := ack.LoadProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	commits, err := ack.ReadHistory(context.Background(), *repository, *revisionRange)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	result, err := ack.ConsumeIXSChangeset(*repository, profile, flags.Arg(0), commits)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "consumed Intent Changesets record %s into %d Intent Changelog records\n", result.ChangesetID, len(result.Records))
	return 0
}

func runChangesetGate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changeset gate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "changeset gate requires --profile")
		return 2
	}
	profile, err := ack.LoadProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	report, err := ack.GatePendingChangesets(*repository, profile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintf(stdout, "ERROR %s %s: %s\n", diagnostic.Changeset, diagnostic.ReleaseUnit, diagnostic.Message)
	}
	if report.HasErrors() {
		return 1
	}
	fmt.Fprintln(stdout, "all pending Intent Changesets targets are consumed")
	return 0
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
