package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/antonionduarte/go-simple-protocol-runtime/cmd/pingpong/protocol"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func main() {
	port := flag.Int("port", 5001, "local TCP port")
	peerPort := flag.Int("peer-port", 5002, "peer TCP port")
	isClient := flag.Bool("client", false, "if true, initiate connection and send first ping")
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

	role := "server"
	if *isClient {
		role = "client"
	}
	selfStr := (&myself).ToString()
	peerStr := (&peer).ToString()
	slog.Info("starting pingpong node",
		"role", role,
		"self", selfStr,
		"peer", peerStr,
	)
	// register the protocol
	// runtime.RegisterProtocol(NewPingPongProtocol())
	pingpong := runtime.NewProtoProtocol(protocol.NewPingPongProtocol(&myself, &peer), &myself)
	instance.RegisterProtocol(pingpong)

	// register a network layer, in this case a TCP layer
	ctx := context.Background()
	tcp := net.NewTCPLayer(myself, ctx)
	session := net.NewSessionLayer(tcp, myself, ctx)
	instance.RegisterNetworkLayer(tcp)
	instance.RegisterSessionLayer(session)

	// If this node is the client, initiate a session to the peer and send
	// the first Ping once the session is established.
	if *isClient {
		go func() {
			slog.Info("client connecting to peer", "peer", peerStr)
			session.Connect(peer)
			for ev := range session.OutChannelEvents() {
				switch e := ev.(type) {
				case *net.SessionConnected:
					host := e.Host()
					slog.Info("session connected", "host", (&host).ToString())
					if net.CompareHost(e.Host(), peer) {
						slog.Info("session established with peer, sending initial Ping")
						runtime.SendMessage(protocol.NewPingMessage(myself), peer)
						return
					}
				case *net.SessionFailed:
					host := e.Host()
					slog.Error("session failed", "host", (&host).ToString())
				case *net.SessionDisconnected:
					host := e.Host()
					slog.Warn("session disconnected", "host", (&host).ToString())
				default:
					fmt.Printf("unknown session event: %#v\n", ev)
				}
			}
		}()
	}

	// start the protocol runtime
	instance.Start()

	// Block main so the process does not exit immediately; terminate with Ctrl+C.
	select {}
}
