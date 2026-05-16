package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/openclaw/crawlkit/output"
	"github.com/uinaf/fincrawl/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		var usage output.UsageError
		if errors.As(err, &usage) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
