package main

import (
	"context"
	"os"

	"github.com/uinaf/fincrawl/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(cli.WriteError(os.Stderr, err, os.Args[1:]))
	}
}
