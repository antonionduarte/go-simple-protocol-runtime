package net

import "bytes"

type (
	NetworkLayer interface {
		Connect(host *Host)
		Disconnect(host *Host)
		Send(msg *NetworkMessage)
		OutChannel() chan *NetworkMessage
		OutChannelEvents() chan *ConnEvents
	}

	NetworkMessage struct {
		Host *Host
		Msg  *bytes.Buffer
	}

	ConnEvents int
)

func NewNetworkMessage(msg *bytes.Buffer, host *Host) *NetworkMessage {
	return &NetworkMessage{Msg: msg, Host: host}
}

const (
	ConnConnected ConnEvents = iota
	ConnDisconnected
	ConnFailed
)
