package net

import (
	"bytes"
	"context"
	"testing"
)

func TestTCPLayerConnection(t *testing.T) {
	first := NewTransportHost(6503, "127.0.0.1")
	second := NewTransportHost(6504, "127.0.0.1")

	firstCtx := context.Background()
	secondCtx := context.Background()

	firstCtx, firstCancel := context.WithCancel(firstCtx)
	secondCtx, secondCancel := context.WithCancel(secondCtx)

	defer firstCancel()
	defer secondCancel()

	firstNode := NewTCPLayer(first, firstCtx)
	secondNode := NewTCPLayer(second, secondCtx)

	firstNode.Connect(second)

	connectCount := 0
	for {
		if connectCount == 2 {
			break
		}
		select {
		case _ = <-secondNode.OutTransportEvents():
			connectCount++
		case _ = <-firstNode.OutTransportEvents():
			connectCount++
		}
	}

	if len(secondNode.activeConnections) != 1 {
		t.Errorf("TCPConnection on secondNode should've been established.")
	}

	if len(firstNode.activeConnections) != 1 {
		t.Errorf("TCPConnection on firstNode should've been established.")
	}
}

func TestTCPLayerSendMessage(t *testing.T) {
	first := NewTransportHost(7501, "127.0.0.1")
	second := NewTransportHost(7502, "127.0.0.1")

	firstCtx := context.Background()
	secondCtx := context.Background()

	firstCtx, firstCancel := context.WithCancel(firstCtx)
	secondCtx, secondCancel := context.WithCancel(secondCtx)

	defer firstCancel()
	defer secondCancel()

	firstNode := NewTCPLayer(first, firstCtx)
	secondNode := NewTCPLayer(second, secondCtx)

	firstNode.Connect(second)

	connectCount := 0
	for {
		if connectCount == 2 {
			break
		}
		select {
		case _ = <-secondNode.OutTransportEvents():
			connectCount++
		case _ = <-firstNode.OutTransportEvents():
			connectCount++
		}
	}

	msg := NewTransportMessage(*bytes.NewBuffer([]byte("Test message")), firstNode.self)
	firstNode.send(msg, secondNode.self)

	receivedMsg := <-secondNode.OutChannel()

	if receivedMsg.Msg.String() != "Test message" {
		t.Errorf("SecondNode received incorrect message.")
	}

	var clientHost TransportHost
	for key := range secondNode.activeConnections {
		clientHost = key
		break
	}

	secondNode.send(msg, clientHost)

	receivedMsg = <-firstNode.OutChannel()
	if receivedMsg.Msg.String() != "Test message" {
		t.Errorf("FirstNode received incorrect message.")
	}
}

func TestDisconnect(t *testing.T) {
	first := NewTransportHost(6913, "127.0.0.1")
	second := NewTransportHost(6912, "127.0.0.1")

	firstCtx := context.Background()
	secondCtx := context.Background()

	firstCtx, firstCancel := context.WithCancel(firstCtx)
	secondCtx, secondCancel := context.WithCancel(secondCtx)

	defer firstCancel()
	defer secondCancel()

	firstNode := NewTCPLayer(first, firstCtx)
	secondNode := NewTCPLayer(second, secondCtx)

	firstNode.Connect(second)

	connectCount := 0
	for {
		if connectCount == 2 {
			break
		}
		select {
		case _ = <-secondNode.OutTransportEvents():
			connectCount++
		case _ = <-firstNode.OutTransportEvents():
			connectCount++
		}
	}

	firstNode.Disconnect(second)

	disconnectCount := 0
	for {
		if disconnectCount == 2 {
			break
		}
		select {
		case _ = <-secondNode.OutTransportEvents():
			disconnectCount++
		case _ = <-firstNode.OutTransportEvents():
			disconnectCount++
		}
	}

	if len(firstNode.activeConnections) != 0 {
		t.Errorf("TCPConnection on firstNode should've been deleted.")
	}

	if len(secondNode.activeConnections) != 0 {
		t.Errorf("TCPConnection on secondNode should've been deleted.")
	}
}
