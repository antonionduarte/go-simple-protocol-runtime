# go-simple-protocol-runtime
Simple protocol runtime in Go.

## TODO:

- [x] Implement basic structure.
- [ ] Implement network layer, for both TCP and UDP.
- [ ] Implement inter-protocol communication, so that we can send messages between protocols within same Runtime.
- [ ] Implement a simple protocol, which can send and receive messages to test this.
- [ ] Implement timer functionality.
- [ ] Implement configuration file parser.

## Net layer:

- [ ] Host struct with serializer and deserializer.

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