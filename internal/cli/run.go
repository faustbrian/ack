package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/faustbrian/ack"
)

const usage = `Usage:
  ack commit check [--profile path] [--format text|json] <file|->
  ack commit create [--profile path]
  ack commit lint [--profile path] [--format text|json] [revision-range]
  ack commit profile validate <file>
  ack changeset check [--profile path] <record|->
  ack changeset create --profile path [--repo path] <record|->
  ack changeset consume --profile path [--repo path] [--revision-range range] <record>
  ack changeset gate --profile path [--repo path]
  ack changeset links --profile path [--repo path] [revision-range]
  ack changelog check [--profile path] <record|->
  ack changelog generate --profile path [--repo path] [revision-range]
  ack changelog init --profile path --release-unit id --release id [--stream id|--channel id]
  ack changelog publish [--profile path] [--repo path] --date YYYY-MM-DD <record>
  ack changelog render <record|->
  ack changelog source [--profile path] [--repo path] [revision-range]
  ack release check [--profile path] <manifest|->
  ack release create --profile path [--repo path] <manifest|->
  ack release verify --profile path [--repo path] <manifest>
  ack pull-request check [--profile path] [--format text|json] <record|->
  ack pull-request create --profile path [--repo path] [--format text|json] <record|->
  ack pull-request verify --profile path [--repo path] [--head revision] [--format text|json] <record|->
  ack profile validate <file>
  ack version
`

const commitUsage = `Usage:
  ack commit check [--profile path] [--format text|json] <file|->
  ack commit create [--profile path]
  ack commit lint [--profile path] [--format text|json] [revision-range]
  ack commit profile validate <file>
`

const changelogUsage = `Usage:
  ack changelog check [--profile path] <record|->
  ack changelog generate --profile path [--repo path] [revision-range]
  ack changelog init --profile path --release-unit id --release id [--stream id|--channel id]
  ack changelog publish [--profile path] [--repo path] --date YYYY-MM-DD <record>
  ack changelog render <record|->
  ack changelog source [--profile path] [--repo path] [revision-range]
`

const changesetUsage = `Usage:
  ack changeset check [--profile path] <record|->
  ack changeset create --profile path [--repo path] <record|->
  ack changeset consume --profile path [--repo path] [--revision-range range] <record>
  ack changeset gate --profile path [--repo path]
  ack changeset links --profile path [--repo path] [revision-range]
`

const releaseUsage = `Usage:
  ack release check [--profile path] <manifest|->
  ack release create --profile path [--repo path] <manifest|->
  ack release verify --profile path [--repo path] <manifest>
`

const pullRequestUsage = `Usage:
  ack pull-request check [--profile path] [--format text|json] <record|->
  ack pull-request create --profile path [--repo path] [--format text|json] <record|->
  ack pull-request verify --profile path [--repo path] [--head revision] [--format text|json] <record|->
`

var Version = "dev"

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "changelog":
		return runChangelogCommand(args[1:], stdin, stdout, stderr)
	case "changeset":
		return runChangesetCommand(args[1:], stdin, stdout, stderr)
	case "commit":
		return runCommitCommand(args[1:], stdin, stdout, stderr)
	case "profile":
		return runProjectProfile(args[1:], stdout, stderr)
	case "release":
		return runReleaseCommand(args[1:], stdin, stdout, stderr)
	case "pull-request":
		return runPullRequestCommand(args[1:], stdin, stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "ack %s\n", Version)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func runPullRequestCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "pull-request requires a command: check, create, or verify")
		return 2
	}
	switch args[0] {
	case "check":
		return runPullRequestCheck(args[1:], stdin, stdout, stderr)
	case "create":
		return runPullRequestCreate(args[1:], stdin, stdout, stderr)
	case "verify":
		return runPullRequestVerify(args[1:], stdin, stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, pullRequestUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown pull-request command %q\n", args[0])
		return 2
	}
}

func runReleaseCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "release requires a command: check, create, or verify")
		return 2
	}
	switch args[0] {
	case "check":
		return runReleaseCheck(args[1:], stdin, stdout, stderr)
	case "create":
		return runReleaseCreate(args[1:], stdin, stdout, stderr)
	case "verify":
		return runReleaseVerify(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, releaseUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown release command %q\n", args[0])
		return 2
	}
}

func runChangesetCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "changeset requires a command: check, create, consume, gate, or links")
		return 2
	}
	switch args[0] {
	case "check":
		return runChangesetCheck(args[1:], stdin, stdout, stderr)
	case "create":
		return runChangesetCreate(args[1:], stdin, stdout, stderr)
	case "consume":
		return runChangesetConsume(args[1:], stdout, stderr)
	case "gate":
		return runChangesetGate(args[1:], stdout, stderr)
	case "links":
		return runChangesetLinks(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, changesetUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown changeset command %q\n", args[0])
		return 2
	}
}

func runCommitCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "commit requires a command: check, create, lint, or profile")
		return 2
	}

	switch args[0] {
	case "check":
		return runCheckMessage(args[1:], stdin, stdout, stderr)
	case "create":
		return runCommitCreate(args[1:], stdin, stdout, stderr)
	case "lint":
		return runLint(args[1:], stdout, stderr)
	case "profile":
		return runProfile(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, commitUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown commit command %q\n", args[0])
		return 2
	}
}

func runChangelogCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "changelog requires a command: check, generate, init, publish, render, or source")
		return 2
	}
	switch args[0] {
	case "check":
		return runChangelogCheck(args[1:], stdin, stdout, stderr)
	case "generate":
		return runChangelogGenerate(args[1:], stdout, stderr)
	case "init":
		return runChangelogInit(args[1:], stdout, stderr)
	case "publish":
		return runChangelogPublish(args[1:], stdout, stderr)
	case "render":
		return runChangelogRender(args[1:], stdin, stdout, stderr)
	case "source":
		return runChangelogSource(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, changelogUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown changelog command %q\n", args[0])
		return 2
	}
}

type commitResult struct {
	Hash   string     `json:"hash"`
	Header string     `json:"header"`
	Report ack.Report `json:"report"`
}

func runCheckMessage(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("commit check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK commit profile")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "commit check requires exactly one file or -")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "unsupported format %q\n", *format)
		return 2
	}

	profile, err := loadOptionalProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	contents, err := readInput(flags.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	message, err := ack.Parse(string(contents))
	if err != nil {
		report := ack.Report{Diagnostics: []ack.Diagnostic{{
			Code: "malformed-message", Severity: ack.SeverityError, Message: err.Error(),
		}}}
		writeReport(stdout, *format, report)
		return 1
	}

	report := ack.Validate(message, profile)
	writeReport(stdout, *format, report)
	if report.HasErrors() {
		return 1
	}

	return 0
}

func runProfile(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "validate" {
		fmt.Fprintln(stderr, "commit profile requires: validate <file>")
		return 2
	}
	if _, err := ack.LoadProfile(args[1]); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "valid ACK commit profile: %s\n", args[1])
	return 0
}

func runProjectProfile(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "validate" {
		fmt.Fprintln(stderr, "profile requires: validate <file>")
		return 2
	}
	if _, err := ack.LoadProfile(args[1]); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "valid ACK project profile: %s\n", args[1])
	return 0
}

func runLint(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("commit lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK commit profile")
	format := flags.String("format", "text", "output format: text or json")
	repository := flags.String("repo", ".", "Git repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(stderr, "commit lint accepts at most one revision range")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "unsupported format %q\n", *format)
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

	results := make([]commitResult, 0, len(commits))
	hasErrors := false
	for _, commit := range commits {
		report := reportCommit(commit, profile)
		if report.HasErrors() {
			hasErrors = true
		}
		results = append(results, commitResult{Hash: commit.Hash, Header: commit.Message.Header(), Report: report})
	}
	writeCommitResults(stdout, *format, results)
	if hasErrors {
		return 1
	}
	return 0
}

func loadOptionalProfile(path string) (ack.Profile, error) {
	if path == "" {
		return ack.Profile{}, nil
	}

	return ack.LoadProfile(path)
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		contents, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read standard input: %w", err)
		}
		return contents, nil
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}
	return contents, nil
}

func writeReport(output io.Writer, format string, report ack.Report) {
	if format == "json" {
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(report)
		return
	}

	if len(report.Diagnostics) == 0 {
		fmt.Fprintf(output, "valid (effective impact: %s)\n", report.EffectiveImpact)
		return
	}
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintf(output, "%s %s: %s\n", strings.ToUpper(string(diagnostic.Severity)), diagnostic.Code, diagnostic.Message)
	}
	fmt.Fprintf(output, "effective impact: %s\n", report.EffectiveImpact)
}

func reportCommit(commit ack.Commit, profile ack.Profile) ack.Report {
	if commit.ParseError != "" {
		return ack.Report{EffectiveImpact: ack.ImpactUnspecified, Diagnostics: []ack.Diagnostic{{
			Code: "malformed-message", Severity: ack.SeverityError, Message: commit.ParseError,
		}}}
	}
	return ack.Review(commit.Message, profile, commit.Stats)
}

func writeCommitResults(output io.Writer, format string, results []commitResult) {
	if format == "json" {
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(results)
		return
	}
	for _, result := range results {
		label := result.Hash
		if len(label) > 12 {
			label = label[:12]
		}
		fmt.Fprintf(output, "%s %s\n", label, result.Header)
		if len(result.Report.Diagnostics) == 0 {
			fmt.Fprintf(output, "  valid (effective impact: %s)\n", result.Report.EffectiveImpact)
			continue
		}
		for _, diagnostic := range result.Report.Diagnostics {
			fmt.Fprintf(output, "  %s %s: %s\n", strings.ToUpper(string(diagnostic.Severity)), diagnostic.Code, diagnostic.Message)
		}
	}
}
