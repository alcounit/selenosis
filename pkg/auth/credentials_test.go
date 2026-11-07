package auth

import (
	"context"
	"testing"
)

func TestCredentialsRoundTrip(t *testing.T) {
	ctx := WithCredentials(context.Background(), "user1", "secret1")

	got, ok := CredentialsFrom(ctx)
	if !ok {
		t.Fatal("expected credentials in the context")
	}
	if got.User != "user1" || got.Pass != "secret1" {
		t.Fatalf("credentials = %+v, want user1/secret1", got)
	}
}

func TestCredentialsAbsent(t *testing.T) {
	if _, ok := CredentialsFrom(context.Background()); ok {
		t.Fatal("expected no credentials in an empty context")
	}
}
