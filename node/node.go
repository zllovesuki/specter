package node

import (
	"context"
	"fmt"

	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/transport"

	"go.uber.org/zap"
)

var (
	ErrLeft = fmt.Errorf("node is not part of the chord ring")
)

func createRPC(ctx context.Context, self chord.VNode, t transport.Transport, logger *zap.Logger, node *protocol.Node) (chord.VNode, error) {
	if node == nil {
		return nil, fmt.Errorf("cannot create rpc with an nil node")
	}
	if node.GetUnknown() {
		return nil, fmt.Errorf("cannot create rpc with an unknown node")
	}
	if node.GetId() == self.ID() {
		return self, nil
	} else {
		return NewRemoteNode(ctx, t, logger, node)
	}
}
