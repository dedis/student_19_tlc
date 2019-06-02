package tlccosi

import (
	"errors"
	"fmt"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
)

// ValidationFunction validates the correctness of message payloads
type ValidationFunction func(msg *MessageBroadcast, sender onet.TreeNodeID) error

// singleRoundCounter (threshold-ack counter) keeps track of threshold conditions for a specific round.
// Only messages and acks for that round should be added.
type singleRoundCounterCoSi struct {
	batch            map[onet.TreeNodeID]*MessageDelivered
	messages         map[msghash]*MessageDelivered
	numTAcked        uint64
	thresholdReached bool
	tMsgs            uint64
	tAcks            uint64
	hasBroadcast     bool
}

func newSingleRoundCounterCoSi(tMsgs, tAcks uint64) *singleRoundCounterCoSi {
	return &singleRoundCounterCoSi{
		batch:            make(map[onet.TreeNodeID]*MessageDelivered),
		messages:         make(map[msghash]*MessageDelivered),
		numTAcked:        0,
		thresholdReached: false,
		tMsgs:            tMsgs,
		tAcks:            tAcks,
		hasBroadcast:     false,
	}
}

func (src *singleRoundCounterCoSi) reset() (oldbatch map[onet.TreeNodeID]*MessageDelivered) {
	oldbatch = src.batch
	*src = *newSingleRoundCounterCoSi(src.tMsgs, src.tAcks)
	return
}

func (src *singleRoundCounterCoSi) addMessageCertified(msg *MessageCertified) error {
	hash := msg.Hash()

	err := src.addMessageBroadcast(msg.MessageBroadcast, msg.ID)
	if err != nil && err.Error() == "already certified" {
		return err
	}

	if src.messages[hash].TDelivered != true {
		src.messages[hash].TDelivered = true
		src.messages[hash].Certification = msg.Certification
		src.numTAcked++
		if src.numTAcked == src.tMsgs {
			src.thresholdReached = true
		}
	} else {
		return errors.New("duplicate certified message")
	}

	return nil
}

func (src *singleRoundCounterCoSi) addMessageBroadcast(msg *MessageBroadcast, sender onet.TreeNodeID) error {
	if v, ok := src.batch[sender]; ok {
		if v.TDelivered == false {
			return errors.New("duplicate message")
		}
		return errors.New("already certified") // already certified
	}
	rmd := &MessageDelivered{MessageCertified: &MessageCertified{sender, msg, nil}, TDelivered: false}
	src.batch[sender] = rmd
	hash := msg.Hash(sender)
	src.messages[hash] = rmd

	return nil
}

type oneRoundMessages map[onet.TreeNodeID]*MessageBroadcast
type oneRoundCertified map[onet.TreeNodeID]*MessageCertified

// MultiRoundCounterCoSi implements the basic tlc logic
type MultiRoundCounterCoSi struct {
	*singleRoundCounterCoSi

	// buffering for messages from future rounds
	msgBroadBuffer map[roundNum]oneRoundMessages
	msgCertBuffer  map[roundNum]oneRoundCertified

	currentRound roundNum
	roundOver    bool

	VF ValidationFunction
}

// NewMultiRoundCounterCoSi creates a new MultiRoundCounterCoSi where tMsgs is the message threshold and
// tAcks is the acknowledgements per message threshold.
func NewMultiRoundCounterCoSi(tMsgs, tAcks uint64, vf ValidationFunction) *MultiRoundCounterCoSi {
	return &MultiRoundCounterCoSi{
		singleRoundCounterCoSi: newSingleRoundCounterCoSi(tMsgs, tAcks),
		msgBroadBuffer:         make(map[roundNum]oneRoundMessages),
		msgCertBuffer:          make(map[roundNum]oneRoundCertified),
		currentRound:           0,
		roundOver:              false,
		VF:                     vf,
	}
}

