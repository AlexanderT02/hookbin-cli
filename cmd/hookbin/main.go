package main

import (
	"fmt"
	"os"

	"github.com/hookbin/hookbin-cli/internal/cli"
)

func main() {
	if err := cli.NewCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "hookbin:", err)
		os.Exit(1)
	}
}

