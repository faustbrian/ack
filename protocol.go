package ack

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

func validateAbsoluteURI(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("must be an absolute URI")
	}
	return nil
}

const maxIntentDocumentBytes = 1024 * 1024

func validateIntentDocument(contents []byte, document *yaml.Node) error {
	if len(contents) > maxIntentDocumentBytes {
		return fmt.Errorf("document exceeds %d bytes", maxIntentDocumentBytes)
	}
	nodes := 0
	var walk func(*yaml.Node, int) error
	walk = func(node *yaml.Node, depth int) error {
		nodes++
		if depth > 64 || nodes > 100000 {
			return fmt.Errorf("document exceeds YAML complexity limits")
		}
		if node.Anchor != "" || node.Kind == yaml.AliasNode {
			return fmt.Errorf("YAML anchors and aliases are not permitted")
		}
		if strings.HasPrefix(node.Tag, "!") && !strings.HasPrefix(node.Tag, "!!") {
			return fmt.Errorf("custom YAML tag %q is not permitted", node.Tag)
		}
		if node.Kind == yaml.MappingNode {
			seen := make(map[string]struct{}, len(node.Content)/2)
			for index := 0; index+1 < len(node.Content); index += 2 {
				key := node.Content[index]
				if key.Value == "<<" {
					return fmt.Errorf("YAML merge keys are not permitted")
				}
				if _, ok := seen[key.Value]; ok {
					return fmt.Errorf("duplicate YAML key %q", key.Value)
				}
				seen[key.Value] = struct{}{}
			}
		}
		for _, child := range node.Content {
			if err := walk(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(document, 0)
}

type Disclosure struct {
	State       string  `json:"state" yaml:"state"`
	NotBefore   *string `json:"not_before,omitempty" yaml:"not-before,omitempty"`
	Policy      string  `json:"policy,omitempty" yaml:"policy,omitempty"`
	Placeholder string  `json:"placeholder,omitempty" yaml:"placeholder,omitempty"`
}

func (disclosure *Disclosure) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		disclosure.State = node.Value
		return nil
	}
	type rawDisclosure Disclosure
	var raw rawDisclosure
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*disclosure = Disclosure(raw)
	return nil
}

func validateDisclosure(disclosure Disclosure, version string, accepted []string) error {
	if !slices.Contains([]string{"public", "embargoed", "redacted"}, disclosure.State) &&
		!slices.Contains(accepted, disclosure.State) {
		return fmt.Errorf("unsupported disclosure %q", disclosure.State)
	}
	if isStructuredIntentVersion(version) && disclosure.State == "embargoed" &&
		disclosure.NotBefore == nil && strings.TrimSpace(disclosure.Policy) == "" {
		return fmt.Errorf("embargoed disclosure requires not-before or policy")
	}
	if disclosure.NotBefore != nil {
		if _, err := time.Parse(time.DateOnly, *disclosure.NotBefore); err != nil {
			return fmt.Errorf("invalid disclosure not-before date %q", *disclosure.NotBefore)
		}
	}
	return nil
}

func isStructuredIntentVersion(version string) bool {
	return version == "0.2.0" || version == "1.0.0"
}
