package net

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

type Host struct {
	Port int
	IP   string
}

func NewHost(port int, ip string) Host {
	return Host{
		Port: port,
		IP:   ip,
	}
}

func (host *Host) ToString() string {
	return fmt.Sprintf("%s:%d", host.IP, host.Port)
}

// SerializeHost encodes an IPv4 Host into a fixed-width 17-byte payload:
// 15 bytes for the dotted-decimal IP (right-padded with spaces) followed by
// a little-endian uint16 port. IPv6 is not supported by this wire format and
// returns an error rather than silently producing an empty buffer.
func SerializeHost(h Host) (bytes.Buffer, error) {
	var buffer bytes.Buffer

	ip := net.ParseIP(h.IP)
	if ip == nil {
		return buffer, fmt.Errorf("SerializeHost: invalid IP %q", h.IP)
	}
	if ip.To4() == nil {
		return buffer, fmt.Errorf("SerializeHost: IPv6 not supported by wire format (got %q)", h.IP)
	}
	if len(h.IP) > 15 {
		return buffer, fmt.Errorf("SerializeHost: IP string too long for 15-byte slot: %q", h.IP)
	}

	buffer.WriteString(fmt.Sprintf("%-15s", h.IP))

	portBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(portBytes, uint16(h.Port))
	buffer.Write(portBytes)

	return buffer, nil
}

// DeserializeHost reverses SerializeHost. It returns an error when the buffer
// is shorter than the fixed 17-byte layout.
func DeserializeHost(buffer bytes.Buffer) (Host, error) {
	var h Host
	if buffer.Len() < 17 {
		return h, fmt.Errorf("DeserializeHost: buffer too short (%d bytes, need 17)", buffer.Len())
	}

	ipBytes := buffer.Next(15)
	h.IP = string(bytes.TrimRight(ipBytes, " "))

	portBytes := buffer.Next(2)
	h.Port = int(binary.LittleEndian.Uint16(portBytes))
	return h, nil
}

func CompareHost(a, b Host) bool {
	return a.IP == b.IP && a.Port == b.Port
}
