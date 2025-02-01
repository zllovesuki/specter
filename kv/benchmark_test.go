package kv

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"go.miragespace.co/specter/kv/aof"
	"go.miragespace.co/specter/kv/memory"
	"go.miragespace.co/specter/kv/sqlite3"
	"go.miragespace.co/specter/spec/chord"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var e1 error
var e2 error
var e3 error

func BenchmarkSqliteKVPut(b *testing.B) {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	// Redirect everything to stderr
	config.OutputPaths = []string{"/dev/null"}
	logger, err := config.Build()
	if err != nil {
		b.Errorf("setting up logger: %v", err)
	}

	dir, err := os.MkdirTemp("", "sqlite")
	if err != nil {
		b.Errorf("creating temporary storage: %v", err)
	}
	b.Cleanup(func() {
		os.RemoveAll(dir)
	})

	cfg := sqlite3.Config{
		Logger:  logger,
		HasnFn:  chord.Hash,
		DataDir: dir,
	}

	kv, err := sqlite3.New(cfg)
	if err != nil {
		b.Errorf("initializing kv: %v", err)
	}

	key := make([]byte, 32)
	value := make([]byte, 256)

	c := context.Background()
	b.ResetTimer()

	var putErr error
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rand.Read(key)
		rand.Read(value)
		b.StartTimer()
		putErr = kv.Put(c, key, value)
	}
	e3 = putErr
}

func BenchmarkAOFKVPut(b *testing.B) {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	// Redirect everything to stderr
	config.OutputPaths = []string{"/dev/null"}
	logger, err := config.Build()
	if err != nil {
		b.Errorf("setting up logger: %v", err)
	}

	dir, err := os.MkdirTemp("", "aof")
	if err != nil {
		b.Errorf("creating temporary storage: %v", err)
	}
	b.Cleanup(func() {
		os.RemoveAll(dir)
	})

	cfg := aof.Config{
		Logger:        logger,
		HasnFn:        chord.Hash,
		DataDir:       dir,
		FlushInterval: time.Second,
	}

	kv, err := aof.New(cfg)
	if err != nil {
		b.Errorf("initializing kv: %v", err)
	}
	b.Cleanup(kv.Stop)

	key := make([]byte, 32)
	value := make([]byte, 256)

	go kv.Start()
	c := context.Background()
	b.ResetTimer()

	var putErr error
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rand.Read(key)
		rand.Read(value)
		b.StartTimer()
		putErr = kv.Put(c, key, value)
	}
	e1 = putErr
}

func BenchmarkMemoryKVPut(b *testing.B) {
	kv := memory.WithHashFn(chord.Hash)

	key := make([]byte, 32)
	value := make([]byte, 256)

	c := context.Background()
	b.ResetTimer()

	var putErr error
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rand.Read(key)
		rand.Read(value)
		b.StartTimer()
		putErr = kv.Put(c, key, value)
	}
	e2 = putErr
}
