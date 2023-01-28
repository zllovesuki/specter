package chord

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"kon.nect.sh/specter/kv/memory"
	"kon.nect.sh/specter/spec/chord"
	mocks "kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/util/testcond"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

const (
	defaultInterval = time.Millisecond * 10
	waitInterval    = defaultInterval * 10
)

func devConfig(t *testing.T, as *require.Assertions) NodeConfig {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
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

	RingCheck(as, nodes, true)

	return nodes, func() {
		for i := 0; i < num; i++ {
			nodes[i].Leave()
		}
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
	var (
		seed int64
		err  error
	)
	if os.Getenv("RAND") == "" {
		seed = time.Now().Unix()
	} else {
		seed, err = strconv.ParseInt(os.Getenv("RAND"), 10, 64)
		if err != nil {
			panic(err)
		}
	}
	log.Printf(" ========== Using %d as seed in this test ==========\n", seed)
	rand.Seed(seed)
	goleak.VerifyTestMain(m)
}

func TestCreate(t *testing.T) {
	as := require.New(t)

	n1 := NewLocalNode(devConfig(t, as))
	n1.Create()

	<-time.After(waitInterval)

	n1.Leave()

	<-time.After(waitInterval)

	RingCheck(as, []*LocalNode{n1}, true)
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

	RingCheck(as, []*LocalNode{
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
	nodes, done := makeRing(t, as, num)
	defer done()

	for i := 0; i < num; i++ {
		as.Equal(nodes[i].getSuccessor().ID(), (*nodes[i].fingers[1].Load()).ID())
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

	node := NewLocalNode(devConfig(t, as))
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
