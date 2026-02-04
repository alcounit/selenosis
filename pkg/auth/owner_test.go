package auth

import (
	"context"
	"testing"
)

func TestWithOwnerAndOwnerFrom(t *testing.T) {
	ctx := WithOwner(context.Background(), Owner{Name: "alice"})

	got, ok := OwnerFrom(ctx)
	if !ok {
		t.Fatal("expected owner to be present")
	}
	if got.Name != "alice" {
		t.Fatalf("unexpected owner name: %q", got.Name)
	}
}

func TestOwnerFromMissing(t *testing.T) {
	got, ok := OwnerFrom(context.Background())
	if ok {
		t.Fatal("expected owner to be absent")
	}
	if got != (Owner{}) {
		t.Fatalf("expected zero owner, got %+v", got)
	}
}

func TestWithOwnerOverridesPreviousValue(t *testing.T) {
	ctx := WithOwner(context.Background(), Owner{Name: "alice"})
	ctx = WithOwner(ctx, Owner{Name: "bob"})

	got, ok := OwnerFrom(ctx)
	if !ok {
		t.Fatal("expected owner to be present")
	}
	if got.Name != "bob" {
		t.Fatalf("expected latest owner, got %q", got.Name)
	}
}

func TestWithOwnerDoesNotAffectParentContext(t *testing.T) {
	parent := context.Background()
	child := WithOwner(parent, Owner{Name: "alice"})

	if _, ok := OwnerFrom(parent); ok {
		t.Fatal("expected parent context to be unchanged")
	}
	if _, ok := OwnerFrom(child); !ok {
		t.Fatal("expected child context to contain owner")
	}
}
