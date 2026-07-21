package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/faustbrian/ack"
)

func runChangelogCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changelog check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "changelog check requires exactly one record or -")
		return 2
	}
	contents, err := readInput(flags.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	record, err := ack.ParseICLSRecord(contents)
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
		if err := ack.ValidateICLSRecordProfile(record, profile); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "valid Intent Changelog record: %s %s (%s)\n", record.ReleaseUnit, record.Release, record.Channel)
	return 0
}

func runChangelogRender(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "changelog render requires exactly one record or -")
		return 2
	}
	contents, err := readInput(args[0], stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	record, err := ack.ParseICLSRecord(contents)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprint(stdout, ack.RenderICLSRecord(record))
	return 0
}

func runChangelogPublish(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changelog publish", flag.ContinueOnError)
	flags.SetOutput(stderr)
	date := flags.String("date", "", "release date in YYYY-MM-DD form")
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *date == "" || flags.NArg() != 1 || flags.Arg(0) == "-" {
		fmt.Fprintln(stderr, "changelog publish requires --date YYYY-MM-DD and one record file")
		return 2
	}
	if *profilePath != "" {
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
		if report.HasErrors() {
			for _, diagnostic := range report.Diagnostics {
				fmt.Fprintf(stderr, "unconsumed %s %s: %s\n", diagnostic.Changeset, diagnostic.ReleaseUnit, diagnostic.Message)
			}
			return 1
		}
		contents, err := os.ReadFile(flags.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		record, err := ack.ParseICLSRecord(contents)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := ack.ValidateICLSRecordProfile(record, profile); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := ack.PublishICLSRecord(flags.Arg(0), *date); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "published Intent Changelog record %s on %s; only date changed\n", flags.Arg(0), *date)
	return 0
}

func runChangelogGenerate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changelog generate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "Git repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() > 1 {
		fmt.Fprintln(stderr, "changelog generate requires --profile and accepts at most one revision range")
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
	results, err := ack.GenerateICLSRecords(context.Background(), *repository, revisionRange, profile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "consumed %d pending changesets into canonical Intent Changelog records\n", len(results))
	return 0
}

func runChangelogInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changelog init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "repository")
	releaseUnit := flags.String("release-unit", "", "release unit")
	release := flags.String("release", "", "planned release identifier")
	channel := flags.String("channel", "", "release channel")
	stream := flags.String("stream", "", "release stream")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || *releaseUnit == "" || *release == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "changelog init requires --profile, --release-unit, and --release")
		return 2
	}
	profile, err := ack.LoadProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	var path string
	if profile.ProfileVersion != "" {
		if *stream == "" {
			fmt.Fprintln(stderr, "changelog init requires --stream for a standard project profile")
			return 2
		}
		path, err = ack.InitializeICLSStreamRecord(*repository, profile, *releaseUnit, *stream, *release)
	} else {
		if *channel == "" {
			fmt.Fprintln(stderr, "changelog init requires --channel for a legacy project profile")
			return 2
		}
		path, err = ack.InitializeICLSRecord(*repository, profile, *releaseUnit, *release, *channel)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "initialized Intent Changelog record: %s\n", path)
	return 0
}

func runChangelogSource(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("changelog source", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK commit profile")
	repository := flags.String("repo", ".", "Git repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(stderr, "changelog source accepts at most one revision range")
		return 2
	}
	profile, err := loadOptionalProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	revisionRange := "HEAD"
	if flags.NArg() == 1 {
		revisionRange = flags.Arg(0)
	}
	commits, err := ack.ReadHistory(context.Background(), *repository, revisionRange)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprint(stdout, ack.GenerateChangelog(commits, profile))
	for _, commit := range commits {
		if reportCommit(commit, profile).HasErrors() {
			fmt.Fprintln(stderr, "changelog source contains invalid or unclassified commits")
			return 1
		}
	}
	return 0
}
