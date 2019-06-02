package tlccosi

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
	mbs, mcs := prepareNodeMessages(treeNodeIDs, round)
	treeNodeIDset := make(map[onet.TreeNodeID]*MessageBroadcast)
	for i, nodeID := range treeNodeIDs {
		treeNodeIDset[nodeID] = mbs[i]
	}

	src := newSingleRoundCounterCoSi(tMsgs, tAcks)

	for i, msg := range mbs {
		err := src.addMessageBroadcast(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("addMessage returned error: %v", err)
		}
	}

	for _, mc := range mcs[:tMsgs-1] {
		err := src.addMessageCertified(mc)
		if err != nil {
			t.Fatalf("addMessageCertified returned error: %v", err)
		}
	}

	if src.thresholdReached != false {
		t.Fatal("Thresholds reached with insufficient certified message")
	}

	// last certified message needed
	err := src.addMessageCertified(mcs[tMsgs-1])
	if err != nil {
		t.Fatalf("addAck returned error: %v", err)
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

func TestSRCEarlyCertifiedMessage(t *testing.T) {
	total, round := 9, 0
	tMsgs := uint64((total + 1) / 2)
	tAcks := tMsgs

	treeNodeIDs := prepareNodeIDs(total)
	mbs, mcs := prepareNodeMessages(treeNodeIDs, round)
	treeNodeIDset := make(map[onet.TreeNodeID]*MessageBroadcast)
	for i, nodeID := range treeNodeIDs {
		treeNodeIDset[nodeID] = mbs[i]
	}

	src := newSingleRoundCounterCoSi(tMsgs, tAcks)

	if src.thresholdReached != false {
		t.Fatal("Thresholds reached despite no certified messages")
	}

	for _, msg := range mcs[:tMsgs] {
		err := src.addMessageCertified(msg)
		if err != nil {
			t.Fatalf("addCertifiedMessage returned error: %v", err)
		}
	}

	for i, msg := range mbs[tMsgs : total-2] {
		err := src.addMessageBroadcast(msg, treeNodeIDs[i+int(tMsgs)])
		if err != nil {
			t.Fatalf("addCertifiedMessage returned error: %v", err)
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
	mbs0, mcs0 := prepareNodeMessages(treeNodeIDs, 0)

	treeNodeIDset0 := make(map[onet.TreeNodeID]*MessageBroadcast)
	for i, nodeID := range treeNodeIDs {
		treeNodeIDset0[nodeID] = mbs0[i]
	}

	valFunction := func(msg *MessageBroadcast, sender onet.TreeNodeID) error { return nil }
	mrc := NewMultiRoundCounterCoSi(tMsgs, tAcks, valFunction)

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
		shouldAck, err := mrc.AddMessageBroadcast(msg, treeNodeIDs[i])
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

	// insufficient certified messages
	for i, mc := range mcs0[:tMsgs-1] {
		err := mrc.AddMessageCertified(mc, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessageCertified returned error: %v", err)
		}
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite insufficient acks")
	}

	// last certified message needed
	err = mrc.AddMessageCertified(mcs0[tMsgs-1], treeNodeIDs[tMsgs-1])
	if err != nil {
		t.Fatalf("AddMessageCertified returned error: %v", err)
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
	mbs0, mcs0 := prepareNodeMessages(treeNodeIDs, 0)
	mbs1, mcs1 := prepareNodeMessages(treeNodeIDs, 1)
	mbs2, mcs2 := prepareNodeMessages(treeNodeIDs, 2)

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
	mrc := NewMultiRoundCounterCoSi(tMsgs, tAcks, valFunction)

	mrc.RoundBroadcast([]byte{byte(15)})

	for i, msg := range mbs0 {
		_, err := mrc.AddMessageBroadcast(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessage returned error: %v", err)
		}
	}

	for i, msg := range mbs1 {
		_, err := mrc.AddMessageBroadcast(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessage returned error: %v", err)
		}
	}

	for i, msg := range mbs2 {
		_, err := mrc.AddMessageBroadcast(msg, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessage returned error: %v", err)
		}
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite no acks")
	}

	for i, mc := range mcs2[:tMsgs] {
		err := mrc.AddMessageCertified(mc, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessageCertified returned error: %v", err)
		}
	}

	for i, mc := range mcs1[:tMsgs-1] {
		err := mrc.AddMessageCertified(mc, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessageCertified returned error: %v", err)
		}
	}

	if _, err := mrc.TryEndRound(); err == nil {
		t.Fatal("Round advanced despite no acks for round 0")
	}

	for i, mc := range mcs0[:tMsgs] {
		err := mrc.AddMessageCertified(mc, treeNodeIDs[i])
		if err != nil {
			t.Fatalf("AddMessageCertified returned error: %v", err)
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

	// last certified message needed for round 1
	err = mrc.AddMessageCertified(mcs1[tMsgs-1], treeNodeIDs[tMsgs-1])
	if err != nil {
		t.Fatalf("AddMessageCertified returned error: %v", err)
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
	if len(mrc.msgBroadBuffer) != 0 {
		t.Error("MRC not garbage collecting msgBuffer")
	}
	if len(mrc.msgCertBuffer) != 0 {
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

func prepareNodeMessages(nodeIDs []onet.TreeNodeID, round int) ([]*MessageBroadcast, []*MessageCertified) {
	nbrNodes := len(nodeIDs)

	mbs := make([]*MessageBroadcast, nbrNodes)
	mcs := make([]*MessageCertified, nbrNodes)
	for i := range mbs {
		mbs[i] = &MessageBroadcast{Round: roundNum(round), Message: []byte{byte(i), byte(round)}}
		mcs[i] = &MessageCertified{nodeIDs[i], mbs[i], &Certification{nil, nil}}
	}

	return mbs, mcs
}

func index(id onet.TreeNodeID, slice []onet.TreeNodeID) int {
	for i, v := range slice {
		if v == id {
			return i
		}
	}
	return -1
}
