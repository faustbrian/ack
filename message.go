package ack

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	headerPattern  = regexp.MustCompile(`^([a-z][a-z0-9-]*)(?:\(([a-z0-9][a-z0-9./-]*)\))?: (.+)$`)
	typePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	scopePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9./-]*$`)
	trailerPattern = regexp.MustCompile(`^([A-Za-z0-9-]+): (.*)$`)
)

type Message struct {
	Type        string
	Scope       string
	Description string
	Body        string
	Trailers    []Trailer
}

type Trailer struct {
	Token string
	Value string
}

func (message Message) Header() string {
	scope := ""
	if message.Scope != "" {
		scope = "(" + message.Scope + ")"
	}

	return message.Type + scope + ": " + message.Description
}

func (message Message) String() string {
	var output strings.Builder
	output.WriteString(message.Header())
	output.WriteByte('\n')
	if message.Body != "" {
		output.WriteByte('\n')
		output.WriteString(message.Body)
		output.WriteByte('\n')
	}
	if len(message.Trailers) > 0 {
		output.WriteByte('\n')
		for _, trailer := range message.Trailers {
			output.WriteString(canonicalTrailerToken(trailer.Token))
			output.WriteString(": ")
			output.WriteString(strings.ReplaceAll(trailer.Value, "\n", "\n "))
			output.WriteByte('\n')
		}
	}

	return output.String()
}

func Parse(raw string) (Message, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.TrimRight(normalized, "\n")
	if normalized == "" {
		return Message{}, fmt.Errorf("message is empty")
	}

	header, remainder, hasRemainder := strings.Cut(normalized, "\n")
	matches := headerPattern.FindStringSubmatch(header)
	if matches == nil || strings.TrimSpace(matches[3]) == "" {
		return Message{}, fmt.Errorf("invalid header %q", header)
	}

	message := Message{
		Type:        matches[1],
		Scope:       matches[2],
		Description: matches[3],
	}
	if !hasRemainder {
		return message, nil
	}
	if !strings.HasPrefix(remainder, "\n") {
		return Message{}, fmt.Errorf("header must be followed by a blank line")
	}

	content := strings.TrimPrefix(remainder, "\n")
	paragraphs := strings.Split(content, "\n\n")
	trailers, ok := parseTrailerBlock(paragraphs[len(paragraphs)-1])
	if ok {
		message.Trailers = trailers
		paragraphs = paragraphs[:len(paragraphs)-1]
	}
	message.Body = strings.Join(paragraphs, "\n\n")

	return message, nil
}

func (message Message) TrailerValues(token string) []string {
	values := make([]string, 0)
	for _, trailer := range message.Trailers {
		if strings.EqualFold(trailer.Token, token) {
			values = append(values, trailer.Value)
		}
	}

	return values
}

func parseTrailerBlock(block string) ([]Trailer, bool) {
	lines := strings.Split(block, "\n")
	trailers := make([]Trailer, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(trailers) == 0 {
				return nil, false
			}
			trailers[len(trailers)-1].Value += "\n" + strings.TrimSpace(line)
			continue
		}

		matches := trailerPattern.FindStringSubmatch(line)
		if matches == nil {
			return nil, false
		}
		trailers = append(trailers, Trailer{Token: matches[1], Value: matches[2]})
	}

	return trailers, len(trailers) > 0
}

func canonicalTrailerToken(token string) string {
	switch {
	case strings.EqualFold(token, "Impact"):
		return "Impact"
	case strings.EqualFold(token, "Migration"):
		return "Migration"
	case strings.EqualFold(token, "Reverts"):
		return "Reverts"
	case strings.EqualFold(token, "Affects"):
		return "Affects"
	case strings.EqualFold(token, "Changeset"):
		return "Changeset"
	case strings.EqualFold(token, "Target-Impact"):
		return "Target-Impact"
	case strings.EqualFold(token, "Target-Migration"):
		return "Target-Migration"
	default:
		return token
	}
}
