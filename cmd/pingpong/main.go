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

	logger := runtime.ApplyConfig(cfg)
	instance := runtime.GetRuntimeInstance()

	myself := net.NewHost(cfg.Runtime.Self.Port, cfg.Runtime.Self.IP)
	peer := net.NewHost(cfg.Runtime.Peer.Port, cfg.Runtime.Peer.IP)

	selfStr := (&myself).ToString()
	peerStr := (&peer).ToString()
	logger.Info("starting pingpong node",
		"role", "node",
		"self", selfStr,
		"peer", peerStr,
	)

	pingpong := runtime.NewProtoProtocol(protocol.NewPingPongProtocol(&myself, &peer), myself)
	instance.RegisterProtocol(pingpong)

	ctx := context.Background()
	tcp := net.NewTCPLayer(myself, ctx)
	session := net.NewSessionLayer(tcp, myself, ctx)
	instance.RegisterNetworkLayer(tcp)
	instance.RegisterSessionLayer(session)

	instance.Start()

	select {}
}
