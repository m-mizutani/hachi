package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/m-mizutani/hachi/internal/cli"
)

// version is overwritten at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args, os.Stdout, os.Stderr, version)
	stop()
	os.Exit(code)
}
