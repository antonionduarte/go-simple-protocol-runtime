package net

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestTCPLayerConnection(t *testing.T) {
	first := NewHost(6503, "127.0.0.1")
	second := NewHost(6504, "127.0.0.1")

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

	if secondNode.activeConnectionCount() != 1 {
		t.Errorf("TCPConnection on secondNode should've been established.")
	}

	if firstNode.activeConnectionCount() != 1 {
		t.Errorf("TCPConnection on firstNode should've been established.")
	}
}

func TestTCPLayerSendMessage(t *testing.T) {
	first := NewHost(7501, "127.0.0.1")
	second := NewHost(7502, "127.0.0.1")

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

	var clientHost Host
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

// TestTCPLayerFramingMultipleMessages ensures that multiple messages sent back-to-back
// are correctly framed and delivered as distinct TransportMessages on the receiver.
func TestTCPLayerFramingMultipleMessages(t *testing.T) {
	first := NewHost(7601, "127.0.0.1")
	second := NewHost(7602, "127.0.0.1")

	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())
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
		case <-secondNode.OutTransportEvents():
			connectCount++
		case <-firstNode.OutTransportEvents():
			connectCount++
		}
	}

	msg1 := NewTransportMessage(*bytes.NewBuffer([]byte("msg1")), firstNode.self)
	msg2 := NewTransportMessage(*bytes.NewBuffer([]byte("msg2")), firstNode.self)

	firstNode.send(msg1, secondNode.self)
	firstNode.send(msg2, secondNode.self)

	rcv1 := <-secondNode.OutChannel()
	rcv2 := <-secondNode.OutChannel()

	if got := rcv1.Msg.String(); got != "msg1" {
		t.Fatalf("first received message mismatch: got %q, want %q", got, "msg1")
	}
	if got := rcv2.Msg.String(); got != "msg2" {
		t.Fatalf("second received message mismatch: got %q, want %q", got, "msg2")
	}
}

func TestDisconnect(t *testing.T) {
	first := NewHost(6913, "127.0.0.1")
	second := NewHost(6912, "127.0.0.1")

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

	if firstNode.activeConnectionCount() != 0 {
		t.Errorf("TCPConnection on firstNode should've been deleted.")
	}

	if secondNode.activeConnectionCount() != 0 {
		t.Errorf("TCPConnection on secondNode should've been deleted.")
	}
}

// TestTCPLayerCancelClosesConnections verifies that Cancel() on TCPLayer
// closes all active connections and leaves no connections tracked.
func TestTCPLayerCancelClosesConnections(t *testing.T) {
	first := NewHost(7801, "127.0.0.1")
	second := NewHost(7802, "127.0.0.1")

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	firstNode := NewTCPLayer(first, ctx1)
	secondNode := NewTCPLayer(second, ctx2)

	firstNode.Connect(second)

	connectCount := 0
	for {
		if connectCount == 2 {
			break
		}
		select {
		case <-secondNode.OutTransportEvents():
			connectCount++
		case <-firstNode.OutTransportEvents():
			connectCount++
		}
	}

	// Now cancel both layers and ensure there are no active connections.
	firstNode.Cancel()
	secondNode.Cancel()

	// Give some time for handlers to observe ctx cancellation.
	time.Sleep(10 * time.Millisecond)

	if got := firstNode.activeConnectionCount(); got != 0 {
		t.Fatalf("expected firstNode to have 0 active connections after Cancel, got %d", got)
	}
	if got := secondNode.activeConnectionCount(); got != 0 {
		t.Fatalf("expected secondNode to have 0 active connections after Cancel, got %d", got)
	}
}
