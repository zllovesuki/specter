package chord

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"kon.nect.sh/specter/kv/memory"
	"kon.nect.sh/specter/spec/chord"
	mocks "kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"
)

const (
	defaultInterval = time.Millisecond * 10
	waitInterval    = defaultInterval * 10
)

func devConfig(as *require.Assertions) NodeConfig {
	logger, err := zap.NewDevelopment()
	as.NoError(err)
	iden := &protocol.Node{
		Id: chord.Random(),
	}

	return NodeConfig{
		Logger:                   logger.With(zap.Uint64("node", iden.GetId())),
		Identity:                 iden,
		RPCClient:                new(mocks.RPC),
		KVProvider:               memory.WithHashFn(chord.Hash),
		FixFingerInterval:        defaultInterval * 3,
		StablizeInterval:         defaultInterval * 5,
		PredecessorCheckInterval: defaultInterval * 7,
	}
}

// taken from https://yangwwei.github.io/2020/05/12/flaky-unit-tests-on-github.html
func WaitForCondition(eval func() bool, interval time.Duration, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if eval() {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for condition")
		}

		time.Sleep(interval)
	}
}

func waitRing(as *require.Assertions, node *LocalNode) {
	as.NoError(WaitForCondition(func() bool {
		ring := node.ringTrace()
		if !strings.HasSuffix(ring, "error") && ring != "unstable" && node.getPredecessor() != nil {
			return true
		}
		return false
	}, waitInterval, time.Second*5))
}

// it looks like a race condition in macos runner but it is impossible to be a race condition
// -- famous last words
func waitRingLong(as *require.Assertions, nodes []*LocalNode) {
	as.NoError(WaitForCondition(func() bool {
		for _, node := range nodes {
			if node.getPredecessor() == nil {
				return false
			}
		}
		return true
	}, waitInterval, time.Second*5))
}

func makeRing(as *require.Assertions, num int) ([]*LocalNode, func()) {
	nodes := make([]*LocalNode, num)
	for i := 0; i < num; i++ {
		node := NewLocalNode(devConfig(as))
		nodes[i] = node
	}

	nodes[0].Create()
	for i := 1; i < num; i++ {
		as.NoError(nodes[i].Join(nodes[0]))
		<-time.After(waitInterval)
	}

	// wait until the ring is stablized before we ring check
	waitRing(as, nodes[0])
	waitRingLong(as, nodes)

	RingCheck(as, nodes, true)

	return nodes, func() {
		for i := 0; i < num; i++ {
			nodes[i].Leave()
		}
		<-time.After(waitInterval)
	}
}

// should not be called after any of the nodes stopped
func RingCheck(as *require.Assertions, nodes []*LocalNode, counter bool) {
	if len(nodes) == 0 {
		return
	}
	for _, node := range nodes {
		as.NotNil(node.getPredecessor(), "node %d has nil predecessor", node.ID())
		as.NotNil(node.getSuccessor())
	}

	fmt.Printf("Ring: %s\n", nodes[0].ringTrace())

	if len(nodes) == 1 {
		as.Equal(nodes[0].ID(), nodes[0].getPredecessor().ID())
		as.Equal(nodes[0].ID(), nodes[0].getSuccessor().ID())
		return
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].ID() < nodes[j].ID()
	})
	if counter {
		// counter clockwise
		for i := 0; i < len(nodes)-1; i++ {
			as.Equal(nodes[i].ID(), nodes[i+1].getPredecessor().ID())
		}
		as.Equal(nodes[len(nodes)-1].ID(), nodes[0].getPredecessor().ID())
	}
	// clockwise
	for i := 0; i < len(nodes)-1; i++ {
		as.Equal(nodes[i+1].ID(), nodes[i].getSuccessor().ID())
	}
	as.Equal(nodes[0].ID(), nodes[len(nodes)-1].getSuccessor().ID())
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCreate(t *testing.T) {
	as := require.New(t)

	n1 := NewLocalNode(devConfig(as))
	n1.Create()

	<-time.After(waitInterval)

	n1.Leave()

	<-time.After(waitInterval)

	RingCheck(as, []*LocalNode{n1}, true)
}

func TestJoin(t *testing.T) {
	as := require.New(t)

	n2 := NewLocalNode(devConfig(as))
	n2.Create()
	defer n2.Leave()

	n1 := NewLocalNode(devConfig(as))
	as.NoError(n1.Join(n2))
	defer n1.Leave()

	waitRing(as, n2)
	waitRingLong(as, []*LocalNode{n1, n2})

	RingCheck(as, []*LocalNode{
		n1,
		n2,
	}, true)
}

func TestRandomNodes(t *testing.T) {
	as := require.New(t)

	num := 8
	nodes, done := makeRing(as, num)
	defer done()

	for i := 0; i < num; i++ {
		as.Equal(nodes[i].getSuccessor().ID(), (*nodes[i].fingers[1].Load()).ID())
		fmt.Printf("%d: %v\n---\n", nodes[i].ID(), nodes[i].fingerTrace())
	}
}

func TestLotsOfNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping many nodes in short mode")
	}
	as := require.New(t)

	num := 64
	nodes, done := makeRing(as, num)
	defer done()

	for i := 0; i < num; i++ {
		as.Equal(nodes[i].getSuccessor().ID(), (*nodes[i].fingers[1].Load()).ID())
		fmt.Printf("%d: %v\n---\n", nodes[i].ID(), nodes[i].fingerTrace())
	}
}

func TestStatsSummaryHandler(t *testing.T) {
	as := require.New(t)

	node := NewLocalNode(devConfig(as))
	as.NoError(node.Create())
	defer node.Leave()

	testKey := "helloworld"
	node.kv.Put(context.Background(), []byte(testKey), []byte("bye"))

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(node.StatsHandler)

	req, err := http.NewRequest("GET", "/", nil)
	as.NoError(err)
	handler.ServeHTTP(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()

	as.Equal(http.StatusOK, resp.StatusCode)

	var body bytes.Buffer
	body.ReadFrom(resp.Body)

	as.Contains(body.String(), "Active")
	as.Contains(body.String(), testKey)
}

func TestStatsKeyHandler(t *testing.T) {
	as := require.New(t)

	node := NewLocalNode(devConfig(as))
	as.NoError(node.Create())
	defer node.Leave()

	testKey := "helloworld"
	node.kv.Put(context.Background(), []byte(testKey), []byte("hello"))

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(node.StatsHandler)

	req, err := http.NewRequest("GET", fmt.Sprintf("/?key=%s", testKey), nil)
	as.NoError(err)
	handler.ServeHTTP(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()

	as.Equal(http.StatusOK, resp.StatusCode)

	var body bytes.Buffer
	body.ReadFrom(resp.Body)

	as.Equal("hello", body.String())
}
