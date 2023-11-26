package net

import (
	"bytes"
	"strconv"
)

type (
	TransportLayer interface {
		Connect(host TransportHost)
		Disconnect(host TransportHost)
		Send(msg TransportMessage, sendTo TransportHost)
		OutChannel() chan TransportMessage
		OutTransportEvents() chan TransportEvent
	}

	TransportMessage struct {
		Host TransportHost
		Msg  bytes.Buffer
	}

	TransportHost struct {
		Port int
		IP   string
	}

	TransportEvent interface {
		// Host returns associated Host of the Transport Event
		Host() TransportHost
	}

	TransportConnected struct {
		host TransportHost
	}

	TransportDisconnected struct {
		host TransportHost
	}

	TransportFailed struct {
		host TransportHost
	}
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

func (e *TransportConnected) Host() TransportHost {
	return e.host
}

func (e *TransportDisconnected) Host() TransportHost {
	return e.host
}

func (e *TransportFailed) Host() TransportHost {
	return e.host
}

func (host *TransportHost) ToString() string {
	return host.IP + ":" + strconv.Itoa(host.Port)
}
