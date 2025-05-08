package main

import (
	"context"
	"fmt"
	"os"

	"go.miragespace.co/specter/cmd/specter"
	"go.miragespace.co/specter/util"

	"github.com/fatih/color"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	util.PrettierHelpPrinter()

	if err := specter.App.RunContext(ctx, os.Args); err != nil {
		errColor := color.New(color.FgRed, color.Bold).SprintFunc()
		fmt.Fprintln(os.Stderr, errColor(err))
		os.Exit(1)
	}
}
