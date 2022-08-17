package kv

import (
	"crypto/rand"
	"os"
	"testing"
	"time"

	"kon.nect.sh/specter/kv/aof"
	"kon.nect.sh/specter/kv/memory"
	"kon.nect.sh/specter/spec/chord"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var e1 error
var e2 error

func BenchmarkDiskKVPut(b *testing.B) {
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

	go kv.Start()

	key := make([]byte, 16)
	value := make([]byte, 256)

	b.ResetTimer()

	var putErr error
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rand.Read(key)
		rand.Read(value)
		b.StartTimer()
		putErr = kv.Put(key, value)
	}
	e1 = putErr
}

func BenchmarkMemoryKVPut(b *testing.B) {
	kv := memory.WithHashFn(chord.Hash)

	key := make([]byte, 16)
	value := make([]byte, 256)

	b.ResetTimer()

	var putErr error
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rand.Read(key)
		rand.Read(value)
		b.StartTimer()
		putErr = kv.Put(key, value)
	}
	e2 = putErr
}
