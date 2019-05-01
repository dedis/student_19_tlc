package tlc

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.dedis.ch/cothority/v3"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
)

var tSuite = cothority.Suite

func TestMain(m *testing.M) {
	log.MainTest(m)
}

func TestInitialize(t *testing.T) {
	nodes := []int{3, 5, 13}
	for _, nbrNodes := range nodes {
		local := onet.NewLocalTest(tSuite)
		servers, _, tree := local.GenTree(nbrNodes, true)
		log.Lvl3(tree.Dump())

		log.LLvl1("Starting protocol")

		pi, _ := local.StartProtocol("TLC", tree)

		rooti := pi.(*TLC)

		pis := make([]*TLC, len(servers))

		select {
		case <-time.After(time.Second * 1):
			for i, server := range servers {
				tns := local.GetTreeNodeInstances(server.ServerIdentity.ID)
				//log.LLvlf1("#TNI = %d in server %v", len(tns), server.ServerIdentity.ID)
				for _, tni := range tns {
					pis[i] = tni.ProtocolInstance().(*TLC)
				}
			}
		}

		for _, pi := range pis {
			pi.Terminate()
		}

		local.CloseAll()

		for _, pi := range pis {
			require.Equal(t, pi.TMsgs, rooti.TMsgs)
			require.Equal(t, pi.TAcks, rooti.TAcks)
		}
	}
}

func TestRoundNoFaultyNodes(t *testing.T) {
	nodes := []int{3, 5, 13, 60}
	for _, nbrNodes := range nodes {
		threshold := uint64(nbrNodes/2 + 1)

		local := onet.NewLocalTest(tSuite)
		servers, _, tree := local.GenTree(nbrNodes, true)
		log.Lvl3(tree.Dump())

		log.LLvl1("Starting protocol")

		local.StartProtocol("TLC", tree)

		pis := make([]*TLC, len(servers))

		select {
		case <-time.After(time.Second * 1):
			for i, server := range servers {
				tns := local.GetTreeNodeInstances(server.ServerIdentity.ID)
				//log.LLvlf1("#TNI = %d in server %v", len(tns), server.ServerIdentity.ID)
				for _, tni := range tns {
					pis[i] = tni.ProtocolInstance().(*TLC)
				}
			}
		}

		start := time.Now()

		for i, pi := range pis {
			msg := []byte(fmt.Sprintf("Hello World TLC from node %d", i))
			pi.Message <- msg
		}

		var wg sync.WaitGroup

		wg.Add(len(pis))

		batches := make([]map[onet.TreeNodeID]*MessageDelivered, len(pis))

		for i, pi := range pis {
			go func(i int, pi *TLC) {
				defer wg.Done()
				batches[i] = <-pi.ThresholdSet
			}(i, pi)
		}

		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-time.After(time.Second * 60):
			t.Fatal("Timed out waiting for threads to deliver batches")
		case <-done:
			elapsed := time.Since(start)
			log.LLvlf1("Nodes: %v. Elapsed time for one round: %v", nbrNodes, elapsed)
		}

		for _, pi := range pis {
			pi.Terminate()
		}

		local.CloseAll()

		for i, pi := range pis {
			require.Equal(t, threshold, pi.TMsgs)
			require.Equal(t, threshold, pi.TAcks)
			require.Equal(t, threshold, countTDelivered(batches[i]))
			for _, m := range batches[i] {
				require.Equal(t, 0, int(m.Round))
			}
		}
	}
}

func TestRoundMinorityFaulty(t *testing.T) {
	nodes := []int{3, 5, 13, 60}
	for _, nbrNodes := range nodes {
		threshold := uint64(nbrNodes/2 + 1)
		f := uint64(nbrNodes) - threshold

		local := onet.NewLocalTest(tSuite)
		servers, _, tree := local.GenTree(nbrNodes, true)
		log.Lvl3(tree.Dump())

		log.LLvl1("Starting protocol")

		local.StartProtocol("TLC", tree)

		pis := make([]*TLC, len(servers))

		select {
		case <-time.After(time.Second * 1):
			for i, server := range servers {
				tns := local.GetTreeNodeInstances(server.ServerIdentity.ID)
				//log.LLvlf1("#TNI = %d in server %v", len(tns), server.ServerIdentity.ID)
				for _, tni := range tns {
					pis[i] = tni.ProtocolInstance().(*TLC)
				}
			}
		}

		start := time.Now()

		// terminate a minory of nodes (faulty)
		for _, pi := range pis[:f] {
			pi.Terminate()
		}

		for i, pi := range pis[f:] {
			msg := []byte(fmt.Sprintf("Hello World TLC from instance %d", i))
			pi.Message <- msg
		}

		var wg sync.WaitGroup

		wg.Add(int(threshold))

		batches := make([]map[onet.TreeNodeID]*MessageDelivered, int(threshold))

		for i, pi := range pis[f:] {
			go func(i int, pi *TLC) {
				defer wg.Done()
				batches[i] = <-pi.ThresholdSet
			}(i, pi)
		}

		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-time.After(time.Second * 60):
			t.Fatal("Timed out waiting for threads to deliver batches")
		case <-done:
			elapsed := time.Since(start)
			log.LLvlf1("Nodes: %v. Elapsed time for one round: %v", nbrNodes, elapsed)
		}

		for _, pi := range pis[f:] {
			pi.Terminate()
		}

		local.CloseAll()

		for i, pi := range pis[f:] {
			require.Equal(t, threshold, pi.TMsgs)
			require.Equal(t, threshold, pi.TAcks)
			require.Equal(t, threshold, countTDelivered(batches[i]))
			for _, m := range batches[i] {
				require.Equal(t, 0, int(m.Round))
			}
		}
	}
}

func countTDelivered(batch map[onet.TreeNodeID]*MessageDelivered) uint64 {
	count := 0
	for _, v := range batch {
		if v.TDelivered {
			count++
		}
	}
	return uint64(count)
}
