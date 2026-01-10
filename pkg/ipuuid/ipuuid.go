package ipuuid

import (
	"errors"
	"net"

	"github.com/google/uuid"
)

func IPToUUID(ip net.IP) (uuid.UUID, error) {
	ip16 := ip.To16()
	if ip16 == nil {
		return uuid.UUID{}, errors.New("invalid IP (not IPv4/IPv6)")
	}
	var u uuid.UUID
	copy(u[:], ip16)
	return u, nil
}

func UUIDToIP(u uuid.UUID) net.IP {
	ip := net.IP(u[:])
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()
}
