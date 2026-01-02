package env

import (
	"os"
	"testing"
	"time"
)

func TestGetEnvOrDefault(t *testing.T) {
	os.Setenv("SEL_TEST_KEY", "value")
	defer os.Unsetenv("SEL_TEST_KEY")
	if v := GetEnvOrDefault("SEL_TEST_KEY", "def"); v != "value" {
		t.Fatalf("expected value got %s", v)
	}
	if v := GetEnvOrDefault("NON_EXISTENT_KEY", "def"); v != "def" {
		t.Fatalf("expected default got %s", v)
	}
}

func TestGetEnvDurationOrDefault(t *testing.T) {
	os.Setenv("SEL_DUR", "2s")
	defer os.Unsetenv("SEL_DUR")
	if d := GetEnvDurationOrDefault("SEL_DUR", 5*time.Second); d != 2*time.Second {
		t.Fatalf("expected 2s got %v", d)
	}
	if d := GetEnvDurationOrDefault("NON", 7*time.Second); d != 7*time.Second {
		t.Fatalf("expected default got %v", d)
	}
	os.Setenv("SEL_DUR_BAD", "notaduration")
	defer os.Unsetenv("SEL_DUR_BAD")
	if d := GetEnvDurationOrDefault("SEL_DUR_BAD", 9*time.Second); d != 9*time.Second {
		t.Fatalf("expected default for bad dur got %v", d)
	}
}

func TestGetEnvIntOrDefault(t *testing.T) {
	os.Setenv("SEL_INT", "42")
	defer os.Unsetenv("SEL_INT")
	if i := GetEnvIntOrDefault("SEL_INT", 1); i != 42 {
		t.Fatalf("expected 42 got %d", i)
	}
	if i := GetEnvIntOrDefault("NON", 3); i != 3 {
		t.Fatalf("expected default got %d", i)
	}
	os.Setenv("SEL_INT_BAD", "bad")
	defer os.Unsetenv("SEL_INT_BAD")
	if i := GetEnvIntOrDefault("SEL_INT_BAD", 5); i != 5 {
		t.Fatalf("expected default for bad int got %d", i)
	}
}
