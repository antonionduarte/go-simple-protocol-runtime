package net

import "bytes"

type (
	NetworkLayer interface {
		Connect(host *Host)
		Disconnect(host *Host)
		Send(msg bytes.Buffer, host *Host)
	}

	NetworkMessage struct {
		Host *Host
		Msg  bytes.Buffer
	}

	ConnEvents int
)

const (
	ConnConnected ConnEvents = iota
	ConnDisconnected
	ConnFailed
)
