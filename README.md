# Simple Protocol Runtime 
Simple protocol runtime in Go. (heavily inspired by [Babel](https://github.com/pfouto/babel-core) (java framework to implement distributed protocols).)

## Extremely WIP please don't mock my code.
## TODO:

- [X] Implement basic structure.
- [ ] Implement network layer, for TCP initially. TODO: Had an error, I probably need to do a handshake.
- [ ] Implement inter-protocol communication, so that we can send messages between protocols within same Runtime.
- [ ] Implement a simple protocol, which can send and receive messages to test this.
- [X] Implement timer functionality.
- [ ] Implement configuration file parser.
- [X] Add contexts **everywhere** in order to gracefully finish the runtime and it's experiments.
- [X] Decide how I actually want to manage the self Host.
- [ ] High Priority - Might break everything - Rethink if I save the Host in the Message Structs.
- [ ] Decide correctly which static functions should be in which file, possibly ask GPT-4 about it his answer will be correct. 
- [ ] Should the cancelation of the runtime be called at the runtime level or?
- [ ] Should the network layer be generic by itself and abstract stuff like cancelation?
- [ ] Some mechanism for the main thread to actually finish execution.
- [ ] Propper channel management, I believe i'm not actually properly closing channels yet.

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

## Errors we could detect and log:

- [ ] Net layer related errors.
- [ ] Protocol not registered.
- [ ] Message serializer not registered.
- [ ] Trying to start runtime without network protocol.

## Timers

- SetupTimer()
- SetupPeriodicTimer()
- CancelTimer()
