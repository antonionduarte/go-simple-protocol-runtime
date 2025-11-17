package main

import (
	"fmt"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func main() {
	host := net.Host{Port: 8080, IP: "192.168.0.1"}
	serialized := net.SerializeHost(host)
	deserialized := net.DeserializeHost(serialized)
	// fmt.Printf(host.IP)
	// fmt.Printf(deserialized.IP + ":")
	// fmt.Printf(strconv.Itoa(deserialized.Port))
	fmt.Print(deserialized.ToString())
}
