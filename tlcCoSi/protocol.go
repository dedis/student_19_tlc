// Package tlccosi tlc using CoSi.
package tlccosi

import (
	"errors"
	"fmt"
	"sort"

	"go.dedis.ch/kyber/pairing"
	"go.dedis.ch/kyber/sign"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"

	"go.dedis.ch/kyber/sign/bdn"
)

// Name can be used to reference the registered protocol.
var Name = "TLCCoSi"

var defaultValidationFunction ValidationFunction

func init() {
	network.RegisterMessage(Initialize{})
	network.RegisterMessage(MessageBroadcast{})
	network.RegisterMessage(MessageCertified{})
	network.RegisterMessage(MessageAck{})
	onet.GlobalProtocolRegister(Name, NewProtocol)

	defaultValidationFunction = func(msg *MessageBroadcast, sender onet.TreeNodeID) error { return nil }
}

// TLC is the main structure holding the onet.Node.
type TLC struct {
	*onet.TreeNodeInstance // The node that represents us

	VF ValidationFunction // Validates message payload. Must be provided by service.

	// Internal listening channels (communication with service):
	Message      chan []byte                                // the message we want to broadcast in the next round
	ThresholdSet chan map[onet.TreeNodeID]*MessageDelivered // the messages to be delivered in this round.

	// External listening channels:
	init              chan chanInitialize       // The channel waiting for the initialization message
	roundMessages     chan chanRoundMessage     // The channel waiting for this round's messages
	acks              chan chanAckMessage       // The channel waiting for this round's acks
	certifiedMessages chan chanCertifiedMessage // The channel waiting for this round's certified messages

	// Internal routine channels:
	die          chan bool // Currently for testing purposes (to terminate). !!Unsafe for deployment!!
	canBroadcast chan bool // If this round's message is yet to be sent.

	TAcks uint64                 // acks per message threshold
	TMsgs uint64                 // acked messages threshold
	mrc   *MultiRoundCounterCoSi // Implements all the threshold counting logic

	sigCounter *signatureCounter
	suite      *pairing.SuiteBn256
}

