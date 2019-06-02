package tlc

import (
	"bytes"
	"testing"

	"go.dedis.ch/onet/v3"
)

func TestSRCThresholdsNormal(t *testing.T) {
	total, round := 9, 0
	tMsgs := uint64((total + 1) / 2)
	tAcks := tMsgs

	treeNodeIDs := prepareNodeIDs(total)
	mbs, acks := prepareNodeMessages(treeNodeIDs, round)
	treeNodeIDset := make(map[onet.TreeNodeID]*MessageBroadcast)
	for i, nodeID := range treeNodeIDs {
		treeNodeIDset[nodeID] = mbs[i]
	}

	src := newSingleRoundCounter(tMsgs, tAcks)

	for i, msg := range mbs {
		err := src.addMessage(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("addMessage returned error: %v", err)
		}
	}

	if src.thresholdReached != false {
		t.Fatal("Thresholds reached with 0 acks")
	}

	// insufficient acks
	for _, ack := range acks[:tMsgs-1] {
		for _, nodeID := range treeNodeIDs {
			err := src.addAck(ack.Hash, nodeID)
			if err != nil {
				t.Fatalf("addAck returned error: %v", err)
			}
		}
	}

	if src.thresholdReached != false {
		t.Fatal("Thresholds reached despite insufficient acks")
	}

	// last ack needed
	for _, nodeID := range treeNodeIDs {
		err := src.addAck(acks[tMsgs-1].Hash, nodeID)
		if err != nil {
			t.Fatalf("addAck returned error: %v", err)
		}
	}

	if src.thresholdReached != true {
		t.Fatal("Thresholds reached but round didn't finish")
	}

	batch := src.reset()
	if len(batch) != total {
		t.Errorf("SRC batch holding incorrect number of messages. has: %v, should have: %v", len(batch), total)
	}
	for id, delMsg := range batch {
		if _, ok := treeNodeIDset[id]; !ok {
			t.Errorf("Delivered message for non-existant node id: %v", id)
		}
		if delMsg.Round != 0 {
			t.Errorf("Delivered message for incorrect round: %v (should be 0)", delMsg.Round)
		}
		if bytes.Compare(delMsg.Message, treeNodeIDset[id].Message) != 0 {
			t.Errorf("Delivered message has incorrect message: %v (should be %v)", delMsg.Message, treeNodeIDset[id].Message)
		}
		if uint64(index(id, treeNodeIDs)) < tMsgs && delMsg.TDelivered != true {
			t.Error("Message incorrectly marked as *NOT* TDelivered (should be true)")
		} else if uint64(index(id, treeNodeIDs)) >= tMsgs && delMsg.TDelivered != false {
			t.Error("Message incorrectly marked as TDelivered (should be false)")
		}
	}
}

