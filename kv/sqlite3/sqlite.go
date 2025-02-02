package sqlite3

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"
	"github.com/ncruces/go-sqlite3/vfs"
	"github.com/tetratelabs/wazero"
	"go.uber.org/zap"
	"golang.org/x/sys/cpu"
	"gorm.io/gorm"
)

var (
	initializeOnce sync.Once
	lastError      error
)

func compilerSupported() bool {
	switch runtime.GOOS {
	case "linux", "android",
		"windows", "darwin",
		"freebsd", "netbsd", "dragonfly",
		"solaris", "illumos":
		break
	default:
		return false
	}
	switch runtime.GOARCH {
	case "amd64":
		return cpu.X86.HasSSE41
	case "arm64":
		return true
	default:
		return false
	}
}

func Initialize(cacheDir string) error {
	initializeOnce.Do(func() {
		cache, err := wazero.NewCompilationCacheWithDir(cacheDir)
		if err != nil {
			lastError = err
			return
		}
		var cfg wazero.RuntimeConfig
		if compilerSupported() {
			cfg = wazero.NewRuntimeConfigCompiler()
		} else {
			cfg = wazero.NewRuntimeConfigInterpreter()
		}
		// errata: testing with this set to 256MB on illumos/amd64
		// will yield "resource temporarily unavailable"
		cfg = cfg.WithMemoryLimitPages(512) // 32MB
		cfg = cfg.WithCompilationCache(cache)
		sqlite3.RuntimeConfig = cfg

		lastError = sqlite3.Initialize()
	})
	return lastError
}

func openSQLite(logger *zap.Logger, dbPath string) (gorm.Dialector, error) {
	logger.Info("SQLite via wazero",
		zap.Bool("compiler", compilerSupported()),
		zap.Bool("lock", vfs.SupportsFileLocking),
		zap.Bool("shm", vfs.SupportsSharedMemory),
	)

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(1)&_txlock=immediate", dbPath)
	db := gormlite.Open(dsn)
	return db, nil
}