// AddMessageBroadcast applies the tlc logic to a received message
func (mrc *MultiRoundCounterCoSi) AddMessageBroadcast(msg *MessageBroadcast, sender onet.TreeNodeID) (shouldAck bool, err error) {
	if msg.Round > mrc.currentRound {
		if _, ok := mrc.msgBroadBuffer[msg.Round]; !ok {
			mrc.msgBroadBuffer[msg.Round] = make(oneRoundMessages)
		}
		if _, ok := mrc.msgBroadBuffer[msg.Round][sender]; ok {
			return false, fmt.Errorf("Duplicate message. round: %v, sender: %s", msg.Round, sender)
		}
		mrc.msgBroadBuffer[msg.Round][sender] = msg
		return false, nil
	}

	if msg.Round == mrc.currentRound {
		if mrc.roundOver {
			return false, errors.New("Cannot add message: round has already ended")
		}

		if err := mrc.VF(msg, sender); err != nil {
			return false, fmt.Errorf("%v", err)
		}

		err := mrc.addMessageBroadcast(msg, sender)
		if err != nil {
			return false, fmt.Errorf("%v. round: %v, sender: %s", err, msg.Round, sender)
		}
	}

	return true, nil
}

// AddMessageCertified applies the tlc logic to a received certified message
func (mrc *MultiRoundCounterCoSi) AddMessageCertified(msg *MessageCertified, sender onet.TreeNodeID) error {
	if msg.Round > mrc.currentRound {
		if _, ok := mrc.msgCertBuffer[msg.Round]; !ok {
			mrc.msgCertBuffer[msg.Round] = make(oneRoundCertified)
		}
		if _, ok := mrc.msgCertBuffer[msg.Round][msg.ID]; ok {
			return fmt.Errorf("Duplicate message. round: %v, sender: %s", msg.Round, msg.ID)
		}

		mrc.msgCertBuffer[msg.Round][msg.ID] = msg
		return nil
	}

	if msg.Round == mrc.currentRound {
		if mrc.roundOver {
			return errors.New("Cannot add certified message: round has already ended")
		}

		err := mrc.addMessageCertified(msg)
		if err != nil {
			return fmt.Errorf("%v. round: %v, sender: %s", err, msg.Round, msg.ID)
		}
	}

	return nil
}

// RoundBroadcast prepares the message to be sent this round
func (mrc *MultiRoundCounterCoSi) RoundBroadcast(msg []byte) (*MessageBroadcast, error) {
	if mrc.hasBroadcast {
		return nil, fmt.Errorf("Duplicate broadcast in round %d", mrc.currentRound)
	}

	mrc.hasBroadcast = true
	return &MessageBroadcast{Round: mrc.currentRound, Message: msg}, nil
}

// TryEndRound ends the round if the minimum conditions are gathered
// (has broadcast this round's message and minimum thresholds are met).
// Returns a batch of the messages received during this round (and of this round),
// and a non-nil error if the minimum conditions for advancement are not met
func (mrc *MultiRoundCounterCoSi) TryEndRound() (map[onet.TreeNodeID]*MessageDelivered, error) {
	if !mrc.hasBroadcast {
		return nil, fmt.Errorf("Broadcast missing for this round (%d)", mrc.currentRound)
	}

	if !mrc.thresholdReached {
		return nil, fmt.Errorf("Thresholds not reached yet. Missing %d out of %d T-Acked messages ",
			mrc.tMsgs-mrc.numTAcked, mrc.tMsgs)
	}

	oldbatch := mrc.singleRoundCounterCoSi.reset()
	delete(mrc.msgBroadBuffer, mrc.currentRound)
	delete(mrc.msgCertBuffer, mrc.currentRound)

	mrc.roundOver = true

	return oldbatch, nil
}

// AdvanceRound advances the round if the round is over.
// This is suposed to be called after updating the Validation Function.
// Returns a batch of the messages for which an acknowledgement should be broadcast.
func (mrc *MultiRoundCounterCoSi) AdvanceRound() (map[onet.TreeNodeID]*MessageBroadcast, error) {
	if !mrc.roundOver {
		return nil, errors.New("Round is not over, cannot advance")
	}

	mrc.currentRound++
	missingAckBroadcast := make(map[onet.TreeNodeID]*MessageBroadcast)

	for sender, msg := range mrc.msgBroadBuffer[mrc.currentRound] {
		if err := mrc.VF(msg, sender); err != nil {
			log.Lvlf1("%v", err)
		} else {
			err := mrc.addMessageBroadcast(msg, sender)
			if err == nil {
				missingAckBroadcast[sender] = msg
			} else {
				log.Lvlf1("%v", err)
			}
		}
	}

	for sender, msg := range mrc.msgCertBuffer[mrc.currentRound] {
		err := mrc.addMessageCertified(msg)
		if err != nil {
			log.Lvlf1("%v. round: %v, sender: %s", err, msg.Round, sender)
		}
	}

	mrc.roundOver = false
	mrc.hasBroadcast = false

	return missingAckBroadcast, nil
}
