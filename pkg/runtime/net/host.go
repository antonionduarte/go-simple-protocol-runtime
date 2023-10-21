package net

import (
	"bytes"
	"encoding/binary"
	"strconv"
)

type Host struct {
	Port int
	IP   string
}

func NewHost(port int, ip string) *Host {
	return &Host{
		Port: port,
		IP:   ip,
	}
}

func SerializeHost(host *Host) bytes.Buffer {
	var buffer bytes.Buffer
	// Serialize the IP address as a null-terminated string with a fixed length of 15 bytes.
	buffer.Write([]byte(host.IP + "\x00"))
	// Serialize the port as a 16-bit unsigned integer in little-endian order.
	portBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(portBytes, uint16(host.Port))
	buffer.Write(portBytes)
	return buffer
}

func DeserializeHost(buffer *bytes.Buffer) *Host {
	host := &Host{}
	// Read the IP address as a null-terminated string.
	ipBytes := make([]byte, 0)
	for {
		b, err := buffer.ReadByte()
		if err != nil || b == 0 {
			break
		}
		ipBytes = append(ipBytes, b)
	}
	host.IP = string(ipBytes)

	// Read the port as a 16-bit unsigned integer in little-endian order.
	portBytes := make([]byte, 2)
	_, err := buffer.Read(portBytes)
	if err != nil {
		return nil
	}
	host.Port = int(binary.LittleEndian.Uint16(portBytes))

	return host
}

func CompareHost(host1, host2 *Host) bool {
	// Compare IP addresses
	if host1.IP != host2.IP {
		return false
	}

	// Compare port numbers
	if host1.Port != host2.Port {
		return false
	}

	return true
}

func (host *Host) ToString() string {
	return host.IP + ":" + strconv.Itoa(host.Port)
}
