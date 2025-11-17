package net

import "bytes"

type TransportLayer interface {
	Connect(host Host)
	Disconnect(host Host)
	Send(msg TransportMessage, sendTo Host)

	OutChannel() chan TransportMessage
	OutTransportEvents() chan TransportEvent

	Cancel()
}

type (
	TransportMessage struct {
		Host Host
		Msg  bytes.Buffer
	}

	TransportEvent interface {
		Host() Host
	}

	TransportConnected struct {
		host Host
	}

	TransportDisconnected struct {
		host Host
	}

	TransportFailed struct {
		host Host
	}
)

func NewTransportMessage(msg bytes.Buffer, host Host) TransportMessage {
	return TransportMessage{Msg: msg, Host: host}
}

func (e *TransportConnected) Host() Host    { return e.host }
func (e *TransportDisconnected) Host() Host { return e.host }
func (e *TransportFailed) Host() Host       { return e.host }
