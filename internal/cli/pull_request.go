package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/faustbrian/ack"
)

func runPullRequestCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pull-request check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, "pull-request check requires one record or - and format text or json")
		return 2
	}
	contents, err := readInput(flags.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	record, err := ack.ParsePullRequest(contents)
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
		if err := ack.ValidatePullRequest(record, profile); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if *format == "json" {
		if err := json.NewEncoder(stdout).Encode(record); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	fmt.Fprintf(stdout, "valid Intent Pull Requests record: %s (%s)\n", record.ID, record.State)
	return 0
}

func runPullRequestCreate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pull-request create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "repository")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() != 1 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, "pull-request create requires --profile, one record or -, and format text or json")
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
	path, err := ack.CreatePullRequest(*repository, profile, contents)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *format == "json" {
		if err := json.NewEncoder(stdout).Encode(struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		}{ID: recordID(contents), Path: path}); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	fmt.Fprintf(stdout, "created Intent Pull Requests record: %s\n", path)
	return 0
}

func runPullRequestVerify(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pull-request verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "Git repository")
	head := flags.String("head", "HEAD", "head revision")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() != 1 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, "pull-request verify requires --profile, one record, and format text or json")
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
	record, err := ack.ParsePullRequest(contents)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := ack.VerifyPullRequest(context.Background(), *repository, *head, record, profile); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *format == "json" {
		if err := json.NewEncoder(stdout).Encode(struct {
			ID               string `json:"id"`
			BaseRevision     string `json:"base_revision"`
			EvidenceRevision string `json:"evidence_revision"`
			Verified         bool   `json:"verified"`
		}{record.ID, record.BaseRevision, record.EvidenceRevision, true}); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	fmt.Fprintf(stdout, "verified Intent Pull Requests record: %s\n", record.ID)
	return 0
}

func recordID(contents []byte) string {
	record, err := ack.ParsePullRequest(contents)
	if err != nil {
		return ""
	}
	return record.ID
}
