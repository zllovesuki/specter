package main

import (
	"context"
	"fmt"
	"os"

	"go.miragespace.co/specter/cmd/specter"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := specter.App.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
