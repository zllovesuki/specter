package chord

import (
	"context"
	"fmt"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"go.uber.org/zap"
)

func createRPC(ctx context.Context, logger *zap.Logger, chordClient rpc.ChordClient, node *protocol.Node) (chord.VNode, error) {
	if node == nil {
		return nil, fmt.Errorf("cannot create rpc with an nil node")
	}
	if node.GetUnknown() {
		return nil, fmt.Errorf("cannot create rpc with an unknown node")
	}

	return NewRemoteNode(ctx, logger, chordClient, node)
}
