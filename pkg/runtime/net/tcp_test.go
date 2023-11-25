package net

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestTCPLayerConnection(t *testing.T) {
	first := NewTransportHost(6001, "127.0.0.1")
	second := NewTransportHost(6002, "127.0.0.1")

	firstCtx := context.Background()
	secondCtx := context.Background()

	firstCtx, firstCancel := context.WithCancel(firstCtx)
	secondCtx, secondCancel := context.WithCancel(secondCtx)

	defer firstCancel()
	defer secondCancel()

	firstNode := NewTCPLayer(first, firstCtx)
	secondNode := NewTCPLayer(second, secondCtx)

	firstNode.Connect(second)

	<-firstNode.OutChannelEvents()
	<-secondNode.OutChannelEvents()

	time.Sleep(1 * time.Second)

	if len(secondNode.activeConnections) != 1 {
		t.Errorf("TCPConnection on secondNode should've been established.")
	}

	if len(firstNode.activeConnections) != 1 {
		t.Errorf("TCPConnection on firstNode should've been established.")
	}
}

func TestTCPLayerSendMessage(t *testing.T) {
	first := NewTransportHost(5001, "127.0.0.1")
	second := NewTransportHost(5002, "127.0.0.1")

	firstCtx := context.Background()
	secondCtx := context.Background()

	firstCtx, firstCancel := context.WithCancel(firstCtx)
	secondCtx, secondCancel := context.WithCancel(secondCtx)

	defer firstCancel()
	defer secondCancel()

	firstNode := NewTCPLayer(first, firstCtx)
	secondNode := NewTCPLayer(second, secondCtx)

	firstNode.Connect(second)

	<-firstNode.outChannelEvents
	<-secondNode.outChannelEvents

	time.Sleep(1 * time.Second)

	msg := NewTransportMessage(*bytes.NewBuffer([]byte("Test message")), firstNode.self)
	firstNode.Send(msg, secondNode.self)

	receivedMsg := <-secondNode.OutChannel()

	if receivedMsg.Msg.String() != "Test message" {
		t.Errorf("SecondNode received incorrect message.")
	}

	var clientHost TransportHost
	for key := range secondNode.activeConnections {
		clientHost = key
		break
	}

	secondNode.Send(msg, clientHost)

	receivedMsg = <-firstNode.OutChannel()
	if receivedMsg.Msg.String() != "Test message" {
		t.Errorf("FirstNode received incorrect message.")
	}
}

func TestDisconnect(t *testing.T) {
	first := NewTransportHost(7001, "127.0.0.1")
	second := NewTransportHost(7002, "127.0.0.1")

	firstCtx := context.Background()
	secondCtx := context.Background()

	firstCtx, firstCancel := context.WithCancel(firstCtx)
	secondCtx, secondCancel := context.WithCancel(secondCtx)

	defer firstCancel()
	defer secondCancel()

	firstNode := NewTCPLayer(first, firstCtx)
	secondNode := NewTCPLayer(second, secondCtx)

	firstNode.Connect(second)

	_ = <-firstNode.OutChannelEvents()
	_ = <-secondNode.OutChannelEvents()

	time.Sleep(1 * time.Second)

	firstNode.Disconnect(second)

	_ = <-secondNode.OutChannelEvents()
	_ = <-firstNode.OutChannelEvents()

	time.Sleep(1 * time.Second)

	if len(firstNode.activeConnections) != 0 {
		t.Errorf("TCPConnection on firstNode should've been deleted.")
	}

	if len(secondNode.activeConnections) != 0 {
		t.Errorf("TCPConnection on secondNode should've been deleted.")
	}
}
