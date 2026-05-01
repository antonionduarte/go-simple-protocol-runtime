package main

import (
	"context"
	"flag"
	"log/slog"

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

	logger := runtime.NewLoggerFromConfig(cfg.Logging)
	slog.SetDefault(logger)

	myself := net.NewHost(*selfPort, *selfIP)
	peer := net.NewHost(*peerPort, *peerIP)

	logger.Info("starting pingpong node",
		"role", "node",
		"self", (&myself).ToString(),
		"peer", (&peer).ToString(),
	)

	rt := runtime.New(myself,
		runtime.WithLogger(logger),
		runtime.WithChannelBuffer(cfg.Runtime.ChannelBuffer),
	)

	ctx := context.Background()
	tcp := net.NewTCPLayer(myself, ctx, cfg.Runtime.ChannelBuffer)
	session := net.NewSessionLayer(tcp, myself, ctx, cfg.Runtime.ChannelBuffer, cfg.Runtime.ChannelBuffer)
	rt.RegisterNetworkLayer(tcp)
	rt.RegisterSessionLayer(session)
	rt.RegisterProtocol(runtime.NewProtoProtocol(
		protocol.NewPingPongProtocol(&myself, &peer), myself,
	))

	if err := rt.Start(); err != nil {
		panic(err)
	}

	select {}
}
