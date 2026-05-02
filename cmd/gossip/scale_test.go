package main

import (
	"os"
	"sync"
	"testing"
	"time"
)

// runScaleBroadcast spins up `total` nodes on a fresh port range,
// waits for each to see at least viewMin neighbors, broadcasts from
// node 0, and asserts every node delivers it exactly once.
//
// Convergence and delivery times are reported via t.Logf so the test
// doubles as a "how does it scale" probe.
func runScaleBroadcast(t *testing.T, total, viewMin int, convergeT, deliverT time.Duration) {
	t.Helper()
	if testing.Short() {
		t.Skipf("skipping scale=%d in -short mode", total)
	}

	base := reservePorts(total)
	nodes := make([]*node, total)
	for i := range total {
		nodes[i] = buildNodeOf(t, base, i, total)
	}
	for _, n := range nodes {
		go func() { _ = n.rt.Run() }()
	}
	t.Cleanup(func() {
		// Parallelise Cancel: each runtime's shutdown is independent
		// (own listener, own goroutines), and the TCP layer has a 1s
		// read-deadline before noticing ctx, so serial cancellation
		// at large N would take minutes of wallclock.
		var wg sync.WaitGroup
		for _, n := range nodes {
			wg.Go(n.rt.Cancel)
		}
		wg.Wait()
	})

	convergeStart := time.Now()
	deadline := convergeStart.Add(convergeT)
	for time.Now().Before(deadline) {
		if everyNodeHasView(nodes, viewMin) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !everyNodeHasView(nodes, viewMin) {
		t.Fatalf("scale=%d did not converge to viewMin=%d in %v: %s",
			total, viewMin, convergeT, viewReport(nodes))
	}
	t.Logf("scale=%d converged in %v", total, time.Since(convergeStart))

	bcastStart := time.Now()
	nodes[0].trigger.Trigger([]byte("hello"))

	bcastDeadline := bcastStart.Add(deliverT)
	for time.Now().Before(bcastDeadline) {
		if allDeliveredOnce(nodes, []string{"hello"}) {
			t.Logf("scale=%d broadcast reached all %d nodes in %v",
				total, total, time.Since(bcastStart))
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	missing := nodesMissing(nodes, "hello", 1)
	t.Fatalf("scale=%d %d/%d nodes missing 'hello' after %v",
		total, len(missing), total, deliverT)
}

// TestGossip_100Nodes_Scale spins up 100 nodes, broadcasts once, and
// confirms every node delivers it exactly once.
func TestGossip_100Nodes_Scale(t *testing.T) {
	runScaleBroadcast(t, 100, 2, 15*time.Second, 5*time.Second)
}

// TestGossip_1000Nodes_Scale exercises a 1000-node cluster.
func TestGossip_1000Nodes_Scale(t *testing.T) {
	runScaleBroadcast(t, 1000, 2, 60*time.Second, 30*time.Second)
}

// TestGossip_10000Nodes_Scale is an exploratory probe rather than a
// regression test. At 10k single-process nodes (~30k TCP connections,
// ~100k goroutines) the connect storm reliably overflows macOS's
// default kern.ipc.somaxconn=128 and Go's network poller starts to
// show meaningful overhead; ~95% of nodes converge in 2 minutes but
// the long tail does not. Skipped by default; opt in with
// `GOSSIP_SCALE_10K=1 go test -run TestGossip_10000Nodes_Scale ./cmd/gossip/`.
func TestGossip_10000Nodes_Scale(t *testing.T) {
	if os.Getenv("GOSSIP_SCALE_10K") == "" {
		t.Skip("set GOSSIP_SCALE_10K=1 to run the 10k-node probe")
	}
	runScaleBroadcast(t, 10000, 2, 120*time.Second, 60*time.Second)
}
