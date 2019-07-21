# TLC Protocol

Current implementations:
- "simple" version: no CoSi, acks are broadcast, blocking, O(N^3) messages
- "CoSi version": each node aggregates own acks, blocking, O(N^2) messages

*Important Considerations*: these implementations currently rely on _eventual message delivery_ from the underlying communication links for their asynchronicity guarantees (e.g. tcp timeouts might pose a problem)

## Usage

Each node/service that uses the protocol is responsible for broadcasting one message per round,
including empty messages:

```go
msg := []byte(fmt.Sprintf("This round's message for this node"))
pi.Message <- msg	// var pi *TLC
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

Whenever a round finishes (i.e. when ```<- pi.ThresholdSet``` returns), the protocol _waits for the validation function to be updated_ before it starts processing the next round's messages. You can signal that the function has been updated by either providing the next round's message (```pi.Message <- msg```) or through the ```tlc.ReadyForNextRound()``` method (useful in case you want to start processing other nodes' messages before your message is ready)

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

## Simulation(s)

You can define the behaviour of _remote_ nodes a priori by modifying the [Dispatch](simulation/TLCWrapper.go) method of ```TLCWrapper```.

The root node's behaviour is defined in the [Run](simulation/protocol.go) method.

Run a simulation with:
```
cd simulation/
go build
./simulation protocol.toml
```

Contact me ([mvidigueira](https://github.com/mvidigueira)) if you have any problems.
