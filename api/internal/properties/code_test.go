package properties

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pms/internal/identity"
)

// TestGenerateCodeFormat: first attempt is free, the produced code is the
// formatted "P-<zero-padded>" shape with the prefix the contract documents.
func TestGenerateCodeFormat(t *testing.T) {
	taken := map[string]bool{}
	seq := func(ctx context.Context) (int64, error) { return 1, nil }
	exists := func(ctx context.Context, n string) (bool, error) { return taken[n], nil }

	got, err := generateCode(context.Background(), seq, exists)
	if err != nil {
		t.Fatalf("generateCode: %v", err)
	}
	if !strings.HasPrefix(got, "P-") {
		t.Fatalf("expected P- prefix, got %s", got)
	}
	if got != "P-001" {
		t.Fatalf("expected P-001, got %s", got)
	}
}

// TestGenerateCodeCollisionWithOverride: the next sequence value collides with
// a code already in use (an administrator-supplied override); the loop
// advances the sequence and returns the next free value.
func TestGenerateCodeCollisionWithOverride(t *testing.T) {
	taken := map[string]bool{
		identity.Canonical("P-001"): true, // override already held P-001
	}
	calls := 0
	seq := func(ctx context.Context) (int64, error) {
		calls++
		return int64(calls), nil
	}
	exists := func(ctx context.Context, n string) (bool, error) { return taken[n], nil }

	got, err := generateCode(context.Background(), seq, exists)
	if err != nil {
		t.Fatalf("generateCode: %v", err)
	}
	if got != "P-002" {
		t.Fatalf("expected P-002 after collision, got %s", got)
	}
	if calls < 2 {
		t.Fatalf("expected at least two sequence reads, got %d", calls)
	}
}

// TestGenerateCodeExhaustsBudget: every sequence value is taken; the loop
// fails closed after exactly maxGenerateAttempts reads.
func TestGenerateCodeExhaustsBudget(t *testing.T) {
	calls := 0
	seq := func(ctx context.Context) (int64, error) {
		calls++
		return int64(calls), nil
	}
	exists := func(ctx context.Context, n string) (bool, error) { return true, nil }

	_, err := generateCode(context.Background(), seq, exists)
	if err == nil {
		t.Fatalf("expected exhaustion error, got nil")
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("expected exhaustion message, got %v", err)
	}
	if calls != maxGenerateAttempts {
		t.Fatalf("expected %d sequence reads on exhaustion, got %d", maxGenerateAttempts, calls)
	}
}

// TestGenerateCodePropagatesSequenceError: a sequence failure surfaces
// immediately rather than being swallowed by the loop.
func TestGenerateCodePropagatesSequenceError(t *testing.T) {
	want := errors.New("boom")
	seq := func(ctx context.Context) (int64, error) { return 0, want }
	exists := func(ctx context.Context, n string) (bool, error) { return false, nil }

	_, err := generateCode(context.Background(), seq, exists)
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
