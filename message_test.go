package ack

import "testing"

func TestParseMessage(t *testing.T) {
	t.Parallel()

	raw := "fix(apps/billing): preserve tax rounding\n\n" +
		"Round the refundable total after summing its lines.\n\n" +
		"Impact: patch\n" +
		"Affects: apps/worker\n"

	message, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if message.Type != "fix" {
		t.Errorf("Type = %q, want fix", message.Type)
	}
	if message.Scope != "apps/billing" {
		t.Errorf("Scope = %q, want apps/billing", message.Scope)
	}
	if message.Description != "preserve tax rounding" {
		t.Errorf("Description = %q, want preserve tax rounding", message.Description)
	}
	if message.Body != "Round the refundable total after summing its lines." {
		t.Errorf("Body = %q", message.Body)
	}
	if got := message.TrailerValues("Impact"); len(got) != 1 || got[0] != "patch" {
		t.Errorf("Impact trailers = %#v, want [patch]", got)
	}
	if got := message.TrailerValues("affects"); len(got) != 1 || got[0] != "apps/worker" {
		t.Errorf("Affects trailers = %#v, want [apps/worker]", got)
	}
}

func TestMessageStringUsesCanonicalTrailerSpelling(t *testing.T) {
	t.Parallel()

	message := Message{
		Type:        "feat",
		Scope:       "apps/gateway",
		Description: "support passkeys",
		Body:        "Keep password authentication during the transition.",
		Trailers: []Trailer{
			{Token: "impact", Value: "minor"},
			{Token: "affects", Value: "apps/admin"},
		},
	}
	want := "feat(apps/gateway): support passkeys\n\n" +
		"Keep password authentication during the transition.\n\n" +
		"Impact: minor\nAffects: apps/admin\n"

	if got := message.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
