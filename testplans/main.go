package main

import (
	"github.com/testground/sdk-go/run"
)

func main() {
	run.InvokeMap(map[string]interface{}{
		"peer-discovery":     runPeerDiscovery,
		"direct-messaging":   runDirectMessaging,
		"broadcast-test":     runBroadcastTest,
		"dht-performance":    runDHTPerformance,
		"connection-stress":  runConnectionStress,
		"network-conditions": runNetworkConditions,
		"nat-traversal":      runNATTraversal,
		"scalability":        runScalability,
	})
}
