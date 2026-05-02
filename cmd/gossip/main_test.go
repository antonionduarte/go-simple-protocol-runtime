package main

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain wires goleak into the gossip example's test suite so any
// leaked goroutine after a test's Cleanup callbacks fails the package,
// the same standard the framework's own tests are held to.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
