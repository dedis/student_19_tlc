package tlccosi

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
	Round     roundNum
	Hash      [32]byte
	Signature []byte
}

// MessageCertified is the format delivered to the service.
type MessageCertified struct {
	ID onet.TreeNodeID // ID is the original sender of the message
	*MessageBroadcast
	*Certification
}

// Certification serves as cryptographic proof that a message has been seen by a threshold of nodes
type Certification struct {
	Signature []byte
	Mask      []byte
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

type chanCertifiedMessage struct {
	*onet.TreeNode
	MessageCertified
}

// MessageDelivered is the format delivered to the service.
type MessageDelivered struct {
	*MessageCertified
	TDelivered bool // true if the message was delivered by TMsgs nodes ("certified")
}

// Hash is used to obtain the Hash for MessageAck
// sender is the original sender of the message (used to differentiate same-content messages)
// * Should perhaps replace this with a better hash function (ask advisor) *
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

// Hash is used to obtain the Hash for MessageCertified
// * Should replace this with a better hash function (ask advisor) *
func (cm *MessageCertified) Hash() (out [32]byte) {
	return cm.MessageBroadcast.Hash(cm.ID)
}
