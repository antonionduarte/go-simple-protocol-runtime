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

func SerializeHost(h Host) bytes.Buffer {
	var buffer bytes.Buffer

	ip := net.ParseIP(h.IP)
	if ip == nil || ip.To4() == nil {
		return buffer
	}

	// store as 15-char + port
	buffer.WriteString(fmt.Sprintf("%-15s", h.IP))

	portBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(portBytes, uint16(h.Port))
	buffer.Write(portBytes)

	return buffer
}

func DeserializeHost(buffer bytes.Buffer) Host {
	var h Host

	ipBytes := buffer.Next(15)
	h.IP = string(bytes.TrimRight(ipBytes, " "))

	portBytes := buffer.Next(2)
	if len(portBytes) < 2 {
		return h
	}
	h.Port = int(binary.LittleEndian.Uint16(portBytes))
	return h
}

func CompareHost(a, b Host) bool {
	return a.IP == b.IP && a.Port == b.Port
}
