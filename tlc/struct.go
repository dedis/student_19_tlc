package tlc

import (
	"crypto/sha256"
	"encoding/binary"

	"go.dedis.ch/onet/v3"
)

type roundNum uint64
type msghash [32]byte

// Initialize is the first message sent to instantiate the protocol in all servers.
// * Currently being used for testing purposes, should be unnecessary eventually *
type Initialize struct {
	TMsgs uint64
	TAcks uint64
}

// MessageBroadcast is broadcast exactly once per round.
type MessageBroadcast struct {
	Round   roundNum
	Message []byte
}

// MessageAck is used to ascertain who has received the message.
type MessageAck struct {
	Round roundNum
	Hash  [32]byte
}

// Overlay-structures to retrieve the sending TreeNode.
type chanInitialize struct {
	*onet.TreeNode
	Initialize
}

type chanRoundMessage struct {
	*onet.TreeNode
	MessageBroadcast
}

type chanAckMessage struct {
	*onet.TreeNode
	MessageAck
}

// Hash is used to obtain the Hash for MessageAck
// * Should replace this with a better hash function (ask advisor) *
func (mb *MessageBroadcast) Hash(sender onet.TreeNodeID) (out [32]byte) {
	h := sha256.New()
	binary.Write(h, binary.LittleEndian,
		uint64(mb.Round))
	h.Write(mb.Message[:])
	h.Write(sender[:])

	copy(out[:], h.Sum(nil))
	return
}

// Hash is used to obtain the Hash for MessageAck
// * Should replace this with a better hash function (ask advisor) *
func (rm *chanRoundMessage) Hash() (out [32]byte) {
	return rm.MessageBroadcast.Hash(rm.TreeNode.ID)
}

// MessageDelivered is the format delivered to the service.
type MessageDelivered struct {
	*MessageBroadcast
	TDelivered bool
}
