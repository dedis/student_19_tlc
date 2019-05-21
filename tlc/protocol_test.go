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

func TestMultipleRoundsNoFaultyNodes(t *testing.T) {
	nodes := []int{3, 5, 13}
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
				for _, tni := range tns {
					pis[i] = tni.ProtocolInstance().(*TLC)
				}
			}
		}

		for round := 0; round < 2; round++ {
			start := time.Now()

			sendMessages(pis, makeRange(0, len(pis)), round)
			batches := waitAll(t, pis)

			elapsed := time.Since(start) // this time is meaningless  (should use simulation)
			log.LLvlf1("Nodes: %v. Round: %v. Elapsed time: %v", nbrNodes, round, elapsed)

			for i, pi := range pis {
				require.Equal(t, threshold, pi.TMsgs)
				require.Equal(t, threshold, pi.TAcks)
				require.Equal(t, threshold, countTDelivered(batches[i]))
				for _, m := range batches[i] {
					require.Equal(t, round, int(m.Round))
				}
			}
		}

		for _, pi := range pis {
			pi.Terminate()
		}

		local.CloseAll()
	}
}

func TestRoundMinorityFaulty(t *testing.T) {
	nodes := []int{3, 5, 13}
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
			msg := []byte(fmt.Sprintf("Hello World TLC from instance %d", i+int(f)))
			pi.Message <- msg
		}

		batches := waitAll(t, pis[f:])

		elapsed := time.Since(start)
		log.LLvlf1("Nodes: %v. Elapsed time for one round: %v", nbrNodes, elapsed)

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

func TestBuffering(t *testing.T) {
	nodes := []int{3, 5, 13}
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
				for _, tni := range tns {
					pis[i] = tni.ProtocolInstance().(*TLC)
				}
			}
		}

		start := time.Now()

		// first round
		sendMessages(pis, makeRange(0, len(pis)), 0)
		batches := waitAll(t, pis)

		elapsed := time.Since(start) // this time is meaningless  (should use simulation)
		log.LLvlf1("Nodes: %v. Round: %v. Elapsed time: %v", nbrNodes, 0, elapsed)

		for i, pi := range pis {
			require.Equal(t, threshold, pi.TMsgs)
			require.Equal(t, threshold, pi.TAcks)
			require.Equal(t, threshold, countTDelivered(batches[i]))
			for _, m := range batches[i] {
				require.Equal(t, 0, int(m.Round))
			}
		}

		// second round
		sendMessages(pis[0:1], makeRange(0, 1), 1)
		time.Sleep(time.Second * 2)

		time.Sleep(time.Second * 2)

		start = time.Now()
		sendMessages(pis[1:], makeRange(1, len(pis)), 1)

		batches = waitAll(t, pis)

		elapsed = time.Since(start) // this time is meaningless  (use simulation)
		log.LLvlf1("Nodes: %v. Round: %v. Elapsed time (this round): %v", nbrNodes, 1, elapsed)

		for i, pi := range pis {
			require.Equal(t, threshold, pi.TMsgs)
			require.Equal(t, threshold, pi.TAcks)
			require.Equal(t, threshold, countTDelivered(batches[i]))
			for _, m := range batches[i] {
				require.Equal(t, 1, int(m.Round))
			}
		}

		for _, pi := range pis {
			pi.Terminate()
		}

		local.CloseAll()
	}
}

func makeRange(min, max int) []int {
	a := make([]int, max-min)
	for i := 0; i < max-min; i++ {
		a[i] = i + min
	}
	return a
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

func sendMessages(pis []*TLC, nodeNames []int, round int) {
	for i, pi := range pis {
		msg := []byte(fmt.Sprintf("Hello World TLC from node %v, message round %d", nodeNames[i], round))
		pi.Message <- msg
	}
}

func waitAll(t *testing.T, pis []*TLC) []map[onet.TreeNodeID]*MessageDelivered {
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
	case <-time.After(time.Second * 30):
		t.Fatal("Timed out waiting for threads to deliver batches")
	case <-done:
	}

	return batches
}
