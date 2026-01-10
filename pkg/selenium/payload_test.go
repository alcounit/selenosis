package selenium

import (
	"testing"
)

func TestUpdateSessionId(t *testing.T) {
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

	p2 := Payload{}
	if p2.UpdateSessionId("anything") {
		t.Error("expected false when sessionId not found")
	}
}

func TestGetSessionId(t *testing.T) {
	p := Payload{"sessionId": "topLevel"}
	id, ok := p.GetSessionId()
	if id != "topLevel" || !ok {
		t.Errorf("expected topLevel/true, got %s/%v", id, ok)
	}

	p2 := Payload{
		"value": map[string]any{
			"sessionId": "nested",
		},
	}
	id2, ok2 := p2.GetSessionId()
	if id2 != "nested" || !ok2 {
		t.Errorf("expected nested/true, got %s/%v", id2, ok2)
	}

	p3 := Payload{}
	id3, ok3 := p3.GetSessionId()
	if id3 != "" || ok3 {
		t.Errorf("expected empty/false, got %s/%v", id3, ok3)
	}
}

func TestUpdateBiDiURL(t *testing.T) {
	p := Payload{
		"value": map[string]any{
			"capabilities": map[string]any{
				"webSocketUrl": "ws://oldhost/session/oldid",
			},
		},
	}

	UpdateBiDiURL("wss", "newhost", "oldid", "newid", p)
	caps := p["value"].(map[string]any)["capabilities"].(map[string]any)
	got := caps["webSocketUrl"].(string)
	if got != "wss://newhost/session/newid" {
		t.Fatalf("unexpected webSocketUrl: %s", got)
	}

	p2 := Payload{}
	UpdateBiDiURL("wss", "newhost", "oldid", "newid", p2)
}

func TestUpdateChromeCDPURL(t *testing.T) {
	p := Payload{
		"value": map[string]any{
			"capabilities": map[string]any{
				"se:cdp": "ws://oldhost/devtools/oldid",
			},
		},
	}

	UpdateChromeCDPURL("wss", "newhost", "oldid", "newid", p)
	caps := p["value"].(map[string]any)["capabilities"].(map[string]any)
	got := caps["se:cdp"].(string)
	if got != "wss://newhost/devtools/newid" {
		t.Fatalf("unexpected se:cdp: %s", got)
	}

	p2 := Payload{}
	UpdateChromeCDPURL("wss", "newhost", "oldid", "newid", p2)
}

func TestUpdateBiDiURLGuards(t *testing.T) {
	UpdateBiDiURL("wss", "host", "old", "new", Payload{"value": "not-map"})
	UpdateBiDiURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": "not-map"}})
	UpdateBiDiURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{}}})
	UpdateBiDiURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{"webSocketUrl": 123}}})
	UpdateBiDiURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{"webSocketUrl": ""}}})
	UpdateBiDiURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{"webSocketUrl": "http://[::1"}}})
}

func TestUpdateChromeCDPURLGuards(t *testing.T) {
	UpdateChromeCDPURL("wss", "host", "old", "new", Payload{"value": "not-map"})
	UpdateChromeCDPURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": "not-map"}})
	UpdateChromeCDPURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{}}})
	UpdateChromeCDPURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{"se:cdp": 123}}})
	UpdateChromeCDPURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{"se:cdp": ""}}})
	UpdateChromeCDPURL("wss", "host", "old", "new", Payload{"value": map[string]any{"capabilities": map[string]any{"se:cdp": "http://[::1"}}})
}
