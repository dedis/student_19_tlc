// Package tlc implements a round of a Collective Signing protocol.
package tlc

import (
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

// Name can be used to reference the registered protocol.
var Name = "TLC"

var defaultValidationFunction ValidationFunction

func init() {
	network.RegisterMessage(Initialize{})
	network.RegisterMessage(MessageBroadcast{})
	network.RegisterMessage(MessageAck{})
	onet.GlobalProtocolRegister(Name, NewProtocol)

	defaultValidationFunction = func(msg *MessageBroadcast, sender onet.TreeNodeID) error { return nil }
}

// TLC is the main structure holding the onet.Node.
type TLC struct {
	*onet.TreeNodeInstance // The node that represents us

	VF ValidationFunction // Validates message payload. Provided by service.

	// Internal listening channels (communication with service):
	Message      chan []byte                                // the message we want to broadcast in the next round
	ThresholdSet chan map[onet.TreeNodeID]*MessageDelivered // the messages to be delivered in this round.

	// External listening channels:
	init          chan chanInitialize   // The channel waiting for the initialization message
	roundMessages chan chanRoundMessage // The channel waiting for this round's messages
	acks          chan chanAckMessage   // The channel waiting for this round's acks

	// Internal routine channels:
	die          chan bool // Currently for testing purposes (to terminate). !!Unsafe for deployment!!
	canBroadcast chan bool // If this round's message is yet to be sent.

	TAcks uint64             // acks per message threshold
	TMsgs uint64             // acked messages threshold
	mrc   *MultiRoundCounter // Implements all the threshold counting logic
}

// NewProtocol returns a TLC protocol instance with with the correct channels.
func NewProtocol(node *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
	var err error

	nbrNodes := len(node.Tree().List())
	TAsyncNotBFT := uint64(nbrNodes/2 + 1)

	tlc := &TLC{
		TreeNodeInstance: node,
		VF:               defaultValidationFunction,
		Message:          make(chan []byte, 5),
		ThresholdSet:     make(chan map[onet.TreeNodeID]*MessageDelivered, 5),
		die:              make(chan bool, 1),
		canBroadcast:     make(chan bool, 1),
		TAcks:            TAsyncNotBFT,
		TMsgs:            TAsyncNotBFT,
	}

	if err := node.RegisterChannelLength(&tlc.init, 10000); err != nil {
		return tlc, err
	}
	if err := node.RegisterChannelLength(&tlc.roundMessages, 10000); err != nil {
		return tlc, err
	}
	if err := node.RegisterChannelLength(&tlc.acks, 10000); err != nil {
		return tlc, err
	}

	return tlc, err
}

// Start will broadcast the Initialization message to all servers (bootstrap).
func (tlc *TLC) Start() error {
	out := &Initialize{TMsgs: tlc.TMsgs, TAcks: tlc.TAcks}

	log.Lvl2(tlc.Name(), "Broadcasting initialization message")
	tlc.Broadcast(out)

	return nil
}

// Dispatch will initialize the protocol in each non-root server, and then listen
// on the three channels we use: for listening (external), broadcasting and termination (internal).
func (tlc *TLC) Dispatch() error {
	defer tlc.Done()

	if !tlc.IsRoot() {
		log.Lvl2(tlc.Name(), "Waiting for initialize")
		init := (<-tlc.init).Initialize
		err := tlc.handleInitialize(&init)
		if err != nil {
			return err
		}
	}

	tlc.mrc = NewMultiRoundCounter(tlc.TMsgs, tlc.TAcks)

	// So that we can start to acknowledge other nodes' messages for this round
	// without having to wait for our own broadcast.
	done := make(chan bool)
	msgToBroadcast := make(chan []byte)
	go func() {
		for {
			select {
			case msg := <-tlc.Message:
				select {
				case <-tlc.canBroadcast:
					msgToBroadcast <- msg
				case <-tlc.die:
					done <- true
					return
				}
			case <-tlc.die:
				done <- true
				return
			}
		}
	}()

	tlc.canBroadcast <- true

	for {
		select {
		case broadcastMsg := <-msgToBroadcast:
			tlc.handleBroadcast(broadcastMsg)
			log.Lvlf2("%s Message from service: %s", tlc.Name(), broadcastMsg)
		case received := <-tlc.roundMessages:
			tlc.handleRoundMessage(&received)
			tlc.tryEndRound()
		case ack := <-tlc.acks:
			tlc.handleAck(&ack)
			tlc.tryEndRound()
		case <-done:
			return nil
		}
	}
}

// SetValidationFunction sets the protocol's validation function. Must be set before protocol start.
func (tlc *TLC) SetValidationFunction(vf ValidationFunction) {
	tlc.VF = vf
}

// Terminate terminates the protocol.
// * used for testing purposes, should not be used in deployment *
func (tlc *TLC) Terminate() {
	log.Lvl2(tlc.Name(), "Shutting down")
	tlc.die <- true
}

// handleInitialize initializes the local protocol to the specs defined in the message.
func (tlc *TLC) handleInitialize(in *Initialize) error {
	log.Lvlf3("%s Received initialization message. TMsgs: %v; TAcks: %v", tlc.Name(), in.TMsgs, in.TAcks)
	tlc.TAcks = in.TAcks
	tlc.TMsgs = in.TMsgs

	return nil
}

// handleBroadcast broadcasts the round message to other nodes.
// As we don't broadcast to ourselves, it simulates this behaviour for the MultiRoundCounter.
func (tlc *TLC) handleBroadcast(broadcastMsg []byte) {
	msg, err := tlc.mrc.RoundBroadcast(broadcastMsg)
	if err != nil {
		panic("Should never happen: double broadcast")
	}

	tlc.Broadcast(msg)
	log.Lvlf3("%v Broadcasted message for round %v", tlc.Name(), tlc.mrc.currentRound)

	ourID := tlc.TreeNodeInstance.TreeNode().ID
	tlc.mrc.AddMessage(msg, ourID)
	if tlc.TAcks > 0 {
		ourAck := &MessageAck{msg.Round, msg.Hash(ourID)}
		tlc.mrc.AddAck(ourAck, ourID)
	}

	return
}

// handleBroadcast collects other nodes' normal round messages.
// As nodes don't acknowledge their own messages, it simulates this behaviour for the MultiRoundCounter.
func (tlc *TLC) handleRoundMessage(roundMsg *chanRoundMessage) {
	log.Lvlf3("%s Received message. Server: %s; Message: %s", tlc.Name(), roundMsg.TreeNode.ID, roundMsg.Message)

	tlc.mrc.AddMessage(&roundMsg.MessageBroadcast, roundMsg.TreeNode.ID)
	theirAck := &MessageAck{roundMsg.Round, roundMsg.Hash()}
	tlc.mrc.AddAck(theirAck, roundMsg.TreeNode.ID)
	if tlc.TAcks > 0 {
		ourID := tlc.TreeNodeInstance.TreeNode().ID
		ourAck := &MessageAck{roundMsg.Round, roundMsg.Hash()}
		tlc.mrc.AddAck(ourAck, ourID)
		tlc.Broadcast(ourAck)
	}
	return
}

// handleAck collects other nodes' message acknowledgements
func (tlc *TLC) handleAck(ack *chanAckMessage) {
	log.Lvlf3("%s Received ack. Server: %s; Hash: %x", tlc.Name(), ack.TreeNode.ID, ack.Hash)

	tlc.mrc.AddAck(&ack.MessageAck, ack.TreeNode.ID)
	return
}

// tryEndRound checks if the round advancement conditions are met advancing the round accordingly.
// If successful, it places that round's batch of delivered messages in the ThresholdSet channel
// and re-enables broadcasting.
func (tlc *TLC) tryEndRound() {
	batch, err := tlc.mrc.TryAdvanceRound()
	if err == nil {
		tlc.ThresholdSet <- batch
		tlc.canBroadcast <- true
		log.Lvlf4("Round advanced. Now at round %d", tlc.mrc.currentRound)
	} else {
		log.Lvlf4("Could not advance round: %s", err)
	}
	return
}
