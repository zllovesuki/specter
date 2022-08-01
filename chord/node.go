package chord

import (
	"context"
	"fmt"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"

	"go.uber.org/zap"
)

func createRPC(ctx context.Context, t transport.Transport, logger *zap.Logger, node *protocol.Node) (chord.VNode, error) {
	if node == nil {
		return nil, fmt.Errorf("cannot create rpc with an nil node")
	}
	if node.GetUnknown() {
		return nil, fmt.Errorf("cannot create rpc with an unknown node")
	}
	return NewRemoteNode(ctx, t, logger, node)
}
