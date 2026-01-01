package selenium

import (
	"testing"
)

func TestUpdateSessionId(t *testing.T) {
	// Обновление из вложенного value/sessionId
	p := Payload{
		"value": map[string]any{
			"sessionId": "old",
		},
	}
	updated := p.UpdateSessionId("new-session")
	if !updated {
		t.Errorf("expected true, got %v", updated)
	}
	if p["value"].(map[string]any)["sessionId"] != "new-session" {
		t.Errorf("expected sessionId updated, got %v", p["value"].(map[string]any)["sessionId"])
	}

	// Нет value или sessionId -> false
	p2 := Payload{}
	if p2.UpdateSessionId("anything") {
		t.Error("expected false when sessionId not found")
	}
}

func TestGetSessionId(t *testing.T) {
	// sessionId на верхнем уровне
	p := Payload{"sessionId": "topLevel"}
	id, ok := p.GetSessionId()
	if id != "topLevel" || !ok {
		t.Errorf("expected topLevel/true, got %s/%v", id, ok)
	}

	// sessionId во вложенном value
	p2 := Payload{
		"value": map[string]any{
			"sessionId": "nested",
		},
	}
	id2, ok2 := p2.GetSessionId()
	if id2 != "nested" || !ok2 {
		t.Errorf("expected nested/true, got %s/%v", id2, ok2)
	}

	// sessionId отсутствует
	p3 := Payload{}
	id3, ok3 := p3.GetSessionId()
	if id3 != "" || ok3 {
		t.Errorf("expected empty/false, got %s/%v", id3, ok3)
	}
}
