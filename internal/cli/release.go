package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/faustbrian/ack"
)

func runReleaseCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "release check requires exactly one manifest or -")
		return 2
	}
	contents, err := readInput(flags.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	manifest, err := ack.ParseReleaseManifest(contents)
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
		if err := ack.ValidateReleaseManifestProfile(manifest, profile); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "valid release manifest: %s %s (%s)\n", manifest.ReleaseUnit, manifest.Release, manifest.Stream)
	return 0
}

func runReleaseVerify(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() != 1 || flags.Arg(0) == "-" {
		fmt.Fprintln(stderr, "release verify requires --profile and one manifest file")
		return 2
	}
	profile, err := ack.LoadProfile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	contents, err := readInput(flags.Arg(0), nil)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	manifest, err := ack.ParseReleaseManifest(contents)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := ack.VerifyReleaseManifest(*repository, profile, manifest); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "verified release manifest: %s\n", flags.Arg(0))
	return 0
}

func runReleaseCreate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("release create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "ACK project profile")
	repository := flags.String("repo", ".", "repository")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || flags.NArg() != 1 {
		fmt.Fprintln(stderr, "release create requires --profile and one manifest or -")
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
	path, err := ack.CreateReleaseManifest(*repository, profile, contents)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "created release manifest: %s\n", path)
	return 0
}