func TestSRCThresholdsThin(t *testing.T) {
	total, round := 9, 0
	tMsgs := uint64((total + 1) / 2)
	tAcks := uint64(0)

	treeNodeIDs := prepareNodeIDs(total)
	mbs, _ := prepareNodeMessages(treeNodeIDs, round)
	treeNodeIDset := make(map[onet.TreeNodeID]*MessageBroadcast)
	for i, nodeID := range treeNodeIDs {
		treeNodeIDset[nodeID] = mbs[i]
	}

	src := newSingleRoundCounter(tMsgs, tAcks)

	for i, msg := range mbs {
		err := src.addMessage(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("addMessage returned error: %v", err)
		}
	}

	if src.thresholdReached != true {
		t.Fatal("Thresholds reached but round didn't finish")
	}

	batch := src.reset()
	if len(batch) != total {
		t.Errorf("SRC batch holding incorrect number of messages. has: %v, should have: %v", len(batch), total)
	}
	for id, delMsg := range batch {
		if _, ok := treeNodeIDset[id]; !ok {
			t.Errorf("Delivered message for non-existant node id: %v", id)
		}
		if delMsg.Round != 0 {
			t.Errorf("Delivered message for incorrect round: %v (should be 0)", delMsg.Round)
		}
		if bytes.Compare(delMsg.Message, treeNodeIDset[id].Message) != 0 {
			t.Errorf("Delivered message has incorrect message: %v (should be %v)", delMsg.Message, treeNodeIDset[id].Message)
		}
		if delMsg.TDelivered != true {
			t.Error("Message incorrectly marked as *NOT* TDelivered (should be true)")
		}
	}
}
func TestSRCEarlyAcks(t *testing.T) {
	total, round := 9, 0
	tMsgs := uint64((total + 1) / 2)
	tAcks := tMsgs

	treeNodeIDs := prepareNodeIDs(total)
	mbs, acks := prepareNodeMessages(treeNodeIDs, round)
	treeNodeIDset := make(map[onet.TreeNodeID]*MessageBroadcast)
	for i, nodeID := range treeNodeIDs {
		treeNodeIDset[nodeID] = mbs[i]
	}

	src := newSingleRoundCounter(tMsgs, tAcks)

	if src.thresholdReached != false {
		t.Fatal("Thresholds reached despite no acks")
	}

	for _, ack := range acks[:tAcks] {
		for _, nodeID := range treeNodeIDs {
			err := src.addAck(ack.Hash, nodeID)
			if err != nil {
				t.Fatalf("addAck returned error: %v", err)
			}
		}
	}

	if src.thresholdReached != false {
		t.Fatal("Thresholds reached despite no messages received")
	}

	for i, msg := range mbs[:total-2] {
		err := src.addMessage(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("addMessage returned error: %v", err)
		}
	}

	if src.thresholdReached != true {
		t.Fatal("Thresholds reached but round didn't finish")
	}

	batch := src.reset()
	if len(batch) != total-2 {
		t.Errorf("SRC batch holding incorrect number of messages. has: %v, should have: %v", len(batch), total-2)
	}
	for id, delMsg := range batch {
		if _, ok := treeNodeIDset[id]; !ok {
			t.Errorf("Delivered message for non-existant node id: %v", id)
		}
		if delMsg.Round != 0 {
			t.Errorf("Delivered message for incorrect round: %v (should be 0)", delMsg.Round)
		}
		if bytes.Compare(delMsg.Message, treeNodeIDset[id].Message) != 0 {
			t.Errorf("Delivered message has incorrect message: %v (should be %v)", delMsg.Message, treeNodeIDset[id].Message)
		}
		if uint64(index(id, treeNodeIDs)) < tMsgs && delMsg.TDelivered != true {
			t.Error("Message incorrectly marked as *NOT* TDelivered (should be true)")
		} else if uint64(index(id, treeNodeIDs)) >= tMsgs && delMsg.TDelivered != false {
			t.Error("Message incorrectly marked as TDelivered (should be false)")
		}
	}
}

