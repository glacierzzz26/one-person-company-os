package main

import (
	"fmt"
	"os"

	"github.com/glacierzzz26/one-person-company-os/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
