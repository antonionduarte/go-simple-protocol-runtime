# TODO

Mostly historical — the framework is at the point where the remaining work
is feature-additive rather than foundational. Most checked items were
completed during the polish passes leading up to v0.1.0.

## Pending

- [ ] Configuration file parser at the runtime level (per-protocol config).
      The pingpong example has YAML for logging only.
- [ ] Errors-we-could-detect-and-log: net-layer wrapper errors, "protocol
      not registered for received wireID" diagnostic, "trying to start
      runtime without a network layer registered" already returns
      `ErrNoNetworkLayer` but we could log louder.

## Done

- [x] Basic structure (Protocol, Runtime, ProtocolContext).
- [x] TCP transport layer with length-prefixed framing.
- [x] SessionLayer with Hello/Ack handshake binding ephemeral
      connections to stable Host identities.
- [x] Type-hashed message dispatch via `WireID[T]` (FNV-1a on Go type
      name; opt-in `WireName()` override for rename-safety).
- [x] Generic typed handlers (`RegisterHandler[*M]`).
- [x] BinaryCodec for fixed-size messages; `pkg/wire` helpers for
      variable-length.
- [x] Per-protocol event-loop concurrency.
- [x] Timer system: `SetupTimer`, `SetupPeriodicTimer`, `CancelTimer`,
      `RegisterTimerHandler`.
- [x] Reconnect policy with exponential backoff + jitter
      (`ConnectWithRetry`).
- [x] Inter-protocol communication: Request/Reply via
      `RegisterRequestHandler` + `SendRequest`, fan-out Notifications
      via `SubscribeNotification` / `PublishNotification`. Local-only.
- [x] Panic recovery for handler dispatch: every handler runs under a
      `recover` so one bad protocol can't take down its event loop.
      Optional `PanicHandler` interface; request handlers auto-fail
      their `Responder` with `ErrHandlerPanicked`.
- [x] Pingpong example.
- [x] Multi-layer gossip example with 10-node integration test.
- [x] Goleak-verified shutdown.
- [x] Sentinel errors, `errors.Is`-able.
- [x] Component-tagged structured logging via `slog`.
- [x] CI: build, vet, lint, race tests on every PR / push.
- [x] LICENSE (MIT).
- [x] README rewrite, CONTRIBUTING.md.

## Considered but punted

- Multi-node gossip-membership (HyParView / SWIM). The static
  membership in `cmd/gossip` is enough for framework validation.
- Plumtree / lazy-push spanning-tree gossip. Eager push is the right
  baseline.
- Wire-level TLS / authentication. Out of scope for the framework
  itself; layer it on top.
- Connection-pool / multiplexing. One TCP conn per peer pair is fine
  for protorun's scope.

## Soon (in the v0.1.0 polish pass)

- [ ] Framework-level invariant assertions, opt-in via `WithStrict(true)`,
      paying nothing in production. Catches misuse the Go type system
      can't express. Concrete asserts: double-registration of a wireID,
      phase tracking (`Connect` only post-`Init`, registrations only
      inside `Start`), slow-handler watchdog (log/panic when a single
      handler invocation exceeds N seconds), reply-without-handler,
      subscribe-without-lifecycle guard.
- [ ] Capability-typed `ProtocolContext`: split into composed
      interfaces (`Connector`, `Sender`, `Timing`, `Identity`) so
      handlers can declare narrower deps.
- [ ] Metrics interface (`Metrics` + `WithMetrics(m)` option) covering
      message dispatch counts, IPC latency, session events.
- [ ] Benchmarks for hot paths (message dispatch, IPC round-trip,
      codec encode/decode, notification fanout).
- [ ] `Runtime.Shutdown(timeout)` for graceful drain (today only
      forceful `Cancel` is exposed).
- [ ] Wire-format version negotiation in the Hello handshake.
- [x] `transport.Address` interface defined; Host implements it.
- [ ] Migrate `transport.Layer` and downstream APIs to take
      `Address` instead of `Host`. Deferred from the v0.1.0 polish
      pass: the interface exists (so users can declare custom
      address types), but the concrete Layer methods still take
      Host. Will land once a real non-TCP backend (UDP / in-memory
      mesh / QUIC) needs the abstraction.
- [ ] Decompose `Runtime` into focused subsystems
      (`ipcRouter`, `retryTable`, `timerTable`, `codecRegistry`).
- [ ] Tag `v0.1.0`.
