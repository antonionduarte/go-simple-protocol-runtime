package net

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
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

func SerializeHost(host *Host) (bytes.Buffer, error) {
	var buffer bytes.Buffer

	// Validate that the provided IP is a valid IPv4 address
	ip := net.ParseIP(host.IP)
	if ip == nil || ip.To4() == nil {
		return buffer, fmt.Errorf("invalid IPv4 address: %s", host.IP)
	}

	// Serialize the IP address as a fixed-length string of 15 bytes.
	buffer.WriteString(fmt.Sprintf("%-15s", host.IP))

	// Serialize the port as a 16-bit unsigned integer in little-endian order.
	portBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(portBytes, uint16(host.Port))
	buffer.Write(portBytes)

	return buffer, nil
}

func DeserializeHost(buffer *bytes.Buffer) (*Host, error) {
	host := &Host{}

	// Read the IP address as a fixed-length string of 15 bytes.
	ipBytes := buffer.Next(15)
	host.IP = string(bytes.TrimRight(ipBytes, " "))

	// Read the port as a 16-bit unsigned integer in little-endian order.
	portBytes := make([]byte, 2)
	_, err := buffer.Read(portBytes)
	if err != nil {
		return host, err
	}
	host.Port = int(binary.LittleEndian.Uint16(portBytes))

	return host, nil
}

func CompareHost(host1, host2 *Host) bool {
	return host1.IP == host2.IP && host1.Port == host2.Port
}

func (host *Host) ToString() string {
	return host.IP + ":" + strconv.Itoa(host.Port)
}
