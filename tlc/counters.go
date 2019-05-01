package tlc

import (
	"errors"
	"fmt"

	"go.dedis.ch/onet/v3"
)

type ackMap map[onet.TreeNodeID]bool

// singleRoundCounter (threshold-ack counter) keeps track of threshold conditions for a specific round.
// Only messages and acks for that round should be added.
type singleRoundCounter struct {
	batch            map[onet.TreeNodeID]*MessageDelivered
	messages         map[msghash]*MessageDelivered
	ackPerMsg        map[msghash]ackMap
	numTAcked        uint64
	thresholdReached bool
	tAcks            uint64
	tMsgs            uint64
	hasBroadcast     bool
}

func newSingleRoundCounter(tMsgs, tAcks uint64) *singleRoundCounter {
	return &singleRoundCounter{
		batch:            make(map[onet.TreeNodeID]*MessageDelivered),
		messages:         make(map[msghash]*MessageDelivered),
		ackPerMsg:        make(map[msghash]ackMap),
		numTAcked:        0,
		thresholdReached: false,
		tAcks:            tAcks,
		tMsgs:            tMsgs,
		hasBroadcast:     false,
	}
}

func (tac *singleRoundCounter) reset() (oldbatch map[onet.TreeNodeID]*MessageDelivered) {
	oldbatch = tac.batch
	*tac = *newSingleRoundCounter(tac.tMsgs, tac.tAcks)
	return
}

func (tac *singleRoundCounter) addAck(hash msghash, sender onet.TreeNodeID) error {
	if _, ok := tac.ackPerMsg[hash]; !ok {
		tac.ackPerMsg[hash] = make(ackMap)
	}

	if _, ok := tac.ackPerMsg[hash][sender]; ok {
		return errors.New("duplicate acknowledgement")
	}

	tac.ackPerMsg[hash][sender] = true

	// if the message exists, check ack threshold condition
	if _, ok := tac.messages[hash]; ok && tac.messages[hash].TDelivered == false {
		if uint64(len(tac.ackPerMsg[hash])) >= tac.tAcks {
			tac.messages[hash].TDelivered = true
			tac.numTAcked++
			if tac.numTAcked == tac.tMsgs {
				tac.thresholdReached = true
			}
		}
	}

	return nil
}

func (tac *singleRoundCounter) addMessage(msg *MessageBroadcast, sender onet.TreeNodeID) error {
	if _, ok := tac.batch[sender]; ok {
		return errors.New("duplicate message")
	}
	rmd := &MessageDelivered{MessageBroadcast: msg, TDelivered: false}
	tac.batch[sender] = rmd
	hash := msg.Hash(sender)
	tac.messages[hash] = rmd

	// if there were acks received before the message, check ack threshold condition
	if v, ok := tac.ackPerMsg[hash]; ok {
		if uint64(len(v)) >= tac.tAcks {
			tac.messages[hash].TDelivered = true
			tac.numTAcked++
			if tac.numTAcked == tac.tMsgs {
				tac.thresholdReached = true
			}
		}
	}

	return nil
}

type oneRoundMessages map[onet.TreeNodeID]*MessageBroadcast
type oneRoundAcks map[onet.TreeNodeID]map[msghash]bool

// MultiRoundCounter implements the basic tlc logic
type MultiRoundCounter struct {
	*singleRoundCounter

	// buffering for messages from future rounds
	msgBuffer map[roundNum]oneRoundMessages
	ackBuffer map[roundNum]oneRoundAcks

	currentRound roundNum
}

// NewMultiRoundCounter creates a new MultiRoundCounter where tMsgs is the message threshold and
// tAcks is the acknowledgements per message threshold.
func NewMultiRoundCounter(tMsgs, tAcks uint64) *MultiRoundCounter {
	return &MultiRoundCounter{
		singleRoundCounter: newSingleRoundCounter(tMsgs, tAcks),
		msgBuffer:          make(map[roundNum]oneRoundMessages),
		ackBuffer:          make(map[roundNum]oneRoundAcks),
		currentRound:       0,
	}
}

// AddMessage applies the tlc logic to a received message
func (mrc *MultiRoundCounter) AddMessage(msg *MessageBroadcast, sender onet.TreeNodeID) error {
	if msg.Round > mrc.currentRound {
		if _, ok := mrc.msgBuffer[msg.Round]; !ok {
			mrc.msgBuffer[msg.Round] = make(oneRoundMessages)
		}
		if _, ok := mrc.msgBuffer[msg.Round][sender]; ok {
			return fmt.Errorf("Duplicate message. round: %v, sender: %s", msg.Round, sender)
		}
		mrc.msgBuffer[msg.Round][sender] = msg
		return nil
	}

	if msg.Round == mrc.currentRound {
		err := mrc.addMessage(msg, sender)
		if err != nil {
			return fmt.Errorf("%v. round: %v, sender: %s", err, msg.Round, sender)
		}
	}

	return nil
}

// AddAck applies the tlc logic to a received acknowledgement
func (mrc *MultiRoundCounter) AddAck(ack *MessageAck, sender onet.TreeNodeID) error {
	if ack.Round > mrc.currentRound {
		if _, ok := mrc.ackBuffer[ack.Round]; !ok {
			mrc.ackBuffer[ack.Round] = make(oneRoundAcks)
		}
		if _, ok := mrc.ackBuffer[ack.Round][sender]; !ok {
			mrc.ackBuffer[ack.Round][sender] = make(map[msghash]bool)
		}
		if _, ok := mrc.ackBuffer[ack.Round][sender][ack.Hash]; ok {
			return fmt.Errorf("Duplicate message. round: %v, sender: %s", ack.Round, sender)
		}

		mrc.ackBuffer[ack.Round][sender][ack.Hash] = true
		return nil
	}

	if ack.Round == mrc.currentRound {
		mrc.addAck(ack.Hash, sender)
		return nil
	}

	return nil
}

// RoundBroadcast prepares the message to be sent this round
func (mrc *MultiRoundCounter) RoundBroadcast(msg []byte) (*MessageBroadcast, error) {
	if mrc.hasBroadcast {
		return nil, fmt.Errorf("Duplicate broadcast in round %d", mrc.currentRound)
	}

	mrc.hasBroadcast = true
	return &MessageBroadcast{Round: mrc.currentRound, Message: msg}, nil
}

// TryAdvanceRound terminates the round if the minimum conditions are gathered
// (broadcast this round's message and minimum thresholds are met).
// Returns a batch of the messages received during this round (and of this round),
// and a non-nil error if the minimum conditons for advancement are not met
func (mrc *MultiRoundCounter) TryAdvanceRound() (map[onet.TreeNodeID]*MessageDelivered, error) {
	if !mrc.hasBroadcast {
		return nil, fmt.Errorf("Broadcast missing for this round (%d)", mrc.currentRound)
	}

	if !mrc.thresholdReached {
		return nil, fmt.Errorf("Thresholds not reached yet. Missing %d out of %d T-Acked messages ",
			mrc.tMsgs-mrc.numTAcked, mrc.tMsgs)
	}

	oldbatch := mrc.singleRoundCounter.reset()
	delete(mrc.msgBuffer, mrc.currentRound)
	delete(mrc.ackBuffer, mrc.currentRound)

	mrc.currentRound++

	for k, v := range mrc.msgBuffer[mrc.currentRound] {
		mrc.addMessage(v, k)
	}

	for node, ora := range mrc.ackBuffer[mrc.currentRound] {
		for hash := range ora {
			mrc.addAck(hash, node)
		}
	}

	return oldbatch, nil
}
