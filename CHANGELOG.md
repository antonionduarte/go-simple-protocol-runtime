# Changelog

All notable changes to protorun will be documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The framework is pre-1.0; minor versions may break API. Wire format
is versioned via the session-layer handshake (`transport.ProtocolVersion`).

## v0.1.0 — 2026-05-02

First tagged release. Established the protocol-runtime core, IPC
plane, recovery semantics, observability hooks, and a runnable
multi-layer example (membership + eager-push gossip with a
10-node integration test).

### Added

- **Core runtime.** `protorun.New(self, ...Option)`, `Register`,
  `Run`, `Cancel`. Per-protocol event-loop concurrency. Component-
  tagged `slog` logging.
- **Type-safe message dispatch.** Wire IDs derived from Go type
  names (FNV-1a) with `WireName()` opt-in to freeze the ID across
  renames. Generic `RegisterCodec[*M]`, `RegisterHandler[*M]`,
  `BinaryCodec[*M]` for fixed-size structs, `pkg/wire` for
  variable-length encoding.
- **TCP transport with handshake.** `WithTCPTransport(ctx)` wires a
  TCPLayer + SessionLayer. Hello/Ack handshake binds ephemeral
  connections to stable Host identities. Session events
  (`SessionConnected` / `SessionDisconnected` / `SessionFailed` /
  `SessionGivenUp` / `SessionVersionMismatch`) deliver to optional
  protocol-side handler interfaces.
- **Reconnect policy.** `ConnectWithRetry(host)` with
  `WithRetryPolicy` (exponential backoff + jitter). Defaults are
  unbounded retries; configurable per-runtime.
- **Inter-protocol communication (IPC).** Same-runtime Request/Reply
  via `RegisterRequestHandler[Req,Rep]` + `SendRequest`, and fan-out
  Notifications via `SubscribeNotification[N]` /
  `PublishNotification[N]`. Local-only by design — cross-node
  coordination still flows through the peer-message path.
- **Panic recovery.** Every handler dispatch runs under a `recover`
  so a single bad protocol can't take down its event loop. Optional
  `PanicHandler` interface for metrics / supervision; request
  handlers auto-fail their `Responder` with `ErrHandlerPanicked`.
- **Capability-typed `ProtocolContext`.** Composes finer-grained
  interfaces: `Connector`, `Sender`, `Timing`, `Identity`. Existing
  code is source-compatible; new code can declare narrower deps.
- **Strict mode.** `WithStrict(bool)` opt-in runtime invariant
  checks: double-registration, phase ordering, slow-handler
  watchdog (configurable via `WithStrictHandlerTimeout`),
  reply-without-handler diagnostics.
- **Metrics interface.** `Metrics` (Counter + Histogram with `Attr`
  attributes) + `WithMetrics(m)`. Default no-op. Instruments core
  paths (`protorun.message.*`, `protorun.ipc.*`,
  `protorun.notification.*`, `protorun.session.*`,
  `protorun.handler.panic`, `protorun.strict.slow_handler`).
- **Bounded shutdown.** `Runtime.Shutdown(timeout)` returns
  `ErrShutdownTimeout` if the WaitGroup hasn't drained when the
  timer fires. `Cancel` stays unbounded.
- **Wire-format version negotiation.** `transport.ProtocolVersion`
  is part of every Hello; mismatched peers see a typed
  `SessionVersionMismatch` event. Lock-stepped at v0.1.0; bump
  alongside any framing change.
- **Address interface.** `transport.Address` (`String` + `Equal`)
  defines the abstract peer-identity surface. `Host` satisfies it.
  Future non-TCP transports plug in here. The Layer migration to
  take Address everywhere is deferred (see TODO.md).
- **Custom transport injection.** `WithTransport(layer, session)`
  closes the IoC seam — pre-built transport stacks plug in via
  the public Option, no internal access required.
- **Examples.** `cmd/pingpong` (canonical two-node) and `cmd/gossip`
  (membership + eager-push gossip with a 10-node integration test
  and 100/1000-node scale probes). 10-node test stable under
  `go test -race -count=10`.
- **Benchmarks.** `BenchmarkProcessMessage`, `BenchmarkSendRequest`,
  `BenchmarkBinaryCodec_*`, `BenchmarkPublishNotification_Fanout`
  (1/10/100 subscribers), `BenchmarkWireID`. `make bench` target.
- **Goleak in TestMain.** Every test package is goroutine-leak
  guarded.
- **CI** (build, vet, race tests, lint with new-from-rev gate,
  govulncheck, coverage threshold gate at 60%).
- **`golangci-lint`** config with Go-community-aligned thresholds
  (`gocyclo`, `cyclop`, `gocognit`, `funlen`, `lll`, `dupl`,
  Uber-aligned style rules).
- **`pre-commit` hook** (lint + tests on staged Go files via
  `make hooks-install`).

### Out of scope (post-v0.1.0)

See [`TODO.md`](TODO.md). Highlights: full `Address` migration of
`transport.Layer`; HyParView / SWIM gossip-membership; wire-level
TLS; richer per-protocol configuration; benchmark baseline tracking.
