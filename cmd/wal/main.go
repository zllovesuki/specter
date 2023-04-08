package main

import (
	"context"
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

	cfg := zap.NewDevelopmentConfig()
	cfg.OutputPaths = []string{"stderr"}

	logger, err := cfg.Build()
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
	case "simple-get":
		val, err := kvProvider.Get(context.Background(), []byte(*key))
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s", val)
	case "prefix-list":
		vals, err := kvProvider.PrefixList(context.Background(), []byte(*key))
		if err != nil {
			panic(err)
		}
		for _, val := range vals {
			fmt.Printf("%s", val)
		}
	case "simple-delete-prefix":
		keys := kvProvider.RangeKeys(0, 0)
		for _, k := range keys {
			if strings.HasPrefix(string(k), *key) {
				fmt.Printf("deleting simple key %s\n", k)
				kvProvider.Delete(context.Background(), k)
			}
		}
	case "prefix-remove":
		vals, err := kvProvider.PrefixList(context.Background(), []byte(*key))
		if err != nil {
			panic(err)
		}
		for _, val := range vals {
			fmt.Printf("deleting prefix %s with child %s\n", *key, val)
			kvProvider.PrefixRemove(context.Background(), []byte(*key), val)
		}
	case "remove-all":
		keys := [][]byte{[]byte(*key)}
		fmt.Printf("removing everything under key %s\n", *key)
		kvProvider.RemoveKeys(keys)
	}
}
