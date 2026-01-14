package selenium

import "testing"

func TestStatusSet(t *testing.T) {
	s := Status{}
	s.Set("all good", true)

	if s.Value["message"] != "all good" {
		t.Errorf("expected message 'all good', got %v", s.Value["message"])
	}
	if s.Value["ready"] != true {
		t.Errorf("expected ready=true, got %v", s.Value["ready"])
	}

	s.Set("not ready", false)
	if s.Value["message"] != "not ready" {
		t.Errorf("expected message 'not ready', got %v", s.Value["message"])
	}
	if s.Value["ready"] != false {
		t.Errorf("expected ready=false, got %v", s.Value["ready"])
	}
}
