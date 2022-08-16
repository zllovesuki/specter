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
	entry := &proto.LogEntry{}
	for i := uint64(1); i <= index; i++ {
		buf, err := d.log.Read(i)
		if err != nil {
			return fmt.Errorf("error reading log at index %d: %w", i, err)
		}
		if err := entry.UnmarshalVT(buf); err != nil {
			return fmt.Errorf("error deserializing log at index %d: %w", i, err)
		}
		if err := d.decodeEntry(entry, mut); err != nil {
			return fmt.Errorf("error decoding entry to mutation at index %d: %w", i, err)
		}
		if err := d.handleMutation(mut); err != nil {
			return fmt.Errorf("error apply mutation to memory state at index %d: %w", i, err)
		}
		entry.Reset()
		mut.Reset()
	}
	d.counter = index + 1
	return nil
}

func (d *DiskKV) decodeEntry(entry *proto.LogEntry, mut *proto.Mutation) (err error) {
	switch entry.GetVersion() {
	case proto.LogVersion_V1:
		// uncompressed
		err = mut.UnmarshalVT(entry.Data)
	default:
		err = fmt.Errorf("unknown log version: %s", entry.GetVersion())
	}
	return
}

func (d *DiskKV) appendLog(mut *proto.Mutation) error {
	mutBuf, err := mut.MarshalVT()
	if err != nil {
		d.logger.Error("Error serializing mutation", zap.String("mutation", mut.GetType().String()), zap.Error(err))
		return err
	}

	entry := proto.LogEntryFromVTPool()
	defer entry.ReturnToVTPool()

	entry.Version = proto.LogVersion_V1
	entry.Data = mutBuf

	logBuf, err := entry.MarshalVT()
	if err != nil {
		d.logger.Error("Error serializing log entry", zap.String("version", entry.GetVersion().String()), zap.String("mutation", mut.GetType().String()), zap.Error(err))
		return err
	}

	if err := d.log.Write(d.counter, logBuf); err != nil {
		d.logger.Error("Error appending to log", zap.Uint64("counter", d.counter), zap.String("mutation", mut.GetType().String()), zap.Error(err))
		return err
	}
	d.counter += 1
	return nil
}

func (d *DiskKV) rollbackOne(mut *proto.Mutation, err error) {
	d.logger.Warn("Rolling back last mutation because of an error",
		zap.String("mutation", mut.GetType().String()),
		zap.Uint64("truncate", d.counter-2),
		zap.Uint64("index", d.counter-1),
		zap.Error(err),
	)
	d.counter -= 1
	if err := d.log.TruncateBack(d.counter - 1); err != nil {
		d.logger.Error("Error applying rollback to the last mutation",
			zap.Error(err))
	}
}
