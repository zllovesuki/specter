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

	"go.miragespace.co/specter/kv/memory"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/util/testcond"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

const (
	defaultInterval = time.Millisecond * 1
	waitInterval    = defaultInterval * 10
)

func devConfig(t *testing.T, as *require.Assertions) NodeConfig {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	iden := &protocol.Node{
		Id: chord.Random(),
	}
	m := new(mocks.Measurement)
	m.On("Snapshot", mock.Anything, mock.Anything).Return(&rtt.Statistics{})

	as.True(true) // placeholder

	return NodeConfig{
		BaseLogger:               logger,
		ChordClient:              new(mocks.ChordClient),
		Identity:                 iden,
		KVProvider:               memory.WithHashFn(chord.Hash),
		StabilizeInterval:        defaultInterval * 3,
		FixFingerInterval:        defaultInterval * 5,
		PredecessorCheckInterval: defaultInterval * 7,
		NodesRTT:                 m,
	}
}

func waitRing(as *require.Assertions, node *LocalNode) {
	as.NoError(testcond.WaitForCondition(func() bool {
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
	as.NoError(testcond.WaitForCondition(func() bool {
		for _, node := range nodes {
			if node.getPredecessor() == nil {
				return false
			}
		}
		return true
	}, waitInterval, time.Second*5))
}

func makeRing(t *testing.T, as *require.Assertions, num int) ([]*LocalNode, func()) {
	nodes := make([]*LocalNode, num)
	for i := 0; i < num; i++ {
		node := NewLocalNode(devConfig(t, as))
		nodes[i] = node
	}

	nodes[0].Create()
	for i := 1; i < num; i++ {
		as.NoError(nodes[i].Join(nodes[0]))
		<-time.After(waitInterval)
	}

	// wait until the ring is mostly stablized before we ring check
	waitRing(as, nodes[0])
	waitRingLong(as, nodes)

	RingCheck(t, as, nodes, true)

	return nodes, func() {
		for i := 0; i < num; i++ {
			nodes[i].Leave()
		}
	}
}

// should not be called after any of the nodes stopped
func RingCheck(t *testing.T, as *require.Assertions, nodes []*LocalNode, counter bool) {
	if len(nodes) == 0 {
		return
	}
	for _, node := range nodes {
		as.NotNil(node.getPredecessor(), "node %d has nil predecessor", node.ID())
		as.NotNil(node.getSuccessor())
	}

	t.Logf("Ring: %s\n", nodes[0].ringTrace())

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

	n1 := NewLocalNode(devConfig(t, as))
	n1.Create()

	<-time.After(waitInterval)

	n1.Leave()

	<-time.After(waitInterval)

	RingCheck(t, as, []*LocalNode{n1}, true)
}

func TestJoin(t *testing.T) {
	as := require.New(t)

	n2 := NewLocalNode(devConfig(t, as))
	n2.Create()
	defer n2.Leave()

	n1 := NewLocalNode(devConfig(t, as))
	as.NoError(n1.Join(n2))
	defer n1.Leave()

	waitRing(as, n2)
	waitRingLong(as, []*LocalNode{n1, n2})

	RingCheck(t, as, []*LocalNode{
		n1,
		n2,
	}, true)
}

func TestRandomNodes(t *testing.T) {
	as := require.New(t)

	num := 8
	nodes, done := makeRing(t, as, num)
	defer done()

	for i := 0; i < num; i++ {
		nodes[i].fingers[1].computeView(func(node chord.VNode) {
			as.Equal(nodes[i].getSuccessor().ID(), node.ID())
		})
		fmt.Printf("%d: %v\n---\n", nodes[i].ID(), nodes[i].fingerTrace())
	}
}

func TestLotsOfNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping many nodes in short mode")
	}
	as := require.New(t)

	num := 64
	nodes, done := makeRing(t, as, num)
	defer done()

	for i := 0; i < num; i++ {
		nodes[i].fingers[1].computeView(func(node chord.VNode) {
			as.Equal(nodes[i].getSuccessor().ID(), node.ID())
		})
		fmt.Printf("%d: %v\n---\n", nodes[i].ID(), nodes[i].fingerTrace())
	}
}

func TestStatsSummaryHandler(t *testing.T) {
	as := require.New(t)

	node := NewLocalNode(devConfig(t, as))
	as.NoError(node.Create())
	defer node.Leave()

	testKey := "helloworld"
	node.kv.Put(context.Background(), []byte(testKey), []byte("bye"))

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(statsHandler([]*LocalNode{node}))

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

	node := NewLocalNode(devConfig(t, as))
	as.NoError(node.Create())
	defer node.Leave()

	testKey := "helloworld"
	node.kv.Put(context.Background(), []byte(testKey), []byte("hello"))

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(statsHandler([]*LocalNode{node}))

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
