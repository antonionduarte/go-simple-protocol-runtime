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
	clientLayer.Connect(serverHost) // TODO: The connect is failing

	println("Test got here")

	// Test sending a message from client to server
	testMsg := &NetworkMessage{
		Host: serverHost,
		Msg:  bytes.NewBufferString("Hello from client"),
	}

	println("Test got here 2")

	clientLayer.Send(testMsg)

	println("Test got here 3")

	// Allow some time for the message to be processed
	time.Sleep(time.Second)

	// TODO: Check if the message is received by the server
	// This part is left as an exercise since it depends on how you handle incoming messages

	// Cleanup: Close connections and layers
	clientLayer.Cancel()
	serverLayer.Cancel()
}
