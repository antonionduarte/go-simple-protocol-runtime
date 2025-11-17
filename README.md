# Simple Protocol Runtime 
Simple protocol runtime in Go. (heavily inspired by [Babel](https://github.com/pfouto/babel-core) (java framework to implement distributed protocols).)

## Extremely WIP please don't mock my code.
## TODO:

- [X] Implement basic structure.
- [X] Implement network layer, for TCP initially. TODO: Had an error, I probably need to do a handshake.
- [ ] Implement inter-protocol communication, so that we can send messages between protocols within same Runtime.
- [X] Implement a simple protocol, which can send and receive messages to test this.
- [X] Implement timer functionality.
- [ ] Implement configuration file parser.
- [X] Add contexts **everywhere** in order to gracefully finish the runtime and it's experiments.
- [X] Decide how I actually want to manage the self Host.
- [X] High Priority - Might break everything - Rethink if I save the Host in the Message Structs.
- [X] Decide correctly which static functions should be in which file, possibly ask GPT-4 about it his answer will be correct. 
- [X] Should the cancelation of the runtime be called at the runtime level or?
- [X] Should the network layer be generic by itself and abstract stuff like cancelation?
- [X] Some mechanism for the main thread to actually finish execution.
- [X] Propper channel management, I believe i'm not actually properly closing channels yet.

```go
package main

type Message struct{}
type Host struct{}

type ProtoProtocol struct {
	msgHandlers map[int]func(msg Message, from Host)
}

func SendMessage(msg Message, sendTo Host) {}
```
- [X] Consider copying Hosts around as values instead of using references. 

## How to write a protocol

Protocols implement the `Protocol` interface in `pkg/runtime`:

```go
type Protocol interface {
    Start(ctx ProtocolContext)
    Init(ctx ProtocolContext)
    ProtocolID() int
    Self() net.Host
}
```

In `Start(ctx)` you register handlers and serializers:

```go
func (p *MyProtocol) Start(ctx runtime.ProtocolContext) {
    ctx.RegisterMessageHandler(MyMessageID, p.HandleMyMessage)
    ctx.RegisterMessageSerializer(MyMessageID, &MySerializer{})
}
```

In `Init(ctx)` you perform bootstrap actions (e.g. connect to peers, set timers):

```go
func (p *MyProtocol) Init(ctx runtime.ProtocolContext) {
    ctx.Connect(p.peer) // initiate a session to a peer
}
```

If your protocol wants to react to session events, it can optionally implement:

```go
type SessionConnectedHandler interface {
    OnSessionConnected(net.Host)
}

type SessionDisconnectedHandler interface {
    OnSessionDisconnected(net.Host)
}
```

The runtime will call these methods from the protocol's own event loop whenever sessions are established or torn down.

All protocol callbacks (message handlers, timer handlers, and the optional
session event handlers) are serialized through a single event loop per
protocol (`ProtoProtocol.eventHandler`). This means you can usually mutate
protocol state directly inside your handlers without additional locking,
as long as all access happens from within those callbacks.

## Net layer:

- [X] Host struct with serializer and deserializer.
- [X] Add the Sender Host to all the messages.

Types of Network / Channel Events:

```go
// Transport-level events (TCP)
type TransportEvent interface {
    Host() Host
}

type TransportConnected struct {
    host Host
}

type TransportDisconnected struct {
    host Host
}

type TransportFailed struct {
    host Host
}

// Session-level events (after handshake)
type SessionEvent interface {
    Host() Host
}

type SessionConnected struct {
    host Host
}

type SessionDisconnected struct {
    host Host
}

type SessionFailed struct {
    host Host
}
```

I think I want message handler format to be:

```go
package main

// Receiving messages:

func handleRandomMessage(msg Message, from Host)

// And sending messages:
// ... messages will propagate downwards, Runtime -> NetworkLayer -> TransportLayer
func SendMessage(msg Message, to Host) // Runtime
func Send(msg NetworkMessage, to Host) // NetworkLayer
func Send(msg TransportMessage, to Host) // TransportLayer
```

## Logging

The runtime and its components use Go's `log/slog` package for structured
logging. There are four main logging layers:

- **runtime**: high-level lifecycle events (starting, stopping, protocol init).
- **session**: handshake and session events (connect / disconnect / fail).
- **transport**: TCP-level events (listen, connect, read/write errors).
- **protocol**: user protocol logs (e.g. \"Ping received\", \"Pong received\").

The `Runtime` holds a base `*slog.Logger`, which you can configure from your
application:

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
runtime.GetRuntimeInstance().SetLogger(logger)
```

Session and transport layers receive component-specific loggers (e.g.
`logger.With(\"component\", \"session\")`, `logger.With(\"component\", \"transport\")`),
and `ProtocolContext.Logger()` returns a logger scoped to the protocol:

```go
func (p *MyProtocol) Start(ctx runtime.ProtocolContext) {
    log := ctx.Logger()
    log.Info(\"protocol starting\", \"self\", ctx.Self().ToString())
    // register handlers...
}
```

## Configuration (YAML)

Example applications can be configured via a simple YAML file using the
`pkg/runtime/config` package. A minimal schema looks like:

```yaml
logging:
  level: debug          # debug, info, warn, error
  components: ["protocol"] # runtime, session, transport, protocol
  format: text          # text or json

runtime:
  self:
    ip: 127.0.0.1
    port: 5001
  peer:
    ip: 127.0.0.1
    port: 5002
```

In `cmd/pingpong/main.go` you can pass `-config pingpong.example.yaml` to load
these settings. The config is used to build the base logger via
`runtime.NewLoggerFromConfig(cfg.Logging)` and to choose the self/peer hosts.
If you also provide `-port` / `-peer-port` flags, those ports override the
values from the config file.

## Errors we could detect and log:

- [ ] Net layer related errors.
- [ ] Protocol not registered.
- [ ] Message serializer not registered.
- [ ] Trying to start runtime without network protocol.

## Timers

The runtime exposes simple timer helpers that deliver `Timer` objects back
into the protocol event loops:

- `SetupTimer(timer, duration)` – fires once after `duration`.
- `SetupPeriodicTimer(timer, duration)` – fires repeatedly every `duration`.
- `CancelTimer(timerID)` – stops a previously scheduled timer.

Timers are identified by `TimerID()`; this ID is used as a key inside the
runtime's `ongoingTimers` map and **must be unique for the lifetime of each
logical timer**. Reusing the same `TimerID` for overlapping timers will cause
newer timers to overwrite older ones.

## Serialized Format of Messages

The serialized format of messages sent in the system is going to be:  
```
LayerID || ProtocolID || MessageID || Contents  

LayerID -> Distinguish between application level messages or Session level messages.
    - ID: 1 - Application Level:    User defined messages.
    - ID: 2 - Session Level:        Messages used to establish session, 
                                    i.e. the ones used for the handshake protocol.
                                    
ProtocolID ->   Used to distribute messages to the correct ProtoProtocol,
                which should be registered in the Runtime.

MessageID ->    Used to distribute messages to the correct MessageHandler of the
                Protocol. Which should be registered in the ProtoProtocol.
```