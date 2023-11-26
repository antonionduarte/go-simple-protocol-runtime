package net

import "bytes"

type (
	SessionLayer struct {
		transport        TransportLayer // the transport layer used, could be TCP, UDP, QUIC.
		serverMappings   map[Host]Host  // mapping of Client Addr to Server Addr.
		outChannelEvents chan TransportConnEvents
		outMessages      chan SessionMessage
	}

	SessionMessage struct {
		Host Host // server host of each peer instead of the client host that the transport layer receives
		Msg  bytes.Buffer
	}

	ConnEvents int
)

const (
	ConnConnected ConnEvents = iota
	ConnDisconnected
	ConnFailed
)

func NewNetworkLayer(transport *TransportLayer) *SessionLayer {
	return nil
}

func (n *SessionLayer) sessionHandler() { // TODO: Contexts and WaitGroups.
	for {
		select {
		case event := <-n.transport.OutChannelEvents():
			n.outChannelEvents <- event
		case msg := <-n.transport.OutChannel():
			n.processMessage(msg)
		}
	}
}

func (n *SessionLayer) processMessage(msg TransportMessage) {

}
