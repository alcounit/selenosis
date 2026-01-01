package ipuuid

import (
	"net"
	"testing"

	"github.com/google/uuid"
)

func TestIPToUUIDAndBackIPv4(t *testing.T) {
	ip := net.ParseIP("192.0.2.1")
	u, err := IPToUUID(ip)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := UUIDToIP(u)
	if !got.Equal(ip.To16()) && !got.Equal(ip) {
		t.Fatalf("roundtrip failed: got %v want %v", got, ip)
	}
	// also ensure uuid has 16 bytes
	if u == uuid.Nil {
		t.Fatalf("uuid should not be nil")
	}
}

func TestIPToUUIDInvalid(t *testing.T) {
	var ip net.IP
	_, err := IPToUUID(ip)
	if err == nil {
		t.Fatalf("expected error for nil IP")
	}
}
