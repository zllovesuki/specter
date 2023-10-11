package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go.miragespace.co/specter/kv/aof"
	chordSpec "go.miragespace.co/specter/spec/chord"

	"github.com/jedib0t/go-pretty/v6/table"
	"go.uber.org/zap"
)

var (
	dataDir = flag.String("data", "data", "data dir")
	op      = flag.String("op", "", "operation to perform")
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
			fmt.Printf("%s\n", val)
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
	case "delete-all":
		keys := [][]byte{[]byte(*key)}
		fmt.Printf("removing everything under key %s\n", *key)
		kvProvider.RemoveKeys(keys)
	default:
		fmt.Printf("available ops:\n")
		opsTable := table.NewWriter()
		opsTable.SetOutputMirror(os.Stdout)

		opsTable.AppendHeader(table.Row{"Op", "Description"})
		opsTable.AppendRow(table.Row{"list", "list all keys from the kv storage"})
		opsTable.AppendRow(table.Row{"simple-get", "corresponds to KV Get"})
		opsTable.AppendRow(table.Row{"prefix-list", "corresponds to KV PrefixList"})
		opsTable.AppendRow(table.Row{"prefix-remove", "remove all children under prefix"})
		opsTable.AppendRow(table.Row{"simple-delete-prefix", "operates KV Delete on keys matching this prefix"})
		opsTable.AppendRow(table.Row{"remove-all", "remove everything under this key, including Simple, Prefix, and Lease"})

		opsTable.SetStyle(table.StyleDefault)
		opsTable.Style().Options.SeparateRows = true
		opsTable.Render()
	}
}
