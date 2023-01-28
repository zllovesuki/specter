package aof

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"kon.nect.sh/specter/kv/aof/proto"
	"kon.nect.sh/specter/kv/memory"
	"kon.nect.sh/specter/spec/chord"

	"github.com/tidwall/wal"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

const (
	LogDir = "wal"
)

type DiskKV struct {
	writeBarrier  sync.RWMutex
	logger        *zap.Logger
	memKv         *memory.MemoryKV
	queue         chan *mutationReq
	log           *wal.Log
	closeCh       chan struct{}
	closeWg       sync.WaitGroup
	closed        *atomic.Bool
	cfg           Config
	counter       uint64
	flushInterval time.Duration
}

type Config struct {
	Logger        *zap.Logger
	HasnFn        memory.HashFn
	DataDir       string
	FlushInterval time.Duration
}

func (c Config) validate() error {
	if c.Logger == nil {
		return fmt.Errorf("nil Logger is invalid")
	}
	if c.HasnFn == nil {
		return fmt.Errorf("nil HashFn is invalid")
	}
	if c.DataDir == "" {
		return fmt.Errorf("empty DataDir is invalid")
	}
	if c.FlushInterval <= 0 {
		return fmt.Errorf("non-positive FlushInterval is invalid")
	}
	return nil
}

type mutationReq struct {
	mut *proto.Mutation
	err chan error
}

func logPath(dir string) string {
	return filepath.Join(dir, LogDir)
}

func New(cfg Config) (*DiskKV, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// store log to wal/ subdirectory to support future snapshot
	l, err := wal.Open(logPath(cfg.DataDir), &wal.Options{
		SegmentSize:      2 * 1024 * 1024, // 2MB
		SegmentCacheSize: 4,               // 8MB
		LogFormat:        wal.Binary,
		NoSync:           true,
		NoCopy:           true,
	})
	if err != nil {
		return nil, fmt.Errorf("error opening log: %w", err)
	}
	d := &DiskKV{
		logger:        cfg.Logger,
		memKv:         memory.WithHashFn(cfg.HasnFn),
		queue:         make(chan *mutationReq),
		log:           l,
		closeCh:       make(chan struct{}),
		closed:        atomic.NewBool(false),
		flushInterval: cfg.FlushInterval,
		cfg:           cfg,
	}
	d.logger.Info("Using append only log for kv storage", zap.String("dir", cfg.DataDir))

	if err := d.replayLogs(); err != nil {
		return nil, err
	}

	d.closeWg.Add(1)

	return d, nil
}

func (d *DiskKV) Start() {
	ticker := time.NewTicker(d.flushInterval)
	defer ticker.Stop()

	defer d.closeWg.Done()

	d.logger.Info("Periodically flushing logs to disk", zap.Duration("interval", d.flushInterval))

	dirty := false
	for {
		select {
		case <-d.closeCh:
			return
		case <-ticker.C:
			if !dirty {
				continue
			}
			dirty = false
			if err := d.log.Sync(); err != nil {
				d.logger.Error("Error flushing logs periodically", zap.Error(err))
			}
		case m := <-d.queue:
			var mutError error
			if logError := d.appendLog(m.mut); logError == nil {
				mutError = d.handleMutation(m.mut)
				if mutError != nil {
					d.rollbackOne(m.mut, mutError)
				}
			} else {
				d.logger.Error("Error appending mutation log",
					zap.String("mutation", m.mut.GetType().String()),
					zap.Error(logError))
				mutError = fs.ErrInvalid
			}
			dirty = true
			m.err <- mutError
		}
	}
}

func (d *DiskKV) Stop() {
	d.writeBarrier.Lock()
	defer d.writeBarrier.Unlock()

	if !d.closed.CompareAndSwap(false, true) {
		return
	}

	close(d.closeCh)
	d.closeWg.Wait()

	d.logger.Info("Flushing logs to disk")

	if err := d.log.Sync(); err != nil {
		d.logger.Error("Error flushing logs to disk", zap.Error(err))
	}
	if err := d.log.Close(); err != nil {
		d.logger.Error("Error closing log file", zap.Error(err))
	}
}

var _ chord.KVProvider = (*DiskKV)(nil)
