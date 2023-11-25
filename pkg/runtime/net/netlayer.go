package net

import "bytes"

type (
	NetworkLayer struct {
		transport        TransportLayer // the transport layer used, could be TCP, UDP, QUIC.
		serverMappings   map[Host]Host  // mapping of Client Addr to Server Addr.
		outChannelEvents chan TransportConnEvents
		outMessages      chan NetworkMessage
	}

	NetworkMessage struct {
		Host Host // server host of each peer instead of the client host that the transport layer receives
		Msg  bytes.Buffer
	}
)

func NewNetworkLayer(transport *TransportLayer) *NetworkLayer {
	return nil
}

func (n *NetworkLayer) networkHandler() { // TODO: Contexts and WaitGroups.
	for {
		select {
		case event := <-n.transport.OutChannelEvents():
			n.outChannelEvents <- event
		case msg := <-n.transport.OutChannel():
			n.processMessage(msg)
		}
	}
}

func (n *NetworkLayer) processMessage(msg TransportMessage) {

}
