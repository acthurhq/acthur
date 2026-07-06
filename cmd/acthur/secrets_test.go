package main

import (
	"strings"
	"testing"
)

func TestResolveSecretValueArg_UsesInlineArg(t *testing.T) {
	got, err := resolveSecretValueArg([]string{"KEY", "value"}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("resolveSecretValueArg: %v", err)
	}
	if got != "value" {
		t.Fatalf("got %q, want %q", got, "value")
	}
}

func TestResolveSecretValueArg_ReadsStdinWhenValueOmitted(t *testing.T) {
	got, err := resolveSecretValueArg([]string{"KEY"}, strings.NewReader("piped-value\n"))
	if err != nil {
		t.Fatalf("resolveSecretValueArg: %v", err)
	}
	if got != "piped-value" {
		t.Fatalf("got %q, want %q", got, "piped-value")
	}
}

func TestResolveSecretValueArg_ReadsStdinWhenValueIsDash(t *testing.T) {
	got, err := resolveSecretValueArg([]string{"KEY", "-"}, strings.NewReader("from-stdin\n"))
	if err != nil {
		t.Fatalf("resolveSecretValueArg: %v", err)
	}
	if got != "from-stdin" {
		t.Fatalf("got %q, want %q", got, "from-stdin")
	}
}

func TestResolveSecretValueArg_EmptyStdinIsError(t *testing.T) {
	_, err := resolveSecretValueArg([]string{"KEY"}, strings.NewReader("   \n"))
	if err == nil {
		t.Fatal("expected error for empty stdin")
	}
}
