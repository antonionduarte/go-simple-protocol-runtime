# go-simple-protocol-runtime
Simple protocol runtime in Go.

## TODO:

- [x] Implement basic structure.
- [X] Implement network layer, for TCP initially, if anyone wants UDP or QUIC they can add it themselves.
- [ ] Implement inter-protocol communication, so that we can send messages between protocols within same Runtime.
- [ ] Implement a simple protocol, which can send and receive messages to test this.
- [ ] Implement timer functionality.
- [ ] Implement configuration file parser.
- [ ] Add contexts **everywhere** in order to gracefully finish the runtime and it's experiments.

## Net layer:

- [X] Host struct with serializer and deserializer.
- [ ] Add the Sender Host to all the messages.

Types of Network / Channel Events:

```go
package main 

func handleOutConnectionDown() {}
func handleOutConnectionUp() {}

func handleInConnectionDown() {}
func handleInConnectionUp() {}

func handleOutConnectionFailed() {}
```

## Errors we could detect and log:

- [ ] Net layer related errors.
- [ ] Protocol not registered.
- [ ] Message serializer not registered.
- [ ] Trying to start runtime without network protocol.