func TestMRCThresholdsNormal(t *testing.T) {
	total := 9
	tMsgs := uint64((total + 1) / 2)
	tAcks := tMsgs

	treeNodeIDs := prepareNodeIDs(total)
	mbs0, acks0 := prepareNodeMessages(treeNodeIDs, 0)

	treeNodeIDset0 := make(map[onet.TreeNodeID]*MessageBroadcast)
	for i, nodeID := range treeNodeIDs {
		treeNodeIDset0[nodeID] = mbs0[i]
	}

	valFunction := func(msg *MessageBroadcast, sender onet.TreeNodeID) error { return nil }
	mrc := NewMultiRoundCounter(tMsgs, tAcks, valFunction)

	msg, err := mrc.RoundBroadcast([]byte{byte(15)})
	if err != nil {
		t.Fatalf("RoundBroadcast returned error on first broadcast: %v", err)
	}
	if msg.Round != 0 {
		t.Fatalf("Wrong round on broadcast. is: %v (should be: %v)", msg.Round, 0)
	}
	if bytes.Compare(msg.Message, []byte{byte(15)}) != 0 {
		t.Fatalf("Wrong message created on broadcast. is: %v (should be: %v)", msg.Message, []byte{byte(15)})
	}
	_, err = mrc.RoundBroadcast([]byte{byte(15)})
	if err == nil {
		t.Fatal("Duplicate broadcast in same round")
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite no broadcast or thresholds reached")
	}

	for i, msg := range mbs0 {
		shouldAck, err := mrc.AddMessage(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessage returned error: %v", err)
		}
		if shouldAck != true {
			t.Fatal("AddMessage returned shouldAck=false. Should be true")
		}
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite no acks")
	}

	// insufficient acks
	for _, ack := range acks0[:tMsgs-1] {
		for _, nodeID := range treeNodeIDs {
			err := mrc.AddAck(ack, nodeID)
			if err != nil {
				t.Fatalf("AddAck returned error: %v", err)
			}
		}
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite insufficient acks")
	}

	// last ack needed
	for _, nodeID := range treeNodeIDs {
		err := mrc.AddAck(acks0[tMsgs-1], nodeID)
		if err != nil {
			t.Fatalf("AddAck returned error: %v", err)
		}
	}

	batch, err := mrc.TryEndRound()
	if err != nil {
		t.Fatal("Thresholds reached but round didn't advance")
	}
	mrc.AdvanceRound()

	if len(batch) != total {
		t.Errorf("MRC batch holding incorrect number of messages. has: %v, should have: %v", len(batch), total)
	}
	for id, delMsg := range batch {
		if _, ok := treeNodeIDset0[id]; !ok {
			t.Errorf("Delivered message for non-existant node id: %v", id)
		}
		if delMsg.Round != 0 {
			t.Errorf("Delivered message for incorrect round: %v (should be 0)", delMsg.Round)
		}
		if bytes.Compare(delMsg.Message, treeNodeIDset0[id].Message) != 0 {
			t.Errorf("Delivered message has incorrect message: %v (should be %v)", delMsg.Message, treeNodeIDset0[id].Message)
		}
		if uint64(index(id, treeNodeIDs)) < tMsgs && delMsg.TDelivered != true {
			t.Error("Message incorrectly marked as *NOT* TDelivered (should be true)")
		} else if uint64(index(id, treeNodeIDs)) >= tMsgs && delMsg.TDelivered != false {
			t.Error("Message incorrectly marked as TDelivered (should be false)")
		}
	}

	if mrc.currentRound != 1 {
		t.Errorf("Incorrect final round. is %v (should be %v)", mrc.currentRound, 1)
	}
	if mrc.hasBroadcast != false {
		t.Error("MRC incorrectly marked as having broadcast")
	}
}

