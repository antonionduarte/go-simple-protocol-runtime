package main

import (
	"context"
	"flag"

	"github.com/antonionduarte/go-simple-protocol-runtime/cmd/pingpong/protocol"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"
	rtconfig "github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/config"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func main() {
	configPath := flag.String("config", "", "YAML config file (required)")
	flag.Parse()

	if *configPath == "" {
		panic("config file is required (use -config path/to/config.yaml)")
	}

	cfg, err := rtconfig.LoadConfig(*configPath)
	if err != nil {
		panic(err)
	}

	// Apply config globally and obtain the configured logger.
	logger := runtime.ApplyConfig(cfg)
	instance := runtime.GetRuntimeInstance()

	// Determine self/peer directly from config (no defaults here; this is an example).
	myself := net.NewHost(cfg.Runtime.Self.Port, cfg.Runtime.Self.IP)
	peer := net.NewHost(cfg.Runtime.Peer.Port, cfg.Runtime.Peer.IP)

	selfStr := (&myself).ToString()
	peerStr := (&peer).ToString()
	logger.Info("starting pingpong node",
		"role", "node",
		"self", selfStr,
		"peer", peerStr,
	)

	// register the protocol
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
