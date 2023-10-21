package net

import "bytes"

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

func (host *Host) Serialize() bytes.Buffer {
	var buffer bytes.Buffer
	buffer.Write([]byte(host.IP))
	buffer.Write([]byte(string(rune(host.Port))))
	return buffer
}

func (host *Host) Deserialize(buffer bytes.Buffer) *Host {
	host.IP = string(buffer.Next(4))
	host.Port = int(buffer.Next(1)[0])
	return host
}

func (host *Host) ToString() string {
	return host.IP + ":" + string(rune(host.Port))
}
