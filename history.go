package ack

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Commit struct {
	Hash       string      `json:"hash"`
	Date       time.Time   `json:"date"`
	RawMessage string      `json:"raw_message"`
	Message    Message     `json:"message"`
	ParseError string      `json:"parse_error,omitempty"`
	Stats      ChangeStats `json:"stats"`
}

func ReadHistory(ctx context.Context, directory, revisionRange string) ([]Commit, error) {
	if revisionRange == "" {
		revisionRange = "HEAD"
	}
	hashesOutput, err := runGitCommand(ctx, directory, "rev-list", "--reverse", revisionRange)
	if err != nil {
		return nil, err
	}
	hashes := strings.Fields(string(hashesOutput))
	commits := make([]Commit, 0, len(hashes))
	for _, hash := range hashes {
		commit, err := readCommit(ctx, directory, hash)
		if err != nil {
			return nil, err
		}
		commits = append(commits, commit)
	}

	return commits, nil
}

func readCommit(ctx context.Context, directory, hash string) (Commit, error) {
	metadata, err := runGitCommand(ctx, directory, "show", "-s", "--format=%cI%x00%B", hash)
	if err != nil {
		return Commit{}, err
	}
	fields := strings.SplitN(string(metadata), "\x00", 2)
	if len(fields) != 2 {
		return Commit{}, fmt.Errorf("git show %s returned malformed metadata", hash)
	}
	date, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[0]))
	if err != nil {
		return Commit{}, fmt.Errorf("parse commit date for %s: %w", hash, err)
	}
	rawMessage := strings.TrimRight(fields[1], "\n")
	message, parseErr := Parse(rawMessage)

	statsOutput, err := runGitCommand(ctx, directory, "show", "--numstat", "--format=", hash)
	if err != nil {
		return Commit{}, err
	}
	stats := parseNumstat(string(statsOutput))
	commit := Commit{Hash: hash, Date: date, RawMessage: rawMessage, Message: message, Stats: stats}
	if parseErr != nil {
		commit.ParseError = parseErr.Error()
	}

	return commit, nil
}

func parseNumstat(output string) ChangeStats {
	var stats ChangeStats
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		stats.Files++
		if added, err := strconv.Atoi(fields[0]); err == nil {
			stats.Added += added
		}
		if deleted, err := strconv.Atoi(fields[1]); err == nil {
			stats.Deleted += deleted
		}
	}

	return stats
}

func runGitCommand(ctx context.Context, directory string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}

	return output, nil
}
