package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/antonionduarte/go-simple-protocol-runtime/cmd/pingpong/protocol"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func main() {
	port := flag.Int("port", 5001, "local TCP port")
	peerPort := flag.Int("peer-port", 5002, "peer TCP port")
	flag.Parse()

	// Configure a global structured logger at debug level so we can
	// see everything that happens in this example.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// get the instance of the protocol runtime
	instance := runtime.GetRuntimeInstance()

	myself := net.NewHost(*port, "127.0.0.1")
	peer := net.NewHost(*peerPort, "127.0.0.1")

	selfStr := (&myself).ToString()
	peerStr := (&peer).ToString()
	slog.Info("starting pingpong node",
		"role", "node",
		"self", selfStr,
		"peer", peerStr,
	)
	// register the protocol
	// runtime.RegisterProtocol(NewPingPongProtocol())
	pingpong := runtime.NewProtoProtocol(protocol.NewPingPongProtocol(&myself, &peer), myself)
	instance.RegisterProtocol(pingpong)

	// register a network layer, in this case a TCP layer
	ctx := context.Background()
	tcp := net.NewTCPLayer(myself, ctx)
	session := net.NewSessionLayer(tcp, myself, ctx)
	instance.RegisterNetworkLayer(tcp)
	instance.RegisterSessionLayer(session)

	// start the protocol runtime
	instance.Start()

	// Block main so the process does not exit immediately; terminate with Ctrl+C.
	select {}
}
