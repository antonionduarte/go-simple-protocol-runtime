package main

import (
	"github.com/antonionduarte/go-simple-protocol-runtime/cmd/pingpong/protocol"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func main() {
	// get the instance of the protocol runtime
	instance := runtime.GetRuntimeInstance()

	myself := net.Host{Port: 5001, IP: "127.0.0.1"}
	// register the protocol
	// runtime.RegisterProtocol(NewPingPongProtocol())
	pingpong := runtime.NewProtoProtocol(protocol.NewPingPongProtocol(&myself), &myself)
	instance.RegisterProtocol(pingpong)

	// register a network layer, in this case a TCP layer
	instance.RegisterNetworkLayer(net.NewTCPLayer(&myself))

	// start the protocol runtime
	instance.Start()
}