// NewProtocol returns a TLC protocol instance with the correct channels.
func NewProtocol(node *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
	var err error

	nbrNodes := len(node.Tree().List())
	TAsyncNotBFT := uint64(nbrNodes/2 + 1)

	tlc := &TLC{
		TreeNodeInstance: node,
		VF:               defaultValidationFunction,
		Message:          make(chan []byte, 20),
		ThresholdSet:     make(chan map[onet.TreeNodeID]*MessageDelivered, 20),
		die:              make(chan bool, 1),
		canBroadcast:     make(chan bool, 1),
		TAcks:            TAsyncNotBFT,
		TMsgs:            TAsyncNotBFT,
		suite:            pairing.NewSuiteBn256(),
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
	if err := node.RegisterChannelLength(&tlc.certifiedMessages, 10000); err != nil {
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
	/*
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
				return
			}
		}()
	*/

	if !tlc.IsRoot() {
		log.Lvl2(tlc.Name(), "Waiting for initialize")
		init := (<-tlc.init).Initialize
		err := tlc.handleInitialize(&init)
		if err != nil {
			return err
		}
	}

	tlc.mrc = NewMultiRoundCounterCoSi(tlc.TMsgs, tlc.TAcks, tlc.VF)

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
			log.Lvlf4("%s Message from service: %s", tlc.Name(), broadcastMsg)
			tlc.handleBroadcast(broadcastMsg)
			tlc.tryEndRound()
		case received := <-tlc.roundMessages:
			log.Lvlf4("%s Received message. Server: %s; Message: %s", tlc.Name(), received.TreeNode.ID, received.Message)
			tlc.handleRoundMessage(&received)
			tlc.tryEndRound()
		case ack := <-tlc.acks:
			log.Lvlf4("%s Received ack. Server: %s; Hash: %x", tlc.Name(), ack.TreeNode.ID, ack.Hash)
			tlc.handleAck(&ack)
			tlc.tryEndRound()
		case certifiedMsg := <-tlc.certifiedMessages:
			log.Lvlf4("%s Received certified message. Server: %s; Hash: %x", tlc.Name(), certifiedMsg.TreeNode.ID, certifiedMsg.Hash())
			tlc.handleCertifiedMessage(&certifiedMsg)
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
	log.Lvlf2("%s Received initialization message. TMsgs: %v; TAcks: %v", tlc.Name(), in.TMsgs, in.TAcks)
	tlc.TAcks = in.TAcks
	tlc.TMsgs = in.TMsgs

	return nil
}

// handleBroadcast broadcasts the round message to other nodes.
// As we don't broadcast to ourselves, it simulates this behaviour for the MultiRoundCounter.
func (tlc *TLC) handleBroadcast(broadcastMsg []byte) {
	err := tlc.ReadyForNextRound() // handle error?
	if tlc.mrc.currentRound != 0 && err != nil {
		panic("This shouldn't be possible")
	}

	msg, err := tlc.mrc.RoundBroadcast(broadcastMsg)
	if err != nil {
		panic("Should never happen: double broadcast")
	}

	ourID := tlc.TreeNodeInstance.TreeNode().ID
	tlc.sigCounter = newSignatureCounter(msg.Hash(ourID), msg)

	roundMsg := &chanRoundMessage{tlc.TreeNodeInstance.TreeNode(), *msg}
	tlc.mrc.AddMessageBroadcast(msg, ourID)

	sig, err := tlc.signMsg(roundMsg)
	if err != nil {
		log.Lvlf2("Could not sign round message. %v", err)
		return
	}

	ourAck := &MessageAck{roundMsg.Round, roundMsg.Hash(), sig}
	tlc.handleAck(&chanAckMessage{tlc.TreeNodeInstance.TreeNode(), *ourAck})

	tlc.Broadcast(msg)
	log.Lvlf3("%v Broadcasted message for round %v", tlc.Name(), tlc.mrc.currentRound)

	return
}

// handleBroadcast collects other nodes' normal round messages.
// As nodes don't acknowledge their own messages, it simulates this behaviour for the MultiRoundCounter.
func (tlc *TLC) handleRoundMessage(roundMsg *chanRoundMessage) {
	shouldAck, err := tlc.mrc.AddMessageBroadcast(&roundMsg.MessageBroadcast, roundMsg.TreeNode.ID)
	if err != nil {
		log.Lvlf2("%v", err)
		return
	}

	if shouldAck && tlc.TAcks > 0 {
		sig, err := tlc.signMsg(roundMsg)
		if err != nil {
			log.Lvlf2("Could not sign round message. %v", err)
			return
		}

		ourAck := &MessageAck{roundMsg.Round, roundMsg.Hash(), sig}
		tlc.SendTo(roundMsg.TreeNode, ourAck)
	}
	return
}

// HandleAck collects message acknowledgements.
// When there are TAcks signatures it aggregates them and broadcasts a certified message
func (tlc *TLC) handleAck(ack *chanAckMessage) {
	err := tlc.addSignature(ack)
	if err != nil {
		log.Lvlf2("addSignature: %s", err)
		return
	}

	if tlc.countSignatures() == int(tlc.TAcks) {
		certMsg, err := tlc.prepareCertifiedMessage()
		if err != nil {
			log.Lvl2(err)
			return
		}

		err = tlc.mrc.AddMessageCertified(certMsg, tlc.TreeNode().ID)
		if err != nil {
			log.Lvlf4("%s", err)
			return
		}

		tlc.Broadcast(certMsg)
	}

	return
}

// handleCertifiedMessage collects other nodes' certified messages
func (tlc *TLC) handleCertifiedMessage(certMsg *chanCertifiedMessage) {
	err := tlc.verifyThresholdCertified(&certMsg.MessageCertified)
	if err != nil {
		log.Lvlf2("Verify threshold: %s", err)
		return
	}

	err = tlc.mrc.AddMessageCertified(&certMsg.MessageCertified, certMsg.TreeNode.ID)
	if err != nil {
		log.Lvlf4("%s", err)
		return
	}

	return
}

// tryEndRound checks if the round advancement conditions are met advancing the round accordingly.
// If successful, it places that round's batch of delivered messages in the ThresholdSet channel
// and re-enables broadcasting.
func (tlc *TLC) tryEndRound() {
	batch, err := tlc.mrc.TryEndRound()
	if err == nil {
		tlc.ThresholdSet <- batch
		tlc.canBroadcast <- true
		log.Lvlf3("%v Round advanced. Now at round %d", tlc.Name(), tlc.mrc.currentRound)
	} else {
		log.Lvlf5("Could not advance round: %s", err)
	}
	return
}

// ReadyForNextRound signals the protocol that the validation function has been updated and it can start
// checking messages of the next round. It gets called automatically when a message to broadcast is
// given, but can be called in advance e.g. when the message isn't ready but you want to start
// acknowledging other node's messages in advance (speeds up the systems's overall performance)
func (tlc *TLC) ReadyForNextRound() error {
	missingAckBroadcast, err := tlc.mrc.AdvanceRound()
	if err != nil {
		return err
	}
	if tlc.TAcks > 0 {
		for sender, msg := range missingAckBroadcast {
			roundMsg := &chanRoundMessage{tlc.Tree().Search(sender), *msg}

			sig, err := tlc.signMsg(roundMsg)
			if err != nil {
				log.Lvlf2("Could not sign round message. %v", err)
				continue
			}

			ourAck := &MessageAck{roundMsg.Round, roundMsg.Hash(), sig}
			tlc.SendTo(roundMsg.TreeNode, ourAck)
		}
	}
	return nil
}

// Signing, counting, and verifying signatures (helper functions)

func (tlc *TLC) signMsg(msgRound *chanRoundMessage) ([]byte, error) {
	log.Lvlf5("%v, Signing message from: %v. Hash: %v", tlc.Name(), msgRound.TreeNode.ID, msgRound.Hash())
	hash := msgRound.Hash()
	return bdn.Sign(tlc.suite, tlc.Private(), hash[:])
}

// Checks that a certified message's signature is correct (and meets the threshold)
func (tlc *TLC) verifyThresholdCertified(certMsg *MessageCertified) error {
	mask, err := sign.NewMask(tlc.suite, tlc.Publics(), nil)
	if err != nil {
		return err
	}

	err = mask.Merge(certMsg.Mask)
	if err != nil {
		return err
	}

	if mask.CountEnabled() < int(tlc.TAcks) {
		return fmt.Errorf("Certificate doesn't meet threshold number of acknowlegdments. Expected %d. Have: %d", tlc.TAcks, mask.CountEnabled())
	}

	aggPubKey, err := bdn.AggregatePublicKeys(tlc.suite, mask)
	if err != nil {
		return err
	}

	hash := certMsg.Hash()
	aggSignature := certMsg.Signature

	err = bdn.Verify(tlc.suite, aggPubKey, hash[:], aggSignature)

	return err
}

type signatureCounter struct {
	messageHash [32]byte
	message     *MessageBroadcast
	signatures  map[int][]byte // the int
}

func newSignatureCounter(hash [32]byte, msg *MessageBroadcast) *signatureCounter {
	return &signatureCounter{messageHash: hash, message: msg, signatures: make(map[int][]byte)}
}

// Checks that an acknowledgement's signature is correct and adds it to the counter
func (tlc *TLC) addSignature(ack *chanAckMessage) error {
	if ack.Hash != tlc.sigCounter.messageHash {
		return errors.New("Ack hash mismatch (ack is likely outdated)")
	}
	err := bdn.Verify(tlc.suite, ack.TreeNode.ServerIdentity.Public, tlc.sigCounter.messageHash[:], ack.Signature)
	if err != nil {
		return err
	}

	idx, _ := tlc.Roster().Search(ack.TreeNode.ServerIdentity.ID)
	tlc.sigCounter.signatures[idx] = ack.Signature
	return nil
}

// Counts the current number of signatures for this round's message
func (tlc *TLC) countSignatures() int {
	return len(tlc.sigCounter.signatures)
}

// Prepares this round's certified message (appends certificate), if possible.
func (tlc *TLC) prepareCertifiedMessage() (*MessageCertified, error) {
	if tlc.countSignatures() != int(tlc.TAcks) {
		return nil, fmt.Errorf("Insufficient signatures for certification. Expected: %d. Have: %d", tlc.TAcks, tlc.countSignatures())
	}

	var sigs [][]byte
	aggMask, err := sign.NewMask(tlc.suite, tlc.Publics(), nil)
	if err != nil {
		return nil, err
	}

	var keys []int
	for k := range tlc.sigCounter.signatures {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		sig := tlc.sigCounter.signatures[k]
		sigs = append(sigs, sig)
		err := aggMask.SetBit(k, true)
		if err != nil {
			return nil, err
		}
	}

	aggSig, err := bdn.AggregateSignatures(tlc.suite, sigs, aggMask)
	if err != nil {
		return nil, err
	}

	aggSigB, err := aggSig.MarshalBinary()
	if err != nil {
		return nil, err
	}

	msg := tlc.sigCounter.message
	ourID := tlc.TreeNode().ID
	cert := &Certification{aggSigB, aggMask.Mask()}

	return &MessageCertified{MessageBroadcast: msg, Certification: cert, ID: ourID}, nil
}
