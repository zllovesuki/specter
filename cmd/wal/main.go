package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"kon.nect.sh/specter/kv/aof"
	chordSpec "kon.nect.sh/specter/spec/chord"

	"go.uber.org/zap"
)

var (
	dataDir = flag.String("data", "data", "data dir")
	op      = flag.String("op", "list", "operation to perform")
	key     = flag.String("key", "example", "key to fetch")
)

func main() {
	flag.Parse()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	kvProvider, err := aof.New(aof.Config{
		Logger:        logger.With(zap.String("component", "kv")),
		HasnFn:        chordSpec.Hash,
		DataDir:       *dataDir,
		FlushInterval: time.Second,
	})
	if err != nil {
		panic(err)
	}
	go kvProvider.Start()

	defer kvProvider.Stop()

	switch *op {
	case "list":
		keys := kvProvider.RangeKeys(0, 0)
		for _, k := range keys {
			fmt.Printf("%s\n", k)
		}
	case "get":
		val, err := kvProvider.Get([]byte(*key))
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s", val)
	case "prefix-delete":
		keys := kvProvider.RangeKeys(0, 0)
		for _, k := range keys {
			if strings.HasPrefix(string(k), *key) {
				fmt.Printf("deleting %s\n", k)
				kvProvider.Delete(k)
			}
		}
	}
}
