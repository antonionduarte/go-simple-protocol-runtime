package net

import (
	"bytes"
	"strconv"
)

type (
	TransportLayer interface {
		Connect(host TransportHost)
		Disconnect(host TransportHost)
		Send(msg TransportMessage)
		OutChannel() chan TransportMessage
		OutChannelEvents() chan ConnEvents
		Cancel()
	}

	TransportMessage struct {
		Host TransportHost
		Msg  bytes.Buffer
	}

	TransportHost struct {
		Port int
		IP   string
	}

	ConnEvents int
)

func NewTransportHost(port int, ip string) TransportHost {
	return TransportHost{
		Port: port,
		IP:   ip,
	}
}

func NewTransportMessage(msg bytes.Buffer, host TransportHost) TransportMessage {
	return TransportMessage{Msg: msg, Host: host}
}

func (host *TransportHost) ToString() string {
	return host.IP + ":" + strconv.Itoa(host.Port)
}

const (
	ConnConnected ConnEvents = iota
	ConnDisconnected
	ConnFailed
)
