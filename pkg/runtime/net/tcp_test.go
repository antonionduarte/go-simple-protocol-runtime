package net

import (
	"bytes"
	"testing"
	"time"
)

func TestTCPLayerConnection(t *testing.T) {
	// Start a server on a random port
	serverHost := NewHost(6501, "127.0.0.1")
	serverLayer := NewTCPLayer(serverHost)

	// Start the server in a separate goroutine
	go func() {
		serverLayer.start(serverHost)
	}()

	// Allow some time for the server to start
	time.Sleep(time.Second)

	// Now set up the client to connect to the server
	clientHost := NewHost(6502, "127.0.0.1")
	clientLayer := NewTCPLayer(clientHost)

	// Attempt to connect the client to the server
	clientLayer.Connect(serverHost)

	clientLayer.Cancel()
	serverLayer.Cancel()
}

func TestTCPLayerSendMessage(t *testing.T) {
	// Start a server on a random port
	serverHost := NewHost(6501, "127.0.0.1")
	serverLayer := NewTCPLayer(serverHost)

	// Start the server in a separate goroutine
	go func() {
		serverLayer.start(serverHost)
	}()

	// Allow some time for the server to start
	time.Sleep(time.Second)

	// Now set up the client to connect to the server
	clientHost := NewHost(6502, "127.0.0.1")
	clientLayer := NewTCPLayer(clientHost)

	// Attempt to connect the client to the server
	clientLayer.Connect(serverHost) // TODO: The connect is failing

	// Test sending a message from client to server
	testMsg := TransportMessage{
		Host: serverHost,
		Msg:  *bytes.NewBufferString("Hello from client"),
	}

	clientLayer.Send(testMsg)

	// Allow some time for the message to be processed
	time.Sleep(time.Second)

	receivedMsg := <-serverLayer.outChannel

	if receivedMsg.Msg.String() != "Hello from client" {
		t.Errorf("Received message is incorrect")
	}

	// Cleanup: Close connections and layers
	clientLayer.Cancel()
	serverLayer.Cancel()
}

func TestDisconnect(t *testing.T) {
	// Start the server
	serverHost := NewHost(6502, "127.0.0.1") // Using a specific port for the server
	serverLayer := NewTCPLayer(serverHost)

	go func() {
		serverLayer.start(serverHost)
	}()

	// Give some time for the server to start
	time.Sleep(time.Second)

	// Start the client and connect to the server
	clientHost := NewHost(6503, "127.0.0.1") // Using a specific port for the client
	clientLayer := NewTCPLayer(clientHost)
	clientLayer.Connect(serverHost)

	// Give some time for the connection to be established
	time.Sleep(time.Second)

	// Check that the server has the client in its active connections
	if _, ok := serverLayer.activeConnections[clientHost]; !ok {
		t.Errorf("Client should be in server's active connections")
	}

	// Now disconnect the client
	clientLayer.Disconnect(serverHost)

	// Give some time for the disconnect to be processed
	time.Sleep(time.Second)

	// Check that the client is no longer in the server's active connections
	if _, ok := serverLayer.activeConnections[clientHost]; ok {
		t.Errorf("Client should not be in server's active connections after disconnect")
	}

	// Cleanup
	clientLayer.Cancel()
	serverLayer.Cancel()
}
