package net

import "bytes"

type (
	TransportLayer interface {
		Connect(host Host)
		Disconnect(host Host)
		Send(msg TransportMessage)
		OutChannel() chan TransportMessage
		OutChannelEvents() chan ConnEvents
		Cancel()
	}

	TransportMessage struct {
		Host Host
		Msg  bytes.Buffer
	}

	ConnEvents int
)

func NewTransportMessage(msg bytes.Buffer, host Host) TransportMessage {
	return TransportMessage{Msg: msg, Host: host}
}

const (
	ConnConnected ConnEvents = iota
	ConnDisconnected
	ConnFailed
)
