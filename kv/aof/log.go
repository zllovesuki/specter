package aof

import (
	"fmt"

	"kon.nect.sh/specter/kv/aof/proto"

	"go.uber.org/zap"
)

func (d *DiskKV) replayLogs() error {
	index, err := d.log.LastIndex()
	if err != nil {
		return fmt.Errorf("error reading last log index: %w", err)
	}
	d.logger.Info("Replaying mutation logs", zap.Uint64("index", index))
	mut := &proto.Mutation{}
	for i := uint64(1); i <= index; i++ {
		buf, err := d.log.Read(i)
		if err != nil {
			return fmt.Errorf("error reading log at index %d: %w", i, err)
		}
		if err := mut.UnmarshalVT(buf); err != nil {
			return fmt.Errorf("error deserializing log at index %d: %w", i, err)
		}
		if err := d.handleMutation(mut); err != nil {
			return fmt.Errorf("error apply mutation to memory state at index %d: %w", i, err)
		}
		mut.Reset()
	}
	d.counter = index + 1
	return nil
}

func (d *DiskKV) appendLog(mut *proto.Mutation) error {
	buf, err := mut.MarshalVT()
	if err != nil {
		d.logger.Error("Error serializing mutation", zap.String("mutation", mut.GetType().String()), zap.Error(err))
		return err
	}
	if err := d.log.Write(d.counter, buf); err != nil {
		d.logger.Error("Error appending to log", zap.Uint64("counter", d.counter), zap.String("mutation", mut.GetType().String()), zap.Error(err))
		return err
	}
	d.counter += 1
	return nil
}