func TestMRCBuffering(t *testing.T) {
	total := 9
	tMsgs := uint64((total + 1) / 2)
	tAcks := tMsgs

	treeNodeIDs := prepareNodeIDs(total)
	mbs0, acks0 := prepareNodeMessages(treeNodeIDs, 0)
	mbs1, acks1 := prepareNodeMessages(treeNodeIDs, 1)
	mbs2, acks2 := prepareNodeMessages(treeNodeIDs, 2)

	treeNodeIDset0 := make(map[onet.TreeNodeID]*MessageBroadcast)
	treeNodeIDset1 := make(map[onet.TreeNodeID]*MessageBroadcast)
	treeNodeIDset2 := make(map[onet.TreeNodeID]*MessageBroadcast)

	for i, nodeID := range treeNodeIDs {
		treeNodeIDset0[nodeID] = mbs0[i]
		treeNodeIDset1[nodeID] = mbs1[i]
		treeNodeIDset2[nodeID] = mbs2[i]
	}

	treeNodeIDsets := []map[onet.TreeNodeID]*MessageBroadcast{treeNodeIDset0, treeNodeIDset1, treeNodeIDset2}

	valFunction := func(msg *MessageBroadcast, sender onet.TreeNodeID) error { return nil }
	mrc := NewMultiRoundCounter(tMsgs, tAcks, valFunction)

	mrc.RoundBroadcast([]byte{byte(15)})

	for i, msg := range mbs0 {
		_, err := mrc.AddMessage(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessage returned error: %v", err)
		}
	}

	for i, msg := range mbs1 {
		_, err := mrc.AddMessage(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessage returned error: %v", err)
		}
	}

	for i, msg := range mbs2 {
		_, err := mrc.AddMessage(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessage returned error: %v", err)
		}
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite no acks")
	}

	for _, ack := range acks2[:tMsgs] {
		for _, nodeID := range treeNodeIDs {
			err := mrc.AddAck(ack, nodeID)
			if err != nil {
				t.Fatalf("addAck returned error: %v", err)
			}
		}
	}

	// purposefully insufficient
	for _, ack := range acks1[:tMsgs-1] {
		for _, nodeID := range treeNodeIDs {
			err := mrc.AddAck(ack, nodeID)
			if err != nil {
				t.Fatalf("addAck returned error: %v", err)
			}
		}
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite no acks for round 0")
	}

	for _, ack := range acks0[:tMsgs] {
		for _, nodeID := range treeNodeIDs {
			err := mrc.AddAck(ack, nodeID)
			if err != nil {
				t.Fatalf("addAck returned error: %v", err)
			}
		}
	}

	batch0, err := mrc.TryEndRound()
	if err != nil {
		t.Fatal("Round didn't advance despite reaching round 0 thresholds")
	}
	mrc.AdvanceRound()

	if _, err = mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite insufficient acks for round 1")
	}

	// last ack needed for round 1
	for _, nodeID := range treeNodeIDs {
		err := mrc.AddAck(acks1[tMsgs-1], nodeID)
		if err != nil {
			t.Fatalf("addAck returned error: %v", err)
		}
	}

	_, err = mrc.TryEndRound()
	if err == nil {
		t.Fatal("Round advanced despite missing round 1 broadcast")
	}

	mrc.RoundBroadcast([]byte{byte(15)})

	batch1, err := mrc.TryEndRound()
	if err != nil {
		t.Fatal("Round didn't advance despite reaching round 1 thresholds")
	}
	mrc.AdvanceRound()

	mrc.RoundBroadcast([]byte{byte(15)})

	batch2, err := mrc.TryEndRound()
	if err != nil {
		t.Fatal("Round didn't advance despite reaching round 2 thresholds")
	}
	mrc.AdvanceRound()

	for i, batch := range []map[onet.TreeNodeID]*MessageDelivered{batch0, batch1, batch2} {
		if len(batch) != total {
			t.Errorf("MRC batch holding incorrect number of messages. has: %v, should have: %v", len(batch), total)
		}
		for id, delMsg := range batch {
			if _, ok := treeNodeIDsets[i][id]; !ok {
				t.Errorf("Delivered message for non-existant node id: %v", id)
			}
			if delMsg.Round != roundNum(i) {
				t.Errorf("Delivered message for incorrect round: %v (should be 0)", delMsg.Round)
			}
			if bytes.Compare(delMsg.Message, treeNodeIDsets[i][id].Message) != 0 {
				t.Errorf("Delivered message has incorrect message: %v (should be %v)", delMsg.Message, treeNodeIDsets[i][id].Message)
			}
			if uint64(index(id, treeNodeIDs)) < tMsgs && delMsg.TDelivered != true {
				t.Error("Message incorrectly marked as *NOT* TDelivered (should be true)")
			} else if uint64(index(id, treeNodeIDs)) >= tMsgs && delMsg.TDelivered != false {
				t.Error("Message incorrectly marked as TDelivered (should be false)")
			}
		}
	}

	if mrc.currentRound != 3 {
		t.Errorf("Incorrect final round. is %v (should be %v)", mrc.currentRound, 3)
	}
	if mrc.hasBroadcast != false {
		t.Error("MRC incorrectly marked as having broadcast")
	}
	if len(mrc.msgBuffer) != 0 {
		t.Error("MRC not garbage collecting msgBuffer")
	}
	if len(mrc.ackBuffer) != 0 {
		t.Error("MRC not garbage collecting ackBuffer")
	}
}

func prepareNodeIDs(nbrNodes int) []onet.TreeNodeID {
	local := onet.NewLocalTest(tSuite)
	_, _, tree := local.GenTree(nbrNodes, true)
	treeNodeIDs := make([]onet.TreeNodeID, nbrNodes)
	for i, v := range tree.List() {
		treeNodeIDs[i] = v.ID
	}
	local.CloseAll()

	return treeNodeIDs
}

func prepareNodeMessages(nodeIDs []onet.TreeNodeID, round int) ([]*MessageBroadcast, []*MessageAck) {
	nbrNodes := len(nodeIDs)

	mbs := make([]*MessageBroadcast, nbrNodes)
	acks := make([]*MessageAck, nbrNodes)
	for i := range mbs {
		mbs[i] = &MessageBroadcast{Round: roundNum(round), Message: []byte{byte(i), byte(round)}}
		acks[i] = &MessageAck{Round: roundNum(round), Hash: mbs[i].Hash(nodeIDs[i])}
	}

	return mbs, acks
}

func index(id onet.TreeNodeID, slice []onet.TreeNodeID) int {
	for i, v := range slice {
		if v == id {
			return i
		}
	}
	return -1
}
