# TLC Protocol

Currently implements the simple version of the TLC protocol.

# Usage

Each node/service that uses the protocol is responsible for braodcasting one message per round,
including empty messages:

```go
message := []byte(fmt.Sprintf("This round's message for this node"))
pi <- message	// var pi *TLC
```

And can collect that round's set of messages:

```go
set := <- pi.ThresholdSet // var set map[onet.TreeNodeID]*MessageDelivered

for node, msg := range set {
	if msg.TDelivered {
		println("The round %v message: %v, sent by node %v,
		was delivered by a threshold of nodes!", msg.Round, msg.Message, node)
	}
}
```

To terminate the local protocol instance "cleanly" (currently for testing purposes only).
```go
pi.Terminate() // Done for each instance. Can be used to simulate crashes. 
```


