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
	configPath := flag.String("config", "", "YAML config file for logging/runtime buffers (required)")
	selfIP := flag.String("self-ip", "127.0.0.1", "IP address for this pingpong node")
	selfPort := flag.Int("self-port", 0, "TCP port for this pingpong node (required)")
	peerIP := flag.String("peer-ip", "127.0.0.1", "IP address for the peer pingpong node")
	peerPort := flag.Int("peer-port", 0, "TCP port for the peer pingpong node (required)")
	flag.Parse()

	if *configPath == "" {
		panic("config file is required (use -config path/to/config.yaml)")
	}
	if *selfPort == 0 {
		panic("self-port is required (use -self-port N)")
	}
	if *peerPort == 0 {
		panic("peer-port is required (use -peer-port N)")
	}

	cfg, err := rtconfig.LoadConfig(*configPath)
	if err != nil {
		panic(err)
	}

	logger := runtime.ApplyConfig(cfg)
	instance := runtime.GetRuntimeInstance()

	myself := net.NewHost(*selfPort, *selfIP)
	peer := net.NewHost(*peerPort, *peerIP)

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
