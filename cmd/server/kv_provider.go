package server

import (
	"fmt"
	"time"

	"go.miragespace.co/specter/kv/aof"
	"go.miragespace.co/specter/kv/memory"
	"go.miragespace.co/specter/kv/sqlite3"
	"go.miragespace.co/specter/spec/chord"

	"go.uber.org/zap"
)

func noop() {}

func getKVProvider(logger *zap.Logger, datadir string, option string) (chord.KVProvider, func(), error) {
	switch option {
	case "memory":
		kv := memory.WithHashFn(chord.Hash)
		logger.Warn("Using memory as storage backend with persistence")
		return kv, noop, nil
	case "aof":
		kv, err := aof.New(aof.Config{
			Logger:        logger,
			HasnFn:        chord.Hash,
			DataDir:       datadir,
			FlushInterval: time.Second * 3,
		})
		if err != nil {
			return nil, nil, err
		}
		go kv.Start()
		logger.Info("Using Append-only File backed memory storage backend")
		return kv, kv.Stop, nil
	case "sqlite":
		kv, err := sqlite3.New(sqlite3.Config{
			Logger:  logger,
			HasnFn:  chord.Hash,
			DataDir: datadir,
		})
		if err != nil {
			return nil, nil, err
		}
		logger.Info("Using SQLite storage backend")
		return kv, noop, nil
	default:
		return nil, nil, fmt.Errorf("unknown kv provider: %s", option)
	}
}
