# Simple Protocol Runtime 
Simple protocol runtime in Go. (heavily inspired by [Babel](https://github.com/pfouto/babel-core) (java framework to implement distributed protocols).)

## Extremely WIP please don't mock my code.
## TODO:

- [X] Implement basic structure.
- [X] Implement network layer, for TCP initially, if anyone wants UDP or QUIC they can add it themselves.
- [ ] Implement inter-protocol communication, so that we can send messages between protocols within same Runtime.
- [ ] Implement a simple protocol, which can send and receive messages to test this.
- [X] Implement timer functionality.
- [ ] Implement configuration file parser.
- [X] Add contexts **everywhere** in order to gracefully finish the runtime and it's experiments.
- [ ] Decide how I actually want to manage the self Host.
- [ ] Protocols might (will) want to send messages to each other, as such I should probably add protocolID as an argument to the Send function.
- [ ] I want message not to include the host that I received the message from, new message interface should be:
- [ ] Decide correctly which static functions should be in which file, possibly ask GPT-4 about it his answer will be correct. 

```go
package main

type Message struct{}
type Host struct{}

type ProtoProtocol struct {
	msgHandlers map[int]func(msg Message, from Host)
}

func SendMessage(msg Message, sendTo Host) {}
```
- [ ] Consider copying Hosts around as values instead of using references. 

## Net layer:

- [X] Host struct with serializer and deserializer.
- [X] Add the Sender Host to all the messages.

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

## Timers

- SetupTimer()
- SetupPeriodicTimer()
- CancelTimer()
