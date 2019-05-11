# TLC Protocol

Currently implements the simple version of the TLC protocol
(no CoSi, ack broadcast per message, blocking)

# Usage

Each node/service that uses the protocol is responsible for broadcasting one message per round,
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

All of the messages returned in the round are of that round.
Buffering of messages from future rounds (nodes further ahead) is done automatically.


To terminate the local protocol instance "cleanly" (currently for testing purposes only).
```go
pi.Terminate() // Should be done for each instance. Can be used to simulate crashes. 
```

For quick reference:
```
type MessageDelivered struct {
	Round   uint64
	Message []byte
	TDelivered bool // true if the message was delivered by TMsgs nodes ("certified")
}
```

# Simulation (WIP)
In the process of implementing it cleanly...
